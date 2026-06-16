/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package model_handlers

import (
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrlfake "sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
)

// modelHandlerWith builds a Handler with a fake client holding the given model.
func modelHandlerWith(t *testing.T, m *v1.Model) *Handler {
	t.Helper()
	b := ctrlfake.NewClientBuilder().WithScheme(modelScheme(t))
	if m != nil {
		b = b.WithObjects(m)
	}
	return &Handler{k8sClient: b.Build()}
}

func localReadyModel() *v1.Model {
	m := &v1.Model{ObjectMeta: metav1.ObjectMeta{Name: "m1"}}
	m.Spec.DisplayName = "My Model"
	m.Spec.Source.AccessMode = v1.AccessModeLocal
	m.Status.Phase = v1.ModelPhaseReady
	return m
}

func TestGetSftConfigLocalReady(t *testing.T) {
	h := modelHandlerWith(t, localReadyModel())
	res, err := h.getSftConfig(modelGinCtx(t, gin.Params{{Key: "id", Value: "m1"}}, "workspace=ws"))
	assert.NoError(t, err)
	assert.NotNil(t, res)
}

func TestGetSftConfigRemoteUnsupported(t *testing.T) {
	m := &v1.Model{ObjectMeta: metav1.ObjectMeta{Name: "m1"}}
	m.Spec.Source.AccessMode = v1.AccessModeRemoteAPI
	m.Status.Phase = v1.ModelPhaseReady
	h := modelHandlerWith(t, m)
	res, err := h.getSftConfig(modelGinCtx(t, gin.Params{{Key: "id", Value: "m1"}}, "workspace=ws"))
	assert.NoError(t, err)
	assert.NotNil(t, res)
}

func TestGetSftConfigEmptyID(t *testing.T) {
	h := modelHandlerWith(t, nil)
	_, err := h.getSftConfig(modelGinCtx(t, nil, ""))
	assert.Error(t, err)
}

func TestGetSftConfigNotFound(t *testing.T) {
	h := modelHandlerWith(t, nil)
	_, err := h.getSftConfig(modelGinCtx(t, gin.Params{{Key: "id", Value: "missing"}}, "workspace=ws"))
	assert.Error(t, err)
}

func TestGetRlConfigLocalReady(t *testing.T) {
	h := modelHandlerWith(t, localReadyModel())
	res, err := h.getRlConfig(modelGinCtx(t, gin.Params{{Key: "id", Value: "m1"}}, "workspace=ws"))
	assert.NoError(t, err)
	assert.NotNil(t, res)
}

func TestGetRlConfigRemoteUnsupported(t *testing.T) {
	m := &v1.Model{ObjectMeta: metav1.ObjectMeta{Name: "m1"}}
	m.Spec.Source.AccessMode = v1.AccessModeRemoteAPI
	m.Status.Phase = v1.ModelPhaseReady
	h := modelHandlerWith(t, m)
	res, err := h.getRlConfig(modelGinCtx(t, gin.Params{{Key: "id", Value: "m1"}}, "workspace=ws"))
	assert.NoError(t, err)
	assert.NotNil(t, res)
}

func TestGetRlConfigEmptyID(t *testing.T) {
	h := modelHandlerWith(t, nil)
	_, err := h.getRlConfig(modelGinCtx(t, nil, ""))
	assert.Error(t, err)
}

func TestGetRlConfigNotFound(t *testing.T) {
	h := modelHandlerWith(t, nil)
	_, err := h.getRlConfig(modelGinCtx(t, gin.Params{{Key: "id", Value: "missing"}}, "workspace=ws"))
	assert.Error(t, err)
}
