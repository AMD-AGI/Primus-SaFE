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

// downloadGitHubZip downloads a repository as ZIP from GitHub
func downloadGitHubZip(ctx context.Context, githubURL string) ([]byte, error) {
	parsed, err := url.Parse(githubURL)
	if err != nil {
		return nil, fmt.Errorf("invalid URL: %w", err)
	}

	if parsed.Host != "github.com" && parsed.Host != "www.github.com" {
		return nil, fmt.Errorf("only github.com URLs are supported")
	}

	// Parse path: /owner/repo or /owner/repo/tree/branch
	pathParts := strings.Split(strings.Trim(parsed.Path, "/"), "/")
	if len(pathParts) < 2 {
		return nil, fmt.Errorf("invalid GitHub repository URL")
	}

	owner := pathParts[0]
	repo := strings.TrimSuffix(pathParts[1], ".git")

	// Determine branch
	branch := ""
	if len(pathParts) >= 4 && pathParts[2] == "tree" {
		branch = pathParts[3]
	}

	// Try branches
	branches := []string{branch, "main", "master"}
	var lastErr error

	for _, b := range branches {
		if b == "" {
			continue
		}
		downloadURL := fmt.Sprintf("https://github.com/%s/%s/archive/refs/heads/%s.zip", owner, repo, b)
		data, err := downloadWithLimit(ctx, downloadURL)
		if err == nil {
			return data, nil
		}
		lastErr = err
	}

	return nil, fmt.Errorf("failed to download from GitHub: %w", lastErr)
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
