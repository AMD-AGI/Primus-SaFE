/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package model_handlers

import (
	"strings"
	"testing"
)

func TestBuildEntrypointMkdirContainsExpName(t *testing.T) {
	cfg := EntrypointConfig{
		DatasetPath: "/wekafs/data/test",
		PrimusPath:  "/tmp/primus",
		ExpName:     "my-test-experiment",
		HfPath:      "Qwen/Qwen3-8B",
		ModelSize:   "8b",
		TrainConfig: SftTrainConfig{
			TrainIters:                100,
			GlobalBatchSize:           8,
			MicroBatchSize:            1,
			SeqLength:                 2048,
			FinetuneLr:                5e-6,
			TensorModelParallelSize:   1,
			PipelineModelParallelSize: 1,
			ContextParallelSize:       1,
			LrWarmupIters:             5,
			SaveInterval:              50,
			Peft:                      "lora",
			PackedSequence:            false,
		},
	}

	script := BuildEntrypoint(cfg)

	expectedMkdir := `mkdir -p "./output/${PRIMUS_TEAM:-amd}/${PRIMUS_USER:-root}/my-test-experiment"`
	if !strings.Contains(script, expectedMkdir) {
		t.Errorf("script missing expected mkdir line.\nWant: %s\nGot script (relevant section):\n%s",
			expectedMkdir, extractSection(script, "EXPEOF", 5))
	}

	if !strings.Contains(script, `sed "s/%MODULE_CONFIG%/$MODULE_CONFIG/g"`) {
		t.Error("sed MODULE_CONFIG replacement is broken")
	}

	if !strings.Contains(script, `printf '%07d'`) {
		t.Error("printf format for checkpoint iteration is broken")
	}

	if !strings.Contains(script, "pretrained_checkpoint:") {
		t.Error("LoRA config missing pretrained_checkpoint")
	}
}

func TestBuildEntrypointFullSFT(t *testing.T) {
	cfg := EntrypointConfig{
		DatasetPath: "/wekafs/data/test",
		PrimusPath:  "/tmp/primus",
		ExpName:     "full-sft-run",
		HfPath:      "Qwen/Qwen3-8B",
		ModelSize:   "8b",
		TrainConfig: SftTrainConfig{
			TrainIters:                100,
			GlobalBatchSize:           8,
			MicroBatchSize:            1,
			SeqLength:                 2048,
			FinetuneLr:                5e-6,
			TensorModelParallelSize:   1,
			PipelineModelParallelSize: 1,
			ContextParallelSize:       1,
			LrWarmupIters:             5,
			SaveInterval:              50,
			Peft:                      "none",
		},
	}

	script := BuildEntrypoint(cfg)

	expectedMkdir := `mkdir -p "./output/${PRIMUS_TEAM:-amd}/${PRIMUS_USER:-root}/full-sft-run"`
	if !strings.Contains(script, expectedMkdir) {
		t.Errorf("full SFT script missing expected mkdir.\nWant: %s", expectedMkdir)
	}

	if strings.Contains(script, "pretrained_checkpoint:") {
		t.Error("full SFT should NOT have pretrained_checkpoint")
	}

	if !strings.Contains(script, `peft: "none"`) {
		t.Error("full SFT should have peft: none")
	}
}

func TestBuildEntrypointExpNameWithSpecialChars(t *testing.T) {
	cfg := EntrypointConfig{
		DatasetPath: "/wekafs/data/test",
		PrimusPath:  "/tmp/primus",
		ExpName:     "sft-m78-lora-8b-multi-58946",
		HfPath:      "Qwen/Qwen3-8B",
		ModelSize:   "8b",
		TrainConfig: SftTrainConfig{
			TrainIters:                1000,
			GlobalBatchSize:           128,
			MicroBatchSize:            1,
			SeqLength:                 2048,
			FinetuneLr:                1e-4,
			TensorModelParallelSize:   1,
			PipelineModelParallelSize: 1,
			ContextParallelSize:       1,
			LrWarmupIters:             50,
			SaveInterval:              500,
			Peft:                      "lora",
		},
	}

	script := BuildEntrypoint(cfg)

	expectedMkdir := `mkdir -p "./output/${PRIMUS_TEAM:-amd}/${PRIMUS_USER:-root}/sft-m78-lora-8b-multi-58946"`
	if !strings.Contains(script, expectedMkdir) {
		t.Errorf("script missing expected mkdir with job-style exp name.\nWant: %s", expectedMkdir)
	}

	if !strings.Contains(script, "pretrained_checkpoint: ./data/megatron_checkpoints/Qwen3-8B") {
		t.Error("LoRA config missing correct pretrained_checkpoint path")
	}
}

func TestBuildEntrypointRequiresUsableCheckpoint(t *testing.T) {
	cfg := EntrypointConfig{
		DatasetPath: "/wekafs/data/test",
		PrimusPath:  "/tmp/primus",
		ExpName:     "checkpoint-guard",
		HfPath:      "/wekafs/models/Qwen-Qwen3-8B",
		ModelSize:   "8b",
		ExportModel: true,
		TrainConfig: SftTrainConfig{
			TrainIters:                100,
			GlobalBatchSize:           8,
			MicroBatchSize:            1,
			SeqLength:                 2048,
			FinetuneLr:                5e-6,
			TensorModelParallelSize:   1,
			PipelineModelParallelSize: 1,
			ContextParallelSize:       1,
			LrWarmupIters:             5,
			SaveInterval:              50,
			Peft:                      "none",
		},
	}

	script := BuildEntrypoint(cfg)

	if !strings.Contains(script, "Training completed but no usable checkpoint was produced") {
		t.Error("script should fail when no usable checkpoint is produced")
	}

	if !strings.Contains(script, "Verified training checkpoint:") {
		t.Error("script should log the verified checkpoint before export")
	}

	if !strings.Contains(script, `CKPT_DIR="$(dirname "${VERIFIED_LATEST_DIR}")"`) {
		t.Error("export should reuse the verified checkpoint directory")
	}
}

func TestBuildEntrypointRequiresSuccessfulModelRegistration(t *testing.T) {
	cfg := EntrypointConfig{
		DatasetPath: "/wekafs/data/test",
		PrimusPath:  "/tmp/primus",
		ExpName:     "register-guard",
		HfPath:      "/wekafs/models/Qwen-Qwen3-8B",
		ModelSize:   "8b",
		ExportModel: true,
		TrainConfig: SftTrainConfig{
			TrainIters:                100,
			GlobalBatchSize:           8,
			MicroBatchSize:            1,
			SeqLength:                 2048,
			FinetuneLr:                5e-6,
			TensorModelParallelSize:   1,
			PipelineModelParallelSize: 1,
			ContextParallelSize:       1,
			LrWarmupIters:             5,
			SaveInterval:              50,
			Peft:                      "none",
		},
	}

	script := BuildEntrypoint(cfg)

	if !strings.Contains(script, "ERROR: HF export incomplete") {
		t.Error("script should refuse to register incomplete HF exports")
	}

	if !strings.Contains(script, `curl -fsS -o "${REGISTER_RESPONSE}" -X POST`) {
		t.Error("model registration should fail on non-2xx HTTP responses")
	}

	if !strings.Contains(script, "ERROR: failed to register model after successful HF export.") {
		t.Error("script should fail when model registration fails")
	}
}

func TestBuildEntrypointUsesSharedSquadCacheAndHonorsPreInjectedAinic(t *testing.T) {
	cfg := EntrypointConfig{
		DatasetPath: "/shared_nfs/data/test",
		PrimusPath:  "/tmp/primus",
		ExpName:     "shared-squad-cache",
		HfPath:      "/shared_nfs/models/Qwen/Qwen3-8B",
		ModelSize:   "8b",
		PfsBasePath: "/shared_nfs",
		TrainConfig: SftTrainConfig{
			TrainIters:                100,
			GlobalBatchSize:           16,
			MicroBatchSize:            1,
			SeqLength:                 2048,
			FinetuneLr:                5e-6,
			TensorModelParallelSize:   1,
			PipelineModelParallelSize: 1,
			ContextParallelSize:       1,
			LrWarmupIters:             5,
			SaveInterval:              50,
			Peft:                      "none",
		},
	}

	script := BuildEntrypoint(cfg)

	if !strings.Contains(script, `SHARED_SQUAD_CACHE="${DATA_PATH}/squad-cache"`) {
		t.Error("script should place squad cache on shared DATA_PATH for multi-node SFT")
	}

	if !strings.Contains(script, `ln -sfn "$SHARED_SQUAD_CACHE" "$SQUAD_CACHE"`) {
		t.Error("script should symlink local squad cache to shared cache")
	}

	if !strings.Contains(script, `if [ "${USING_AINIC:-0}" = "1" ] ||`) {
		t.Error("entrypoint should honor pre-injected USING_AINIC=1")
	}

	if !strings.Contains(script, `export NCCL_IB_GID_INDEX="${NCCL_IB_GID_INDEX:-1}"`) {
		t.Error("entrypoint should preserve pre-injected GID index")
	}
}

func extractSection(s, marker string, lines int) string {
	idx := strings.Index(s, marker)
	if idx < 0 {
		return "(marker not found)"
	}
	end := idx + len(marker)
	count := 0
	for i := end; i < len(s) && count < lines; i++ {
		if s[i] == '\n' {
			count++
		}
		end = i + 1
	}
	start := idx - 200
	if start < 0 {
		start = 0
	}
	return s[start:end]
}

// --- merged from entrypoint_sft_helpers_test.go ---

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
