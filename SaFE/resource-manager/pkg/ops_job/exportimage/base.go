/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package exportimage

import (
	"archive/tar"
	"context"
	"errors"
	"fmt"
	"io"
	"path"
	"strings"

	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/google/go-containerregistry/pkg/v1/remote"
)

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

// BaseCandidates lists where the base image may be read, in order: the same repository
// path and digest on the target registry first, then the registry the node pulled from.
// The digest pins the content, so the copy in the target registry is the same image; it
// is preferred because the push can then mount its layers instead of copying them, and
// because the node's registry may be one this controller cannot reach or does not trust.
func BaseCandidates(base name.Digest, targetRegistry string) []name.Digest {
	var out []name.Digest
	if targetRegistry != "" && base.RegistryStr() != targetRegistry {
		if d, err := name.NewDigest(fmt.Sprintf("%s/%s@%s", targetRegistry, base.RepositoryStr(), base.DigestStr())); err == nil {
			out = append(out, d)
		}
	}
	return append(out, base)
}

// ResolveBase returns the first candidate that can be read.
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
	return nil, name.Digest{}, fmt.Errorf("cannot read the base image: %w", errors.Join(errs...))
}

// BaseFileSet returns every path of the base image's flattened file system, absolute.
// It reads every layer: the registry is the only record of what the container started
// with, so this is what deletions are measured against.
func BaseFileSet(img v1.Image) (map[string]bool, error) {
	rc := mutate.Extract(img)
	defer rc.Close()
	return fileSetFromTar(rc)
}

func fileSetFromTar(r io.Reader) (map[string]bool, error) {
	set := map[string]bool{}
	tr := tar.NewReader(r)
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return set, nil
		}
		if err != nil {
			return nil, fmt.Errorf("reading the base image: %w", err)
		}
		p := path.Clean("/" + hdr.Name)
		if p == "/" {
			continue
		}
		set[p] = true
		// A layer may omit a parent directory; it still exists in the flattened tree.
		for dir := path.Dir(p); dir != "/"; dir = path.Dir(dir) {
			set[dir] = true
		}
	}
}
