/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package model_handlers

import (
	"net/http"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	testifyassert "github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrlfake "sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	dbclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
)

// TestExtractHfModelNameFromURLOrModelName verifies HF prefix/suffix trimming and fallback.
func TestExtractHfModelNameFromURLOrModelName(t *testing.T) {
	if got := extractHfModelNameFromURLOrModelName("https://huggingface.co/Qwen/Qwen3-8B/", "fallback"); got != "Qwen/Qwen3-8B" {
		t.Errorf("unexpected extracted name: %s", got)
	}
	if got := extractHfModelNameFromURLOrModelName("", "fallback"); got != "fallback" {
		t.Errorf("expected fallback model name, got %s", got)
	}
}

// TestExtractHfModelName verifies extraction from a db model record.
func TestExtractHfModelName(t *testing.T) {
	m := &dbclient.Model{SourceURL: "https://huggingface.co/meta/Llama", ModelName: "fallback"}
	if got := extractHfModelName(m); got != "meta/Llama" {
		t.Errorf("unexpected extracted name: %s", got)
	}
}

// TestResolveTrainingBaseModelNameFromK8sModel verifies base model resolution precedence.
func TestResolveTrainingBaseModelNameFromK8sModel(t *testing.T) {
	localPath := &v1.Model{}
	localPath.Spec.Source.AccessMode = v1.AccessModeLocalPath
	localPath.Spec.BaseModel = "base-x"
	if got := resolveTrainingBaseModelNameFromK8sModel(localPath); got != "base-x" {
		t.Errorf("local-path model should use BaseModel, got %s", got)
	}

	urlModel := &v1.Model{}
	urlModel.Spec.Source.URL = "https://huggingface.co/Qwen/Qwen3-8B"
	if got := resolveTrainingBaseModelNameFromK8sModel(urlModel); got != "Qwen/Qwen3-8B" {
		t.Errorf("url model should resolve from source URL, got %s", got)
	}

	dispModel := &v1.Model{}
	dispModel.Spec.DisplayName = "display-name"
	if got := resolveTrainingBaseModelNameFromK8sModel(dispModel); got != "display-name" {
		t.Errorf("model without source should fall back to DisplayName, got %s", got)
	}
}

// TestGenerateRlWorkloadName verifies name normalization and prefix.
func TestGenerateRlWorkloadName(t *testing.T) {
	name := generateRlWorkloadName("My Model Name")
	if !strings.HasPrefix(name, "rl-my-model-name-") {
		t.Errorf("unexpected workload name: %s", name)
	}

	long := generateRlWorkloadName(strings.Repeat("a", 100))
	if len(long) > 3+40+1+5 {
		t.Errorf("workload name should be bounded, got len=%d", len(long))
	}
}

// --- merged from sft_rl_config_test.go ---

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
	testifyassert.NoError(t, err)
	testifyassert.NotNil(t, res)
}

func TestGetSftConfigRemoteUnsupported(t *testing.T) {
	m := &v1.Model{ObjectMeta: metav1.ObjectMeta{Name: "m1"}}
	m.Spec.Source.AccessMode = v1.AccessModeRemoteAPI
	m.Status.Phase = v1.ModelPhaseReady
	h := modelHandlerWith(t, m)
	res, err := h.getSftConfig(modelGinCtx(t, gin.Params{{Key: "id", Value: "m1"}}, "workspace=ws"))
	testifyassert.NoError(t, err)
	testifyassert.NotNil(t, res)
}

func TestGetSftConfigEmptyID(t *testing.T) {
	h := modelHandlerWith(t, nil)
	_, err := h.getSftConfig(modelGinCtx(t, nil, ""))
	testifyassert.Error(t, err)
}

func TestGetSftConfigNotFound(t *testing.T) {
	h := modelHandlerWith(t, nil)
	_, err := h.getSftConfig(modelGinCtx(t, gin.Params{{Key: "id", Value: "missing"}}, "workspace=ws"))
	testifyassert.Error(t, err)
}

func TestGetRlConfigLocalReady(t *testing.T) {
	h := modelHandlerWith(t, localReadyModel())
	res, err := h.getRlConfig(modelGinCtx(t, gin.Params{{Key: "id", Value: "m1"}}, "workspace=ws"))
	testifyassert.NoError(t, err)
	testifyassert.NotNil(t, res)
}

func TestGetRlConfigRemoteUnsupported(t *testing.T) {
	m := &v1.Model{ObjectMeta: metav1.ObjectMeta{Name: "m1"}}
	m.Spec.Source.AccessMode = v1.AccessModeRemoteAPI
	m.Status.Phase = v1.ModelPhaseReady
	h := modelHandlerWith(t, m)
	res, err := h.getRlConfig(modelGinCtx(t, gin.Params{{Key: "id", Value: "m1"}}, "workspace=ws"))
	testifyassert.NoError(t, err)
	testifyassert.NotNil(t, res)
}

func TestGetRlConfigEmptyID(t *testing.T) {
	h := modelHandlerWith(t, nil)
	_, err := h.getRlConfig(modelGinCtx(t, nil, ""))
	testifyassert.Error(t, err)
}

func TestGetRlConfigNotFound(t *testing.T) {
	h := modelHandlerWith(t, nil)
	_, err := h.getRlConfig(modelGinCtx(t, gin.Params{{Key: "id", Value: "missing"}}, "workspace=ws"))
	testifyassert.Error(t, err)
}

// --- merged from sft_rl_create_test.go ---

// TestCreateSftJobBadBody verifies invalid JSON is rejected.
func TestCreateSftJobBadBody(t *testing.T) {
	h := modelHandlerWith(t, nil)
	_, err := h.createSftJob(sessCtx(t, http.MethodPost, "{bad", "", nil))
	testifyassert.Error(t, err)
}

// TestCreateSftJobModelNotFound verifies a missing model yields an error.
func TestCreateSftJobModelNotFound(t *testing.T) {
	h := modelHandlerWith(t, nil)
	_, err := h.createSftJob(sessCtx(t, http.MethodPost, `{"modelId":"missing"}`, "", nil))
	testifyassert.Error(t, err)
}

// TestCreateSftJobWrongAccessMode verifies a remote_api model is rejected for SFT.
func TestCreateSftJobWrongAccessMode(t *testing.T) {
	m := &v1.Model{ObjectMeta: metav1.ObjectMeta{Name: "m1"}}
	m.Spec.Source.AccessMode = v1.AccessModeRemoteAPI
	m.Status.Phase = v1.ModelPhaseReady
	h := modelHandlerWith(t, m)
	_, err := h.createSftJob(sessCtx(t, http.MethodPost, `{"modelId":"m1"}`, "", nil))
	testifyassert.Error(t, err)
}

// TestCreateSftJobModelNotReady verifies a non-ready model is rejected.
func TestCreateSftJobModelNotReady(t *testing.T) {
	m := &v1.Model{ObjectMeta: metav1.ObjectMeta{Name: "m1"}}
	m.Spec.Source.AccessMode = v1.AccessModeLocal
	m.Status.Phase = v1.ModelPhasePending
	h := modelHandlerWith(t, m)
	_, err := h.createSftJob(sessCtx(t, http.MethodPost, `{"modelId":"m1"}`, "", nil))
	testifyassert.Error(t, err)
}

// TestCreateRlJobBadBody verifies invalid JSON is rejected.
func TestCreateRlJobBadBody(t *testing.T) {
	h := modelHandlerWith(t, nil)
	_, err := h.createRlJob(sessCtx(t, http.MethodPost, "{bad", "", nil))
	testifyassert.Error(t, err)
}

// TestCreateRlJobModelNotFound verifies a missing model yields an error.
func TestCreateRlJobModelNotFound(t *testing.T) {
	h := modelHandlerWith(t, nil)
	_, err := h.createRlJob(sessCtx(t, http.MethodPost, `{"modelId":"missing"}`, "", nil))
	testifyassert.Error(t, err)
}
