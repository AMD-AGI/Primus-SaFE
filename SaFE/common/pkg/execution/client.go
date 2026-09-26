/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package execution

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	// mockHeader marks a response from the contract test double shipped with the handoff
	// package. A production client refuses it: a synthetic reservation that reached the
	// dispatcher would put pods on capacity that does not exist.
	mockHeader = "X-Execution-Contract-Mock"

	// defaultTimeout bounds a single call. The scheduler polls on its own cadence, so a
	// request that outlives that cadence is a stall rather than slow progress.
	defaultTimeout = 10 * time.Second

	// maxResponseBytes caps a response body. The contract responses are small; anything
	// larger is a misrouted endpoint rather than a valid answer.
	maxResponseBytes = 4 << 20
)

// Config describes how to reach the external capacity controller. The connection material
// comes from the restricted secret referenced by the cluster, never from a user object.
type Config struct {
	// BaseURL is the controller root, for example https://capacity.internal:8443
	BaseURL string
	// CACert verifies the controller certificate
	CACert []byte
	// ClientCert and ClientKey are the SaFE service identity presented for mTLS
	ClientCert []byte
	ClientKey  []byte
	// Timeout bounds a single call; zero selects the default
	Timeout time.Duration
	// AllowMockServer permits the handoff test double. It must stay false outside local
	// contract tests, and no production configuration path may set it.
	AllowMockServer bool
}

// Client speaks contract 0.2.1 to an external capacity controller.
type Client struct {
	baseURL   *url.URL
	http      *http.Client
	allowMock bool
}

// NewClient builds a client with service mTLS. It fails rather than falling back to an
// unverified connection, because the caller cannot tell a downgraded transport apart from
// a working one once reservations start flowing over it.
func NewClient(cfg Config) (*Client, error) {
	if strings.TrimSpace(cfg.BaseURL) == "" {
		return nil, errors.New("external execution: base url is required")
	}
	parsed, err := url.Parse(cfg.BaseURL)
	if err != nil {
		return nil, fmt.Errorf("external execution: parse base url: %w", err)
	}
	if parsed.Scheme != "https" && !cfg.AllowMockServer {
		return nil, fmt.Errorf("external execution: base url must be https, got %q", parsed.Scheme)
	}

	transport := &http.Transport{}
	if len(cfg.CACert) > 0 || len(cfg.ClientCert) > 0 {
		tlsConfig := &tls.Config{MinVersion: tls.VersionTLS12}
		if len(cfg.CACert) > 0 {
			pool := x509.NewCertPool()
			if !pool.AppendCertsFromPEM(cfg.CACert) {
				return nil, errors.New("external execution: ca certificate is not valid PEM")
			}
			tlsConfig.RootCAs = pool
		}
		if len(cfg.ClientCert) > 0 || len(cfg.ClientKey) > 0 {
			pair, err := tls.X509KeyPair(cfg.ClientCert, cfg.ClientKey)
			if err != nil {
				return nil, fmt.Errorf("external execution: load client keypair: %w", err)
			}
			tlsConfig.Certificates = []tls.Certificate{pair}
		}
		transport.TLSClientConfig = tlsConfig
	}

	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = defaultTimeout
	}
	return &Client{
		baseURL:   parsed,
		allowMock: cfg.AllowMockServer,
		http: &http.Client{
			Timeout:   timeout,
			Transport: transport,
			// A redirect would take a request carrying the service identity to an
			// address the configuration never approved.
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
	}, nil
}

// PublishDemand records the current unmet need for one workload. Publishing a revision is
// idempotent: replaying the same request id with the same body returns the stored demand.
func (c *Client) PublishDemand(ctx context.Context, demand *CapacityDemand) (*CapacityDemand, error) {
	if demand == nil {
		return nil, errors.New("external execution: demand is required")
	}
	demand.APIVersion = APIVersion
	var out CapacityDemand
	path := "/v1alpha1/capacity-demands/" + url.PathEscape(demand.DemandID)
	if err := c.do(ctx, http.MethodPut, path, demand, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// PlanPlacements asks for candidate seats. The plan holds no capacity: another claim may
// take the same devices before this one is submitted, which surfaces as a conflict.
func (c *Client) PlanPlacements(ctx context.Context, req *PlacementPlanRequest) (*PlacementPlanResponse, error) {
	if req == nil {
		return nil, errors.New("external execution: plan request is required")
	}
	req.APIVersion = APIVersion
	var out PlacementPlanResponse
	if err := c.do(ctx, http.MethodPost, "/v1alpha1/placement-plans", req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// CreateClaim converts approved placements into a reservation, all units or none.
//
// On an uncertain outcome the caller must replay this exact request id and body rather
// than issue a new claim id: a second id would reserve a second set of devices for the
// same workload, and nothing later would reconcile the surplus.
func (c *Client) CreateClaim(ctx context.Context, req *ClaimRequest) (*ClaimResponse, error) {
	if req == nil {
		return nil, errors.New("external execution: claim request is required")
	}
	req.APIVersion = APIVersion
	var out ClaimResponse
	if err := c.do(ctx, http.MethodPost, "/v1alpha1/claims", req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// GetClaim re-reads a reservation. It is the recovery path after a lost response and the
// mandatory recheck before dispatch.
func (c *Client) GetClaim(ctx context.Context, claimID string) (*ClaimResponse, error) {
	if strings.TrimSpace(claimID) == "" {
		return nil, errors.New("external execution: claim id is required")
	}
	var out ClaimResponse
	path := "/v1alpha1/claims/" + url.PathEscape(claimID)
	if err := c.do(ctx, http.MethodGet, path, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ReleaseClaim withdraws a reservation. A Revoking phase in the reply records that the
// withdrawal was accepted, not that the devices are free; the caller keeps the resources
// charged until the provider reports Released.
func (c *Client) ReleaseClaim(ctx context.Context, claimID string, req *ReleaseRequest) (*ClaimResponse, error) {
	if strings.TrimSpace(claimID) == "" {
		return nil, errors.New("external execution: claim id is required")
	}
	if req == nil {
		return nil, errors.New("external execution: release request is required")
	}
	req.APIVersion = APIVersion
	var out ClaimResponse
	path := "/v1alpha1/claims/" + url.PathEscape(claimID) + "/release"
	if err := c.do(ctx, http.MethodPost, path, req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// do performs one contract call and decodes the reply.
func (c *Client) do(ctx context.Context, method, path string, body, out any) error {
	var reader io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("external execution: encode request: %w", err)
		}
		reader = bytes.NewReader(encoded)
	}

	endpoint := *c.baseURL
	endpoint.Path = strings.TrimSuffix(endpoint.Path, "/") + path
	req, err := http.NewRequestWithContext(ctx, method, endpoint.String(), reader)
	if err != nil {
		return fmt.Errorf("external execution: build request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("external execution: %s %s: %w", method, path, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if !c.allowMock && strings.EqualFold(resp.Header.Get(mockHeader), "true") {
		return fmt.Errorf("external execution: refusing response from contract mock at %s", endpoint.Host)
	}

	payload, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if err != nil {
		return fmt.Errorf("external execution: read response: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return decodeAPIError(resp.StatusCode, payload)
	}
	if out == nil {
		return nil
	}

	decoder := json.NewDecoder(bytes.NewReader(payload))
	// An unknown field means the peer speaks a contract this build does not implement.
	// Ignoring it would let a profile this client cannot honour look like a supported one.
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(out); err != nil {
		return fmt.Errorf("external execution: decode response: %w", err)
	}
	return nil
}

// decodeAPIError turns a refusal into a typed error. A body that does not parse still
// produces an APIError so the caller sees the status rather than a decode failure.
func decodeAPIError(status int, payload []byte) error {
	apiErr := &APIError{StatusCode: status, Code: CodeUnavailable, Message: strings.TrimSpace(string(payload))}
	var wire wireError
	if err := json.Unmarshal(payload, &wire); err == nil && wire.Code != "" {
		apiErr.RequestID = wire.RequestID
		apiErr.Code = wire.Code
		apiErr.Message = wire.Message
		apiErr.Retryable = wire.Retryable
		apiErr.RetryAfterS = wire.RetryAfterS
	}
	if apiErr.Message == "" {
		apiErr.Message = http.StatusText(status)
	}
	return apiErr
}
