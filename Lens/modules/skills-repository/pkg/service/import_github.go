// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package service

import (
	"archive/zip"
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
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

// verifyZipBranch checks whether the downloaded ZIP archive is actually for the
// given candidate branch. GitHub's archive endpoint does prefix matching on refs:
// requesting "refs/heads/feature/foo/bar.zip" may return the archive for branch
// "feature/foo" if "feature/foo/bar" doesn't exist.
//
// GitHub archive ZIPs have a single root directory named
// "{repo}-{branch_sanitized}-{short_sha}" where slashes in the branch name are
// replaced with hyphens. We verify by checking this root directory name.
func verifyZipBranch(zipData []byte, repo, candidateBranch string) bool {
	zr, err := zip.NewReader(bytes.NewReader(zipData), int64(len(zipData)))
	if err != nil || len(zr.File) == 0 {
		return false
	}

	// Extract root directory from the first ZIP entry
	firstEntry := zr.File[0].Name
	rootDir := strings.SplitN(firstEntry, "/", 2)[0]

	// Expected root prefix: "{repo}-{branch_with_slashes_to_hyphens}-"
	// The trailing hyphen separates the branch name from the commit SHA.
	expectedPrefix := repo + "-" + strings.ReplaceAll(candidateBranch, "/", "-") + "-"
	return strings.HasPrefix(rootDir, expectedPrefix)
}

// resolveAndDownload tries candidate branch names (longest first) against
// archiveBaseURL and returns the ZIP data along with the remaining subdirectory
// path. Branch names with slashes (e.g. "feature/foo/bar") are resolved by
// trying progressively shorter segments until an archive download succeeds AND
// the ZIP root directory confirms the branch name matches (to defeat GitHub's
// prefix matching on refs).
func resolveAndDownload(ctx context.Context, owner, repo string, refAndPath []string, dl downloadFunc) ([]byte, string, error) {
	archiveBase := fmt.Sprintf("https://github.com/%s/%s/archive/refs/heads", owner, repo)

	var lastErr error
	var cachedData []byte // reuse data when GitHub prefix-matched to a shorter branch

	// Try progressively shorter ref names (longest match first).
	// For segments [feature, foo, bar, some, dir] we try:
	//   1. branch = "feature/foo/bar/some/dir", subDir = ""
	//   2. branch = "feature/foo/bar/some",     subDir = "dir"
	//   3. branch = "feature/foo/bar",           subDir = "some/dir"
	//   ...
	// Each candidate is verified against the ZIP root directory to detect
	// GitHub's prefix matching (where a request for branch A/B/C may return
	// the archive for branch A/B if A/B/C doesn't exist).
	if len(refAndPath) > 0 {
		for i := len(refAndPath); i >= 1; i-- {
			candidateBranch := strings.Join(refAndPath[:i], "/")
			downloadURL := fmt.Sprintf("%s/%s.zip", archiveBase, candidateBranch)
			log.Printf("[GitHub Import] trying branch=%q url=%s", candidateBranch, downloadURL)

			var data []byte
			var err error
			if cachedData != nil {
				// Reuse previously downloaded data (GitHub prefix-matched, so
				// the archive is the same for shorter branch candidates).
				data = cachedData
				err = nil
				log.Printf("[GitHub Import] reusing cached archive for branch=%q", candidateBranch)
			} else {
				data, err = dl(ctx, downloadURL)
			}

			if err != nil {
				log.Printf("[GitHub Import] branch=%q failed: %v", candidateBranch, err)
				lastErr = err
				continue
			}

			// Verify the ZIP is actually for this branch (not a prefix match)
			if !verifyZipBranch(data, repo, candidateBranch) {
				log.Printf("[GitHub Import] branch=%q download OK but ZIP root mismatch (GitHub prefix-matched to shorter branch), skipping", candidateBranch)
				cachedData = data // cache for next shorter candidate
				lastErr = fmt.Errorf("branch %q resolved to different ref via prefix match", candidateBranch)
				continue
			}

			subDir := ""
			if i < len(refAndPath) {
				subDir = strings.Join(refAndPath[i:], "/")
			}
			log.Printf("[GitHub Import] verified branch=%q subDir=%q zipSize=%d", candidateBranch, subDir, len(data))
			return data, subDir, nil
		}
	}

	// Fallback: try default branches
	for _, b := range []string{"main", "master"} {
		downloadURL := fmt.Sprintf("%s/%s.zip", archiveBase, b)
		log.Printf("[GitHub Import] fallback trying branch=%q url=%s", b, downloadURL)
		data, err := dl(ctx, downloadURL)
		if err == nil {
			log.Printf("[GitHub Import] fallback resolved branch=%q zipSize=%d", b, len(data))
			return data, "", nil
		}
		log.Printf("[GitHub Import] fallback branch=%q failed: %v", b, err)
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
