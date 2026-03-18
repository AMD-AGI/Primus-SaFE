// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package registry

import (
	"regexp"
	"strings"

	"github.com/AMD-AGI/Primus-SaFE/Lens/ai-advisor/pkg/intent"
)

// LayerAnalyzer extracts framework hints and installed packages from image
// layer history. It parses Dockerfile instructions (RUN, COPY, ENV) to find
// pip/apt/conda install commands and derive framework signals.
type LayerAnalyzer struct {
	pipInstallRe  *regexp.Regexp
	aptInstallRe  *regexp.Regexp
	condaInstallRe *regexp.Regexp
}

// NewLayerAnalyzer creates a new LayerAnalyzer
func NewLayerAnalyzer() *LayerAnalyzer {
	return &LayerAnalyzer{
		pipInstallRe:   regexp.MustCompile(`pip[3]?\s+install\s+(.+?)(?:\s*&&|\s*$|\\)`),
		aptInstallRe:   regexp.MustCompile(`apt(?:-get)?\s+install\s+(?:-y\s+)?(.+?)(?:\s*&&|\s*$|\\)`),
		condaInstallRe: regexp.MustCompile(`conda\s+install\s+(?:-y\s+)?(.+?)(?:\s*&&|\s*$|\\)`),
	}
}

// AnalysisResult holds the result of layer analysis
type AnalysisResult struct {
	BaseImage         string
	InstalledPackages []intent.PackageInfo
	FrameworkHints    map[string]interface{}
	LayerHistory      []intent.LayerInfo
	Entrypoint        []string
	Cmd               []string
	EnvVars           map[string]string
	WorkingDir        string
}

// Analyze analyzes an image config and extracts intent-relevant information
func (a *LayerAnalyzer) Analyze(config *ImageConfig) *AnalysisResult {
	result := &AnalysisResult{
		FrameworkHints: make(map[string]interface{}),
		EnvVars:        make(map[string]string),
	}

	if config == nil {
		return result
	}

	// Extract container config
	if config.Config.Entrypoint != nil {
		result.Entrypoint = config.Config.Entrypoint
	}
	if config.Config.Cmd != nil {
		result.Cmd = config.Config.Cmd
	}
	result.WorkingDir = config.Config.WorkingDir

	// Parse environment variables
	for _, envStr := range config.Config.Env {
		parts := strings.SplitN(envStr, "=", 2)
		if len(parts) == 2 {
			result.EnvVars[parts[0]] = parts[1]
		}
	}

	// Analyze layer history
	for i, entry := range config.History {
		layerInfo := intent.LayerInfo{
			CreatedBy: entry.CreatedBy,
			Comment:   entry.Comment,
		}
		result.LayerHistory = append(result.LayerHistory, layerInfo)

		// Extract base image from first non-empty FROM instruction
		if i == 0 && strings.Contains(entry.CreatedBy, "FROM") {
			result.BaseImage = extractBaseImage(entry.CreatedBy)
		}

		// Parse package installations
		packages := a.extractPackages(entry.CreatedBy)
		result.InstalledPackages = append(result.InstalledPackages, packages...)
	}

	// Derive framework hints from packages and env vars
	a.deriveFrameworkHints(result)

	return result
}

// extractPackages extracts installed packages from a Dockerfile instruction
func (a *LayerAnalyzer) extractPackages(instruction string) []intent.PackageInfo {
	var packages []intent.PackageInfo

	// pip install
	if matches := a.pipInstallRe.FindStringSubmatch(instruction); len(matches) > 1 {
		pkgs := parsePackageList(matches[1], "pip")
		packages = append(packages, pkgs...)
	}

	// apt install
	if matches := a.aptInstallRe.FindStringSubmatch(instruction); len(matches) > 1 {
		pkgs := parsePackageList(matches[1], "apt")
		packages = append(packages, pkgs...)
	}

	// conda install
	if matches := a.condaInstallRe.FindStringSubmatch(instruction); len(matches) > 1 {
		pkgs := parsePackageList(matches[1], "conda")
		packages = append(packages, pkgs...)
	}

	return packages
}

// parsePackageList parses a space-separated package list (possibly with version specifiers)
func parsePackageList(raw string, manager string) []intent.PackageInfo {
	var packages []intent.PackageInfo

	// Split by whitespace
	tokens := strings.Fields(raw)
	for _, token := range tokens {
		token = strings.TrimSpace(token)
		if token == "" || strings.HasPrefix(token, "-") || strings.HasPrefix(token, "#") {
			continue
		}

		pkg := intent.PackageInfo{Manager: manager}

		// Handle version specifiers
		for _, sep := range []string{"==", ">=", "<=", "~=", "!="} {
			if idx := strings.Index(token, sep); idx > 0 {
				pkg.Name = token[:idx]
				pkg.Version = token[idx+len(sep):]
				break
			}
		}

		if pkg.Name == "" {
			pkg.Name = token
		}

		// Skip common non-package tokens
		if pkg.Name == "." || pkg.Name == "--" || strings.HasPrefix(pkg.Name, "/") {
			continue
		}

		packages = append(packages, pkg)
	}

	return packages
}

// deriveFrameworkHints analyzes installed packages to generate high-level framework hints
func (a *LayerAnalyzer) deriveFrameworkHints(result *AnalysisResult) {
	packageNames := make(map[string]bool)
	for _, pkg := range result.InstalledPackages {
		packageNames[strings.ToLower(pkg.Name)] = true
	}

	// Serving frameworks
	servingFrameworks := map[string]string{
		"vllm":                      "vllm",
		"text-generation-inference": "tgi",
		"sglang":                    "sglang",
		"tritonserver":              "triton",
		"torchserve":                "torchserve",
	}
	for pkg, name := range servingFrameworks {
		if packageNames[pkg] {
			result.FrameworkHints["serving_framework"] = name
			break
		}
	}

	// Training frameworks
	trainingFrameworks := map[string]string{
		"deepspeed":     "deepspeed",
		"megatron-core": "megatron",
		"megatron-lm":   "megatron",
		"trl":           "trl",
		"peft":          "peft",
		"transformers":  "huggingface",
		"lightning":     "lightning",
	}
	for pkg, name := range trainingFrameworks {
		if packageNames[pkg] {
			result.FrameworkHints["training_framework"] = name
			break
		}
	}

	// Runtime framework
	if packageNames["torch"] || packageNames["pytorch"] {
		result.FrameworkHints["runtime"] = "pytorch"
	} else if packageNames["jax"] || packageNames["jaxlib"] {
		result.FrameworkHints["runtime"] = "jax"
	} else if packageNames["tensorflow"] || packageNames["tf-nightly"] {
		result.FrameworkHints["runtime"] = "tensorflow"
	}

	// ROCm vs CUDA
	for _, env := range []string{"ROCR_VISIBLE_DEVICES", "HSA_ENABLE_SDMA"} {
		if _, ok := result.EnvVars[env]; ok {
			result.FrameworkHints["gpu_backend"] = "rocm"
			break
		}
	}
	if _, ok := result.FrameworkHints["gpu_backend"]; !ok {
		if _, ok := result.EnvVars["NVIDIA_VISIBLE_DEVICES"]; ok {
			result.FrameworkHints["gpu_backend"] = "cuda"
		}
	}
}

// extractBaseImage extracts the base image name from a FROM instruction
func extractBaseImage(instruction string) string {
	// Typical format: "/bin/sh -c #(nop)  FROM baseimage:tag"
	// Or: "FROM baseimage:tag AS builder"
	instruction = strings.TrimSpace(instruction)

	if idx := strings.Index(instruction, "FROM "); idx != -1 {
		rest := instruction[idx+5:]
		parts := strings.Fields(rest)
		if len(parts) > 0 {
			return parts[0]
		}
	}

	return ""
}
