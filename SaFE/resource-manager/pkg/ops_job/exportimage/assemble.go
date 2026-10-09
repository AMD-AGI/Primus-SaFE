/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package exportimage

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/google/go-containerregistry/pkg/v1/remote/transport"
	"github.com/google/go-containerregistry/pkg/v1/types"
)

// ErrNotMounted is returned when the registry would not mount a blob from another
// repository. The blob is never copied instead: that would move it through this process.
var ErrNotMounted = errors.New("the registry did not mount the blob")

// StagedLayer is the layer the container uploaded, as the registry reports it, and the
// diff ID the container computed (which only the image's user can be harmed by).
type StagedLayer struct {
	Digest v1.Hash
	DiffID v1.Hash
	Size   int64
}

// stagedLayer stands for the uploaded layer in mutate.Append, which needs its descriptor
// and diff ID but never its contents.
type stagedLayer struct {
	staged    StagedLayer
	mediaType types.MediaType
}

var errNotLocal = errors.New("the layer is in the registry, not in this process")

func (l stagedLayer) Digest() (v1.Hash, error)            { return l.staged.Digest, nil }
func (l stagedLayer) DiffID() (v1.Hash, error)            { return l.staged.DiffID, nil }
func (l stagedLayer) Size() (int64, error)                { return l.staged.Size, nil }
func (l stagedLayer) MediaType() (types.MediaType, error) { return l.mediaType, nil }
func (l stagedLayer) Compressed() (io.ReadCloser, error)  { return nil, errNotLocal }
func (l stagedLayer) Uncompressed() (io.ReadCloser, error) {
	return nil, errNotLocal
}

// registryClient speaks the distribution API with a token for the repositories it was
// made for. Every request it makes is a few kilobytes: no blob passes through it.
type registryClient struct {
	http *http.Client
}

func newRegistryClient(ctx context.Context, reg name.Registry, auth authn.Authenticator, tr http.RoundTripper, scopes ...string) (*registryClient, error) {
	rt, err := transport.NewWithContext(ctx, reg, auth, tr, scopes)
	if err != nil {
		return nil, fmt.Errorf("authenticating to %s: %w", reg, err)
	}
	return &registryClient{http: &http.Client{Transport: rt}}, nil
}

func repoURL(repo name.Repository, suffix string) string {
	return fmt.Sprintf("%s://%s/v2/%s/%s", repo.Scheme(), repo.RegistryStr(), repo.RepositoryStr(), suffix)
}

func (c *registryClient) do(ctx context.Context, method, u string, body []byte, header http.Header) (*http.Response, error) {
	var r io.Reader
	if body != nil {
		r = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, u, r)
	if err != nil {
		return nil, err
	}
	for k, v := range header {
		req.Header[k] = v
	}
	if body != nil {
		req.ContentLength = int64(len(body))
	}
	return c.http.Do(req)
}

// BlobSize returns the size of a blob in repo, and whether it is there at all.
func (c *registryClient) BlobSize(ctx context.Context, repo name.Repository, d v1.Hash) (int64, bool, error) {
	resp, err := c.do(ctx, http.MethodHead, repoURL(repo, "blobs/"+d.String()), nil, nil)
	if err != nil {
		return 0, false, err
	}
	defer resp.Body.Close()
	switch resp.StatusCode {
	case http.StatusOK:
		return resp.ContentLength, true, nil
	case http.StatusNotFound:
		return 0, false, nil
	}
	return 0, false, transport.CheckError(resp, http.StatusOK)
}

// Mount makes a blob of src available in dst, which must be in the same registry, without
// copying it. A registry that answers with an upload instead has the upload cancelled.
func (c *registryClient) Mount(ctx context.Context, dst, src name.Repository, d v1.Hash) error {
	if _, ok, err := c.BlobSize(ctx, dst, d); err != nil {
		return err
	} else if ok {
		return nil
	}
	q := url.Values{"mount": {d.String()}, "from": {src.RepositoryStr()}}
	resp, err := c.do(ctx, http.MethodPost, repoURL(dst, "blobs/uploads/")+"?"+q.Encode(), []byte{}, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	switch resp.StatusCode {
	case http.StatusCreated:
		return nil
	case http.StatusAccepted:
		if loc, err := resp.Request.URL.Parse(resp.Header.Get("Location")); err == nil && loc.String() != "" {
			if cancel, err := c.do(ctx, http.MethodDelete, loc.String(), nil, nil); err == nil {
				cancel.Body.Close()
			}
		}
		return fmt.Errorf("%w: %s from %s into %s", ErrNotMounted, d, src, dst)
	}
	return transport.CheckError(resp, http.StatusCreated)
}

// Upload writes a small blob (an image config) in one request pair.
func (c *registryClient) Upload(ctx context.Context, repo name.Repository, d v1.Hash, data []byte) error {
	if _, ok, err := c.BlobSize(ctx, repo, d); err != nil {
		return err
	} else if ok {
		return nil
	}
	resp, err := c.do(ctx, http.MethodPost, repoURL(repo, "blobs/uploads/"), []byte{}, nil)
	if err != nil {
		return err
	}
	resp.Body.Close()
	if err := transport.CheckError(resp, http.StatusAccepted); err != nil {
		return err
	}
	loc, err := resp.Request.URL.Parse(resp.Header.Get("Location"))
	if err != nil {
		return err
	}
	q := loc.Query()
	q.Set("digest", d.String())
	loc.RawQuery = q.Encode()
	resp, err = c.do(ctx, http.MethodPut, loc.String(), data, http.Header{"Content-Type": {"application/octet-stream"}})
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return transport.CheckError(resp, http.StatusCreated)
}

// PutManifest tags a manifest and returns the digest the registry then reports for the tag.
func (c *registryClient) PutManifest(ctx context.Context, tag name.Tag, mt types.MediaType, raw []byte) (v1.Hash, error) {
	repo := tag.Context()
	resp, err := c.do(ctx, http.MethodPut, repoURL(repo, "manifests/"+tag.TagStr()), raw, http.Header{"Content-Type": {string(mt)}})
	if err != nil {
		return v1.Hash{}, err
	}
	resp.Body.Close()
	if err := transport.CheckError(resp, http.StatusCreated, http.StatusOK); err != nil {
		return v1.Hash{}, err
	}
	resp, err = c.do(ctx, http.MethodHead, repoURL(repo, "manifests/"+tag.TagStr()), nil, http.Header{"Accept": {string(mt)}})
	if err != nil {
		return v1.Hash{}, err
	}
	resp.Body.Close()
	if err := transport.CheckError(resp, http.StatusOK); err != nil {
		return v1.Hash{}, err
	}
	return v1.NewHash(resp.Header.Get("Docker-Content-Digest"))
}

// Assemble puts the saved image together in dst: the base image's layers and the staged
// layer are mounted from their repositories, and only the new config and manifest are
// written. All three repositories must be in one registry, the only place a blob can be
// mounted from. It returns the manifest digest the registry reports for dst.
func Assemble(ctx context.Context, base v1.Image, baseRepo, staging name.Repository, layer StagedLayer, dst name.Tag,
	auth authn.Authenticator, tr http.RoundTripper) (v1.Hash, error) {
	reg := dst.RegistryStr()
	if baseRepo.RegistryStr() != reg || staging.RegistryStr() != reg {
		return v1.Hash{}, fmt.Errorf("the base %s, the staged layer %s and the image %s are not in one registry", baseRepo, staging, dst)
	}
	baseType, err := base.MediaType()
	if err != nil {
		return v1.Hash{}, err
	}
	layerType := types.DockerLayer
	if baseType == types.OCIManifestSchema1 {
		layerType = types.OCILayer
	}
	img, err := mutate.Append(base, mutate.Addendum{
		Layer: stagedLayer{staged: layer, mediaType: layerType},
		History: v1.History{
			Created:   v1.Time{Time: time.Now().UTC()},
			CreatedBy: "primus-safe exportimage",
			Comment:   "files changed in the running container",
		},
	})
	if err != nil {
		return v1.Hash{}, err
	}
	manifest, err := img.Manifest()
	if err != nil {
		return v1.Hash{}, err
	}
	rawManifest, err := img.RawManifest()
	if err != nil {
		return v1.Hash{}, err
	}
	rawConfig, err := img.RawConfigFile()
	if err != nil {
		return v1.Hash{}, err
	}
	want, err := img.Digest()
	if err != nil {
		return v1.Hash{}, err
	}

	c, err := newRegistryClient(ctx, dst.Registry, auth, tr,
		dst.Context().Scope(transport.PushScope), baseRepo.Scope(transport.PullScope), staging.Scope(transport.PullScope))
	if err != nil {
		return v1.Hash{}, err
	}
	for _, l := range manifest.Layers {
		src := baseRepo
		if l.Digest == layer.Digest {
			src = staging
		}
		if err := c.Mount(ctx, dst.Context(), src, l.Digest); err != nil {
			return v1.Hash{}, err
		}
	}
	if err := c.Upload(ctx, dst.Context(), manifest.Config.Digest, rawConfig); err != nil {
		return v1.Hash{}, fmt.Errorf("writing the image config: %w", err)
	}
	got, err := c.PutManifest(ctx, dst, manifest.MediaType, rawManifest)
	if err != nil {
		return v1.Hash{}, fmt.Errorf("writing the manifest of %s: %w", dst, err)
	}
	if got != want {
		return v1.Hash{}, fmt.Errorf("the registry reports %s for %s, not the manifest written (%s)", got, dst, want)
	}
	return got, nil
}
