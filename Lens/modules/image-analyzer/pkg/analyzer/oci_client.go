// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package analyzer

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
)

const (
	ociClientTimeout  = 60 * time.Second
	maxManifestSize   = 10 * 1024 * 1024 // 10MB
	maxConfigSize     = 10 * 1024 * 1024 // 10MB
)

// OCIClient is an HTTP client for OCI Distribution API.
// Unlike HarborClient, it supports dynamic registry resolution based on
// the image reference, rather than a single hardcoded base URL.
type OCIClient struct {
	httpClient   *http.Client
	authResolver *AuthResolver
}

// NewOCIClient creates a new OCI Distribution API client.
// It uses a custom TLS transport that skips certificate verification
// to support internal registries with self-signed certificates.
func NewOCIClient(authResolver *AuthResolver) *OCIClient {
	transport := &http.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: true, //nolint:gosec // Internal registries may use self-signed certs
		},
	}
	return &OCIClient{
		httpClient: &http.Client{
			Timeout:   ociClientTimeout,
			Transport: transport,
		},
		authResolver: authResolver,
	}
}

// OCIManifest represents an OCI/Docker image manifest
type OCIManifest struct {
	SchemaVersion int              `json:"schemaVersion"`
	MediaType     string           `json:"mediaType"`
	Config        OCIDescriptor    `json:"config"`
	Layers        []OCIDescriptor  `json:"layers"`
}

// OCIDescriptor is a content descriptor for a blob
type OCIDescriptor struct {
	MediaType string `json:"mediaType"`
	Size      int64  `json:"size"`
	Digest    string `json:"digest"`
}

// OCIImageConfig represents the OCI image configuration blob
type OCIImageConfig struct {
	Architecture string         `json:"architecture"`
	OS           string         `json:"os"`
	Created      string         `json:"created"`
	Config       OCIContainerCfg `json:"config"`
	History      []OCIHistory   `json:"history"`
	RootFS       OCIRootFS      `json:"rootfs"`
}

// OCIContainerCfg holds container runtime config
type OCIContainerCfg struct {
	Env        []string          `json:"Env"`
	Cmd        []string          `json:"Cmd"`
	Entrypoint []string          `json:"Entrypoint"`
	Labels     map[string]string `json:"Labels"`
	WorkingDir string            `json:"WorkingDir"`
}

// OCIHistory is a single layer history entry
type OCIHistory struct {
	Created    string `json:"created"`
	CreatedBy  string `json:"created_by"`
	Comment    string `json:"comment"`
	EmptyLayer bool   `json:"empty_layer"`
}

// OCIRootFS describes the image rootfs
type OCIRootFS struct {
	Type    string   `json:"type"`
	DiffIDs []string `json:"diff_ids"`
}

// ImageRef holds a parsed image reference
type ImageRef struct {
	Registry   string // e.g., "harbor.example.com"
	Repository string // e.g., "sync/rocm/verl"
	Tag        string // e.g., "v1.0" or "sha256:abc..."
	Raw        string // original reference
}

// ParseImageRef parses an image reference string into components.
func ParseImageRef(ref string) ImageRef {
	result := ImageRef{Raw: ref, Tag: "latest"}

	r := ref

	// Handle digest references: repo@sha256:xxx
	if idx := strings.Index(r, "@"); idx != -1 {
		result.Tag = r[idx+1:]
		r = r[:idx]
	} else {
		// Handle tag references
		// Need to distinguish port from tag
		parts := strings.Split(r, "/")
		lastPart := parts[len(parts)-1]
		if colonIdx := strings.LastIndex(lastPart, ":"); colonIdx != -1 {
			result.Tag = lastPart[colonIdx+1:]
			parts[len(parts)-1] = lastPart[:colonIdx]
			r = strings.Join(parts, "/")
		}
	}

	// Split registry from repository
	slashParts := strings.SplitN(r, "/", 2)
	if len(slashParts) == 2 && (strings.Contains(slashParts[0], ".") || strings.Contains(slashParts[0], ":")) {
		result.Registry = slashParts[0]
		result.Repository = slashParts[1]
	} else {
		// Docker Hub default
		result.Registry = "registry-1.docker.io"
		result.Repository = r
	}

	return result
}

// FetchManifest retrieves the image manifest from the registry
func (c *OCIClient) FetchManifest(ctx context.Context, ref ImageRef, auth *RegistryAuth) (*OCIManifest, error) {
	url := fmt.Sprintf("https://%s/v2/%s/manifests/%s", ref.Registry, ref.Repository, ref.Tag)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create manifest request: %w", err)
	}

	req.Header.Set("Accept", strings.Join([]string{
		"application/vnd.oci.image.manifest.v1+json",
		"application/vnd.docker.distribution.manifest.v2+json",
	}, ", "))

	if auth != nil {
		req.SetBasicAuth(auth.Username, auth.Password)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("manifest fetch failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("manifest request returned status %d for %s", resp.StatusCode, url)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxManifestSize))
	if err != nil {
		return nil, fmt.Errorf("failed to read manifest body: %w", err)
	}

	var manifest OCIManifest
	if err := json.Unmarshal(body, &manifest); err != nil {
		return nil, fmt.Errorf("failed to parse manifest: %w", err)
	}

	return &manifest, nil
}

// FetchConfig retrieves the image configuration blob
func (c *OCIClient) FetchConfig(ctx context.Context, ref ImageRef, configDigest string, auth *RegistryAuth) (*OCIImageConfig, error) {
	url := fmt.Sprintf("https://%s/v2/%s/blobs/%s", ref.Registry, ref.Repository, configDigest)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create config request: %w", err)
	}

	if auth != nil {
		req.SetBasicAuth(auth.Username, auth.Password)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("config fetch failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("config request returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxConfigSize))
	if err != nil {
		return nil, fmt.Errorf("failed to read config body: %w", err)
	}

	var config OCIImageConfig
	if err := json.Unmarshal(body, &config); err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}

	return &config, nil
}

// StreamLayerBlob opens a streaming connection to download a layer blob.
// The caller is responsible for closing the returned ReadCloser.
func (c *OCIClient) StreamLayerBlob(ctx context.Context, ref ImageRef, layerDigest string, auth *RegistryAuth) (io.ReadCloser, int64, error) {
	url := fmt.Sprintf("https://%s/v2/%s/blobs/%s", ref.Registry, ref.Repository, layerDigest)

	// Use a separate client with no timeout for streaming large blobs
	streamClient := &http.Client{
		Timeout: 0, // No timeout for streaming
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true, //nolint:gosec // Internal registries may use self-signed certs
			},
		},
	}

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to create blob request: %w", err)
	}

	if auth != nil {
		req.SetBasicAuth(auth.Username, auth.Password)
	}

	resp, err := streamClient.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("blob fetch failed: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, 0, fmt.Errorf("blob request returned status %d for %s", resp.StatusCode, layerDigest)
	}

	log.Debugf("OCIClient: streaming layer %s (content-length: %d)", layerDigest, resp.ContentLength)
	return resp.Body, resp.ContentLength, nil
}
