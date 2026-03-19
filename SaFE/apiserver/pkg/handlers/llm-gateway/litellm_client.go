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

func (e *litellmError) Error() string {
	return fmt.Sprintf("LiteLLM returned HTTP %d: %s", e.StatusCode, e.Body)
}

func isNotFoundErr(err error) bool {
	if e, ok := err.(*litellmError); ok {
		return e.StatusCode == http.StatusNotFound
	}
	return false
}

// ── Budget & Tag API Methods ──────────────────────────────────────────────

// GetKeyInfo queries a Virtual Key's spend and budget status via GET /key/info.
func (c *LiteLLMClient) GetKeyInfo(ctx context.Context, keyHash string) (*KeyInfoData, error) {
	reqURL := fmt.Sprintf("%s/key/info?key=%s", c.endpoint, keyHash)

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
		return nil, fmt.Errorf("LiteLLM returned HTTP %d: %s", resp.StatusCode, string(respBody))
	}

	var result KeyInfoResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &result.Info, nil
}

// UpdateKeyBudget sets or removes the max_budget on a Virtual Key via POST /key/update.
// Pass nil to remove the budget limit.
func (c *LiteLLMClient) UpdateKeyBudget(ctx context.Context, keyHash string, maxBudget *float64) error {
	reqBody := UpdateKeyBudgetRequest{
		Key:       keyHash,
		MaxBudget: maxBudget,
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
		return fmt.Errorf("LiteLLM returned HTTP %d: %s", resp.StatusCode, string(respBody))
	}

	return nil
}

// GetSpendLogs queries a single page of spend logs for a specific user via GET /spend/logs/v2.
func (c *LiteLLMClient) GetSpendLogs(ctx context.Context, userID, startDate, endDate string, page, pageSize int) (*SpendLogsResponse, error) {
	if pageSize <= 0 {
		pageSize = 100
	}
	if page <= 0 {
		page = 1
	}

	reqURL := fmt.Sprintf("%s/spend/logs/v2?user_id=%s&start_date=%s&end_date=%s&page=%d&page_size=%d",
		c.endpoint, userID, startDate, endDate, page, pageSize)

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
		return nil, fmt.Errorf("LiteLLM returned HTTP %d: %s", resp.StatusCode, string(respBody))
	}

	var result SpendLogsResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &result, nil
}

// GetAllSpendLogs fetches all spend logs by iterating through pages.
// maxPages limits the total pages to avoid runaway queries (0 = no limit).
func (c *LiteLLMClient) GetAllSpendLogs(ctx context.Context, userID, startDate, endDate string, maxPages int) ([]SpendLogEntry, error) {
	const pageSize = 100
	var allLogs []SpendLogEntry

	for page := 1; ; page++ {
		if maxPages > 0 && page > maxPages {
			klog.Warningf("GetAllSpendLogs: reached max pages %d for user %s, returning partial data", maxPages, userID)
			break
		}

		resp, err := c.GetSpendLogs(ctx, userID, startDate, endDate, page, pageSize)
		if err != nil {
			return nil, err
		}

		allLogs = append(allLogs, resp.Data...)

		if page >= resp.TotalPages || len(resp.Data) == 0 {
			break
		}
	}

	return allLogs, nil
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

// GetUserInfo queries LiteLLM for a user's cumulative spend and key info.
func (c *LiteLLMClient) GetUserInfo(ctx context.Context, userID string) (*UserInfoResponse, error) {
	reqURL := fmt.Sprintf("%s/user/info?user_id=%s", c.endpoint, userID)

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
		klog.ErrorS(nil, "LiteLLM get user info failed",
			"status", resp.StatusCode, "body", string(respBody))
		return nil, fmt.Errorf("LiteLLM returned HTTP %d: %s", resp.StatusCode, string(respBody))
	}

	var result UserInfoResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("failed to decode LiteLLM response: %w", err)
	}

	return &result, nil
}
