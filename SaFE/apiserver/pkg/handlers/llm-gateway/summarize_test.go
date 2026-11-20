/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package llmgateway

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/golang/mock/gomock"
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
