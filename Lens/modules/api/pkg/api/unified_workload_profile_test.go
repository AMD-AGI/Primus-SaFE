// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package api

import (
	"testing"
)

// TestWorkloadProfileEndpoints validates the full intent analysis flow
// through the Unified API layer. This covers the E2E path:
//   workload_detection (DB) -> API -> workload profile response

func TestConvertDetectionToProfile_FullFields(t *testing.T) {
	// Simulate a fully populated workload_detection record
	framework := "deepspeed"
	frameworks := "deepspeed,pytorch"
	workloadType := "training"
	wrapperFW := "huggingface_trainer"
	baseFW := "deepspeed"
	confidence := 0.95
	category := "fine_tuning"
	expectedBehavior := "long_running"
	modelPath := "/models/meta-llama/Llama-3-8B"
	modelFamily := "llama"
	modelScale := "8B"
	modelVariant := "base"
	runtimeFW := "pytorch"
	intentConfidence := 0.88
	intentSource := "deterministic"
	intentReasoning := "L1 cmdline extraction: --use_lora flag detected"
	intentAnalysisMode := "cmdline_rich"
	intentState := "confirmed"

	det := &dbModel.WorkloadDetection{
		WorkloadUID:        "test-uid-001",
		Framework:          &framework,
		Frameworks:         &frameworks,
		WorkloadType:       &workloadType,
		WrapperFramework:   &wrapperFW,
		BaseFramework:      &baseFW,
		Confidence:         &confidence,
		Category:           &category,
		ExpectedBehavior:   &expectedBehavior,
		ModelPath:          &modelPath,
		ModelFamily:        &modelFamily,
		ModelScale:         &modelScale,
		ModelVariant:       &modelVariant,
		RuntimeFramework:   &runtimeFW,
		IntentConfidence:   &intentConfidence,
		IntentSource:       &intentSource,
		IntentReasoning:    &intentReasoning,
		IntentAnalysisMode: &intentAnalysisMode,
		IntentState:        &intentState,
	}

	resp := convertDetectionToProfile(det)

	// Validate Phase 1 detection fields
	if resp.WorkloadUID != "test-uid-001" {
		t.Errorf("workload_uid: got %q, want %q", resp.WorkloadUID, "test-uid-001")
	}
	if resp.Framework != "deepspeed" {
		t.Errorf("framework: got %q, want %q", resp.Framework, "deepspeed")
	}
	if resp.WorkloadType != "training" {
		t.Errorf("workload_type: got %q, want %q", resp.WorkloadType, "training")
	}
	if resp.WrapperFramework != "huggingface_trainer" {
		t.Errorf("wrapper_framework: got %q", resp.WrapperFramework)
	}
	if resp.Confidence != 0.95 {
		t.Errorf("confidence: got %v, want 0.95", resp.Confidence)
	}

	// Validate Phase 2 intent fields
	if resp.Category != "fine_tuning" {
		t.Errorf("category: got %q, want %q", resp.Category, "fine_tuning")
	}
	if resp.ModelFamily != "llama" {
		t.Errorf("model_family: got %q, want %q", resp.ModelFamily, "llama")
	}
	if resp.ModelScale != "8B" {
		t.Errorf("model_scale: got %q, want %q", resp.ModelScale, "8B")
	}
	if resp.IntentConfidence != 0.88 {
		t.Errorf("intent_confidence: got %v, want 0.88", resp.IntentConfidence)
	}
	if resp.IntentSource != "deterministic" {
		t.Errorf("intent_source: got %q", resp.IntentSource)
	}
	if resp.IntentState != "confirmed" {
		t.Errorf("intent_state: got %q, want %q", resp.IntentState, "confirmed")
	}
	if resp.IntentAnalysisMode != "cmdline_rich" {
		t.Errorf("intent_analysis_mode: got %q", resp.IntentAnalysisMode)
	}
}

func TestConvertDetectionToProfile_MinimalFields(t *testing.T) {
	// Simulate a workload with only Phase 1 detection, no intent yet
	framework := "pytorch"
	intentState := "pending"

	det := &dbModel.WorkloadDetection{
		WorkloadUID: "test-uid-002",
		Framework:   &framework,
		IntentState: &intentState,
	}

	resp := convertDetectionToProfile(det)

	if resp.WorkloadUID != "test-uid-002" {
		t.Errorf("workload_uid: got %q", resp.WorkloadUID)
	}
	if resp.Framework != "pytorch" {
		t.Errorf("framework: got %q", resp.Framework)
	}
	if resp.IntentState != "pending" {
		t.Errorf("intent_state: got %q, want %q", resp.IntentState, "pending")
	}
	// Intent fields should be empty/zero
	if resp.Category != "" {
		t.Errorf("category should be empty, got %q", resp.Category)
	}
	if resp.ModelFamily != "" {
		t.Errorf("model_family should be empty, got %q", resp.ModelFamily)
	}
	if resp.IntentConfidence != 0 {
		t.Errorf("intent_confidence should be 0, got %v", resp.IntentConfidence)
	}
}

func TestConvertDetectionToProfile_InferenceWorkload(t *testing.T) {
	// Simulate a vLLM inference workload
	framework := "vllm"
	workloadType := "inference"
	confidence := 0.99
	category := "inference"
	modelPath := "/models/meta-llama/Llama-3-70B-Instruct"
	modelFamily := "llama"
	modelScale := "70B"
	modelVariant := "instruct"
	intentConfidence := 0.95
	intentSource := "deterministic"
	intentAnalysisMode := "cmdline_rich"
	intentState := "confirmed"

	det := &dbModel.WorkloadDetection{
		WorkloadUID:        "test-uid-003",
		Framework:          &framework,
		WorkloadType:       &workloadType,
		Confidence:         &confidence,
		Category:           &category,
		ModelPath:          &modelPath,
		ModelFamily:        &modelFamily,
		ModelScale:         &modelScale,
		ModelVariant:       &modelVariant,
		IntentConfidence:   &intentConfidence,
		IntentSource:       &intentSource,
		IntentAnalysisMode: &intentAnalysisMode,
		IntentState:        &intentState,
	}

	resp := convertDetectionToProfile(det)

	if resp.Category != "inference" {
		t.Errorf("category: got %q, want inference", resp.Category)
	}
	if resp.ModelFamily != "llama" {
		t.Errorf("model_family: got %q", resp.ModelFamily)
	}
	if resp.ModelScale != "70B" {
		t.Errorf("model_scale: got %q", resp.ModelScale)
	}
	if resp.ModelVariant != "instruct" {
		t.Errorf("model_variant: got %q", resp.ModelVariant)
	}
}
