/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package optimization

import (
	"strings"
	"testing"

	"gotest.tools/assert"
)

func TestBuildHyperloomPromptClawMode(t *testing.T) {
	prompt := BuildHyperloomPrompt(PromptConfig{
		DisplayName:    "Qwen3-30B-A3B",
		ModelPath:      "/shared_nfs/models/Qwen3-30B-A3B",
		Mode:           ModeClaw,
		Framework:      FrameworkSGLang,
		Precision:      "FP4",
		TP:             1,
		EP:             1,
		GPUType:        "MI355X",
		ISL:            1024,
		OSL:            1024,
		Concurrency:    64,
		KernelBackends: []string{KernelBackendGEAK, KernelBackendCodex},
		GeakStepLimit:  120,
		Image:          "harbor.example/sglang:test",
		InferenceXPath: "/hyperloom/InferenceX",
		Workspace:      "control-plane-sandbox",
		ResultsPath:    "/workspace/hyperloom/",
		RayReplica:     1,
		RayGpu:         1,
		RayCpu:         32,
		RayMemoryGi:    128,
		TargetGpu:      "b300",
		BaselineCSV:    "model,gpu,tps\nQwen3-30B-A3B,b300,999",
		BaselineCount:  1,
	})

	assert.Assert(t, strings.Contains(prompt,
		"Use the inference-optimization skill to optimize qwen3-30b-a3b inference performance."))
	assert.Assert(t, strings.Contains(prompt, "mode: claw"))
	assert.Assert(t, strings.Contains(prompt, "Model path: /shared_nfs/models/Qwen3-30B-A3B"))
	assert.Assert(t, strings.Contains(prompt, "Framework: sglang"))
	assert.Assert(t, strings.Contains(prompt, "KERNEL_OPT_BACKENDS: geak, codex"))
	assert.Assert(t, strings.Contains(prompt, "GEAK step_limit: 120"))
	assert.Assert(t, strings.Contains(prompt, "RayJob image: harbor.example/sglang:test"))
	assert.Assert(t, strings.Contains(prompt, "Target GPU: b300"))
	assert.Assert(t, strings.Contains(prompt, "model,gpu,tps"))
}

func TestBuildHyperloomPromptLocalModeOmitsRaySection(t *testing.T) {
	prompt := BuildHyperloomPrompt(PromptConfig{
		DisplayName:    "Kimi-K2.5",
		ModelPath:      "/workspace/models/Kimi-K2.5",
		Mode:           ModeLocal,
		Framework:      FrameworkVLLM,
		KernelBackends: []string{KernelBackendClaude},
	})

	assert.Assert(t, strings.Contains(prompt, "mode: local"))
	assert.Assert(t, strings.Contains(prompt, "SandboxImage:"))
	assert.Assert(t, strings.Contains(prompt, "KERNEL_OPT_BACKENDS: claude"))
	assert.Assert(t, !strings.Contains(prompt, "Task submission:"))
	assert.Assert(t, !strings.Contains(prompt, "RayJob image:"))
}

// Defaults must stay aligned with Hyperloom-Web apps/hyperloom/src/composables/useInferOptTemplate.ts
func TestNormalizePromptConfigDefaultsMirrorHyperloomWeb(t *testing.T) {
	cfg := NormalizePromptConfig(PromptConfig{
		DisplayName: "M",
		ModelPath:   "/wekafs/models/x",
		Workspace:   "ws-test",
	})
	assert.Equal(t, cfg.Mode, ModeLocal)
	assert.Equal(t, cfg.Framework, FrameworkSGLang)
	assert.Equal(t, cfg.Precision, "FP4")
	assert.Equal(t, cfg.GPUType, "MI355X")
	assert.Equal(t, cfg.ISL, 1024)
	assert.Equal(t, cfg.OSL, 1024)
	assert.Equal(t, cfg.Concurrency, 64)
	assert.Equal(t, cfg.TP, 1)
	assert.Equal(t, cfg.EP, 1)
	assert.Equal(t, cfg.GeakStepLimit, 100)
	assert.Equal(t, cfg.InferenceXPath, "/hyperloom/InferenceX")
	assert.Equal(t, cfg.ResultsPath, "/workspace/hyperloom/")
	assert.Equal(t, cfg.Image, defaultSGLangBaseImage)
	assert.DeepEqual(t, cfg.KernelBackends, []string{KernelBackendClaude})
	assert.Equal(t, cfg.RayReplica, 1)
	assert.Equal(t, cfg.RayGpu, 1)
	assert.Equal(t, cfg.RayCpu, 32)
	assert.Equal(t, cfg.RayMemoryGi, 128)

	cfgV := NormalizePromptConfig(PromptConfig{DisplayName: "M", ModelPath: "/p", Workspace: "w", Framework: FrameworkVLLM})
	assert.Equal(t, cfgV.Image, defaultVLLMBaseImage)
}

func TestBuildHyperloomPromptEmptyRequestUsesLocalAndClaudeDefaultBackends(t *testing.T) {
	prompt := BuildHyperloomPrompt(PromptConfig{
		DisplayName: "TestModel",
		ModelPath:   "/mnt/models/foo",
		Workspace:   "core42-sandbox",
	})
	assert.Assert(t, strings.Contains(prompt, "mode: local"))
	assert.Assert(t, strings.Contains(prompt, "SandboxImage: "+defaultSGLangBaseImage))
	assert.Assert(t, strings.Contains(prompt, "KERNEL_OPT_BACKENDS: claude"))
	assert.Assert(t, !strings.Contains(prompt, "GEAK step_limit:"))
	assert.Assert(t, !strings.Contains(prompt, "Task submission:"))
}
