/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package model_handlers

import (
	"strings"
	"testing"

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
