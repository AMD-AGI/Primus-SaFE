// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

// Package registry provides a client for querying Harbor and OCI-compatible
// container registries. It retrieves image manifests, configuration blobs, and
// layer history to support intent analysis from container images.
package registry

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
)

const (
	defaultTimeout  = 30 * time.Second
	maxResponseSize = 10 * 1024 * 1024 // 10MB
)

// HarborClient is an HTTP client for the Harbor V2 / OCI Distribution API.
// It supports fetching image manifests, config blobs, and layer history.
type HarborClient struct {
	baseURL    string
	httpClient *http.Client
	username   string
	password   string
}

// HarborClientConfig holds client configuration
type HarborClientConfig struct {
	BaseURL  string
	Username string
	Password string
	Timeout  time.Duration
}

// NewHarborClient creates a new Harbor API client
func NewHarborClient(cfg *HarborClientConfig) *HarborClient {
	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = defaultTimeout
	}

	return &HarborClient{
		baseURL: strings.TrimRight(cfg.BaseURL, "/"),
		httpClient: &http.Client{
			Timeout: timeout,
		},
		username: cfg.Username,
		password: cfg.Password,
	}
}

// ImageManifest represents the OCI image manifest
type ImageManifest struct {
	SchemaVersion int              `json:"schemaVersion"`
	MediaType     string           `json:"mediaType"`
	Config        ManifestConfig   `json:"config"`
	Layers        []ManifestLayer  `json:"layers"`
}

// ManifestConfig is the config descriptor in the manifest
type ManifestConfig struct {
	MediaType string `json:"mediaType"`
	Size      int64  `json:"size"`
	Digest    string `json:"digest"`
}

// ManifestLayer is a single layer descriptor
type ManifestLayer struct {
	MediaType string `json:"mediaType"`
	Size      int64  `json:"size"`
	Digest    string `json:"digest"`
}

// ImageConfig represents the OCI image config (history, rootfs, etc.)
type ImageConfig struct {
	Architecture string        `json:"architecture"`
	OS           string        `json:"os"`
	Created      string        `json:"created"`
	Config       ContainerCfg  `json:"config"`
	History      []HistoryEntry `json:"history"`
	RootFS       RootFS        `json:"rootfs"`
}

// ContainerCfg holds the container runtime config from the image
type ContainerCfg struct {
	Env        []string          `json:"Env"`
	Cmd        []string          `json:"Cmd"`
	Entrypoint []string          `json:"Entrypoint"`
	Labels     map[string]string `json:"Labels"`
	WorkingDir string            `json:"WorkingDir"`
}

// HistoryEntry is a single layer history entry
type HistoryEntry struct {
	Created    string `json:"created"`
	CreatedBy  string `json:"created_by"`
	Comment    string `json:"comment"`
	EmptyLayer bool   `json:"empty_layer"`
}

// RootFS describes the image's rootfs
type RootFS struct {
	Type    string   `json:"type"`
	DiffIDs []string `json:"diff_ids"`
}

// GetManifest retrieves the image manifest for a given repository and reference (tag or digest)
func (c *HarborClient) GetManifest(ctx context.Context, repository, reference string) (*ImageManifest, error) {
	url := fmt.Sprintf("%s/v2/%s/manifests/%s", c.baseURL, repository, reference)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Accept OCI and Docker manifest types
	req.Header.Set("Accept", strings.Join([]string{
		"application/vnd.oci.image.manifest.v1+json",
		"application/vnd.docker.distribution.manifest.v2+json",
	}, ", "))

	if c.username != "" {
		req.SetBasicAuth(c.username, c.password)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch manifest: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("manifest request failed with status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseSize))
	if err != nil {
		return nil, fmt.Errorf("failed to read manifest body: %w", err)
	}

	var manifest ImageManifest
	if err := json.Unmarshal(body, &manifest); err != nil {
		return nil, fmt.Errorf("failed to parse manifest: %w", err)
	}

	return &manifest, nil
}

// GetImageConfig retrieves the image configuration blob
func (c *HarborClient) GetImageConfig(ctx context.Context, repository, configDigest string) (*ImageConfig, error) {
	url := fmt.Sprintf("%s/v2/%s/blobs/%s", c.baseURL, repository, configDigest)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	if c.username != "" {
		req.SetBasicAuth(c.username, c.password)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch image config: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("image config request failed with status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseSize))
	if err != nil {
		return nil, fmt.Errorf("failed to read config body: %w", err)
	}

	var config ImageConfig
	if err := json.Unmarshal(body, &config); err != nil {
		return nil, fmt.Errorf("failed to parse image config: %w", err)
	}

	return &config, nil
}

// FetchImageMetadata is a convenience method that fetches both manifest and config
// in a single call, returning the full image metadata needed for intent analysis.
func (c *HarborClient) FetchImageMetadata(ctx context.Context, imageRef string) (*ImageConfig, string, error) {
	repository, reference := parseImageReference(imageRef)
	if repository == "" {
		return nil, "", fmt.Errorf("invalid image reference: %s", imageRef)
	}

	log.Debugf("Fetching image metadata for %s:%s", repository, reference)

	// Get manifest
	manifest, err := c.GetManifest(ctx, repository, reference)
	if err != nil {
		return nil, "", fmt.Errorf("failed to get manifest for %s: %w", imageRef, err)
	}

	digest := manifest.Config.Digest

	// Get config
	config, err := c.GetImageConfig(ctx, repository, digest)
	if err != nil {
		return nil, digest, fmt.Errorf("failed to get config for %s: %w", imageRef, err)
	}

	return config, digest, nil
}

// parseImageReference splits an image reference into repository and tag/digest
func parseImageReference(ref string) (string, string) {
	// Handle digest references: repo@sha256:xxx
	if idx := strings.Index(ref, "@"); idx != -1 {
		return ref[:idx], ref[idx+1:]
	}

	// Handle tag references: repo:tag
	// Need to distinguish port numbers from tags
	parts := strings.Split(ref, "/")
	lastPart := parts[len(parts)-1]

	if idx := strings.LastIndex(lastPart, ":"); idx != -1 {
		tag := lastPart[idx+1:]
		repo := strings.Join(parts[:len(parts)-1], "/") + "/" + lastPart[:idx]
		return repo, tag
	}

	// No tag, use latest
	return ref, "latest"
}
