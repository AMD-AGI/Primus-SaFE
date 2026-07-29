/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package model_handlers

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"testing"
	"time"

	dbclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
	mock_client "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client/mock"
	"github.com/gin-gonic/gin"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	testifyassert "github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	ctrlfake "sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/lib/pq"
)

// TestFormatMetricValue verifies value formatting for each supported type.
func TestFormatMetricValue(t *testing.T) {
	if formatMetricValue(nil) != "-" {
		t.Error("nil should format to -")
	}
	if formatMetricValue("") != "-" {
		t.Error("empty string should format to -")
	}
	if formatMetricValue("abc") != "abc" {
		t.Error("string should pass through")
	}
	if formatMetricValue(float64(5)) != "5" {
		t.Error("integral float should format without decimals")
	}
	if formatMetricValue(float64(1.5)) != "1.5" {
		t.Error("fractional float should keep precision")
	}
	if formatMetricValue(true) != "true" || formatMetricValue(false) != "false" {
		t.Error("bool formatting mismatch")
	}
	if formatMetricValue(42) != "42" {
		t.Error("default formatting mismatch")
	}
}

// TestBuildRLSnapshots verifies RL parameter/resource snapshots are serialized.
func TestBuildRLSnapshots(t *testing.T) {
	req := CreateRlJobRequest{NodeCount: 2, GpuCount: 8, Cpu: "128"}
	params, resources, err := buildRLSnapshots(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if params == "" || resources == "" {
		t.Error("expected non-empty snapshots")
	}
}

// TestDecodeJSONString verifies JSON decoding with fallbacks.
func TestDecodeJSONString(t *testing.T) {
	if decodeJSONString(sql.NullString{}) != nil {
		t.Error("invalid NullString should decode to nil")
	}
	out := decodeJSONString(sql.NullString{String: `{"a":1}`, Valid: true})
	if out["a"].(float64) != 1 {
		t.Error("valid JSON should decode")
	}
	bad := decodeJSONString(sql.NullString{String: "not-json", Valid: true})
	if bad["raw"] != "not-json" {
		t.Error("invalid JSON should fall back to raw")
	}
}

// TestSummarizeParameters verifies parameter summaries for sft and rl variants.
func TestSummarizeParameters(t *testing.T) {
	sftLora := summarizeParameters("sft", "lora", sql.NullString{},
		sql.NullString{String: `{"finetuneLr":0.0001,"peftDim":16,"peftAlpha":32}`, Valid: true})
	if sftLora == "" {
		t.Error("expected non-empty sft lora summary")
	}
	sftFull := summarizeParameters("sft", "full", sql.NullString{},
		sql.NullString{String: `{"finetuneLr":0.00001,"trainIters":100,"saveInterval":50}`, Valid: true})
	if sftFull == "" {
		t.Error("expected non-empty sft full summary")
	}
	rlMega := summarizeParameters("rl", "megatron", sql.NullString{String: "grpo", Valid: true},
		sql.NullString{String: `{"megatronTpSize":4,"megatronPpSize":1,"megatronCpSize":1,"trainBatchSize":128}`, Valid: true})
	if rlMega == "" {
		t.Error("expected non-empty rl megatron summary")
	}
	rlFsdp := summarizeParameters("rl", "fsdp2", sql.NullString{},
		sql.NullString{String: `{"trainBatchSize":64,"totalEpochs":2}`, Valid: true})
	if rlFsdp == "" {
		t.Error("expected non-empty rl fsdp summary")
	}
	if summarizeParameters("sft", "full", sql.NullString{}, sql.NullString{}) != "" {
		t.Error("empty snapshot should yield empty summary")
	}
}

// TestFormatDuration verifies duration formatting for valid and invalid timestamps.
func TestFormatDuration(t *testing.T) {
	if formatDuration(pq.NullTime{}, pq.NullTime{}, pq.NullTime{}) != "" {
		t.Error("invalid start should yield empty")
	}
	start := pq.NullTime{Time: time.Now().Add(-time.Hour), Valid: true}
	end := pq.NullTime{Time: time.Now(), Valid: true}
	if formatDuration(start, end, pq.NullTime{}) == "" {
		t.Error("valid range should yield a duration string")
	}
}

// TestDefaultLensTimeRangeForItem verifies time range derivation from item timestamps.
func TestDefaultLensTimeRangeForItem(t *testing.T) {
	_, _, err := defaultLensTimeRangeForItem(PosttrainRunItem{})
	if err == nil {
		t.Error("expected error when no timestamps are available")
	}
	s, e, err := defaultLensTimeRangeForItem(PosttrainRunItem{
		StartTime: "2026-01-01T00:00:00.000Z",
		EndTime:   "2026-01-01T01:00:00.000Z",
	})
	if err != nil || s == "" || e == "" {
		t.Errorf("expected valid range, got s=%s e=%s err=%v", s, e, err)
	}
}

// TestDefaultStatus verifies the status default falls back to Pending.
func TestDefaultStatus(t *testing.T) {
	if defaultStatus(sql.NullString{}) != "Pending" {
		t.Error("invalid status should default to Pending")
	}
	if defaultStatus(sql.NullString{String: "", Valid: true}) != "Pending" {
		t.Error("empty status should default to Pending")
	}
	if defaultStatus(sql.NullString{String: "Running", Valid: true}) != "Running" {
		t.Error("valid status should be returned as-is")
	}
}

// TestNullStringValue verifies NullString extraction.
func TestNullStringValue(t *testing.T) {
	if nullStringValue(sql.NullString{}) != "" {
		t.Error("invalid NullString should yield empty")
	}
	if nullStringValue(sql.NullString{String: "x", Valid: true}) != "x" {
		t.Error("valid NullString should yield value")
	}
}

// TestNullInt32Value verifies NullInt32 extraction.
func TestNullInt32Value(t *testing.T) {
	if nullInt32Value(sql.NullInt32{}) != 0 {
		t.Error("invalid NullInt32 should yield 0")
	}
	if nullInt32Value(sql.NullInt32{Int32: 7, Valid: true}) != 7 {
		t.Error("valid NullInt32 should yield value")
	}
}

// TestNullTimeString verifies NullTime formatting.
func TestNullTimeString(t *testing.T) {
	if nullTimeString(pq.NullTime{}) != "" {
		t.Error("invalid NullTime should yield empty string")
	}
	if nullTimeString(pq.NullTime{Time: time.Now(), Valid: true}) == "" {
		t.Error("valid NullTime should yield a formatted string")
	}
}

// TestNullStringAndInt32 verifies round-trip helpers for building null values.
func TestNullStringAndInt32(t *testing.T) {
	if nullString("").Valid {
		t.Error("empty string should produce invalid NullString")
	}
	if v := nullString("a"); !v.Valid || v.String != "a" {
		t.Error("non-empty string should produce valid NullString")
	}
	if nullInt32(0).Valid {
		t.Error("zero should produce invalid NullInt32")
	}
	if v := nullInt32(5); !v.Valid || v.Int32 != 5 {
		t.Error("non-zero should produce valid NullInt32")
	}
}

// TestSftStrategy verifies peft maps to a strategy name.
func TestSftStrategy(t *testing.T) {
	if sftStrategy("lora") != "lora" {
		t.Error("lora peft should map to lora strategy")
	}
	if sftStrategy("none") != "full" {
		t.Error("non-lora peft should map to full strategy")
	}
}

// TestDefaultString verifies fallback selection.
func TestDefaultString(t *testing.T) {
	if defaultString("", "fb") != "fb" {
		t.Error("empty value should use fallback")
	}
	if defaultString("v", "fb") != "v" {
		t.Error("non-empty value should be kept")
	}
}

// TestLossAccessors verifies nil-safe loss summary accessors.
func TestLossAccessors(t *testing.T) {
	if lossValue(nil) != nil || lossMetricName(nil) != "" || lossDataSource(nil) != "" {
		t.Error("nil loss should yield zero values")
	}
	loss := &lossSummary{Value: 1.5, MetricName: "loss", DataSource: "lens"}
	if v := lossValue(loss); v == nil || *v != 1.5 {
		t.Error("lossValue should return pointer to value")
	}
	if lossMetricName(loss) != "loss" {
		t.Error("lossMetricName mismatch")
	}
	if lossDataSource(loss) != "lens" {
		t.Error("lossDataSource mismatch")
	}
}

// --- merged from posttrain_handler_test.go ---

// fakePosttrainStore embeds the generated mock (to satisfy dbclient.Interface)
// and adds the posttrainStore methods so the handler type assertion succeeds.
type fakePosttrainStore struct {
	*mock_client.MockInterface
	views     []*dbclient.PosttrainRunView
	total     int
	getView   *dbclient.PosttrainRunView
	getErr    error
	deleteErr error
	upsertErr error
}

func (f *fakePosttrainStore) UpsertPosttrainRun(ctx context.Context, run *dbclient.PosttrainRun) error {
	return f.upsertErr
}

func (f *fakePosttrainStore) ListPosttrainRunViews(ctx context.Context, filter *dbclient.PosttrainRunFilter) ([]*dbclient.PosttrainRunView, int, error) {
	return f.views, f.total, nil
}

func (f *fakePosttrainStore) GetPosttrainRunView(ctx context.Context, runID string) (*dbclient.PosttrainRunView, error) {
	return f.getView, f.getErr
}

func (f *fakePosttrainStore) SetPosttrainRunDeleted(ctx context.Context, runID string) error {
	return f.deleteErr
}

func newPosttrainHandler(t *testing.T, store *fakePosttrainStore) *Handler {
	t.Helper()
	store.MockInterface = mock_client.NewMockInterface(gomock.NewController(t))
	return &Handler{
		dbClient:  store,
		k8sClient: ctrlfake.NewClientBuilder().WithScheme(modelScheme(t)).Build(),
	}
}

// TestListPosttrainRunsSuccess verifies runs are listed via the store.
func TestListPosttrainRunsSuccess(t *testing.T) {
	store := &fakePosttrainStore{
		views: []*dbclient.PosttrainRunView{
			{RunID: "r1", WorkloadID: "w1", DisplayName: "run1", TrainType: "sft", Strategy: "full"},
		},
		total: 1,
	}
	h := newPosttrainHandler(t, store)

	res, err := h.listPosttrainRuns(sessCtx(t, http.MethodGet, "", "", nil))
	require.NoError(t, err)
	resp, ok := res.(*ListPosttrainRunResponse)
	require.True(t, ok)
	assert.Equal(t, 1, resp.Total)
	testifyassert.Len(t, resp.Items, 1)
}

// TestListPosttrainRunsNoStore verifies an error when the db client is not a posttrain store.
func TestListPosttrainRunsNoStore(t *testing.T) {
	h := &Handler{
		dbClient:  mock_client.NewMockInterface(gomock.NewController(t)),
		k8sClient: ctrlfake.NewClientBuilder().WithScheme(modelScheme(t)).Build(),
	}
	_, err := h.listPosttrainRuns(sessCtx(t, http.MethodGet, "", "", nil))
	testifyassert.Error(t, err)
}

// TestGetPosttrainRunSuccess verifies a single run is fetched and rendered.
func TestGetPosttrainRunSuccess(t *testing.T) {
	store := &fakePosttrainStore{
		getView: &dbclient.PosttrainRunView{RunID: "r1", WorkloadID: "", DisplayName: "run1", TrainType: "sft", Strategy: "full"},
	}
	h := newPosttrainHandler(t, store)

	res, err := h.getPosttrainRun(sessCtx(t, http.MethodGet, "", "", gin.Params{{Key: "id", Value: "r1"}}))
	require.NoError(t, err)
	resp, ok := res.(*PosttrainRunDetailResponse)
	require.True(t, ok)
	assert.Equal(t, "r1", resp.RunID)
}

// TestGetPosttrainRunMissingID verifies an empty id is rejected.
func TestGetPosttrainRunMissingID(t *testing.T) {
	store := &fakePosttrainStore{}
	h := newPosttrainHandler(t, store)
	_, err := h.getPosttrainRun(sessCtx(t, http.MethodGet, "", "", nil))
	testifyassert.Error(t, err)
}

// TestGetPosttrainRunNotFound verifies a store error is propagated.
func TestGetPosttrainRunNotFound(t *testing.T) {
	store := &fakePosttrainStore{getErr: errors.New("not found")}
	h := newPosttrainHandler(t, store)
	_, err := h.getPosttrainRun(sessCtx(t, http.MethodGet, "", "", gin.Params{{Key: "id", Value: "missing"}}))
	testifyassert.Error(t, err)
}

// TestDeletePosttrainRunSuccess verifies a run is marked deleted.
func TestDeletePosttrainRunSuccess(t *testing.T) {
	store := &fakePosttrainStore{}
	h := newPosttrainHandler(t, store)

	res, err := h.deletePosttrainRun(sessCtx(t, http.MethodDelete, "", "", gin.Params{{Key: "id", Value: "r1"}}))
	require.NoError(t, err)
	testifyassert.NotNil(t, res)
}

// TestDeletePosttrainRunMissingID verifies an empty id is rejected.
func TestDeletePosttrainRunMissingID(t *testing.T) {
	store := &fakePosttrainStore{}
	h := newPosttrainHandler(t, store)
	_, err := h.deletePosttrainRun(sessCtx(t, http.MethodDelete, "", "", nil))
	testifyassert.Error(t, err)
}

// TestDeletePosttrainRunError verifies a store delete error is propagated.
func TestDeletePosttrainRunError(t *testing.T) {
	store := &fakePosttrainStore{deleteErr: errors.New("db error")}
	h := newPosttrainHandler(t, store)
	_, err := h.deletePosttrainRun(sessCtx(t, http.MethodDelete, "", "", gin.Params{{Key: "id", Value: "r1"}}))
	testifyassert.Error(t, err)
}

// TestPosttrainPublicWrappers exercises the thin gin wrapper methods end-to-end.
func TestPosttrainPublicWrappers(t *testing.T) {
	store := &fakePosttrainStore{
		views:   []*dbclient.PosttrainRunView{{RunID: "r1", DisplayName: "run1", TrainType: "sft", Strategy: "full"}},
		total:   1,
		getView: &dbclient.PosttrainRunView{RunID: "r1", DisplayName: "run1", TrainType: "sft", Strategy: "full"},
	}
	h := newPosttrainHandler(t, store)

	h.ListPosttrainRuns(sessCtx(t, http.MethodGet, "", "", nil))
	h.GetPosttrainRun(sessCtx(t, http.MethodGet, "", "", gin.Params{{Key: "id", Value: "r1"}}))
	h.DeletePosttrainRun(sessCtx(t, http.MethodDelete, "", "", gin.Params{{Key: "id", Value: "r1"}}))
	h.GetPosttrainRunMetrics(sessCtx(t, http.MethodGet, "", "", gin.Params{{Key: "id", Value: "r1"}}))
}
