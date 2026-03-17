/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package llmgateway

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"k8s.io/klog/v2"
)

// LiteLLMClient encapsulates LiteLLM management API calls.
type LiteLLMClient struct {
	endpoint   string // e.g. "http://10.32.80.50:4000"
	adminKey   string // LiteLLM Master Key
	teamID     string // Global Team ID
	httpClient *http.Client
}

// NewLiteLLMClient creates a new LiteLLM admin client.
func NewLiteLLMClient(endpoint, adminKey, teamID string) *LiteLLMClient {
	return &LiteLLMClient{
		endpoint: endpoint,
		adminKey: adminKey,
		teamID:   teamID,
		httpClient: &http.Client{
			Timeout: 15 * time.Second,
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, // nolint:gosec // skip TLS verify for internal/self-signed certs
			},
		},
	}
}

// ── Request/Response types ────────────────────────────────────────────────

// CreateUserRequest is the request body for POST /user/new
type CreateUserRequest struct {
	UserID    string   `json:"user_id"`
	UserEmail string   `json:"user_email"`
	Teams     []string `json:"teams,omitempty"`
}

// CreateKeyRequest is the request body for POST /key/generate
type CreateKeyRequest struct {
	UserID   string            `json:"user_id"`
	TeamID   string            `json:"team_id"`
	Metadata map[string]string `json:"metadata"`
	KeyAlias string            `json:"key_alias"`
}

// CreateKeyResponse is the response from POST /key/generate
type CreateKeyResponse struct {
	Key     string `json:"key"`      // The generated virtual key (sk-xxx)
	KeyName string `json:"key_name"` // Abbreviated display name (sk-...xxxx), for UI display only
	TokenID string `json:"token"`    // Hashed token stored in LiteLLM DB, used as key identifier for update/delete
	Expires string `json:"expires"`  // Expiration time
}

// UpdateKeyRequest is the request body for POST /key/update
type UpdateKeyRequest struct {
	Key      string            `json:"key,omitempty"`
	KeyAlias string            `json:"key_alias,omitempty"`
	Metadata map[string]string `json:"metadata,omitempty"`
}

// DeleteKeyRequest is the request body for POST /key/delete
type DeleteKeyRequest struct {
	Keys []string `json:"keys"` // List of token hashes to delete
}

// ── API Methods ───────────────────────────────────────────────────────────

// CreateUser creates a LiteLLM User (idempotent — returns existing user if already exists).
func (c *LiteLLMClient) CreateUser(ctx context.Context, email string) error {
	reqBody := CreateUserRequest{
		UserID:    email,
		UserEmail: email,
		Teams:     []string{c.teamID},
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.endpoint+"/user/new", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if c.adminKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.adminKey)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to call LiteLLM: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		respBody, _ := io.ReadAll(resp.Body)
		klog.ErrorS(nil, "LiteLLM create user failed",
			"status", resp.StatusCode, "body", string(respBody))
		return fmt.Errorf("LiteLLM returned HTTP %d: %s", resp.StatusCode, string(respBody))
	}

	klog.Infof("LiteLLM: ensured user exists for %s", email)
	return nil
}

// CreateKey creates a Virtual Key bound to a LiteLLM User via POST /key/generate.
func (c *LiteLLMClient) CreateKey(ctx context.Context, email, apimKey string) (*CreateKeyResponse, error) {
	reqBody := CreateKeyRequest{
		UserID: email,
		TeamID: c.teamID,
		Metadata: map[string]string{
			"apim_key":     apimKey,
			"safe_user_id": email,
		},
		KeyAlias: email,
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.endpoint+"/key/generate", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if c.adminKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.adminKey)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to call LiteLLM: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		klog.ErrorS(nil, "LiteLLM create key failed",
			"status", resp.StatusCode, "body", string(respBody))
		return nil, fmt.Errorf("LiteLLM returned HTTP %d: %s", resp.StatusCode, string(respBody))
	}

	var result CreateKeyResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("failed to decode LiteLLM response: %w", err)
	}

	klog.Infof("LiteLLM: created key for %s, token=%s, key_name=%s", email, result.TokenID, result.KeyName)
	return &result, nil
}

// UpdateKeyMetadata updates the metadata of an existing Virtual Key.
// Both apim_key and safe_user_id are included to prevent metadata loss
// since LiteLLM /key/update replaces the entire metadata object.
func (c *LiteLLMClient) UpdateKeyMetadata(ctx context.Context, tokenHash string, apimKey string, email string) error {
	reqBody := UpdateKeyRequest{
		Key: tokenHash,
		Metadata: map[string]string{
			"apim_key":     apimKey,
			"safe_user_id": email,
		},
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.endpoint+"/key/update", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if c.adminKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.adminKey)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to call LiteLLM: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		klog.ErrorS(nil, "LiteLLM update key failed",
			"status", resp.StatusCode, "body", string(respBody))
		return fmt.Errorf("LiteLLM returned HTTP %d: %s", resp.StatusCode, string(respBody))
	}

	klog.Infof("LiteLLM: updated key metadata for token_hash=%s", tokenHash[:16]+"...")
	return nil
}

// DeleteKey deletes a Virtual Key by its token hash.
func (c *LiteLLMClient) DeleteKey(ctx context.Context, tokenHash string) error {
	reqBody := DeleteKeyRequest{
		Keys: []string{tokenHash},
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.endpoint+"/key/delete", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if c.adminKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.adminKey)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to call LiteLLM: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		klog.ErrorS(nil, "LiteLLM delete key failed",
			"status", resp.StatusCode, "body", string(respBody))
		return fmt.Errorf("LiteLLM returned HTTP %d: %s", resp.StatusCode, string(respBody))
	}

	klog.Infof("LiteLLM: deleted key token_hash=%s", tokenHash[:16]+"...")
	return nil
}
