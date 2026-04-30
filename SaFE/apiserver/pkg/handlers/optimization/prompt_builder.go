/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package optimization

import (
	"fmt"
	"strings"
)

// Default values used when the client request leaves a field empty. These
// intentionally mirror the defaults in Hyperloom-Web's useInferOptTemplate.ts
// so that a task created via SaFE produces the same prompt as the same form
// submitted through the Hyperloom UI.
const (
	// Defaults mirror Hyperloom-Web useInferOptTemplate.ts (initial refs +
	// resetDefaults + DEFAULT_* constants). Update both sides together.
	defaultMode           = ModeLocal
	defaultFramework      = FrameworkSGLang
	defaultPrecision      = "FP8"
	defaultGPUType        = "MI355X"
	defaultISL            = 1024
	defaultOSL            = 1024
	defaultConcurrency    = 64
	defaultTP             = 1
	defaultEP             = 1
	defaultInferenceXPath = "/hyperloom/InferenceX"
	defaultResultsPath    = "/workspace/hyperloom/"
	defaultGeakStepLimit  = 100
	defaultRayReplica     = 1
	defaultRayGpu         = 1
	defaultRayCPU         = 32
	defaultRayMemoryGi    = 128
	raySharedMemoryGi     = 500

	defaultSGLangBaseImage = "primussafe/sglang:202603270958"
	defaultVLLMBaseImage   = "primussafe/vllm-openai-rocm:202604030417"
)

// Maps Hyperloom-Web's display string to the lowercased tag the skill expects.
var kernelBackendPromptMap = map[string]string{
	KernelBackendGEAK:   "geak",
	KernelBackendClaude: "claude",
	KernelBackendCodex:  "codex",
}

// PromptConfig is the normalized view consumed by BuildHyperloomPrompt. It is
// intentionally decoupled from CreateTaskRequest so callers can reuse the
// builder for imported tasks, retry flows, or CLI tools.
type PromptConfig struct {
	DisplayName    string
	ModelName      string
	ModelPath      string
	Mode           string
	Framework      string
	Precision      string
	TP             int
	EP             int
	GPUType        string
	ISL            int
	OSL            int
	Concurrency    int
	KernelBackends      []string
	GeakStepLimit       int
	Image               string
	ProxyImageRegistry  string
	InferenceXPath      string
	Workspace      string
	ResultsPath    string
	RayReplica     int
	RayGpu         int
	RayCpu         int
	RayMemoryGi    int
	TargetGpu      string
	BaselineCSV    string
	BaselineCount  int
}

// NormalizePromptConfig fills zero-valued fields with sensible defaults.
// The function also selects a framework-aware default image when Image is
// empty, matching Hyperloom-Web's behavior.
func NormalizePromptConfig(cfg PromptConfig) PromptConfig {
	if cfg.Mode == "" {
		cfg.Mode = defaultMode
	}
	if cfg.Framework == "" {
		cfg.Framework = defaultFramework
	}
	if cfg.Precision == "" {
		cfg.Precision = defaultPrecision
	}
	if cfg.GPUType == "" {
		cfg.GPUType = defaultGPUType
	}
	if cfg.TP <= 0 {
		cfg.TP = defaultTP
	}
	if cfg.EP <= 0 {
		cfg.EP = defaultEP
	}
	if cfg.ISL <= 0 {
		cfg.ISL = defaultISL
	}
	if cfg.OSL <= 0 {
		cfg.OSL = defaultOSL
	}
	if cfg.Concurrency <= 0 {
		cfg.Concurrency = defaultConcurrency
	}
	if cfg.GeakStepLimit <= 0 {
		cfg.GeakStepLimit = defaultGeakStepLimit
	}
	if cfg.InferenceXPath == "" {
		cfg.InferenceXPath = defaultInferenceXPath
	}
	if cfg.ResultsPath == "" {
		cfg.ResultsPath = defaultResultsPath
	}
	if cfg.Image == "" {
		base := defaultSGLangBaseImage
		if cfg.Framework == FrameworkVLLM {
			base = defaultVLLMBaseImage
		}
		if cfg.ProxyImageRegistry != "" {
			cfg.Image = cfg.ProxyImageRegistry + "/" + base
		} else {
			cfg.Image = base
		}
	}
	if cfg.RayReplica <= 0 {
		cfg.RayReplica = defaultRayReplica
	}
	if cfg.RayGpu <= 0 {
		cfg.RayGpu = cfg.TP * cfg.EP
		if cfg.RayGpu <= 0 {
			cfg.RayGpu = defaultRayGpu
		}
	}
	if cfg.RayCpu <= 0 {
		cfg.RayCpu = defaultRayCPU
	}
	if cfg.RayMemoryGi <= 0 {
		cfg.RayMemoryGi = defaultRayMemoryGi
	}
	if len(cfg.KernelBackends) == 0 {
		cfg.KernelBackends = []string{KernelBackendClaude}
	}
	return cfg
}

// BuildHyperloomPrompt emits the exact text body that the Claude Agent SDK
// (driven by the Hyperloom inference-optimization skill) expects as the first
// user message of a Claw session. Keep this in sync with Hyperloom-Web's
// useInferOptTemplate.ts `compile()` — divergence will break the skill's
// ability to pick up fields like Framework / Precision / KERNEL_OPT_*.
func BuildHyperloomPrompt(cfg PromptConfig) string {
	cfg = NormalizePromptConfig(cfg)

	displayName := firstNonEmpty(cfg.DisplayName, cfg.ModelName, "(TBD)")
	modelPath := firstNonEmpty(cfg.ModelPath, "(TBD)")

	backendValues := make([]string, 0, len(cfg.KernelBackends))
	hasGEAK := false
	for _, b := range cfg.KernelBackends {
		tag, ok := kernelBackendPromptMap[b]
		if !ok {
			tag = strings.ToLower(strings.TrimSpace(b))
		}
		if tag == "geak" {
			hasGEAK = true
		}
		backendValues = append(backendValues, tag)
	}

	sharedRoot := "/hyperloom"
	if parts := splitPath(cfg.ModelPath); len(parts) > 0 {
		sharedRoot = "/" + parts[0]
	}

	envAccessors := []string{"RayJob"}
	if hasGEAK {
		envAccessors = append(envAccessors, "GEAK")
	}
	envAccessors = append(envAccessors, "TraceLens")

	lines := make([]string, 0, 64)
	push := func(s string) { lines = append(lines, s) }

	push(fmt.Sprintf(
		"Use the inference-optimization skill to optimize %s inference performance.",
		strings.ToLower(displayName),
	))
	push(fmt.Sprintf("mode: %s", cfg.Mode))
	push("")

	push("Configuration:")
	push(fmt.Sprintf("Model path: %s", modelPath))
	push(fmt.Sprintf("Framework: %s", cfg.Framework))
	push(fmt.Sprintf("Precision: %s", cfg.Precision))
	push(fmt.Sprintf("Inference params: ISL=%d, OSL=%d, CONC=%d", cfg.ISL, cfg.OSL, cfg.Concurrency))
	push(fmt.Sprintf("TP=%d, EP=%d", cfg.TP, cfg.EP))
	push(fmt.Sprintf("GPU type: %s", cfg.GPUType))
	push(fmt.Sprintf("InferenceX path: %s", cfg.InferenceXPath))
	push("")

	if cfg.Mode == ModeLocal {
		push(fmt.Sprintf("SandboxImage: %s", cfg.Image))
		push("")
	}

	if cfg.Mode == ModeClaw {
		push("Environment:")
		push(fmt.Sprintf("The current runtime (Claw client) cannot access %s directly", sharedRoot))
		push(fmt.Sprintf("%s can all access %s", strings.Join(envAccessors, " / "), sharedRoot))
		push("The default Python on the Claw client does not have the ray package; use /opt/ray-venv/bin/python3 to execute ray_submit.py")
		push("")

		push("Task submission:")
		tail := ""
		if hasGEAK {
			tail = " and kernel tasks"
		}
		push(fmt.Sprintf("RayJob%s submit to the %s workspace", tail, cfg.Workspace))
		push(fmt.Sprintf("RayJob image: %s", cfg.Image))
		push(fmt.Sprintf(
			"RayJob resources: %d replica, %d GPU, %d CPU, %dGi memory, %dGi ephemeral",
			cfg.RayReplica, cfg.RayGpu, cfg.RayCpu, cfg.RayMemoryGi, raySharedMemoryGi,
		))
		push("")
	}

	push("Kernel Optimization:")
	push(fmt.Sprintf("KERNEL_OPT_BACKENDS: %s", strings.Join(backendValues, ", ")))
	push(fmt.Sprintf("KERNEL_OPT_IMAGE: %s", cfg.Image))
	push(fmt.Sprintf("KERNEL_OPT_WORKSPACE: %s", cfg.Workspace))
	if hasGEAK {
		push(fmt.Sprintf("GEAK step_limit: %d", cfg.GeakStepLimit))
	}
	push("Must optimize at least 5 kernels")
	push("")

	push("Requirements:")
	push(fmt.Sprintf("Save all results and the optimization report to %s", cfg.ResultsPath))
	push("Execute the full skill pipeline (Phase 0-10), including parameter sweep.")
	push("Use python3 common/safe_submit.py (not node common/safe_submit.mjs) for all SaFE API calls — Node.js fetch does not honour NODE_TLS_REJECT_UNAUTHORIZED and will fail with self-signed certs.")
	push("The Claw sandbox mounts /wekafs as read-only (hostPath). Workload pods submitted via SaFE API have a writable /wekafs via workspace PVC. Do not probe write access from the sandbox; trust that submitted workload pods can write to /wekafs.")

	if cfg.TargetGpu != "" && cfg.BaselineCount > 0 && cfg.BaselineCSV != "" {
		push("")
		push("InferenceX Baseline:")
		push(fmt.Sprintf("Target GPU: %s", cfg.TargetGpu))
		push("Raw performance values:")
		push(cfg.BaselineCSV)
		push(fmt.Sprintf(
			"Optimize and push ahead of %s. Use InferenceX data from Hyperloom as starting point for %s %s baseline.",
			cfg.TargetGpu, cfg.Framework, strings.ToLower(cfg.GPUType),
		))
	}

	return strings.Join(lines, "\n")
}

// firstNonEmpty returns the first non-empty string in the argument list, or an
// empty string if all are empty.
func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

// splitPath splits a filesystem path into its non-empty components, independent
// of the path separator used by the OS.
func splitPath(p string) []string {
	p = strings.TrimSpace(p)
	p = strings.ReplaceAll(p, "\\", "/")
	parts := strings.Split(p, "/")
	out := parts[:0]
	for _, part := range parts {
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}
