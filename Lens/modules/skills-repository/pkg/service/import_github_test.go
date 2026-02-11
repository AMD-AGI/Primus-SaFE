// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package service

import (
	"context"
	"fmt"
	"testing"
)

func TestParseGitHubURL(t *testing.T) {
	tests := []struct {
		name        string
		url         string
		wantOwner   string
		wantRepo    string
		wantRefPath []string
		wantErr     bool
	}{
		{
			name:        "simple repo URL",
			url:         "https://github.com/owner/repo",
			wantOwner:   "owner",
			wantRepo:    "repo",
			wantRefPath: nil,
		},
		{
			name:        "repo with .git suffix",
			url:         "https://github.com/owner/repo.git",
			wantOwner:   "owner",
			wantRepo:    "repo",
			wantRefPath: nil,
		},
		{
			name:        "simple branch",
			url:         "https://github.com/owner/repo/tree/main",
			wantOwner:   "owner",
			wantRepo:    "repo",
			wantRefPath: []string{"main"},
		},
		{
			name:        "branch with subdirectory",
			url:         "https://github.com/owner/repo/tree/main/src/skills",
			wantOwner:   "owner",
			wantRepo:    "repo",
			wantRefPath: []string{"main", "src", "skills"},
		},
		{
			name:        "slashed branch with subdirectory",
			url:         "https://github.com/xiaofei-zheng/GEAK/tree/feature/xiaofei/geak-online/server/geak-optimize",
			wantOwner:   "xiaofei-zheng",
			wantRepo:    "GEAK",
			wantRefPath: []string{"feature", "xiaofei", "geak-online", "server", "geak-optimize"},
		},
		{
			name:    "non-github host",
			url:     "https://gitlab.com/owner/repo",
			wantErr: true,
		},
		{
			name:    "invalid URL - no repo",
			url:     "https://github.com/owner",
			wantErr: true,
		},
		{
			name:      "www.github.com",
			url:       "https://www.github.com/owner/repo/tree/dev",
			wantOwner: "owner",
			wantRepo:  "repo",
			wantRefPath: []string{"dev"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			owner, repo, refPath, err := parseGitHubURL(tt.url)
			if (err != nil) != tt.wantErr {
				t.Fatalf("parseGitHubURL() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if owner != tt.wantOwner {
				t.Errorf("owner = %q, want %q", owner, tt.wantOwner)
			}
			if repo != tt.wantRepo {
				t.Errorf("repo = %q, want %q", repo, tt.wantRepo)
			}
			if len(refPath) != len(tt.wantRefPath) {
				t.Fatalf("refAndPath = %v, want %v", refPath, tt.wantRefPath)
			}
			for i := range refPath {
				if refPath[i] != tt.wantRefPath[i] {
					t.Errorf("refAndPath[%d] = %q, want %q", i, refPath[i], tt.wantRefPath[i])
				}
			}
		})
	}
}

func TestResolveAndDownload(t *testing.T) {
	// mockDownloader returns a download function that succeeds only for URLs
	// whose branch component is in the validBranches set.
	mockDownloader := func(validBranches map[string]bool) downloadFunc {
		return func(ctx context.Context, url string) ([]byte, error) {
			// Extract branch from URL pattern:
			// https://github.com/{owner}/{repo}/archive/refs/heads/{branch}.zip
			const prefix = "https://github.com/owner/repo/archive/refs/heads/"
			const suffix = ".zip"
			if len(url) <= len(prefix)+len(suffix) {
				return nil, fmt.Errorf("HTTP 404")
			}
			branch := url[len(prefix) : len(url)-len(suffix)]
			if validBranches[branch] {
				return []byte("fake-zip-data"), nil
			}
			return nil, fmt.Errorf("HTTP 404")
		}
	}

	tests := []struct {
		name          string
		refAndPath    []string
		validBranches map[string]bool
		wantSubDir    string
		wantErr       bool
	}{
		{
			name:          "simple branch - no subdir",
			refAndPath:    []string{"main"},
			validBranches: map[string]bool{"main": true},
			wantSubDir:    "",
		},
		{
			name:          "simple branch with subdir",
			refAndPath:    []string{"main", "src", "skills"},
			validBranches: map[string]bool{"main": true},
			wantSubDir:    "src/skills",
		},
		{
			name:          "slashed branch - no subdir",
			refAndPath:    []string{"feature", "xiaofei", "geak-online"},
			validBranches: map[string]bool{"feature/xiaofei/geak-online": true},
			wantSubDir:    "",
		},
		{
			name:          "slashed branch with subdir",
			refAndPath:    []string{"feature", "xiaofei", "geak-online", "server", "geak-optimize"},
			validBranches: map[string]bool{"feature/xiaofei/geak-online": true},
			wantSubDir:    "server/geak-optimize",
		},
		{
			name:          "longest branch wins over shorter",
			refAndPath:    []string{"feature", "sub", "dir"},
			validBranches: map[string]bool{"feature": true, "feature/sub": true, "feature/sub/dir": true},
			wantSubDir:    "", // longest match: "feature/sub/dir" is tried first
		},
		{
			name:          "fallback to default branch when no segments",
			refAndPath:    nil,
			validBranches: map[string]bool{"main": true},
			wantSubDir:    "",
		},
		{
			name:          "fallback to master when main missing",
			refAndPath:    nil,
			validBranches: map[string]bool{"master": true},
			wantSubDir:    "",
		},
		{
			name:          "fallback to main when branch not found",
			refAndPath:    []string{"nonexistent"},
			validBranches: map[string]bool{"main": true},
			wantSubDir:    "",
		},
		{
			name:          "all branches fail",
			refAndPath:    []string{"nonexistent"},
			validBranches: map[string]bool{},
			wantErr:       true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dl := mockDownloader(tt.validBranches)
			data, subDir, err := resolveAndDownload(context.Background(), "owner", "repo", tt.refAndPath, dl)
			if (err != nil) != tt.wantErr {
				t.Fatalf("resolveAndDownload() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if data == nil {
				t.Fatal("expected non-nil data")
			}
			if subDir != tt.wantSubDir {
				t.Errorf("subDir = %q, want %q", subDir, tt.wantSubDir)
			}
		})
	}
}
