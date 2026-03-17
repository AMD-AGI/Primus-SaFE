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
	Keys       []string `json:"keys,omitempty"`        // List of token hashes to delete
	KeyAliases []string `json:"key_aliases,omitempty"` // Alternative: delete by key alias (e.g. user email)
}

// ── Usage types ───────────────────────────────────────────────────────────

// DailyActivityResponse is the response from GET /user/daily/activity
type DailyActivityResponse struct {
	Results  []DailyResult  `json:"results"`
	Metadata ActivityTotals `json:"metadata"`
}

type DailyResult struct {
	Date      string           `json:"date"`
	Metrics   DailyMetrics     `json:"metrics"`
	Breakdown *DailyBreakdown  `json:"breakdown,omitempty"`
}

type DailyMetrics struct {
	Spend            float64 `json:"spend"`
	PromptTokens     int64   `json:"prompt_tokens"`
	CompletionTokens int64   `json:"completion_tokens"`
	TotalTokens      int64   `json:"total_tokens"`
	APIRequests      int64   `json:"api_requests"`
}

type DailyBreakdown struct {
	Models    map[string]DailyMetrics `json:"models,omitempty"`
	Providers map[string]DailyMetrics `json:"providers,omitempty"`
}

type ActivityTotals struct {
	TotalSpend            float64 `json:"total_spend"`
	TotalPromptTokens     int64   `json:"total_prompt_tokens"`
	TotalCompletionTokens int64   `json:"total_completion_tokens"`
	TotalAPIRequests      int64   `json:"total_api_requests"`
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

	switch resp.StatusCode {
	case http.StatusOK, http.StatusCreated:
		klog.Infof("LiteLLM: created user for %s", email)
	case http.StatusConflict:
		klog.Infof("LiteLLM: user already exists for %s, skipping", email)
	default:
		respBody, _ := io.ReadAll(resp.Body)
		klog.ErrorS(nil, "LiteLLM create user failed",
			"status", resp.StatusCode, "body", string(respBody))
		return fmt.Errorf("LiteLLM returned HTTP %d: %s", resp.StatusCode, string(respBody))
	}

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
// If the hash-based delete returns 404, it falls back to deleting by key_alias
// to handle cases where the stored hash doesn't match LiteLLM's record.
func (c *LiteLLMClient) DeleteKey(ctx context.Context, tokenHash string, keyAlias string) error {
	err := c.doDeleteKey(ctx, DeleteKeyRequest{Keys: []string{tokenHash}})
	if err == nil {
		klog.Infof("LiteLLM: deleted key by hash, token_hash=%s", tokenHash[:16]+"...")
		return nil
	}

	if !isNotFoundErr(err) {
		return err
	}

	if keyAlias == "" {
		klog.Warningf("LiteLLM: key not found by hash and no alias to fallback, token_hash=%s", tokenHash[:16]+"...")
		return nil
	}

	klog.Warningf("LiteLLM: key not found by hash (404), retrying by alias=%s", keyAlias)
	if err := c.doDeleteKey(ctx, DeleteKeyRequest{KeyAliases: []string{keyAlias}}); err != nil {
		return fmt.Errorf("failed to delete key by alias %s: %w", keyAlias, err)
	}

	klog.Infof("LiteLLM: deleted key by alias=%s", keyAlias)
	return nil
}

func (c *LiteLLMClient) doDeleteKey(ctx context.Context, reqBody DeleteKeyRequest) error {
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

	if resp.StatusCode == http.StatusOK {
		return nil
	}

	respBody, _ := io.ReadAll(resp.Body)
	klog.ErrorS(nil, "LiteLLM delete key failed",
		"status", resp.StatusCode, "body", string(respBody))
	return &litellmError{StatusCode: resp.StatusCode, Body: string(respBody)}
}

type litellmError struct {
	StatusCode int
	Body       string
}

func (e *litellmError) Error() string {
	return fmt.Sprintf("LiteLLM returned HTTP %d: %s", e.StatusCode, e.Body)
}

func isNotFoundErr(err error) bool {
	if e, ok := err.(*litellmError); ok {
		return e.StatusCode == http.StatusNotFound
	}
	return false
}

// GetUserDailyActivity queries LiteLLM for a user's daily usage breakdown.
func (c *LiteLLMClient) GetUserDailyActivity(ctx context.Context, userID, startDate, endDate string) (*DailyActivityResponse, error) {
	reqURL := fmt.Sprintf("%s/user/daily/activity?user_id=%s&start_date=%s&end_date=%s",
		c.endpoint, userID, startDate, endDate)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
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
		klog.ErrorS(nil, "LiteLLM get user daily activity failed",
			"status", resp.StatusCode, "body", string(respBody))
		return nil, fmt.Errorf("LiteLLM returned HTTP %d: %s", resp.StatusCode, string(respBody))
	}

	var result DailyActivityResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("failed to decode LiteLLM response: %w", err)
	}

	return &result, nil
}
