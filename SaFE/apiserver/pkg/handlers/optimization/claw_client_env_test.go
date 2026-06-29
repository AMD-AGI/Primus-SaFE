/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package optimization

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestMessageRequestEnvMarshal verifies the session-scoped Env map serializes
// to the wire field `env` (consumed by Claw as body.env / session_env) and is
// omitted entirely when empty so existing callers send no env key.
func TestMessageRequestEnvMarshal(t *testing.T) {
	withEnv, err := json.Marshal(&MessageRequest{
		Content: "p",
		Env:     map[string]string{"CLAUDE_MODEL": "claude-opus-4-8"},
	})
	assert.NoError(t, err)
	assert.Contains(t, string(withEnv), `"env":{"CLAUDE_MODEL":"claude-opus-4-8"}`)

	// nil Env -> omitempty drops the key (no behavior change for callers that
	// don't set env).
	noEnv, err := json.Marshal(&MessageRequest{Content: "p"})
	assert.NoError(t, err)
	assert.NotContains(t, string(noEnv), `"env"`)
}

// TestSendMessageForwardsEnv asserts SendMessage puts the Env map onto the POST
// /sessions/{id}/messages body as `env`, which is what Claw validates and
// injects into the sandbox as session_env (highest precedence).
func TestSendMessageForwardsEnv(t *testing.T) {
	var captured map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &captured)
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	client := NewClawClient(srv.URL, "test-key")
	err := client.SendMessage(context.Background(), "s1", &MessageRequest{
		Content: "prompt",
		Env: map[string]string{
			"CLAUDE_MODEL": "claude-opus-4-8",
			"INFERENCE_OPTIMIZER_ALLOW_CUSTOM_ORCH_MODEL": "1",
		},
	})
	assert.NoError(t, err)

	envMap, ok := captured["env"].(map[string]any)
	assert.True(t, ok, "request body must carry an env object")
	assert.Equal(t, "claude-opus-4-8", envMap["CLAUDE_MODEL"])
	assert.Equal(t, "1", envMap["INFERENCE_OPTIMIZER_ALLOW_CUSTOM_ORCH_MODEL"])
}

// TestSendMessageOmitsEnvWhenUnset asserts a message without Env sends no `env`
// field, so Claw treats it as an empty session_env.
func TestSendMessageOmitsEnvWhenUnset(t *testing.T) {
	var captured map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &captured)
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	client := NewClawClient(srv.URL, "test-key")
	err := client.SendMessage(context.Background(), "s1", &MessageRequest{Content: "prompt"})
	assert.NoError(t, err)

	_, hasEnv := captured["env"]
	assert.False(t, hasEnv, "request body must not carry env when unset")
}
