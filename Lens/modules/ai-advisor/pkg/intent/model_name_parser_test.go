// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package intent

import (
	"testing"
)

func TestModelNameParser(t *testing.T) {
	p := NewModelNameParser()

	tests := []struct {
		name       string
		path       string
		wantFamily string
		wantScale  string
		wantVar    string
	}{
		{
			name:       "Llama3_70B_Instruct",
			path:       "/models/meta-llama/Llama-3-70B-Instruct",
			wantFamily: "llama",
			wantScale:  "70B",
			wantVar:    "instruct",
		},
		{
			name:       "Mixtral_8x7B",
			path:       "mistralai/Mixtral-8x7B-v0.1",
			wantFamily: "mixtral",
			wantScale:  "8X7B",
			wantVar:    "base",
		},
		{
			name:       "Qwen2_72B",
			path:       "Qwen/Qwen2-72B-Instruct",
			wantFamily: "qwen",
			wantScale:  "72B",
			wantVar:    "instruct",
		},
		{
			name:       "DeepSeek_Coder_33B",
			path:       "deepseek-ai/deepseek-coder-33b-instruct",
			wantFamily: "deepseek",
			wantScale:  "33B",
			wantVar:    "instruct",
		},
		{
			name:       "Phi3_Mini",
			path:       "microsoft/Phi-3-mini-4k-instruct",
			wantFamily: "phi",
			wantScale:  "",
			wantVar:    "instruct",
		},
		{
			name:       "Gemma_7B",
			path:       "google/gemma-7b",
			wantFamily: "gemma",
			wantScale:  "7B",
			wantVar:    "base",
		},
		{
			name:       "Llama3_8B_GPTQ",
			path:       "/models/meta-llama/Llama-3-8B-GPTQ",
			wantFamily: "llama",
			wantScale:  "8B",
			wantVar:    "gptq",
		},
		{
			name:       "GGUF_model",
			path:       "/models/llama-3-8b-instruct.Q4_K_M.gguf",
			wantFamily: "llama",
			wantScale:  "8B",
			wantVar:    "instruct",
		},
		{
			name:       "ChatGLM3_6B",
			path:       "THUDM/chatglm3-6b",
			wantFamily: "chatglm",
			wantScale:  "6B",
			wantVar:    "base",
		},
		{
			name:       "InternLM_20B",
			path:       "internlm/internlm-20b",
			wantFamily: "internlm",
			wantScale:  "20B",
			wantVar:    "base",
		},
		{
			name:       "Falcon_180B",
			path:       "tiiuae/falcon-180B-chat",
			wantFamily: "falcon",
			wantScale:  "180B",
			wantVar:    "chat",
		},
		{
			name:       "Unknown_model",
			path:       "/models/custom-model-v1",
			wantFamily: "",
			wantScale:  "",
			wantVar:    "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info := p.Parse(tt.path)
			if tt.wantFamily == "" && info == nil {
				return // Expected nil
			}
			if info == nil {
				t.Fatal("Parse returned nil, expected non-nil")
			}
			if info.Family != tt.wantFamily {
				t.Errorf("family: got %q, want %q", info.Family, tt.wantFamily)
			}
			if info.Scale != tt.wantScale {
				t.Errorf("scale: got %q, want %q", info.Scale, tt.wantScale)
			}
			if tt.wantVar != "" && info.Variant != tt.wantVar {
				t.Errorf("variant: got %q, want %q", info.Variant, tt.wantVar)
			}
		})
	}
}
