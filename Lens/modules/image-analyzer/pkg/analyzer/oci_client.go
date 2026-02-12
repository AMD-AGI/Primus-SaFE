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
	"net/url"
	"strings"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
)

const (
	ociClientTimeout = 60 * time.Second
	maxManifestSize  = 10 * 1024 * 1024 // 10MB
	maxConfigSize    = 10 * 1024 * 1024 // 10MB
)

// OCIClient is an HTTP client for OCI Distribution API.
// It supports dynamic registry resolution and implements the standard
// Docker v2 token authentication flow (WWW-Authenticate challenge).
type OCIClient struct {
	httpClient   *http.Client
	authResolver *AuthResolver
	transport    *http.Transport
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
		transport:    transport,
	}
}

// OCIManifest represents an OCI/Docker image manifest
type OCIManifest struct {
	SchemaVersion int             `json:"schemaVersion"`
	MediaType     string          `json:"mediaType"`
	Config        OCIDescriptor   `json:"config"`
	Layers        []OCIDescriptor `json:"layers"`
}

// OCIDescriptor is a content descriptor for a blob
type OCIDescriptor struct {
	MediaType string `json:"mediaType"`
	Size      int64  `json:"size"`
	Digest    string `json:"digest"`
}

// OCIImageConfig represents the OCI image configuration blob
type OCIImageConfig struct {
	Architecture string          `json:"architecture"`
	OS           string          `json:"os"`
	Created      string          `json:"created"`
	Config       OCIContainerCfg `json:"config"`
	History      []OCIHistory    `json:"history"`
	RootFS       OCIRootFS       `json:"rootfs"`
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

// doWithAuth performs an HTTP request with Docker v2 token authentication.
// It first tries the request directly, and if it gets a 401 with a
// WWW-Authenticate Bearer challenge, it obtains a token and retries.
func (c *OCIClient) doWithAuth(ctx context.Context, req *http.Request, auth *RegistryAuth) (*http.Response, error) {
	// First attempt: try with basic auth if credentials available
	if auth != nil {
		req.SetBasicAuth(auth.Username, auth.Password)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}

	// If not 401, return as-is
	if resp.StatusCode != http.StatusUnauthorized {
		return resp, nil
	}

	// Parse WWW-Authenticate header for Bearer challenge
	wwwAuth := resp.Header.Get("Www-Authenticate")
	resp.Body.Close()

	if wwwAuth == "" || !strings.HasPrefix(wwwAuth, "Bearer ") {
		// Not a Bearer challenge, nothing we can do
		return nil, fmt.Errorf("received 401 without Bearer challenge (Www-Authenticate: %q)", wwwAuth)
	}

	// Parse Bearer realm, service, scope from the challenge
	token, err := c.fetchBearerToken(ctx, wwwAuth, auth)
	if err != nil {
		return nil, fmt.Errorf("failed to obtain bearer token: %w", err)
	}

	// Retry with Bearer token
	retryReq, err := http.NewRequestWithContext(ctx, req.Method, req.URL.String(), nil)
	if err != nil {
		return nil, err
	}
	// Copy headers from original request
	for k, v := range req.Header {
		retryReq.Header[k] = v
	}
	retryReq.Header.Set("Authorization", "Bearer "+token)

	return c.httpClient.Do(retryReq)
}

// doStreamWithAuth is like doWithAuth but uses a no-timeout client for streaming.
func (c *OCIClient) doStreamWithAuth(ctx context.Context, reqURL string, auth *RegistryAuth) (*http.Response, error) {
	streamClient := &http.Client{
		Timeout:   0,
		Transport: c.transport,
	}

	req, err := http.NewRequestWithContext(ctx, "GET", reqURL, nil)
	if err != nil {
		return nil, err
	}
	if auth != nil {
		req.SetBasicAuth(auth.Username, auth.Password)
	}

	resp, err := streamClient.Do(req)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusUnauthorized {
		return resp, nil
	}

	wwwAuth := resp.Header.Get("Www-Authenticate")
	resp.Body.Close()

	if wwwAuth == "" || !strings.HasPrefix(wwwAuth, "Bearer ") {
		return nil, fmt.Errorf("received 401 without Bearer challenge for blob")
	}

	token, err := c.fetchBearerToken(ctx, wwwAuth, auth)
	if err != nil {
		return nil, fmt.Errorf("failed to obtain bearer token for blob: %w", err)
	}

	retryReq, err := http.NewRequestWithContext(ctx, "GET", reqURL, nil)
	if err != nil {
		return nil, err
	}
	retryReq.Header.Set("Authorization", "Bearer "+token)

	return streamClient.Do(retryReq)
}

// fetchBearerToken obtains a Bearer token from the token endpoint
// specified in the WWW-Authenticate challenge header.
func (c *OCIClient) fetchBearerToken(ctx context.Context, wwwAuth string, auth *RegistryAuth) (string, error) {
	params := parseWWWAuthenticate(wwwAuth)

	realm := params["realm"]
	if realm == "" {
		return "", fmt.Errorf("no realm in WWW-Authenticate header")
	}

	// Build token request URL
	tokenURL, err := url.Parse(realm)
	if err != nil {
		return "", fmt.Errorf("invalid realm URL: %w", err)
	}

	q := tokenURL.Query()
	if service, ok := params["service"]; ok {
		q.Set("service", service)
	}
	if scope, ok := params["scope"]; ok {
		q.Set("scope", scope)
	}
	tokenURL.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, "GET", tokenURL.String(), nil)
	if err != nil {
		return "", err
	}

	// Use basic auth for the token request if we have credentials
	if auth != nil {
		req.SetBasicAuth(auth.Username, auth.Password)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("token request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return "", fmt.Errorf("token endpoint returned status %d: %s", resp.StatusCode, string(body))
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	if err != nil {
		return "", fmt.Errorf("failed to read token response: %w", err)
	}

	var tokenResp struct {
		Token       string `json:"token"`
		AccessToken string `json:"access_token"`
	}
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return "", fmt.Errorf("failed to parse token response: %w", err)
	}

	token := tokenResp.Token
	if token == "" {
		token = tokenResp.AccessToken
	}
	if token == "" {
		return "", fmt.Errorf("empty token in response")
	}

	log.Debugf("OCIClient: obtained bearer token from %s", realm)
	return token, nil
}

// parseWWWAuthenticate parses a Bearer WWW-Authenticate header value.
// Example: Bearer realm="https://auth.docker.io/token",service="registry.docker.io",scope="repository:library/alpine:pull"
func parseWWWAuthenticate(header string) map[string]string {
	params := make(map[string]string)
	// Strip "Bearer " prefix
	s := strings.TrimPrefix(header, "Bearer ")

	for _, part := range splitParams(s) {
		part = strings.TrimSpace(part)
		eqIdx := strings.Index(part, "=")
		if eqIdx < 0 {
			continue
		}
		key := strings.TrimSpace(part[:eqIdx])
		val := strings.TrimSpace(part[eqIdx+1:])
		// Remove surrounding quotes
		val = strings.Trim(val, "\"")
		params[key] = val
	}
	return params
}

// splitParams splits comma-separated key=value pairs, respecting quoted values
func splitParams(s string) []string {
	var parts []string
	var current strings.Builder
	inQuote := false
	for _, ch := range s {
		if ch == '"' {
			inQuote = !inQuote
			current.WriteRune(ch)
		} else if ch == ',' && !inQuote {
			parts = append(parts, current.String())
			current.Reset()
		} else {
			current.WriteRune(ch)
		}
	}
	if current.Len() > 0 {
		parts = append(parts, current.String())
	}
	return parts
}

// FetchManifest retrieves the image manifest from the registry
func (c *OCIClient) FetchManifest(ctx context.Context, ref ImageRef, auth *RegistryAuth) (*OCIManifest, error) {
	reqURL := fmt.Sprintf("https://%s/v2/%s/manifests/%s", ref.Registry, ref.Repository, ref.Tag)

	req, err := http.NewRequestWithContext(ctx, "GET", reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create manifest request: %w", err)
	}

	req.Header.Set("Accept", strings.Join([]string{
		"application/vnd.oci.image.manifest.v1+json",
		"application/vnd.docker.distribution.manifest.v2+json",
	}, ", "))

	resp, err := c.doWithAuth(ctx, req, auth)
	if err != nil {
		return nil, fmt.Errorf("manifest fetch failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return nil, fmt.Errorf("manifest request returned status %d for %s: %s", resp.StatusCode, reqURL, string(body))
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxManifestSize))
	if err != nil {
		return nil, fmt.Errorf("failed to read manifest body: %w", err)
	}

	// Handle manifest list / fat manifest: pick the linux/amd64 manifest
	var rawManifest map[string]interface{}
	if err := json.Unmarshal(body, &rawManifest); err != nil {
		return nil, fmt.Errorf("failed to parse manifest JSON: %w", err)
	}

	mediaType, _ := rawManifest["mediaType"].(string)
	if mediaType == "application/vnd.docker.distribution.manifest.list.v2+json" ||
		mediaType == "application/vnd.oci.image.index.v1+json" {
		// This is a manifest list, resolve to a specific platform manifest
		return c.resolveManifestList(ctx, ref, body, auth)
	}

	var manifest OCIManifest
	if err := json.Unmarshal(body, &manifest); err != nil {
		return nil, fmt.Errorf("failed to parse manifest: %w", err)
	}

	return &manifest, nil
}

// resolveManifestList picks the linux/amd64 manifest from a manifest list
func (c *OCIClient) resolveManifestList(ctx context.Context, ref ImageRef, listBody []byte, auth *RegistryAuth) (*OCIManifest, error) {
	var manifestList struct {
		Manifests []struct {
			MediaType string `json:"mediaType"`
			Digest    string `json:"digest"`
			Size      int64  `json:"size"`
			Platform  struct {
				Architecture string `json:"architecture"`
				OS           string `json:"os"`
			} `json:"platform"`
		} `json:"manifests"`
	}

	if err := json.Unmarshal(listBody, &manifestList); err != nil {
		return nil, fmt.Errorf("failed to parse manifest list: %w", err)
	}

	// Find linux/amd64 manifest
	var targetDigest string
	for _, m := range manifestList.Manifests {
		if m.Platform.OS == "linux" && m.Platform.Architecture == "amd64" {
			targetDigest = m.Digest
			break
		}
	}
	if targetDigest == "" && len(manifestList.Manifests) > 0 {
		// Fallback to first manifest
		targetDigest = manifestList.Manifests[0].Digest
	}
	if targetDigest == "" {
		return nil, fmt.Errorf("no suitable manifest found in manifest list")
	}

	log.Debugf("OCIClient: resolved manifest list to digest %s", targetDigest)

	// Fetch the specific platform manifest by digest
	digestRef := ref
	digestRef.Tag = targetDigest
	return c.FetchManifest(ctx, digestRef, auth)
}

// FetchConfig retrieves the image configuration blob
func (c *OCIClient) FetchConfig(ctx context.Context, ref ImageRef, configDigest string, auth *RegistryAuth) (*OCIImageConfig, error) {
	reqURL := fmt.Sprintf("https://%s/v2/%s/blobs/%s", ref.Registry, ref.Repository, configDigest)

	req, err := http.NewRequestWithContext(ctx, "GET", reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create config request: %w", err)
	}

	resp, err := c.doWithAuth(ctx, req, auth)
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
	reqURL := fmt.Sprintf("https://%s/v2/%s/blobs/%s", ref.Registry, ref.Repository, layerDigest)

	resp, err := c.doStreamWithAuth(ctx, reqURL, auth)
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
