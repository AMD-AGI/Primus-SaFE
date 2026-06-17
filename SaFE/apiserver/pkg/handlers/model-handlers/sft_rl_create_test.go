/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package model_handlers

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
)

// TestCreateSftJobBadBody verifies invalid JSON is rejected.
func TestCreateSftJobBadBody(t *testing.T) {
	h := modelHandlerWith(t, nil)
	_, err := h.createSftJob(sessCtx(t, http.MethodPost, "{bad", "", nil))
	assert.Error(t, err)
}

// TestCreateSftJobModelNotFound verifies a missing model yields an error.
func TestCreateSftJobModelNotFound(t *testing.T) {
	h := modelHandlerWith(t, nil)
	_, err := h.createSftJob(sessCtx(t, http.MethodPost, `{"modelId":"missing"}`, "", nil))
	assert.Error(t, err)
}

// TestCreateSftJobWrongAccessMode verifies a remote_api model is rejected for SFT.
func TestCreateSftJobWrongAccessMode(t *testing.T) {
	m := &v1.Model{ObjectMeta: metav1.ObjectMeta{Name: "m1"}}
	m.Spec.Source.AccessMode = v1.AccessModeRemoteAPI
	m.Status.Phase = v1.ModelPhaseReady
	h := modelHandlerWith(t, m)
	_, err := h.createSftJob(sessCtx(t, http.MethodPost, `{"modelId":"m1"}`, "", nil))
	assert.Error(t, err)
}

// TestCreateSftJobModelNotReady verifies a non-ready model is rejected.
func TestCreateSftJobModelNotReady(t *testing.T) {
	m := &v1.Model{ObjectMeta: metav1.ObjectMeta{Name: "m1"}}
	m.Spec.Source.AccessMode = v1.AccessModeLocal
	m.Status.Phase = v1.ModelPhasePending
	h := modelHandlerWith(t, m)
	_, err := h.createSftJob(sessCtx(t, http.MethodPost, `{"modelId":"m1"}`, "", nil))
	assert.Error(t, err)
}

// TestCreateRlJobBadBody verifies invalid JSON is rejected.
func TestCreateRlJobBadBody(t *testing.T) {
	h := modelHandlerWith(t, nil)
	_, err := h.createRlJob(sessCtx(t, http.MethodPost, "{bad", "", nil))
	assert.Error(t, err)
}

// TestCreateRlJobModelNotFound verifies a missing model yields an error.
func TestCreateRlJobModelNotFound(t *testing.T) {
	h := modelHandlerWith(t, nil)
	_, err := h.createRlJob(sessCtx(t, http.MethodPost, `{"modelId":"missing"}`, "", nil))
	assert.Error(t, err)
}
