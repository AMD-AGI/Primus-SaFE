/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package llmgateway

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	commonconfig "github.com/AMD-AIG-AIMA/SAFE/common/pkg/config"
	"k8s.io/klog/v2"
)

const pendingSummarySystemPrompt = "You explain to a non-technical user why a workload is waiting. Reply in one or two short plain sentences: first what is blocking it and why, then what the user or an administrator can do. Avoid internal terms such as scheduling gate, scale-up, QoS or provisioning request; when one is unavoidable, say it in everyday words, for example that the account is not allowed to use this kind of node. Use only the evidence. Do not invent causes, names, or numbers that are not in the evidence."

const pendingSummaryInputLimit = 6000

// pendingSummaryModel is the configured model. Tests may replace it.
var pendingSummaryModel = commonconfig.GetLLMGatewaySummaryModel

type pendingSummaryRequest struct {
	Model        string                  `json:"model"`
	Messages     []pendingSummaryMessage `json:"messages"`
	MaxTokens    int                     `json:"max_tokens"`
	Temperature  float64                 `json:"temperature"`
	ExtraHeaders map[string]string       `json:"extra_headers,omitempty"`
}

type pendingSummaryMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type pendingSummaryResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
		Text string `json:"text"`
	} `json:"choices"`
}

// SummarizePendingReason asks LiteLLM for one sentence, billed to the caller's virtual key.
// email selects the binding. When the binding has an APIM key, the upstream user is chosen
// as on the proxy path: the shared account for a shared key, otherwise ntid.
func (h *Handler) SummarizePendingReason(ctx context.Context, email, ntid, evidence string) (string, error) {
	if h == nil || h.litellmClient == nil || h.crypto == nil {
		return "", fmt.Errorf("litellm client is not configured")
	}
	model := strings.TrimSpace(pendingSummaryModel())
	if model == "" {
		return "", fmt.Errorf("summary model is not configured")
	}
	email = strings.TrimSpace(email)
	if email == "" {
		return "", fmt.Errorf("caller email is empty")
	}
	evidence = clipRunes(strings.TrimSpace(evidence), pendingSummaryInputLimit)
	if evidence == "" {
		return "", fmt.Errorf("pending evidence is empty")
	}

	binding, err := h.resolveBinding(ctx, email)
	if err != nil {
		return "", err
	}
	if binding == nil || binding.LiteLLMVirtualKey == "" {
		return "", fmt.Errorf("litellm virtual key is missing")
	}
	virtualKey, err := h.crypto.Decrypt(binding.LiteLLMVirtualKey)
	if err != nil {
		return "", fmt.Errorf("failed to decrypt virtual key: %w", err)
	}

	reqBody := pendingSummaryRequest{
		Model: model,
		Messages: []pendingSummaryMessage{
			{Role: "system", Content: pendingSummarySystemPrompt},
			{Role: "user", Content: evidence},
		},
		MaxTokens:   200,
		Temperature: 0,
	}
	if err = h.applyPendingSummaryHeaders(&reqBody, binding.ApimKey, upstreamUser(binding.ApimKeyHash, ntid)); err != nil {
		return "", err
	}
	body, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("failed to marshal summary request: %w", err)
	}

	endpoint := strings.TrimRight(h.litellmClient.endpoint, "/")
	if endpoint == "" {
		return "", fmt.Errorf("litellm endpoint is empty")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint+"/v1/chat/completions", bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("failed to create summary request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+virtualKey)

	resp, err := h.litellmClient.do(req)
	if err != nil {
		return "", fmt.Errorf("failed to call litellm: %w", err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusOK {
		klog.ErrorS(nil, "LiteLLM pending summary failed", "status", resp.StatusCode, "email", email)
		return "", fmt.Errorf("litellm returned HTTP %d", resp.StatusCode)
	}
	var parsed pendingSummaryResponse
	if err = json.Unmarshal(respBody, &parsed); err != nil {
		return "", fmt.Errorf("failed to decode litellm response: %w", err)
	}
	if len(parsed.Choices) == 0 {
		return "", fmt.Errorf("litellm returned no choices")
	}
	text := strings.TrimSpace(parsed.Choices[0].Message.Content)
	if text == "" {
		text = strings.TrimSpace(parsed.Choices[0].Text)
	}
	if text == "" {
		return "", fmt.Errorf("litellm returned an empty summary")
	}
	return text, nil
}

// applyPendingSummaryHeaders adds the APIM key and upstream user the proxy would stamp.
func (h *Handler) applyPendingSummaryHeaders(req *pendingSummaryRequest, encryptedApimKey, ntid string) error {
	if req == nil || encryptedApimKey == "" {
		return nil
	}
	apimKey, err := h.crypto.Decrypt(encryptedApimKey)
	if err != nil {
		return fmt.Errorf("failed to decrypt apim key: %w", err)
	}
	apimKey = strings.TrimSpace(apimKey)
	ntid = strings.TrimSpace(ntid)
	if apimKey == "" && ntid == "" {
		return nil
	}
	req.ExtraHeaders = map[string]string{}
	if apimKey != "" {
		req.ExtraHeaders[apimKeyHeader] = apimKey
	}
	if ntid != "" {
		req.ExtraHeaders[upstreamUserHeader] = ntid
	}
	return nil
}

// clipRunes shortens s to at most n runes.
func clipRunes(s string, n int) string {
	if n <= 0 {
		return ""
	}
	runes := []rune(s)
	if len(runes) <= n {
		return s
	}
	return string(runes[:n])
}
