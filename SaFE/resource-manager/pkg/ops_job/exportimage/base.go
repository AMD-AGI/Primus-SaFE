/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package exportimage

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/remote"
)

// ErrBaseNotInRegistry is returned when the image the container was started from is not
// in the registry the saved image is put together in.
var ErrBaseNotInRegistry = errors.New("the image the container was started from is not in the registry the saved image is put together in")

// ParseImageID turns a container status imageID into a digest reference. Runtimes report
// it as "repo@sha256:..." and some prefix it with a scheme ("docker-pullable://"). A bare
// "sha256:..." names a local image and cannot be fetched from any registry.
func ParseImageID(imageID string) (name.Digest, error) {
	if _, rest, ok := strings.Cut(imageID, "://"); ok {
		imageID = rest
	}
	if !strings.Contains(imageID, "@") {
		return name.Digest{}, fmt.Errorf("the container's image ID %q names no repository digest", imageID)
	}
	return name.NewDigest(imageID)
}

// BaseCandidates lists where the base image may be read in the export registry: the
// image ID itself when the node pulled from that registry, else the same repository path
// and digest there. The digest pins the content, so a copy found there is the same image.
// The image is put together in that registry by mounting the base's layers, so a base
// that is not there cannot be used: copying it would move gigabytes through this process.
func BaseCandidates(base name.Digest, registry string) []name.Digest {
	if base.RegistryStr() == registry {
		return []name.Digest{base}
	}
	d, err := name.NewDigest(fmt.Sprintf("%s/%s@%s", registry, base.RepositoryStr(), base.DigestStr()))
	if err != nil {
		return nil
	}
	return []name.Digest{d}
}

// ResolveBase returns the first candidate that can be read. Only its manifest and config
// are read.
func ResolveBase(ctx context.Context, candidates []name.Digest, opts ...remote.Option) (v1.Image, name.Digest, error) {
	var errs []error
	for _, c := range candidates {
		img, err := remote.Image(c, append([]remote.Option{remote.WithContext(ctx)}, opts...)...)
		if err == nil {
			// Reading the manifest proves the candidate is reachable and holds the digest.
			if _, err = img.Manifest(); err == nil {
				return img, c, nil
			}
		}
		errs = append(errs, fmt.Errorf("%s: %w", c, err))
	}
	if len(errs) == 0 {
		return nil, name.Digest{}, ErrBaseNotInRegistry
	}
	return nil, name.Digest{}, fmt.Errorf("%w: %w", ErrBaseNotInRegistry, errors.Join(errs...))
}
