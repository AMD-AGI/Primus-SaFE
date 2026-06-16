/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package model_handlers

import (
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrlfake "sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
)

func failedModelClient(t *testing.T, phase v1.ModelPhase) *Handler {
	t.Helper()
	model := &v1.Model{ObjectMeta: metav1.ObjectMeta{Name: "m1"}}
	model.Status.Phase = phase
	cl := ctrlfake.NewClientBuilder().
		WithScheme(modelScheme(t)).
		WithObjects(model).
		WithStatusSubresource(&v1.Model{}).
		Build()
	return &Handler{k8sClient: cl}
}

func TestRetryModelHandler(t *testing.T) {
	h := failedModelClient(t, v1.ModelPhaseFailed)
	res, err := h.retryModel(sessCtx(t, http.MethodPost, "", "", gin.Params{{Key: "id", Value: "m1"}}))
	assert.NoError(t, err)
	assert.NotNil(t, res)
}

func TestRetryModelHandlerNotFailed(t *testing.T) {
	h := failedModelClient(t, v1.ModelPhaseReady)
	_, err := h.retryModel(sessCtx(t, http.MethodPost, "", "", gin.Params{{Key: "id", Value: "m1"}}))
	assert.Error(t, err)
}

func TestRetryModelHandlerNotFound(t *testing.T) {
	cl := ctrlfake.NewClientBuilder().WithScheme(modelScheme(t)).WithStatusSubresource(&v1.Model{}).Build()
	h := &Handler{k8sClient: cl}
	_, err := h.retryModel(sessCtx(t, http.MethodPost, "", "", gin.Params{{Key: "id", Value: "missing"}}))
	assert.Error(t, err)
}

func TestPatchModelHandler(t *testing.T) {
	model := &v1.Model{ObjectMeta: metav1.ObjectMeta{Name: "m1"}}
	cl := ctrlfake.NewClientBuilder().WithScheme(modelScheme(t)).WithObjects(model).Build()
	h := &Handler{k8sClient: cl}
	res, err := h.patchModel(sessCtx(t, http.MethodPatch, `{"displayName":"new"}`, "", gin.Params{{Key: "id", Value: "m1"}}))
	assert.NoError(t, err)
	assert.NotNil(t, res)
}

func TestPatchModelHandlerNoFields(t *testing.T) {
	model := &v1.Model{ObjectMeta: metav1.ObjectMeta{Name: "m1"}}
	cl := ctrlfake.NewClientBuilder().WithScheme(modelScheme(t)).WithObjects(model).Build()
	h := &Handler{k8sClient: cl}
	_, err := h.patchModel(sessCtx(t, http.MethodPatch, `{}`, "", gin.Params{{Key: "id", Value: "m1"}}))
	assert.Error(t, err)
}

func TestPatchModelHandlerBadID(t *testing.T) {
	h := &Handler{}
	_, err := h.patchModel(sessCtx(t, http.MethodPatch, `{"displayName":"x"}`, "", nil))
	assert.Error(t, err)
}

func TestGetModelWorkloadsHandler(t *testing.T) {
	model := &v1.Model{ObjectMeta: metav1.ObjectMeta{Name: "m1"}}
	cl := ctrlfake.NewClientBuilder().WithScheme(modelScheme(t)).WithObjects(model).Build()
	h := &Handler{k8sClient: cl}
	res, err := h.getModelWorkloads(sessCtx(t, http.MethodGet, "", "", gin.Params{{Key: "id", Value: "m1"}}))
	assert.NoError(t, err)
	assert.NotNil(t, res)
}

func TestGetModelWorkloadsHandlerNotFound(t *testing.T) {
	cl := ctrlfake.NewClientBuilder().WithScheme(modelScheme(t)).Build()
	h := &Handler{k8sClient: cl}
	_, err := h.getModelWorkloads(sessCtx(t, http.MethodGet, "", "", gin.Params{{Key: "id", Value: "missing"}}))
	assert.Error(t, err)
}
