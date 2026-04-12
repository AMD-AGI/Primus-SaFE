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
