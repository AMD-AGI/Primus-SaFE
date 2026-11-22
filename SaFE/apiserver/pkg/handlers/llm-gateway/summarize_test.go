/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package llmgateway

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	dbclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
	mock_client "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client/mock"
)

func TestSummarizePendingReasonUsesCallerVirtualKey(t *testing.T) {
	orig := pendingSummaryModel
	pendingSummaryModel = func() string { return "summary-model" }
	t.Cleanup(func() { pendingSummaryModel = orig })

	ctrl := gomock.NewController(t)
	mockDB := mock_client.NewMockInterface(ctrl)
	var gotAuth string
	var gotBody pendingSummaryRequest
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		raw, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		require.NoError(t, json.Unmarshal(raw, &gotBody))
		assert.Equal(t, "/v1/chat/completions", r.URL.Path)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"The image cannot be pulled."}}]}`))
	}))
	t.Cleanup(backend.Close)

	handler := newTestHandler(t, mockDB, backend)
	mockDB.EXPECT().GetLLMBindingByEmail(gomock.Any(), "user@amd.com").Return(&dbclient.LLMGatewayUserBinding{
		UserEmail:         "user@amd.com",
		LiteLLMVirtualKey: "virtual-key",
		ApimKey:           "apim-key",
	}, nil)

	got, err := handler.SummarizePendingReason(context.Background(), "user@amd.com", "ntid1", "pod waiting ImagePullBackOff")
	require.NoError(t, err)
	assert.Equal(t, "The image cannot be pulled.", got)
	assert.Equal(t, "Bearer virtual-key", gotAuth)
	assert.Equal(t, "summary-model", gotBody.Model)
	assert.Equal(t, "apim-key", gotBody.ExtraHeaders[apimKeyHeader])
	assert.Equal(t, "ntid1", gotBody.ExtraHeaders[upstreamUserHeader])
	require.Len(t, gotBody.Messages, 2)
	assert.Contains(t, gotBody.Messages[1].Content, "ImagePullBackOff")
}

func TestSummarizePendingReasonRequiresModel(t *testing.T) {
	orig := pendingSummaryModel
	pendingSummaryModel = func() string { return "  " }
	t.Cleanup(func() { pendingSummaryModel = orig })

	backend := httptest.NewServer(http.NotFoundHandler())
	t.Cleanup(backend.Close)
	handler := newTestHandler(t, nil, backend)
	_, err := handler.SummarizePendingReason(context.Background(), "user@amd.com", "", "evidence")
	assert.ErrorContains(t, err, "summary model is not configured")
}

func TestSummarizePendingReasonPreflightErrors(t *testing.T) {
	orig := pendingSummaryModel
	pendingSummaryModel = func() string { return "summary-model" }
	t.Cleanup(func() { pendingSummaryModel = orig })

	var nilHandler *Handler
	_, err := nilHandler.SummarizePendingReason(context.Background(), "user@amd.com", "", "evidence")
	assert.ErrorContains(t, err, "litellm client is not configured")

	backend := httptest.NewServer(http.NotFoundHandler())
	t.Cleanup(backend.Close)
	handler := newTestHandler(t, nil, backend)
	_, err = handler.SummarizePendingReason(context.Background(), "  ", "", "evidence")
	assert.ErrorContains(t, err, "caller email is empty")
	_, err = handler.SummarizePendingReason(context.Background(), "user@amd.com", "", "   ")
	assert.ErrorContains(t, err, "pending evidence is empty")
}

func TestSummarizePendingReasonBindingErrors(t *testing.T) {
	orig := pendingSummaryModel
	pendingSummaryModel = func() string { return "summary-model" }
	t.Cleanup(func() { pendingSummaryModel = orig })

	backend := httptest.NewServer(http.NotFoundHandler())
	t.Cleanup(backend.Close)

	tests := []struct {
		name    string
		binding *dbclient.LLMGatewayUserBinding
		dbErr   error
		wantErr string
	}{
		{name: "binding lookup fails", dbErr: errors.New("db down"), wantErr: "db down"},
		{name: "binding without virtual key", binding: &dbclient.LLMGatewayUserBinding{UserEmail: "user@amd.com"}, wantErr: "virtual key is missing"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			mockDB := mock_client.NewMockInterface(ctrl)
			mockDB.EXPECT().GetLLMBindingByEmail(gomock.Any(), "user@amd.com").Return(tt.binding, tt.dbErr)
			handler := newTestHandler(t, mockDB, backend)
			_, err := handler.SummarizePendingReason(context.Background(), "user@amd.com", "", "evidence")
			assert.ErrorContains(t, err, tt.wantErr)
		})
	}
}

func TestSummarizePendingReasonResponses(t *testing.T) {
	orig := pendingSummaryModel
	pendingSummaryModel = func() string { return "summary-model" }
	t.Cleanup(func() { pendingSummaryModel = orig })

	tests := []struct {
		name    string
		status  int
		body    string
		want    string
		wantErr string
	}{
		{name: "text field is used when content is empty", status: http.StatusOK, body: `{"choices":[{"message":{"content":""},"text":" Waiting for GPUs. "}]}`, want: "Waiting for GPUs."},
		{name: "non 200 status", status: http.StatusBadGateway, body: `{}`, wantErr: "HTTP 502"},
		{name: "invalid json", status: http.StatusOK, body: `not json`, wantErr: "failed to decode"},
		{name: "no choices", status: http.StatusOK, body: `{"choices":[]}`, wantErr: "no choices"},
		{name: "empty summary", status: http.StatusOK, body: `{"choices":[{"message":{"content":"  "}}]}`, wantErr: "empty summary"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.status)
				_, _ = w.Write([]byte(tt.body))
			}))
			t.Cleanup(backend.Close)
			ctrl := gomock.NewController(t)
			mockDB := mock_client.NewMockInterface(ctrl)
			mockDB.EXPECT().GetLLMBindingByEmail(gomock.Any(), "user@amd.com").Return(&dbclient.LLMGatewayUserBinding{
				UserEmail:         "user@amd.com",
				LiteLLMVirtualKey: "virtual-key",
			}, nil)
			handler := newTestHandler(t, mockDB, backend)
			got, err := handler.SummarizePendingReason(context.Background(), "user@amd.com", "", "evidence")
			if tt.wantErr != "" {
				assert.ErrorContains(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestApplyPendingSummaryHeaders(t *testing.T) {
	backend := httptest.NewServer(http.NotFoundHandler())
	t.Cleanup(backend.Close)
	handler := newTestHandler(t, nil, backend)

	// nil request and missing key leave the request alone
	require.NoError(t, handler.applyPendingSummaryHeaders(nil, "apim", "ntid"))
	req := &pendingSummaryRequest{}
	require.NoError(t, handler.applyPendingSummaryHeaders(req, "", "ntid"))
	assert.Nil(t, req.ExtraHeaders)

	// blank key and blank ntid add nothing
	require.NoError(t, handler.applyPendingSummaryHeaders(req, "  ", " "))
	assert.Nil(t, req.ExtraHeaders)

	// the key alone is stamped without an upstream user
	require.NoError(t, handler.applyPendingSummaryHeaders(req, "apim", " "))
	assert.Equal(t, map[string]string{apimKeyHeader: "apim"}, req.ExtraHeaders)
}

func TestClipRunes(t *testing.T) {
	assert.Equal(t, "", clipRunes("abc", 0))
	assert.Equal(t, "abc", clipRunes("abc", 5))
	assert.Equal(t, "ab", clipRunes("abc", 2))
	assert.Equal(t, "\u4e2d\u6587", clipRunes("\u4e2d\u6587\u5b57", 2))
}

func TestSummarizePendingReasonSharedApimUser(t *testing.T) {
	orig := pendingSummaryModel
	pendingSummaryModel = func() string { return "summary-model" }
	t.Cleanup(func() { pendingSummaryModel = orig })
	dir := t.TempDir()
	viper.Set("llm_gateway.secret_path", dir)
	t.Cleanup(func() { viper.Set("llm_gateway.secret_path", "") })
	require.NoError(t, os.WriteFile(filepath.Join(dir, "shared_apim_key_hashes"), []byte(dbclient.ComputeApimKeyHash("shared-key")), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "shared_apim_user"), []byte("shared-account"), 0o600))

	tests := []struct {
		name     string
		apimKey  string
		wantUser string
	}{
		{name: "shared key is attributed to the shared account", apimKey: "shared-key", wantUser: "shared-account"},
		{name: "own key is attributed to the caller", apimKey: "own-key", wantUser: "ntid1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var gotBody pendingSummaryRequest
			backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				raw, _ := io.ReadAll(r.Body)
				_ = json.Unmarshal(raw, &gotBody)
				_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"ok"}}]}`))
			}))
			t.Cleanup(backend.Close)
			ctrl := gomock.NewController(t)
			mockDB := mock_client.NewMockInterface(ctrl)
			mockDB.EXPECT().GetLLMBindingByEmail(gomock.Any(), "user@amd.com").Return(&dbclient.LLMGatewayUserBinding{
				UserEmail:         "user@amd.com",
				LiteLLMVirtualKey: "virtual-key",
				ApimKey:           tt.apimKey,
				ApimKeyHash:       dbclient.ComputeApimKeyHash(tt.apimKey),
			}, nil)
			handler := newTestHandler(t, mockDB, backend)
			_, err := handler.SummarizePendingReason(context.Background(), "user@amd.com", "ntid1", "evidence")
			require.NoError(t, err)
			assert.Equal(t, tt.wantUser, gotBody.ExtraHeaders[upstreamUserHeader])
		})
	}
}
