/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package model_handlers

import (
	"strings"
	"testing"
)

// TestInferModelRecipeExactMatch verifies a known HF model maps to its recipe.
func TestInferModelRecipeExactMatch(t *testing.T) {
	r, err := InferModelRecipe("Qwen/Qwen3-8B")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if r.Recipe != "qwen.qwen3" || r.Flavor != "qwen3_8b_finetune_config" || r.Size != "8b" {
		t.Errorf("unexpected recipe: %+v", r)
	}
}

// TestInferModelRecipeFallback verifies an unknown model falls back to a size-based default.
func TestInferModelRecipeFallback(t *testing.T) {
	r, err := InferModelRecipe("totally-unknown-model")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if r.Recipe == "" || r.Flavor == "" || r.Size == "" {
		t.Errorf("fallback recipe should be fully populated: %+v", r)
	}
}

// TestResolveModelRecipeWithOverride verifies a complete override is honored.
func TestResolveModelRecipeWithOverride(t *testing.T) {
	r, err := ResolveModelRecipe("Qwen/Qwen3-8B", ModelRecipeOverride{
		Recipe: "custom.recipe",
		Flavor: "custom_flavor",
		Size:   "70b",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if r.Recipe != "custom.recipe" || r.Flavor != "custom_flavor" || r.Size != "70b" {
		t.Errorf("override not honored: %+v", r)
	}
}

// TestResolveModelRecipeIncompleteOverride verifies a partial override errors out.
func TestResolveModelRecipeIncompleteOverride(t *testing.T) {
	_, err := ResolveModelRecipe("Qwen/Qwen3-8B", ModelRecipeOverride{
		Recipe: "custom.recipe",
	})
	if err == nil {
		t.Error("expected error for incomplete override")
	}
}

// TestResolveModelRecipeNoOverride verifies it falls back to inference when no override given.
func TestResolveModelRecipeNoOverride(t *testing.T) {
	r, err := ResolveModelRecipe("Qwen/Qwen3-8B", ModelRecipeOverride{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if r.Flavor != "qwen3_8b_finetune_config" {
		t.Errorf("expected inferred recipe, got: %+v", r)
	}
}

// TestResolveModelRecipeInvalidSizeOverride verifies an unsupported size override errors.
func TestResolveModelRecipeInvalidSizeOverride(t *testing.T) {
	_, err := ResolveModelRecipe("x", ModelRecipeOverride{
		Recipe: "r", Flavor: "f", Size: "999b",
	})
	if err == nil {
		t.Error("expected error for unsupported size override")
	}
}

// TestGetDefaultSftImage verifies the default image always references the primus tag.
func TestGetDefaultSftImage(t *testing.T) {
	img := GetDefaultSftImage()
	if !strings.Contains(img, "primus:v26.1") {
		t.Errorf("default SFT image should reference primus tag, got: %s", img)
	}
}

// TestFillSftDefaults verifies zero-valued fields are populated with smart defaults.
func TestFillSftDefaults(t *testing.T) {
	req := &CreateSftJobRequest{}
	FillSftDefaults(req, "8b")

	if req.Priority != DefaultPriority {
		t.Errorf("expected default priority %d, got %d", DefaultPriority, req.Priority)
	}
	if req.ExportModel == nil || *req.ExportModel != true {
		t.Error("expected ExportModel to default to true")
	}
	if req.TrainConfig.Peft != "none" {
		t.Errorf("expected default peft none, got %s", req.TrainConfig.Peft)
	}
	if req.TrainConfig.DatasetFormat != "alpaca" {
		t.Errorf("expected default dataset format alpaca, got %s", req.TrainConfig.DatasetFormat)
	}
	if req.TrainConfig.TrainIters == 0 || req.TrainConfig.GlobalBatchSize == 0 {
		t.Error("expected training hyperparameters to be populated from preset")
	}
	if req.NodeCount != 1 || req.GpuCount != DefaultGpuCount {
		t.Errorf("expected default node/gpu counts, got node=%d gpu=%d", req.NodeCount, req.GpuCount)
	}
	if req.Cpu != DefaultCpu || req.Memory != DefaultMemory {
		t.Error("expected default cpu/memory to be populated")
	}
}

// TestFillSftDefaultsLoraPeft verifies LoRA-specific defaults are populated.
func TestFillSftDefaultsLoraPeft(t *testing.T) {
	req := &CreateSftJobRequest{}
	req.TrainConfig.Peft = "lora"
	FillSftDefaults(req, "8b")

	if req.TrainConfig.PeftDim == 0 || req.TrainConfig.PeftAlpha == 0 {
		t.Error("expected LoRA peft dim/alpha to be populated")
	}
}
