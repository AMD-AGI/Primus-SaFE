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
	defaultMode          = ModeClaw
	defaultFramework     = FrameworkSGLang
	defaultPrecision     = "FP4"
	defaultGPUType       = "MI355X"
	defaultISL           = 1024
	defaultOSL           = 1024
	defaultConcurrency   = 64
	defaultTP            = 1
	defaultEP            = 1
	defaultResultsPath   = "/workspace/hyperloom/"
	defaultGeakStepLimit = 100
	defaultMaxHours      = 3.0
	defaultTargetGain    = 30.0
	defaultRayReplica    = 1
	defaultRayGpu        = 1
	defaultRayCPU        = 12
	defaultRayMemoryGi   = 128
	raySharedMemoryGi    = 500

	defaultSGLangImage = "harbor.core42.primus-safe.amd.com/sync/sglang:v0.5.11-rocm720-mi30x"
	defaultVLLMImage   = "harbor.core42.primus-safe.amd.com/proxy/vllm/vllm-openai-rocm:v0.19.0"
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
	KernelBackends []string
	GeakStepLimit  int
	MaxHours       float64
	TargetGain     float64
	Image          string
	OOBPath        string
	TraceLensRoot  string
	Workspace      string
	ResultsPath    string
	RayReplica     int
	RayGpu         int
	RayCpu         int
	RayMemoryGi    int
	TargetGpu      string
	BaselineCSV    string
	BaselineCount  int

	// PromptPrefix / PromptSuffix are optional free-form text snippets the
	// caller can inject before / after the generated prompt body. They are
	// emitted verbatim with a single blank line of separation. Used by the
	// Hyperloom CI batch job to point the skill at an alternate SKILL.md
	// (e.g. /wekafs/HyperloomV2/inference_optimizer/SKILL.md) before the
	// pipeline starts.
	PromptPrefix string
	PromptSuffix string
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
	if cfg.MaxHours <= 0 {
		cfg.MaxHours = defaultMaxHours
	}
	if cfg.TargetGain <= 0 {
		cfg.TargetGain = defaultTargetGain
	}
	if cfg.ResultsPath == "" {
		cfg.ResultsPath = defaultResultsPath
	}
	if cfg.Image == "" {
		if cfg.Framework == FrameworkVLLM {
			cfg.Image = defaultVLLMImage
		} else {
			cfg.Image = defaultSGLangImage
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
		cfg.RayCpu = rayCPUForTP(cfg.TP)
	}
	if cfg.RayMemoryGi <= 0 {
		cfg.RayMemoryGi = rayMemoryForTP(cfg.TP)
	}
	if len(cfg.KernelBackends) == 0 {
		cfg.KernelBackends = []string{KernelBackendGEAK, KernelBackendClaude}
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

	hasGEAK := false
	for _, b := range cfg.KernelBackends {
		tag, ok := kernelBackendPromptMap[b]
		if !ok {
			tag = strings.ToLower(strings.TrimSpace(b))
		}
		if tag == "geak" {
			hasGEAK = true
		}
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
	push("")

	push("Run time:")
	push(fmt.Sprintf("When launching inference_optimizer optimize, pass --max-hours %.1f and --target-gain %g", cfg.MaxHours, cfg.TargetGain))
	push("Do not rely on the V2 cli default max-hours.")
	push("")

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

	push("Requirements:")
	push("1. Install packages and save artifacts to writable folder.")
	push("2. Report the session ID, log path, PID, and initial health check result.")
	push("3. Then monitor the process every 300s, until work is done.")

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

	push("")
	push("4. To recover an unexpected crash, ONLY DO `optimize --resume` (same session")
	push("   dir). Which means, after the first launch, NEVER start a new `optimize` —")
	push("   that spawns a new <UTC_ts> session and is forbidden. If a `stop_reason` in")
	push("   current session state.json is final: stop and exit.")

	body := strings.Join(lines, "\n")

	// Splice in the optional prefix / suffix with one blank line of padding
	// on each side so the generated prompt still parses cleanly. We use
	// TrimRight to avoid emitting trailing blank lines when only one side is
	// set.
	prefix := strings.TrimSpace(cfg.PromptPrefix)
	suffix := strings.TrimSpace(cfg.PromptSuffix)
	if prefix != "" {
		body = prefix + "\n\n" + body
	}
	if suffix != "" {
		body = body + "\n\n" + suffix
	}

	return body
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

// rayCPUForTP returns the Ray CPU request for a given TP, using a linear
// 12 CPU per GPU ratio (e.g. TP=1 → 12, TP=8 → 96).
func rayCPUForTP(tp int) int {
	if tp <= 0 {
		return defaultRayCPU
	}
	switch tp {
	case 1:
		return 12
	case 2:
		return 24
	case 4:
		return 48
	case 8:
		return 96
	default:
		// Non-power-of-two TP (3/5/6/7): scale linearly at 12 CPU per GPU
		// rather than collapsing to the fixed default.
		return defaultRayCPU * tp
	}
}

// rayMemoryForTP mirrors Hyperloom-Web's GPU_RESOURCE_MAP memory presets (Gi).
func rayMemoryForTP(tp int) int {
	if tp <= 0 {
		return defaultRayMemoryGi
	}
	switch tp {
	case 1:
		return 128
	case 2:
		return 256
	case 4:
		return 512
	case 8:
		return 1024
	default:
		// Non-power-of-two TP (3/5/6/7): scale linearly at 128Gi per GPU
		// rather than collapsing to the fixed default.
		return defaultRayMemoryGi * tp
	}
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
