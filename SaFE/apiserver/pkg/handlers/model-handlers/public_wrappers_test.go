/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package model_handlers

import (
	"net/http"
	"testing"
)

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
