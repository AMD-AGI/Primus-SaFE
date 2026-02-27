// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package runner

import (
	"context"
	"errors"
	"testing"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/controlplane/database/model"
)

// MockBackend is a mock implementation of Backend for testing
type MockBackend struct {
	getRunURLFunc func(ctx context.Context, tools []*model.Tool) (*RunURLResult, error)
}

func (m *MockBackend) GetRunURL(ctx context.Context, tools []*model.Tool) (*RunURLResult, error) {
	if m.getRunURLFunc != nil {
		return m.getRunURLFunc(ctx, tools)
	}
	return &RunURLResult{
		RedirectURL: "http://test.com/session/new",
		SessionID:   "test-session",
	}, nil
}

func TestNewRunner(t *testing.T) {
	backend := &MockBackend{}
	runner := NewRunner(backend)

	if runner == nil {
		t.Fatal("NewRunner() returned nil")
	}
	if runner.backend != backend {
		t.Error("NewRunner() did not set backend correctly")
	}
}

func TestRunner_GetRunURL(t *testing.T) {
	tests := []struct {
		name        string
		tools       []*model.Tool
		backendFunc func(ctx context.Context, tools []*model.Tool) (*RunURLResult, error)
		wantErr     bool
		errContains string
	}{
		{
			name:    "empty tools",
			tools:   []*model.Tool{},
			wantErr: true,
			errContains: "no tools provided",
		},
		{
			name: "inactive tool",
			tools: []*model.Tool{
				{
					Name:   "test-tool",
					Status: model.AppStatusInactive,
				},
			},
			wantErr: true,
			errContains: "not active",
		},
		{
			name: "single active tool",
			tools: []*model.Tool{
				{
					Name:   "test-tool",
					Status: model.AppStatusActive,
				},
			},
			wantErr: false,
		},
		{
			name: "multiple active tools",
			tools: []*model.Tool{
				{
					Name:   "tool1",
					Status: model.AppStatusActive,
				},
				{
					Name:   "tool2",
					Status: model.AppStatusActive,
				},
			},
			wantErr: false,
		},
		{
			name: "backend error",
			tools: []*model.Tool{
				{
					Name:   "test-tool",
					Status: model.AppStatusActive,
				},
			},
			backendFunc: func(ctx context.Context, tools []*model.Tool) (*RunURLResult, error) {
				return nil, errors.New("backend error")
			},
			wantErr: true,
			errContains: "backend error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			backend := &MockBackend{getRunURLFunc: tt.backendFunc}
			runner := NewRunner(backend)

			result, err := runner.GetRunURL(context.Background(), tt.tools)

			if tt.wantErr {
				if err == nil {
					t.Error("GetRunURL() expected error, got nil")
				} else if tt.errContains != "" && !contains(err.Error(), tt.errContains) {
					t.Errorf("GetRunURL() error = %v, want error containing %v", err, tt.errContains)
				}
				return
			}

			if err != nil {
				t.Errorf("GetRunURL() unexpected error = %v", err)
				return
			}

			if result == nil {
				t.Error("GetRunURL() returned nil result")
			}
		})
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsAt(s, substr, 0))
}

func containsAt(s, substr string, start int) bool {
	if start+len(substr) > len(s) {
		return false
	}
	for i := start; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
