/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package image_handlers

import (
	"encoding/json"
	"testing"

	imagespecv1 "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/stretchr/testify/assert"
)

// TestCalculateManifestSizeOCI verifies layer+config sizes are summed for OCI manifests.
func TestCalculateManifestSizeOCI(t *testing.T) {
	h := &ImageHandler{}
	m := imagespecv1.Manifest{
		Config: imagespecv1.Descriptor{Size: 50},
		Layers: []imagespecv1.Descriptor{{Size: 100}, {Size: 200}},
	}
	raw, err := json.Marshal(m)
	assert.NoError(t, err)

	size, err := h.calculateManifestSize(raw, imagespecv1.MediaTypeImageManifest)
	assert.NoError(t, err)
	assert.Equal(t, int64(350), size)
}

// TestCalculateManifestSizeUnsupported verifies unsupported manifest types error out.
func TestCalculateManifestSizeUnsupported(t *testing.T) {
	h := &ImageHandler{}
	_, err := h.calculateManifestSize([]byte("{}"), "application/unknown")
	assert.Error(t, err)
}

// TestCalculateManifestSizeBadJSON verifies parsing errors are reported.
func TestCalculateManifestSizeBadJSON(t *testing.T) {
	h := &ImageHandler{}
	_, err := h.calculateManifestSize([]byte("not-json"), imagespecv1.MediaTypeImageManifest)
	assert.Error(t, err)
}

// TestExtractPlatformDigestOCIIndex verifies the matching platform digest is returned.
func TestExtractPlatformDigestOCIIndex(t *testing.T) {
	h := &ImageHandler{}
	index := imagespecv1.Index{
		Manifests: []imagespecv1.Descriptor{
			{
				Digest:   "sha256:abc",
				Platform: &imagespecv1.Platform{OS: "linux", Architecture: "amd64"},
			},
		},
	}
	raw, err := json.Marshal(index)
	assert.NoError(t, err)

	digest, err := h.extractPlatformDigest(raw, imagespecv1.MediaTypeImageIndex, "linux", "amd64")
	assert.NoError(t, err)
	assert.Equal(t, "sha256:abc", digest.String())
}

// TestExtractPlatformDigestNoMatch verifies an error when no platform matches.
func TestExtractPlatformDigestNoMatch(t *testing.T) {
	h := &ImageHandler{}
	index := imagespecv1.Index{
		Manifests: []imagespecv1.Descriptor{
			{
				Digest:   "sha256:abc",
				Platform: &imagespecv1.Platform{OS: "linux", Architecture: "arm64"},
			},
		},
	}
	raw, err := json.Marshal(index)
	assert.NoError(t, err)

	_, err = h.extractPlatformDigest(raw, imagespecv1.MediaTypeImageIndex, "linux", "amd64")
	assert.Error(t, err)
}

// TestExtractPlatformDigestBadIndex verifies parse errors are reported.
func TestExtractPlatformDigestBadIndex(t *testing.T) {
	h := &ImageHandler{}
	_, err := h.extractPlatformDigest([]byte("bad"), imagespecv1.MediaTypeImageIndex, "linux", "amd64")
	assert.Error(t, err)
}