/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package model_handlers

import (
	"strings"
	"testing"
)

// TestGetDefaultRlImage verifies the default RL image references the verl tag.
func TestGetDefaultRlImage(t *testing.T) {
	img := GetDefaultRlImage()
	if !strings.Contains(img, "verl:") {
		t.Errorf("default RL image should reference verl tag, got: %s", img)
	}
}

// TestGetDefaultRlMegatronImage verifies the default megatron RL image references verl.
func TestGetDefaultRlMegatronImage(t *testing.T) {
	img := GetDefaultRlMegatronImage()
	if !strings.Contains(img, "verl:") {
		t.Errorf("default megatron RL image should reference verl tag, got: %s", img)
	}
}

// TestFillRlDefaultsFsdp verifies FSDP2 defaults are populated on an empty request.
func TestFillRlDefaultsFsdp(t *testing.T) {
	req := &CreateRlJobRequest{}
	FillRlDefaults(req, "8b")

	if req.Priority != DefaultPriority {
		t.Errorf("expected default priority, got %d", req.Priority)
	}
	if req.ExportModel == nil || *req.ExportModel != true {
		t.Error("expected ExportModel to default to true")
	}
	if req.TrainConfig.Algorithm != "grpo" {
		t.Errorf("expected default algorithm grpo, got %s", req.TrainConfig.Algorithm)
	}
	if req.TrainConfig.Strategy != "fsdp2" {
		t.Errorf("expected default strategy fsdp2, got %s", req.TrainConfig.Strategy)
	}
	if req.TrainConfig.TrainBatchSize == 0 || req.TrainConfig.ActorLr == 0 {
		t.Error("expected RL hyperparameters to be populated from preset")
	}
	if req.NodeCount != DefaultRlNodeCount || req.GpuCount != DefaultRlGpuCount {
		t.Errorf("expected default node/gpu counts, got node=%d gpu=%d", req.NodeCount, req.GpuCount)
	}
	if !req.TrainConfig.UseTorchCompile {
		t.Error("FSDP2 should enable torch compile by default")
	}
}

// TestFillRlDefaultsMegatron verifies megatron-specific defaults are populated.
func TestFillRlDefaultsMegatron(t *testing.T) {
	req := &CreateRlJobRequest{}
	req.TrainConfig.Strategy = "megatron"
	FillRlDefaults(req, "8b")

	if req.TrainConfig.MegatronTpSize == 0 || req.TrainConfig.MegatronPpSize == 0 {
		t.Error("expected megatron tp/pp sizes to be populated")
	}
	if !req.TrainConfig.GradientCheckpointing {
		t.Error("megatron should enable gradient checkpointing")
	}
}

// TestSanitizeForVerlConfig verifies lowercasing, replacement, and truncation.
func TestSanitizeForVerlConfig(t *testing.T) {
	got := sanitizeForVerlConfig("My-Model Name")
	if got != "my_model_name" {
		t.Errorf("unexpected sanitized value: %s", got)
	}

	long := strings.Repeat("a", 100)
	if len(sanitizeForVerlConfig(long)) != 50 {
		t.Error("expected sanitized value to be truncated to 50 chars")
	}
}

// TestBuildRlContainerEntrypointHead verifies the head node writes the train script.
func TestBuildRlContainerEntrypointHead(t *testing.T) {
	script := BuildRlContainerEntrypoint("echo train", true)
	if !strings.Contains(script, "/tmp/rl_train.sh") {
		t.Error("head entrypoint should write the train script")
	}
	if !strings.Contains(script, "echo train") {
		t.Error("head entrypoint should embed the train script content")
	}
}

// TestBuildRlContainerEntrypointWorker verifies the worker node does not write the train script.
func TestBuildRlContainerEntrypointWorker(t *testing.T) {
	script := BuildRlContainerEntrypoint("echo train", false)
	if strings.Contains(script, "RL_TRAIN_SCRIPT_EOF") {
		t.Error("worker entrypoint should not write the train script")
	}
	if !strings.Contains(script, "[RL Init]") {
		t.Error("worker entrypoint should still run init steps")
	}
}

// TestBuildRlTrainScript verifies the RL train script is generated non-empty.
func TestBuildRlTrainScript(t *testing.T) {
	cfg := RlEntrypointConfig{
		ModelPath:   "/wekafs/models/Qwen-Qwen3-8B",
		ModelName:   "Qwen/Qwen3-8B",
		DatasetPath: "/wekafs/data/test",
		NodeCount:   2,
		GpuCount:    8,
		ExpName:     "rl-test",
	}
	FillRlDefaults(&CreateRlJobRequest{TrainConfig: cfg.TrainConfig}, "8b")
	script := BuildRlTrainScript(cfg)
	if len(script) == 0 {
		t.Error("expected non-empty RL train script")
	}
}

// TestBuildRlTrainScriptWithExport verifies the export/registration block is emitted.
func TestBuildRlTrainScriptWithExport(t *testing.T) {
	cfg := RlEntrypointConfig{
		ModelPath:   "/wekafs/models/Qwen-Qwen3-8B",
		ModelName:   "Qwen/Qwen3-8B",
		DatasetPath: "/wekafs/data/test",
		NodeCount:   2,
		GpuCount:    8,
		ExpName:     "rl-export",
		ExportModel: true,
		ExportPath:  "/wekafs/custom/models/rl-export",
		ModelId:     "model-123",
		Workspace:   "ws-1",
		RlJobId:     "rljob-1",
	}
	script := BuildRlTrainScript(cfg)
	if len(script) == 0 {
		t.Error("expected non-empty RL train script with export")
	}
}
