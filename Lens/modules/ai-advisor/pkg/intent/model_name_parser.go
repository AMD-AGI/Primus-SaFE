// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package intent

import (
	"regexp"
	"strings"
)

// ModelNameParser parses a model path or HuggingFace-style model identifier
// into structured ModelInfo (family, scale, variant).
//
// Examples:
//
//	"/models/meta-llama/Llama-3-70B-Instruct" → family=llama, scale=70B, variant=instruct
//	"mistralai/Mixtral-8x7B-v0.1"             → family=mixtral, scale=8x7B, variant=base
//	"Qwen/Qwen2-72B-Instruct"                 → family=qwen, scale=72B, variant=instruct
//	"/workspace/models/deepseek-coder-33b"     → family=deepseek, scale=33B, variant=base
type ModelNameParser struct {
	familyPatterns []familyPattern
	scaleRe        *regexp.Regexp
	variantRe      *regexp.Regexp
}

type familyPattern struct {
	re     *regexp.Regexp
	family string
}

// NewModelNameParser creates a parser with all known model families
func NewModelNameParser() *ModelNameParser {
	p := &ModelNameParser{
		scaleRe:   regexp.MustCompile(`(?i)(\d+[x\xd7]?\d*[BMK])\b`),
		variantRe: regexp.MustCompile(`(?i)(instruct|chat|base|it|hf|gptq|awq|gguf|fp16|bf16|int4|int8)\b`),
	}
	p.initFamilies()
	return p
}

// Parse parses a model path or identifier into ModelInfo
func (p *ModelNameParser) Parse(modelPath string) *ModelInfo {
	if modelPath == "" {
		return nil
	}

	info := &ModelInfo{
		Path: modelPath,
	}

	// Normalize: take the last meaningful path component(s)
	normalized := p.normalizePath(modelPath)

	// Extract family
	info.Family = p.extractFamily(normalized)

	// Extract scale
	info.Scale = p.extractScale(normalized)

	// Extract variant
	info.Variant = p.extractVariant(normalized)
	if info.Variant == "" {
		info.Variant = "base"
	}

	// If we couldn't extract anything useful, return nil
	if info.Family == "" && info.Scale == "" {
		return nil
	}

	return info
}

// normalizePath extracts the meaningful part of a model path
func (p *ModelNameParser) normalizePath(path string) string {
	// Remove common prefixes
	path = strings.TrimSpace(path)

	// Remove trailing slashes
	path = strings.TrimRight(path, "/")

	// Get the last 1-2 path components (org/model or just model)
	parts := strings.Split(path, "/")
	if len(parts) >= 2 {
		return parts[len(parts)-2] + "/" + parts[len(parts)-1]
	}
	if len(parts) == 1 {
		return parts[0]
	}

	return path
}

// extractFamily identifies the model family
func (p *ModelNameParser) extractFamily(normalized string) string {
	lower := strings.ToLower(normalized)
	for _, fp := range p.familyPatterns {
		if fp.re.MatchString(lower) {
			return fp.family
		}
	}
	return ""
}

// extractScale extracts parameter scale (e.g., 70B, 8x7B)
func (p *ModelNameParser) extractScale(normalized string) string {
	matches := p.scaleRe.FindStringSubmatch(normalized)
	if len(matches) > 1 {
		return strings.ToUpper(matches[1])
	}
	return ""
}

// extractVariant extracts the model variant
func (p *ModelNameParser) extractVariant(normalized string) string {
	matches := p.variantRe.FindStringSubmatch(normalized)
	if len(matches) > 1 {
		return strings.ToLower(matches[1])
	}
	return ""
}

// initFamilies registers all known model family patterns
func (p *ModelNameParser) initFamilies() {
	families := []struct {
		pattern string
		family  string
	}{
		// Meta / Llama family
		{`llama`, "llama"},
		{`codellama`, "codellama"},

		// Mistral / Mixtral
		{`mixtral`, "mixtral"},
		{`mistral`, "mistral"},

		// Qwen (Alibaba)
		{`qwen`, "qwen"},

		// Phi (Microsoft)
		{`phi[-_]?\d`, "phi"},

		// Gemma (Google)
		{`gemma`, "gemma"},

		// DeepSeek
		{`deepseek`, "deepseek"},

		// Falcon
		{`falcon`, "falcon"},

		// Yi (01.AI)
		{`\byi[-_]`, "yi"},

		// StarCoder
		{`starcoder`, "starcoder"},

		// ChatGLM (Tsinghua)
		{`chatglm`, "chatglm"},
		{`glm`, "glm"},

		// Baichuan
		{`baichuan`, "baichuan"},

		// InternLM (Shanghai AI Lab)
		{`internlm`, "internlm"},

		// Vicuna
		{`vicuna`, "vicuna"},

		// WizardLM
		{`wizardlm`, "wizardlm"},
		{`wizard[-_]?coder`, "wizardcoder"},

		// MPT (MosaicML)
		{`\bmpt[-_]`, "mpt"},

		// Command (Cohere)
		{`command[-_]?r`, "command-r"},

		// DBRX (Databricks)
		{`dbrx`, "dbrx"},

		// OLMo (AI2)
		{`olmo`, "olmo"},

		// Nemotron (NVIDIA)
		{`nemotron`, "nemotron"},

		// Arctic (Snowflake)
		{`arctic`, "arctic"},

		// Jamba (AI21)
		{`jamba`, "jamba"},

		// BLOOM
		{`bloom`, "bloom"},

		// OPT (Meta)
		{`\bopt[-_]`, "opt"},

		// GPT-NeoX / GPT-J
		{`gpt[-_]?neox`, "gpt-neox"},
		{`gpt[-_]?j`, "gpt-j"},

		// T5 / Flan-T5
		{`flan[-_]?t5`, "flan-t5"},
		{`\bt5[-_]`, "t5"},

		// Stable Diffusion (just in case)
		{`stable[-_]?diffusion`, "stable-diffusion"},
		{`sdxl`, "sdxl"},
	}

	for _, f := range families {
		p.familyPatterns = append(p.familyPatterns, familyPattern{
			re:     regexp.MustCompile(`(?i)` + f.pattern),
			family: f.family,
		})
	}
}
