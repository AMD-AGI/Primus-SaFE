// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package runner

import (
	"context"
	"net/url"
	"strings"
	"testing"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/controlplane/database/model"
)

func TestNewHTTPBackend(t *testing.T) {
	tests := []struct {
		name    string
		config  HTTPBackendConfig
		wantURL string
	}{
		{
			name: "basic URL",
			config: HTTPBackendConfig{
				BaseURL: "http://localhost:8000",
			},
			wantURL: "http://localhost:8000",
		},
		{
			name: "URL with trailing slash",
			config: HTTPBackendConfig{
				BaseURL: "http://localhost:8000/",
			},
			wantURL: "http://localhost:8000",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			backend := NewHTTPBackend(tt.config)
			if backend.baseURL != tt.wantURL {
				t.Errorf("NewHTTPBackend() baseURL = %v, want %v", backend.baseURL, tt.wantURL)
			}
		})
	}
}

func TestHTTPBackend_GetRunURL(t *testing.T) {
	tests := []struct {
		name      string
		baseURL   string
		tools     []*model.Tool
		wantErr   bool
		checkURL  func(t *testing.T, urlStr string)
	}{
		{
			name:    "empty tools",
			baseURL: "http://localhost:8000",
			tools:   []*model.Tool{},
			wantErr: true,
		},
		{
			name:    "single skill",
			baseURL: "http://localhost:8000",
			tools: []*model.Tool{
				{
					Type: model.AppTypeSkill,
					Name: "web-search",
				},
			},
			checkURL: func(t *testing.T, urlStr string) {
				// URL encoded colon: %3A
				if !strings.Contains(urlStr, "skill") || !strings.Contains(urlStr, "web-search") {
					t.Errorf("URL should contain skill and web-search, got %s", urlStr)
				}
			},
		},
		{
			name:    "single mcp",
			baseURL: "http://localhost:8000",
			tools: []*model.Tool{
				{
					Type: model.AppTypeMCP,
					Name: "filesystem-server",
				},
			},
			checkURL: func(t *testing.T, urlStr string) {
				// URL encoded colon: %3A
				if !strings.Contains(urlStr, "mcp") || !strings.Contains(urlStr, "filesystem-server") {
					t.Errorf("URL should contain mcp and filesystem-server, got %s", urlStr)
				}
			},
		},
		{
			name:    "multiple tools",
			baseURL: "http://localhost:8000",
			tools: []*model.Tool{
				{
					Type: model.AppTypeSkill,
					Name: "web-search",
				},
				{
					Type: model.AppTypeMCP,
					Name: "filesystem",
				},
			},
			checkURL: func(t *testing.T, urlStr string) {
				// URL should be properly encoded
				parsed, err := url.Parse(urlStr)
				if err != nil {
					t.Errorf("Failed to parse URL: %v", err)
					return
				}
				tools := parsed.Query().Get("tools")
				if tools == "" {
					// tools might be in the raw query due to encoding
					if !strings.Contains(urlStr, "tools=") {
						t.Errorf("URL should contain tools parameter")
					}
				}
			},
		},
		{
			name:    "URL construction",
			baseURL: "https://poco.example.com",
			tools: []*model.Tool{
				{
					Type: model.AppTypeSkill,
					Name: "test",
				},
			},
			checkURL: func(t *testing.T, urlStr string) {
				if !strings.HasPrefix(urlStr, "https://poco.example.com/session/new") {
					t.Errorf("URL should start with base URL + /session/new, got %s", urlStr)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			backend := NewHTTPBackend(HTTPBackendConfig{
				BaseURL: tt.baseURL,
			})

			result, err := backend.GetRunURL(context.Background(), tt.tools)

			if tt.wantErr {
				if err == nil {
					t.Error("GetRunURL() expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Errorf("GetRunURL() unexpected error = %v", err)
				return
			}

			if result == nil {
				t.Fatal("GetRunURL() returned nil result")
			}

			if result.RedirectURL == "" {
				t.Error("GetRunURL() returned empty redirect URL")
			}

			if tt.checkURL != nil {
				tt.checkURL(t, result.RedirectURL)
			}
		})
	}
}
