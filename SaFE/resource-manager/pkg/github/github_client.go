/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package github

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"k8s.io/klog/v2"
)

type GitHubClient struct {
	baseURL    string
	token      string
	httpClient *http.Client
}

func NewGitHubClient(token string) *GitHubClient {
	return &GitHubClient{
		baseURL:    "https://api.github.com",
		token:      token,
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
}

type GitHubRun struct {
	ID              int64  `json:"id"`
	Name            string `json:"name"`
	Status          string `json:"status"`
	Conclusion      string `json:"conclusion"`
	HTMLURL         string `json:"html_url"`
	JobsURL         string `json:"jobs_url"`
	LogsURL         string `json:"logs_url"`
	Event           string `json:"event"`
	HeadBranch      string `json:"head_branch"`
	HeadSHA         string `json:"head_sha"`
	Path            string `json:"path"`
	TriggeringActor struct {
		Login string `json:"login"`
	} `json:"triggering_actor"`
	PullRequests []struct {
		Number int `json:"number"`
	} `json:"pull_requests"`
}

type GitHubJob struct {
	ID            int64      `json:"id"`
	Name          string     `json:"name"`
	Status        string     `json:"status"`
	Conclusion    string     `json:"conclusion"`
	StartedAt     *time.Time `json:"started_at"`
	CompletedAt   *time.Time `json:"completed_at"`
	RunnerName    string     `json:"runner_name"`
	RunnerGroupName string  `json:"runner_group_name"`
	Steps         []GitHubStep `json:"steps"`
}

type GitHubStep struct {
	Number      int        `json:"number"`
	Name        string     `json:"name"`
	Status      string     `json:"status"`
	Conclusion  string     `json:"conclusion"`
	StartedAt   *time.Time `json:"started_at"`
	CompletedAt *time.Time `json:"completed_at"`
}

type GitHubCommit struct {
	SHA     string `json:"sha"`
	Commit  struct {
		Message string `json:"message"`
		Author  struct {
			Name  string     `json:"name"`
			Email string     `json:"email"`
			Date  *time.Time `json:"date"`
		} `json:"author"`
	} `json:"commit"`
	Stats struct {
		Additions int `json:"additions"`
		Deletions int `json:"deletions"`
		Total     int `json:"total"`
	} `json:"stats"`
	Files []struct{} `json:"files"`
}

func (c *GitHubClient) GetWorkflowRun(ctx context.Context, owner, repo string, runID int64) (*GitHubRun, error) {
	path := fmt.Sprintf("/repos/%s/%s/actions/runs/%d", owner, repo, runID)
	var run GitHubRun
	if err := c.doWithRetry(ctx, path, &run); err != nil {
		return nil, err
	}
	return &run, nil
}

func (c *GitHubClient) GetAllJobs(ctx context.Context, owner, repo string, runID int64) ([]GitHubJob, error) {
	var allJobs []GitHubJob
	page := 1
	for {
		path := fmt.Sprintf("/repos/%s/%s/actions/runs/%d/jobs?per_page=100&page=%d", owner, repo, runID, page)
		var result struct {
			TotalCount int         `json:"total_count"`
			Jobs       []GitHubJob `json:"jobs"`
		}
		if err := c.doWithRetry(ctx, path, &result); err != nil {
			return allJobs, err
		}
		allJobs = append(allJobs, result.Jobs...)
		if len(result.Jobs) < 100 {
			break
		}
		page++
		if page > 20 {
			klog.Warningf("[github-client] too many pages for run %d, stopping at page %d", runID, page)
			break
		}
	}
	return allJobs, nil
}

func (c *GitHubClient) GetCommit(ctx context.Context, owner, repo, sha string) (*GitHubCommit, error) {
	path := fmt.Sprintf("/repos/%s/%s/commits/%s", owner, repo, sha)
	var commit GitHubCommit
	if err := c.doWithRetry(ctx, path, &commit); err != nil {
		return nil, err
	}
	return &commit, nil
}

func (c *GitHubClient) doWithRetry(ctx context.Context, path string, result interface{}) error {
	url := c.baseURL + path
	for attempt := 0; attempt < 5; attempt++ {
		req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
		if err != nil {
			return err
		}
		req.Header.Set("Accept", "application/vnd.github+json")
		req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
		if c.token != "" {
			req.Header.Set("Authorization", "Bearer "+c.token)
		}

		resp, err := c.httpClient.Do(req)
		if err != nil {
			if attempt < 4 {
				time.Sleep(time.Duration(1<<attempt) * time.Second)
				continue
			}
			return fmt.Errorf("github api: %w", err)
		}

		if resp.StatusCode == 429 || resp.StatusCode == 403 {
			resp.Body.Close()
			waitTime := time.Duration(1<<attempt) * time.Second
			if retryAfter := resp.Header.Get("Retry-After"); retryAfter != "" {
				if d, err := strconv.Atoi(retryAfter); err == nil {
					waitTime = time.Duration(d) * time.Second
				}
			}
			if remaining := resp.Header.Get("X-RateLimit-Remaining"); remaining == "0" {
				if resetStr := resp.Header.Get("X-RateLimit-Reset"); resetStr != "" {
					if resetUnix, err := strconv.ParseInt(resetStr, 10, 64); err == nil {
						waitTime = time.Until(time.Unix(resetUnix, 0)) + time.Second
					}
				}
			}
			klog.V(1).Infof("[github-client] rate limited on %s, waiting %v (attempt %d)", path, waitTime, attempt+1)
			time.Sleep(waitTime)
			continue
		}

		if resp.StatusCode >= 500 {
			resp.Body.Close()
			if attempt < 4 {
				time.Sleep(time.Duration(1<<attempt) * time.Second)
				continue
			}
			return fmt.Errorf("github api: %d %s", resp.StatusCode, path)
		}

		defer resp.Body.Close()
		if resp.StatusCode == 404 {
			return fmt.Errorf("github api: not found %s", path)
		}
		if resp.StatusCode >= 400 {
			body, _ := io.ReadAll(resp.Body)
			return fmt.Errorf("github api: %d %s: %s", resp.StatusCode, path, strings.TrimSpace(string(body)))
		}

		return json.NewDecoder(resp.Body).Decode(result)
	}
	return fmt.Errorf("github api: max retries for %s", path)
}
