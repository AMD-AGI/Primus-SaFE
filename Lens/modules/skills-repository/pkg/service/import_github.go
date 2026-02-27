// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package service

import (
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

// downloadResult holds the result of an archive download.
type downloadResult struct {
	data     []byte
	finalURL string // URL after redirects (used to verify the actual resolved branch)
}

// downloadFunc is a function type for downloading a URL. Used for testing.
type downloadFunc func(ctx context.Context, url string) (*downloadResult, error)

// extractBranchFromRedirectURL extracts the actual branch name from the final
// redirect URL. GitHub redirects archive requests to codeload.github.com:
//
//	Request:  https://github.com/{owner}/{repo}/archive/refs/heads/{branch}.zip
//	Redirect: https://codeload.github.com/{owner}/{repo}/zip/refs/heads/{actual_branch}
//
// This is the most reliable way to determine which branch GitHub actually resolved,
// since the archive endpoint does prefix matching on refs.
func extractBranchFromRedirectURL(finalURL, owner, repo string) string {
	if finalURL == "" {
		return ""
	}

	// Look for the refs/heads/ marker in the final URL
	marker := "/refs/heads/"
	idx := strings.Index(finalURL, marker)
	if idx >= 0 {
		return finalURL[idx+len(marker):]
	}

	// Fallback: look for /zip/{branch} pattern (some GitHub versions)
	// e.g. https://codeload.github.com/{owner}/{repo}/zip/{branch}
	prefix := fmt.Sprintf("/%s/%s/zip/", owner, repo)
	idx = strings.Index(finalURL, prefix)
	if idx >= 0 {
		return finalURL[idx+len(prefix):]
	}

	return ""
}

// resolveAndDownload tries candidate branch names (longest first) against the
// GitHub archive endpoint and returns the ZIP data along with the remaining
// subdirectory path.
//
// Branch names with slashes (e.g. "feature/foo/bar") create URL ambiguity.
// Additionally, GitHub's archive endpoint does prefix matching on refs: requesting
// "refs/heads/A/B/C.zip" may return the archive for branch "A/B" if "A/B/C"
// doesn't exist. We detect this by inspecting the HTTP redirect URL, which
// contains the actual resolved branch name.
func resolveAndDownload(ctx context.Context, owner, repo string, refAndPath []string, dl downloadFunc) ([]byte, string, error) {
	archiveBase := fmt.Sprintf("https://github.com/%s/%s/archive/refs/heads", owner, repo)

	var lastErr error
	var cachedResult *downloadResult // reuse when GitHub prefix-matched to shorter branch

	if len(refAndPath) > 0 {
		for i := len(refAndPath); i >= 1; i-- {
			candidateBranch := strings.Join(refAndPath[:i], "/")
			downloadURL := fmt.Sprintf("%s/%s.zip", archiveBase, candidateBranch)
			log.Printf("[GitHub Import] trying branch=%q url=%s", candidateBranch, downloadURL)

			var result *downloadResult
			var err error
			if cachedResult != nil {
				result = cachedResult
				log.Printf("[GitHub Import] reusing cached archive for branch=%q", candidateBranch)
			} else {
				result, err = dl(ctx, downloadURL)
			}

			if err != nil {
				log.Printf("[GitHub Import] branch=%q failed: %v", candidateBranch, err)
				lastErr = err
				continue
			}

			// Verify via redirect URL: check which branch GitHub actually resolved
			actualBranch := extractBranchFromRedirectURL(result.finalURL, owner, repo)
			if actualBranch != "" && actualBranch != candidateBranch {
				log.Printf("[GitHub Import] branch=%q redirect resolved to %q (prefix match), skipping", candidateBranch, actualBranch)
				cachedResult = result
				lastErr = fmt.Errorf("branch %q resolved to %q via prefix match", candidateBranch, actualBranch)
				continue
			}

			subDir := ""
			if i < len(refAndPath) {
				subDir = strings.Join(refAndPath[i:], "/")
			}
			log.Printf("[GitHub Import] verified branch=%q subDir=%q zipSize=%d", candidateBranch, subDir, len(result.data))
			return result.data, subDir, nil
		}
	}

	// Fallback: try default branches
	for _, b := range []string{"main", "master"} {
		downloadURL := fmt.Sprintf("%s/%s.zip", archiveBase, b)
		log.Printf("[GitHub Import] fallback trying branch=%q url=%s", b, downloadURL)
		result, err := dl(ctx, downloadURL)
		if err == nil {
			log.Printf("[GitHub Import] fallback resolved branch=%q zipSize=%d", b, len(result.data))
			return result.data, "", nil
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
	return resolveAndDownload(ctx, owner, repo, refAndPath, downloadWithRedirect)
}

// downloadWithRedirect downloads a URL with size limit (100MB max) and captures
// the final URL after HTTP redirects. GitHub redirects archive requests to
// codeload.github.com, and the redirect URL contains the actual resolved branch.
func downloadWithRedirect(ctx context.Context, downloadURL string) (*downloadResult, error) {
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

	// Capture the final URL after redirects
	finalURL := ""
	if resp.Request != nil && resp.Request.URL != nil {
		finalURL = resp.Request.URL.String()
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

	log.Printf("[GitHub Import] downloaded %d bytes, finalURL=%s", len(data), finalURL)
	return &downloadResult{data: data, finalURL: finalURL}, nil
}
