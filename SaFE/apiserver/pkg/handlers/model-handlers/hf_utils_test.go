/*
 * Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package model_handlers

import (
	"strings"
	"testing"
)

// TestGetHFModelInfo tests the GetHFModelInfo function with various real Hugging Face models
func TestGetHFModelInfo(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		wantError   bool
		checkFields []string // Fields to verify are not empty
	}{
		{
			name:        "Full URL - Meta Llama 2",
			input:       "https://huggingface.co/meta-llama/Llama-2-7b-hf",
			wantError:   false,
			checkFields: []string{"DisplayName", "Label"},
		},
		{
			name:        "Repo ID only - Qwen",
			input:       "Qwen/Qwen2.5-7B-Instruct",
			wantError:   false,
			checkFields: []string{"DisplayName", "Label"},
		},
		{
			name:        "Full URL - BERT Base",
			input:       "https://huggingface.co/bert-base-uncased",
			wantError:   false,
			checkFields: []string{"DisplayName"},
		},
		{
			name:        "Repo ID with trailing slash",
			input:       "google/flan-t5-base/",
			wantError:   false,
			checkFields: []string{"DisplayName", "Label"},
		},
		{
			name:        "API URL format",
			input:       "https://huggingface.co/api/models/facebook/opt-350m",
			wantError:   false,
			checkFields: []string{"DisplayName", "Label"},
		},
		{
			name:        "Small popular model - GPT2",
			input:       "gpt2",
			wantError:   false,
			checkFields: []string{"DisplayName"},
		},
		{
			name:      "Invalid URL",
			input:     "invalid-model-path",
			wantError: false, // Function should still return something, even if data is incomplete
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Logf("\n%s", strings.Repeat("=", 80))
			t.Logf("Testing: %s", tt.name)
			t.Logf("Input: %s", tt.input)
			t.Logf("%s", strings.Repeat("=", 80))

			info, err := GetHFModelInfo(tt.input)

			if tt.wantError {
				if err == nil {
					t.Errorf("Expected error but got none")
				}
				return
			}

			if err != nil {
				t.Logf("Warning: Got error (may be expected for invalid models): %v", err)
			}

			if info == nil {
				t.Fatal("Expected info to be non-nil")
			}

			// Print all extracted information
			t.Logf("\n--- Extracted Model Information ---")
			t.Logf("DisplayName: %s", info.DisplayName)
			t.Logf("Label:       %s", info.Label)
			t.Logf("Description: %s", truncateForLog(info.Description, 200))
			t.Logf("Icon:        %s", info.Icon)
			t.Logf("Tags:        %v", info.Tags)
			t.Logf("Tags Count:  %d", len(info.Tags))
			t.Logf("-----------------------------------\n")

			// Check specified fields
			for _, field := range tt.checkFields {
				switch field {
				case "DisplayName":
					if info.DisplayName == "" {
						t.Errorf("Expected DisplayName to be non-empty")
					}
				case "Label":
					if info.Label == "" {
						t.Errorf("Expected Label to be non-empty")
					}
				case "Description":
					if info.Description == "" {
						t.Errorf("Expected Description to be non-empty")
					}
				case "Icon":
					if info.Icon == "" {
						t.Errorf("Expected Icon to be non-empty")
					}
				case "Tags":
					if len(info.Tags) == 0 {
						t.Errorf("Expected Tags to be non-empty")
					}
				}
			}
		})
	}
}

// TestCleanRepoID tests the cleanRepoID function
func TestCleanRepoID(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "Full HTTPS URL",
			input: "https://huggingface.co/meta-llama/Llama-2-7b-hf",
			want:  "meta-llama/Llama-2-7b-hf",
		},
		{
			name:  "Full HTTP URL",
			input: "http://huggingface.co/bert-base-uncased",
			want:  "bert-base-uncased",
		},
		{
			name:  "Repo ID only",
			input: "openai/gpt-3",
			want:  "openai/gpt-3",
		},
		{
			name:  "With trailing slash",
			input: "facebook/opt-125m/",
			want:  "facebook/opt-125m",
		},
		{
			name:  "API URL format",
			input: "https://huggingface.co/api/models/google/flan-t5-small",
			want:  "google/flan-t5-small",
		},
		{
			name:  "With spaces",
			input: "  microsoft/phi-2  ",
			want:  "microsoft/phi-2",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := cleanRepoID(tt.input)
			if got != tt.want {
				t.Errorf("cleanRepoID() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestExtractDescription tests the extractDescription function
func TestExtractDescription(t *testing.T) {
	tests := []struct {
		name         string
		readme       string
		wantNonEmpty bool
	}{
		{
			name: "With Model Card header",
			readme: `# Model Card

This is a test model for natural language processing tasks.
It can be used for text generation and classification.

## Training Data
The model was trained on...`,
			wantNonEmpty: true,
		},
		{
			name: "With Introduction header",
			readme: `## Introduction

This model provides state-of-the-art performance on various benchmarks.

## Usage
To use this model...`,
			wantNonEmpty: true,
		},
		{
			name: "With HTML paragraph",
			readme: `<p>This is a powerful language model trained on diverse datasets.</p>

<p>It supports multiple languages.</p>`,
			wantNonEmpty: true,
		},
		{
			name: "With badges and then text",
			readme: `# Model Name

![badge](https://shields.io/badge/test)
[![license](https://shields.io/license)]

This is a description that comes after badges and should be extracted.`,
			wantNonEmpty: true,
		},
		{
			name:         "Empty readme",
			readme:       "",
			wantNonEmpty: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractDescription(tt.readme)
			t.Logf("Extracted description: %s", truncateForLog(got, 150))

			if tt.wantNonEmpty && got == "" {
				t.Errorf("Expected non-empty description")
			}
		})
	}
}

// TestGetHFModelInfo_DetailedOutput is a verbose test that prints full details
// This test can be run individually to see complete output for specific models
func TestGetHFModelInfo_DetailedOutput(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping detailed test in short mode")
	}

	models := []string{
		"meta-llama/Llama-2-7b-hf",
		"gpt2",
		"bert-base-uncased",
	}

	for _, modelPath := range models {
		t.Run(modelPath, func(t *testing.T) {
			t.Logf("\n%s", strings.Repeat("=", 100))
			t.Logf("DETAILED TEST FOR: %s", modelPath)
			t.Logf("%s", strings.Repeat("=", 100))

			info, err := GetHFModelInfo(modelPath)
			if err != nil {
				t.Logf("Error occurred: %v", err)
			}

			if info != nil {
				t.Logf("\n📋 FULL MODEL INFORMATION:")
				t.Logf("%s", strings.Repeat("━", 100))
				t.Logf("DisplayName:  %s", info.DisplayName)
				t.Logf("Label:        %s", info.Label)
				t.Logf("Icon:         %s", info.Icon)
				t.Logf("Description:\n%s", wrapText(info.Description, 90))
				t.Logf("\nTags (%d):", len(info.Tags))
				for i, tag := range info.Tags {
					if i < 20 { // Limit to first 20 tags for readability
						t.Logf("  - %s", tag)
					}
				}
				if len(info.Tags) > 20 {
					t.Logf("  ... and %d more tags", len(info.Tags)-20)
				}
				t.Logf("%s\n", strings.Repeat("━", 100))
			}
		})
	}
}

// TestSingleURL is a simple test to check a single URL
// Modify the url variable to test different models
// Run with: go test -v -run TestSingleURL
func TestSingleURL(t *testing.T) {
	// ⭐ 修改这里的 URL 来测试不同的模型 ⭐
	url := "Qwen/Qwen2.5-7B-Instruct"
	// 其他示例:
	// url := "meta-llama/Llama-2-7b-hf"
	// url := "https://huggingface.co/gpt2"
	// url := "bert-base-uncased"
	// url := "facebook/opt-350m"

	t.Logf("\n%s", strings.Repeat("=", 100))
	t.Logf("🔍 测试模型: %s", url)
	t.Logf("%s", strings.Repeat("=", 100))

	info, err := GetHFModelInfo(url)

	if err != nil {
		t.Logf("⚠️  警告: %v", err)
	}

	if info == nil {
		t.Fatal("❌ 错误: 无法获取模型信息")
	}

	// 打印所有提取的信息
	t.Logf("\n📋 模型信息:")
	t.Logf("%s", strings.Repeat("-", 100))
	t.Logf("✓ DisplayName (显示名称):  %s", info.DisplayName)
	t.Logf("✓ Label (标签/作者):       %s", info.Label)
	t.Logf("✓ Icon (图标):             %s", info.Icon)
	t.Logf("\n✓ Description (描述):")
	if info.Description != "" {
		t.Logf(" %s", info.Description)
	} else {
		t.Logf("  (无描述)")
	}
	t.Logf("\n✓ Tags (标签列表) - 总共 %d 个:", len(info.Tags))
	if len(info.Tags) > 0 {
		for i, tag := range info.Tags {
			if i < 30 { // 显示前30个标签
				t.Logf("  %2d. %s", i+1, tag)
			}
		}
		if len(info.Tags) > 30 {
			t.Logf("  ... 还有 %d 个标签", len(info.Tags)-30)
		}
	} else {
		t.Logf("  (无标签)")
	}
	t.Logf("%s", strings.Repeat("-", 100))
	t.Logf("\n✅ 测试完成！")
}

// Helper function to truncate text for logging
func truncateForLog(text string, maxLen int) string {
	if len(text) <= maxLen {
		return text
	}
	return text[:maxLen] + "..."
}

// Helper function to wrap text for better display
func wrapText(text string, width int) string {
	if len(text) == 0 {
		return "(empty)"
	}
	if len(text) <= width {
		return text
	}

	var result string
	for len(text) > width {
		result += text[:width] + "\n"
		text = text[width:]
	}
	result += text
	return result
}
