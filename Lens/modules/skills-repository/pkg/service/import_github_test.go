// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package service

import (
	"archive/zip"
	"bytes"
	"context"
	"fmt"
	"strings"
	"testing"
)

// makeTestZip creates a minimal ZIP archive with the expected GitHub root
// directory naming convention: "{repo}-{branch_sanitized}-{sha}/".
func makeTestZip(repo, branch string) []byte {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	rootDir := repo + "-" + strings.ReplaceAll(branch, "/", "-") + "-abc1234"
	w, _ := zw.Create(rootDir + "/dummy.txt")
	w.Write([]byte("test"))
	zw.Close()
	return buf.Bytes()
}

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
			name:        "www.github.com",
			url:         "https://www.github.com/owner/repo/tree/dev",
			wantOwner:   "owner",
			wantRepo:    "repo",
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

func TestVerifyZipBranch(t *testing.T) {
	tests := []struct {
		name            string
		repo            string
		candidateBranch string
		actualBranch    string // branch used to generate ZIP root
		want            bool
	}{
		{
			name:            "exact match",
			repo:            "GEAK",
			candidateBranch: "feature/xiaofei/geak-online",
			actualBranch:    "feature/xiaofei/geak-online",
			want:            true,
		},
		{
			name:            "prefix match detected - candidate too long",
			repo:            "GEAK",
			candidateBranch: "feature/xiaofei/geak-online/server",
			actualBranch:    "feature/xiaofei/geak-online",
			want:            false,
		},
		{
			name:            "simple branch match",
			repo:            "repo",
			candidateBranch: "main",
			actualBranch:    "main",
			want:            true,
		},
		{
			name:            "simple branch mismatch",
			repo:            "repo",
			candidateBranch: "develop",
			actualBranch:    "main",
			want:            false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			zipData := makeTestZip(tt.repo, tt.actualBranch)
			got := verifyZipBranch(zipData, tt.repo, tt.candidateBranch)
			if got != tt.want {
				t.Errorf("verifyZipBranch() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestResolveAndDownload(t *testing.T) {
	const repo = "repo"

	// mockDownloader simulates GitHub's archive endpoint behavior:
	// - Returns ZIP for exact branch matches
	// - Simulates GitHub's prefix matching: if candidate doesn't exist but a
	//   shorter prefix does, returns that shorter branch's archive (like GitHub does)
	mockDownloader := func(validBranches map[string]bool) downloadFunc {
		return func(ctx context.Context, url string) ([]byte, error) {
			const prefix = "https://github.com/owner/repo/archive/refs/heads/"
			const suffix = ".zip"
			if len(url) <= len(prefix)+len(suffix) {
				return nil, fmt.Errorf("HTTP 404")
			}
			candidate := url[len(prefix) : len(url)-len(suffix)]

			// Exact match
			if validBranches[candidate] {
				return makeTestZip(repo, candidate), nil
			}

			// Simulate GitHub prefix matching: try progressively shorter
			// prefixes at "/" boundaries (this is what GitHub actually does)
			parts := strings.Split(candidate, "/")
			for i := len(parts) - 1; i >= 1; i-- {
				shorter := strings.Join(parts[:i], "/")
				if validBranches[shorter] {
					// Return archive for the shorter branch (prefix match)
					return makeTestZip(repo, shorter), nil
				}
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
			name:          "slashed branch with subdir - defeats prefix matching",
			refAndPath:    []string{"feature", "xiaofei", "geak-online", "server", "geak-optimize"},
			validBranches: map[string]bool{"feature/xiaofei/geak-online": true},
			wantSubDir:    "server/geak-optimize",
		},
		{
			name:          "longest exact branch wins over shorter",
			refAndPath:    []string{"feature", "sub", "dir"},
			validBranches: map[string]bool{"feature": true, "feature/sub": true, "feature/sub/dir": true},
			wantSubDir:    "", // longest match: "feature/sub/dir" exists and is tried first
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
		{
			name:          "prefix match skipped - finds correct shorter branch",
			refAndPath:    []string{"feature", "branch", "sub", "path"},
			validBranches: map[string]bool{"feature/branch": true},
			wantSubDir:    "sub/path",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dl := mockDownloader(tt.validBranches)
			data, subDir, err := resolveAndDownload(context.Background(), "owner", repo, tt.refAndPath, dl)
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
