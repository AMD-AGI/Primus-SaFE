// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package service

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// parseGitHubURL extracts the owner, repo, and ref+path segments from a GitHub URL.
// For "https://github.com/owner/repo/tree/branch/sub/dir" it returns
// owner="owner", repo="repo", refAndPath=["branch","sub","dir"].
func parseGitHubURL(githubURL string) (owner, repo string, refAndPath []string, err error) {
	parsed, err := url.Parse(githubURL)
	if err != nil {
		return "", "", nil, fmt.Errorf("invalid URL: %w", err)
	}

	if parsed.Host != "github.com" && parsed.Host != "www.github.com" {
		return "", "", nil, fmt.Errorf("only github.com URLs are supported")
	}

	pathParts := strings.Split(strings.Trim(parsed.Path, "/"), "/")
	if len(pathParts) < 2 {
		return "", "", nil, fmt.Errorf("invalid GitHub repository URL")
	}

	owner = pathParts[0]
	repo = strings.TrimSuffix(pathParts[1], ".git")

	if len(pathParts) >= 4 && pathParts[2] == "tree" {
		refAndPath = pathParts[3:]
	}

	return owner, repo, refAndPath, nil
}

// downloadFunc is a function type for downloading a URL. Used for testing.
type downloadFunc func(ctx context.Context, url string) ([]byte, error)

// resolveAndDownload tries candidate branch names (longest first) against
// archiveBaseURL and returns the ZIP data along with the remaining subdirectory
// path. Branch names with slashes (e.g. "feature/foo/bar") are resolved by
// trying progressively shorter segments until an archive download succeeds.
func resolveAndDownload(ctx context.Context, owner, repo string, refAndPath []string, dl downloadFunc) ([]byte, string, error) {
	archiveBase := fmt.Sprintf("https://github.com/%s/%s/archive/refs/heads", owner, repo)

	var lastErr error

	// Try progressively shorter ref names (longest match first).
	// For segments [feature, foo, bar, some, dir] we try:
	//   1. branch = "feature/foo/bar/some/dir", subDir = ""
	//   2. branch = "feature/foo/bar/some",     subDir = "dir"
	//   3. branch = "feature/foo/bar",           subDir = "some/dir"
	//   ...
	if len(refAndPath) > 0 {
		for i := len(refAndPath); i >= 1; i-- {
			candidateBranch := strings.Join(refAndPath[:i], "/")
			downloadURL := fmt.Sprintf("%s/%s.zip", archiveBase, candidateBranch)
			data, err := dl(ctx, downloadURL)
			if err == nil {
				subDir := ""
				if i < len(refAndPath) {
					subDir = strings.Join(refAndPath[i:], "/")
				}
				return data, subDir, nil
			}
			lastErr = err
		}
	}

	// Fallback: try default branches
	for _, b := range []string{"main", "master"} {
		downloadURL := fmt.Sprintf("%s/%s.zip", archiveBase, b)
		data, err := dl(ctx, downloadURL)
		if err == nil {
			return data, "", nil
		}
		lastErr = err
	}

	if lastErr == nil {
		lastErr = fmt.Errorf("no valid branch found")
	}
	return nil, "", fmt.Errorf("failed to download from GitHub: %w", lastErr)
}

// downloadGitHubZip downloads a repository as ZIP from GitHub.
// It returns the ZIP data and the subdirectory path within the repo (if the URL
// pointed to a subdirectory rather than the repo root).
func downloadGitHubZip(ctx context.Context, githubURL string) ([]byte, string, error) {
	owner, repo, refAndPath, err := parseGitHubURL(githubURL)
	if err != nil {
		return nil, "", err
	}
	return resolveAndDownload(ctx, owner, repo, refAndPath, downloadWithLimit)
}

// downloadWithLimit downloads a URL with size limit (100MB max)
func downloadWithLimit(ctx context.Context, downloadURL string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, downloadURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "tools-importer/1.0")

	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	// Limit to 100MB
	maxSize := int64(100 * 1024 * 1024)
	limitedReader := io.LimitReader(resp.Body, maxSize+1)
	data, err := io.ReadAll(limitedReader)
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > maxSize {
		return nil, fmt.Errorf("file too large (max 100MB)")
	}

	return data, nil
}
