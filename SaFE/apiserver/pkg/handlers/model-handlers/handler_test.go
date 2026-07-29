/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package model_handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	testifyassert "github.com/stretchr/testify/assert"
	ctrlfake "sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func TestSetAndGetRobustClient(t *testing.T) {
	// Default/reset state is nil.
	SetRobustClient(nil)
	assert.Nil(t, GetRobustClient())
}

func TestNewHandlerConstructors(t *testing.T) {
	h := NewHandler(nil, nil, nil)
	assert.NotNil(t, h)
	assert.False(t, h.IsDatasetEnabled())

	h2 := NewHandlerWithS3(nil, nil, nil, nil)
	assert.NotNil(t, h2)
	// s3 client is nil -> dataset disabled.
	assert.False(t, h2.IsDatasetEnabled())
}

func TestHandleWrapper(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// Success path.
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	handle(c, func(*gin.Context) (interface{}, error) { return gin.H{"ok": true}, nil })
	assert.Equal(t, http.StatusOK, w.Code)

	// Error path -> 500 via getHTTPStatusCode.
	w2 := httptest.NewRecorder()
	c2, _ := gin.CreateTestContext(w2)
	c2.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	handle(c2, func(*gin.Context) (interface{}, error) { return nil, errors.New("boom") })
	assert.Equal(t, http.StatusInternalServerError, w2.Code)
}

func TestHandleDatasetWrapper(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// Success with struct response.
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	handleDataset(c, func(*gin.Context) (interface{}, error) { return gin.H{"ok": true}, nil })
	assert.Equal(t, http.StatusOK, w.Code)

	// Success with []byte response.
	w2 := httptest.NewRecorder()
	c2, _ := gin.CreateTestContext(w2)
	c2.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	handleDataset(c2, func(*gin.Context) (interface{}, error) { return []byte(`{"a":1}`), nil })
	assert.Equal(t, http.StatusOK, w2.Code)

	// Error path.
	w3 := httptest.NewRecorder()
	c3, _ := gin.CreateTestContext(w3)
	c3.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	handleDataset(c3, func(*gin.Context) (interface{}, error) { return nil, errors.New("boom") })
	assert.NotEqual(t, http.StatusOK, w3.Code)
}

// --- merged from config_read_rbac_test.go ---

// newConfigReadRBACHandler seeds a private remote_api model in ws-2 (owned by
// stranger-1) so the SFT/RL config read paths can be exercised against read
// visibility. The DB client is nil, so lookups take the K8s path.
func newConfigReadRBACHandler(t *testing.T) *Handler {
	t.Helper()
	k8s := ctrlfake.NewClientBuilder().WithScheme(modelScheme(t)).WithObjects(
		newReadModel("m-ws2", "stranger-1", "ws-2"),
	).Build()
	return &Handler{k8sClient: k8s, accessController: newReadRBACAC(t)}
}

// TestGetSftConfigReadRBAC verifies getSftConfig enforces model read visibility
// before returning the model's SFT configuration.
func TestGetSftConfigReadRBAC(t *testing.T) {
	h := newConfigReadRBACHandler(t)
	id := gin.Params{{Key: "id", Value: "m-ws2"}}

	// member-1 (member of ws-1, not ws-2, not owner) is denied.
	_, err := h.getSftConfig(readRBACCtx("member-1", "workspace=ws-2", id))
	testifyassert.Error(t, err)
	testifyassert.Contains(t, err.Error(), "not allowed")

	// The owner and a system admin pass the visibility gate.
	_, err = h.getSftConfig(readRBACCtx("stranger-1", "workspace=ws-2", id))
	testifyassert.NoError(t, err)
	_, err = h.getSftConfig(readRBACCtx("admin-1", "workspace=ws-2", id))
	testifyassert.NoError(t, err)
}

// TestGetRlConfigReadRBAC verifies getRlConfig enforces model read visibility
// before returning the model's RL configuration.
func TestGetRlConfigReadRBAC(t *testing.T) {
	h := newConfigReadRBACHandler(t)
	id := gin.Params{{Key: "id", Value: "m-ws2"}}

	// member-1 is denied.
	_, err := h.getRlConfig(readRBACCtx("member-1", "workspace=ws-2", id))
	testifyassert.Error(t, err)
	testifyassert.Contains(t, err.Error(), "not allowed")

	// The owner and a system admin pass the visibility gate.
	_, err = h.getRlConfig(readRBACCtx("stranger-1", "workspace=ws-2", id))
	testifyassert.NoError(t, err)
	_, err = h.getRlConfig(readRBACCtx("admin-1", "workspace=ws-2", id))
	testifyassert.NoError(t, err)
}

// --- merged from createmodel_test.go ---

func TestCreateModelBadBody(t *testing.T) {
	h := &Handler{}
	_, err := h.createModel(sessCtx(t, http.MethodPost, "{invalid", "u1", nil))
	testifyassert.Error(t, err)
}

func TestCreateModelURLRequired(t *testing.T) {
	h := &Handler{}
	// local mode without url.
	_, err := h.createModel(sessCtx(t, http.MethodPost, `{"source":{"accessMode":"local"}}`, "u1", nil))
	testifyassert.Error(t, err)
}

func TestCreateModelInvalidAccessMode(t *testing.T) {
	h := &Handler{}
	_, err := h.createModel(sessCtx(t, http.MethodPost, `{"source":{"accessMode":"bad","url":"http://x"}}`, "u1", nil))
	testifyassert.Error(t, err)
}

func TestCreateModelRemoteMissingModelName(t *testing.T) {
	h := &Handler{}
	_, err := h.createModel(sessCtx(t, http.MethodPost,
		`{"source":{"accessMode":"remote_api","url":"http://x"}}`, "u1", nil))
	testifyassert.Error(t, err)
}

func TestCreateModelRemoteMissingDisplayName(t *testing.T) {
	h := &Handler{}
	_, err := h.createModel(sessCtx(t, http.MethodPost,
		`{"source":{"accessMode":"remote_api","url":"http://x","modelName":"gpt"}}`, "u1", nil))
	testifyassert.Error(t, err)
}

// --- merged from public_wrappers_test.go ---

// TestModelPublicWrappers exercises the thin gin wrapper methods via early error paths.
// These call handle(c, h.inner) and cover the wrapper lines; inner handlers fail fast
// (bad body / missing id / missing model) without needing live backends.
func TestModelPublicWrappers(t *testing.T) {
	h := modelHandlerWith(t, nil)

	// Bad JSON body -> createModel returns early.
	h.CreateModel(sessCtx(t, http.MethodPost, "{bad", "u1", nil))
	// Missing id param -> getModel/patchModel return bad request.
	h.GetModel(sessCtx(t, http.MethodGet, "", "u1", nil))
	h.PatchModel(sessCtx(t, http.MethodPatch, `{"displayName":"x"}`, "u1", nil))
	// Missing model in fake k8s -> retry/workloads return errors.
	h.RetryModel(sessCtx(t, http.MethodPost, "", "u1", nil))
	h.GetModelWorkloads(sessCtx(t, http.MethodGet, "", "u1", nil))
}

// TestSftRlPublicWrappers exercises the SFT/RL gin wrappers via early error paths.
func TestSftRlPublicWrappers(t *testing.T) {
	h := modelHandlerWith(t, nil)

	h.CreateSftJob(sessCtx(t, http.MethodPost, "{bad", "u1", nil))
	h.CreateRlJob(sessCtx(t, http.MethodPost, "{bad", "u1", nil))
	h.GetSftConfig(sessCtx(t, http.MethodGet, "", "u1", nil))
}

// TestDatasetPublicWrappers exercises dataset gin wrappers via early error paths.
func TestDatasetPublicWrappers(t *testing.T) {
	h := modelHandlerWith(t, nil)

	// Static type listing needs no backend.
	h.ListDatasetTypes(sessCtx(t, http.MethodGet, "", "u1", nil))
	// Bad form body -> createDataset returns early.
	h.CreateDataset(sessCtx(t, http.MethodPost, "{bad", "u1", nil))
}

// TestPlaygroundSessionPublicWrappers exercises playground session gin wrappers.
// With a nil dbClient the inner handlers fail fast ("requires database").
func TestPlaygroundSessionPublicWrappers(t *testing.T) {
	h := &Handler{}

	h.SaveSession(sessCtx(t, http.MethodPost, `{"modelName":"m"}`, "u1", nil))
	h.ListPlaygroundSession(sessCtx(t, http.MethodGet, "", "u1", nil))
	h.GetPlaygroundSession(sessCtx(t, http.MethodGet, "", "u1", nil))
	h.DeletePlaygroundSession(sessCtx(t, http.MethodDelete, "", "u1", nil))
}

// --- merged from pure_more_test.go ---

func TestCleanDatasetRepoID(t *testing.T) {
	assert.Equal(t, "owner/name", cleanDatasetRepoID("owner/name"))
	assert.Equal(t, "owner/name", cleanDatasetRepoID("https://huggingface.co/datasets/owner/name"))
	assert.Equal(t, "owner/name", cleanDatasetRepoID("http://huggingface.co/datasets/owner/name/"))
	assert.Equal(t, "owner/name", cleanDatasetRepoID("api/datasets/owner/name"))
	assert.Equal(t, "owner/name", cleanDatasetRepoID("  huggingface.co/owner/name  "))
}

func TestNormalizeHFDatasetURL(t *testing.T) {
	// Full URL returned as-is (trailing slash trimmed).
	assert.Equal(t, "https://huggingface.co/datasets/owner/name",
		normalizeHFDatasetURL("https://huggingface.co/datasets/owner/name"))
	// Repo ID constructed into full URL.
	assert.Equal(t, "https://huggingface.co/datasets/owner/name",
		normalizeHFDatasetURL("owner/name"))
}

func TestParseStringOrArray(t *testing.T) {
	testifyassert.Nil(t, parseStringOrArray(nil))
	testifyassert.Nil(t, parseStringOrArray(json.RawMessage(`""`)))
	assert.Equal(t, []string{"mit"}, parseStringOrArray(json.RawMessage(`"mit"`)))
	assert.Equal(t, []string{"a", "b"}, parseStringOrArray(json.RawMessage(`["a","b"]`)))
	testifyassert.Nil(t, parseStringOrArray(json.RawMessage(`{bad`)))
}

func TestGetHTTPStatusCode(t *testing.T) {
	assert.Equal(t, http.StatusOK, getHTTPStatusCode(nil))
	assert.Equal(t, http.StatusInternalServerError, getHTTPStatusCode(errors.New("x")))
}

func TestSupportedModelNames(t *testing.T) {
	// Should return a comma-joined, non-empty list of recipe names.
	names := supportedModelNames()
	testifyassert.NotEmpty(t, names)
}

func TestCategorizeTagString(t *testing.T) {
	testifyassert.Empty(t, CategorizeTagString("", true))
	got := CategorizeTagString("llama-3, text-generation", true)
	testifyassert.NotEmpty(t, got)
}

func TestCategorizeTags(t *testing.T) {
	got := CategorizeTags([]string{"llama-3", "text-generation"}, true)
	testifyassert.NotEmpty(t, got)
	// Unmatched excluded in local mode may yield fewer entries but should not panic.
	_ = CategorizeTags([]string{"some-unknown-tag-xyz"}, false)
}
