// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package github

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/go-resty/resty/v2"
)

const (
	defaultBaseURL = "https://api.github.com"
	defaultTimeout = 30 * time.Second
)

// Client is a GitHub API client
type Client struct {
	baseURL    string
	token      string
	httpClient *resty.Client
}

// NewClient creates a new GitHub API client
func NewClient(token string) *Client {
	return NewClientWithBaseURL(token, defaultBaseURL)
}

// NewClientWithBaseURL creates a new GitHub API client with a custom base URL
func NewClientWithBaseURL(token, baseURL string) *Client {
	client := resty.New().
		SetTimeout(defaultTimeout).
		SetHeader("Accept", "application/vnd.github.v3+json").
		SetHeader("X-GitHub-Api-Version", "2022-11-28")

	if token != "" {
		client.SetHeader("Authorization", "Bearer "+token)
	}

	return &Client{
		baseURL:    strings.TrimSuffix(baseURL, "/"),
		token:      token,
		httpClient: client,
	}
}

// GetCommit fetches commit details
func (c *Client) GetCommit(ctx context.Context, owner, repo, sha string) (*CommitInfo, error) {
	url := fmt.Sprintf("%s/repos/%s/%s/commits/%s", c.baseURL, owner, repo, sha)

	resp, err := c.httpClient.R().
		SetContext(ctx).
		Get(url)

	if err != nil {
		return nil, fmt.Errorf("failed to get commit: %w", err)
	}

	if resp.StatusCode() != http.StatusOK {
		return nil, fmt.Errorf("GitHub API error: %s", resp.Status())
	}

	var ghCommit struct {
		SHA     string `json:"sha"`
		HTMLURL string `json:"html_url"`
		Commit  struct {
			Message string `json:"message"`
			Author  struct {
				Name  string    `json:"name"`
				Email string    `json:"email"`
				Date  time.Time `json:"date"`
			} `json:"author"`
			Committer struct {
				Name  string    `json:"name"`
				Email string    `json:"email"`
				Date  time.Time `json:"date"`
			} `json:"committer"`
		} `json:"commit"`
		Stats struct {
			Additions int `json:"additions"`
			Deletions int `json:"deletions"`
			Total     int `json:"total"`
		} `json:"stats"`
		Files []struct {
			Filename         string `json:"filename"`
			Status           string `json:"status"`
			Additions        int    `json:"additions"`
			Deletions        int    `json:"deletions"`
			Changes          int    `json:"changes"`
			Patch            string `json:"patch"`
			PreviousFilename string `json:"previous_filename"`
		} `json:"files"`
		Parents []struct {
			SHA string `json:"sha"`
		} `json:"parents"`
	}

	if err := json.Unmarshal(resp.Body(), &ghCommit); err != nil {
		return nil, fmt.Errorf("failed to parse commit response: %w", err)
	}

	// Convert to our type
	commit := &CommitInfo{
		SHA:     ghCommit.SHA,
		Message: ghCommit.Commit.Message,
		HTMLURL: ghCommit.HTMLURL,
		Author: &GitUser{
			Name:  ghCommit.Commit.Author.Name,
			Email: ghCommit.Commit.Author.Email,
			Date:  ghCommit.Commit.Author.Date,
		},
		Committer: &GitUser{
			Name:  ghCommit.Commit.Committer.Name,
			Email: ghCommit.Commit.Committer.Email,
			Date:  ghCommit.Commit.Committer.Date,
		},
		Stats: &CommitStats{
			Additions: ghCommit.Stats.Additions,
			Deletions: ghCommit.Stats.Deletions,
			Total:     ghCommit.Stats.Total,
		},
		Parents: make([]string, len(ghCommit.Parents)),
		Files:   make([]FileChange, len(ghCommit.Files)),
	}

	for i, p := range ghCommit.Parents {
		commit.Parents[i] = p.SHA
	}

	for i, f := range ghCommit.Files {
		commit.Files[i] = FileChange{
			Filename:         f.Filename,
			Status:           f.Status,
			Additions:        f.Additions,
			Deletions:        f.Deletions,
			Changes:          f.Changes,
			Patch:            f.Patch,
			PreviousFilename: f.PreviousFilename,
		}
	}

	return commit, nil
}

// GetWorkflowRun fetches workflow run details
func (c *Client) GetWorkflowRun(ctx context.Context, owner, repo string, runID int64) (*WorkflowRunInfo, error) {
	url := fmt.Sprintf("%s/repos/%s/%s/actions/runs/%d", c.baseURL, owner, repo, runID)

	resp, err := c.httpClient.R().
		SetContext(ctx).
		Get(url)

	if err != nil {
		return nil, fmt.Errorf("failed to get workflow run: %w", err)
	}

	if resp.StatusCode() != http.StatusOK {
		return nil, fmt.Errorf("GitHub API error: %s", resp.Status())
	}

	var ghRun struct {
		ID             int64  `json:"id"`
		RunNumber      int    `json:"run_number"`
		RunAttempt     int    `json:"run_attempt"`
		WorkflowID     int64  `json:"workflow_id"`
		Name           string `json:"name"`
		Path           string `json:"path"`
		DisplayTitle   string `json:"display_title"`
		Status         string `json:"status"`
		Conclusion     string `json:"conclusion"`
		HTMLURL        string `json:"html_url"`
		JobsURL        string `json:"jobs_url"`
		LogsURL        string `json:"logs_url"`
		ArtifactsURL   string `json:"artifacts_url"`
		CreatedAt      time.Time `json:"created_at"`
		UpdatedAt      time.Time `json:"updated_at"`
		RunStartedAt   *time.Time `json:"run_started_at"`
		Event          string `json:"event"`
		HeadSHA        string `json:"head_sha"`
		HeadBranch     string `json:"head_branch"`
		HeadRepository struct {
			FullName string `json:"full_name"`
		} `json:"head_repository"`
		Actor struct {
			Login string `json:"login"`
			ID    int64  `json:"id"`
		} `json:"actor"`
		TriggeringActor struct {
			Login string `json:"login"`
			ID    int64  `json:"id"`
		} `json:"triggering_actor"`
		PullRequests []struct {
			Number int    `json:"number"`
			URL    string `json:"url"`
			Head   struct {
				SHA string `json:"sha"`
				Ref string `json:"ref"`
			} `json:"head"`
			Base struct {
				SHA string `json:"sha"`
				Ref string `json:"ref"`
			} `json:"base"`
		} `json:"pull_requests"`
	}

	if err := json.Unmarshal(resp.Body(), &ghRun); err != nil {
		return nil, fmt.Errorf("failed to parse workflow run response: %w", err)
	}

	run := &WorkflowRunInfo{
		ID:             ghRun.ID,
		RunNumber:      ghRun.RunNumber,
		RunAttempt:     ghRun.RunAttempt,
		WorkflowID:     ghRun.WorkflowID,
		WorkflowName:   ghRun.Name,
		WorkflowPath:   ghRun.Path,
		DisplayTitle:   ghRun.DisplayTitle,
		Status:         ghRun.Status,
		Conclusion:     ghRun.Conclusion,
		HTMLURL:        ghRun.HTMLURL,
		JobsURL:        ghRun.JobsURL,
		LogsURL:        ghRun.LogsURL,
		ArtifactsURL:   ghRun.ArtifactsURL,
		CreatedAt:      ghRun.CreatedAt,
		UpdatedAt:      ghRun.UpdatedAt,
		RunStartedAt:   ghRun.RunStartedAt,
		Event:          ghRun.Event,
		HeadSHA:        ghRun.HeadSHA,
		HeadBranch:     ghRun.HeadBranch,
		HeadRepository: ghRun.HeadRepository.FullName,
		Actor: &Actor{
			Login: ghRun.Actor.Login,
			ID:    ghRun.Actor.ID,
		},
		TriggerActor: &Actor{
			Login: ghRun.TriggeringActor.Login,
			ID:    ghRun.TriggeringActor.ID,
		},
	}

	// Handle pull request info
	if len(ghRun.PullRequests) > 0 {
		pr := ghRun.PullRequests[0]
		run.PullRequest = &PullRequestInfo{
			Number: pr.Number,
			URL:    pr.URL,
		}
		run.BaseSHA = pr.Base.SHA
		run.BaseBranch = pr.Base.Ref
	}

	// Calculate duration if completed
	if run.Status == "completed" && run.RunStartedAt != nil {
		run.RunCompletedAt = &ghRun.UpdatedAt
		run.DurationSeconds = int(ghRun.UpdatedAt.Sub(*run.RunStartedAt).Seconds())
	}

	return run, nil
}

// GetWorkflowRunJobs fetches jobs for a workflow run
func (c *Client) GetWorkflowRunJobs(ctx context.Context, owner, repo string, runID int64) ([]JobInfo, error) {
	url := fmt.Sprintf("%s/repos/%s/%s/actions/runs/%d/jobs", c.baseURL, owner, repo, runID)

	resp, err := c.httpClient.R().
		SetContext(ctx).
		SetQueryParam("per_page", "100").
		Get(url)

	if err != nil {
		return nil, fmt.Errorf("failed to get workflow jobs: %w", err)
	}

	if resp.StatusCode() != http.StatusOK {
		return nil, fmt.Errorf("GitHub API error: %s", resp.Status())
	}

	var ghJobs struct {
		TotalCount int `json:"total_count"`
		Jobs       []struct {
			ID          int64      `json:"id"`
			Name        string     `json:"name"`
			Status      string     `json:"status"`
			Conclusion  string     `json:"conclusion"`
			StartedAt   *time.Time `json:"started_at"`
			CompletedAt *time.Time `json:"completed_at"`
			RunnerID    int64      `json:"runner_id"`
			RunnerName  string     `json:"runner_name"`
			HTMLURL     string     `json:"html_url"`
			Steps       []struct {
				Name        string     `json:"name"`
				Status      string     `json:"status"`
				Conclusion  string     `json:"conclusion"`
				Number      int        `json:"number"`
				StartedAt   *time.Time `json:"started_at"`
				CompletedAt *time.Time `json:"completed_at"`
			} `json:"steps"`
		} `json:"jobs"`
	}

	if err := json.Unmarshal(resp.Body(), &ghJobs); err != nil {
		return nil, fmt.Errorf("failed to parse jobs response: %w", err)
	}

	jobs := make([]JobInfo, len(ghJobs.Jobs))
	for i, j := range ghJobs.Jobs {
		jobs[i] = JobInfo{
			ID:          j.ID,
			Name:        j.Name,
			Status:      j.Status,
			Conclusion:  j.Conclusion,
			StartedAt:   j.StartedAt,
			CompletedAt: j.CompletedAt,
			RunnerID:    j.RunnerID,
			RunnerName:  j.RunnerName,
			HTMLURL:     j.HTMLURL,
			Steps:       make([]StepInfo, len(j.Steps)),
		}
		for k, s := range j.Steps {
			jobs[i].Steps[k] = StepInfo{
				Name:        s.Name,
				Status:      s.Status,
				Conclusion:  s.Conclusion,
				Number:      s.Number,
				StartedAt:   s.StartedAt,
				CompletedAt: s.CompletedAt,
			}
		}
	}

	return jobs, nil
}

// GetWorkflowRunWithJobs fetches workflow run details including jobs
func (c *Client) GetWorkflowRunWithJobs(ctx context.Context, owner, repo string, runID int64) (*WorkflowRunInfo, error) {
	run, err := c.GetWorkflowRun(ctx, owner, repo, runID)
	if err != nil {
		return nil, err
	}

	jobs, err := c.GetWorkflowRunJobs(ctx, owner, repo, runID)
	if err != nil {
		// Log but don't fail - jobs are optional
		return run, nil
	}

	run.Jobs = jobs
	return run, nil
}

// CompareCommits compares two commits
func (c *Client) CompareCommits(ctx context.Context, owner, repo, base, head string) (*CommitComparison, error) {
	url := fmt.Sprintf("%s/repos/%s/%s/compare/%s...%s", c.baseURL, owner, repo, base, head)

	resp, err := c.httpClient.R().
		SetContext(ctx).
		Get(url)

	if err != nil {
		return nil, fmt.Errorf("failed to compare commits: %w", err)
	}

	if resp.StatusCode() != http.StatusOK {
		return nil, fmt.Errorf("GitHub API error: %s", resp.Status())
	}

	var ghCompare struct {
		Status       string `json:"status"`
		AheadBy      int    `json:"ahead_by"`
		BehindBy     int    `json:"behind_by"`
		TotalCommits int    `json:"total_commits"`
		BaseCommit   struct {
			SHA string `json:"sha"`
		} `json:"base_commit"`
		Commits []struct {
			SHA    string `json:"sha"`
			Commit struct {
				Message string `json:"message"`
				Author  struct {
					Name  string    `json:"name"`
					Email string    `json:"email"`
					Date  time.Time `json:"date"`
				} `json:"author"`
			} `json:"commit"`
		} `json:"commits"`
		Files []struct {
			Filename  string `json:"filename"`
			Status    string `json:"status"`
			Additions int    `json:"additions"`
			Deletions int    `json:"deletions"`
			Changes   int    `json:"changes"`
		} `json:"files"`
	}

	if err := json.Unmarshal(resp.Body(), &ghCompare); err != nil {
		return nil, fmt.Errorf("failed to parse compare response: %w", err)
	}

	comparison := &CommitComparison{
		BaseCommit:   ghCompare.BaseCommit.SHA,
		HeadCommit:   head,
		Status:       ghCompare.Status,
		AheadBy:      ghCompare.AheadBy,
		BehindBy:     ghCompare.BehindBy,
		TotalCommits: ghCompare.TotalCommits,
		Commits:      make([]CommitInfo, len(ghCompare.Commits)),
		Files:        make([]FileChange, len(ghCompare.Files)),
	}

	for i, c := range ghCompare.Commits {
		comparison.Commits[i] = CommitInfo{
			SHA:     c.SHA,
			Message: c.Commit.Message,
			Author: &GitUser{
				Name:  c.Commit.Author.Name,
				Email: c.Commit.Author.Email,
				Date:  c.Commit.Author.Date,
			},
		}
	}

	for i, f := range ghCompare.Files {
		comparison.Files[i] = FileChange{
			Filename:  f.Filename,
			Status:    f.Status,
			Additions: f.Additions,
			Deletions: f.Deletions,
			Changes:   f.Changes,
		}
	}

	return comparison, nil
}

// ValidateToken validates the GitHub token
func (c *Client) ValidateToken(ctx context.Context) (bool, error) {
	url := fmt.Sprintf("%s/user", c.baseURL)

	resp, err := c.httpClient.R().
		SetContext(ctx).
		Get(url)

	if err != nil {
		return false, fmt.Errorf("failed to validate token: %w", err)
	}

	return resp.StatusCode() == http.StatusOK, nil
}

// GetJobLogs fetches logs for a specific job
// GitHub API returns a redirect to a time-limited URL for downloading logs
func (c *Client) GetJobLogs(ctx context.Context, owner, repo string, jobID int64) (string, error) {
	url := fmt.Sprintf("%s/repos/%s/%s/actions/jobs/%d/logs", c.baseURL, owner, repo, jobID)

	// Create a client that doesn't follow redirects
	noRedirectClient := resty.New().
		SetTimeout(defaultTimeout).
		SetHeader("Accept", "application/vnd.github.v3+json").
		SetHeader("X-GitHub-Api-Version", "2022-11-28").
		SetRedirectPolicy(resty.NoRedirectPolicy())

	if c.token != "" {
		noRedirectClient.SetHeader("Authorization", "Bearer "+c.token)
	}

	resp, err := noRedirectClient.R().
		SetContext(ctx).
		Get(url)

	// Redirect is expected - GitHub returns 302 with Location header
	if err != nil {
		// Check if it's a redirect (expected behavior)
		if resp != nil && resp.StatusCode() == http.StatusFound {
			location := resp.Header().Get("Location")
			if location != "" {
				// Fetch the actual logs from the redirect URL
				return c.fetchLogsFromURL(ctx, location)
			}
		}
		return "", fmt.Errorf("failed to get job logs: %w", err)
	}

	// If we got a redirect, follow it
	if resp.StatusCode() == http.StatusFound {
		location := resp.Header().Get("Location")
		if location != "" {
			return c.fetchLogsFromURL(ctx, location)
		}
	}

	if resp.StatusCode() != http.StatusOK {
		return "", fmt.Errorf("GitHub API error: %s", resp.Status())
	}

	return string(resp.Body()), nil
}

// fetchLogsFromURL fetches logs from a pre-signed URL
func (c *Client) fetchLogsFromURL(ctx context.Context, url string) (string, error) {
	// Create a new client without auth headers for the pre-signed URL
	client := resty.New().SetTimeout(60 * time.Second)

	resp, err := client.R().
		SetContext(ctx).
		Get(url)

	if err != nil {
		return "", fmt.Errorf("failed to fetch logs from URL: %w", err)
	}

	if resp.StatusCode() != http.StatusOK {
		return "", fmt.Errorf("failed to fetch logs: %s", resp.Status())
	}

	return string(resp.Body()), nil
}

// GetWorkflowRunLogs fetches logs for an entire workflow run (zip file URL)
func (c *Client) GetWorkflowRunLogs(ctx context.Context, owner, repo string, runID int64) (string, error) {
	url := fmt.Sprintf("%s/repos/%s/%s/actions/runs/%d/logs", c.baseURL, owner, repo, runID)

	// Create a client that doesn't follow redirects
	noRedirectClient := resty.New().
		SetTimeout(defaultTimeout).
		SetHeader("Accept", "application/vnd.github.v3+json").
		SetHeader("X-GitHub-Api-Version", "2022-11-28").
		SetRedirectPolicy(resty.NoRedirectPolicy())

	if c.token != "" {
		noRedirectClient.SetHeader("Authorization", "Bearer "+c.token)
	}

	resp, err := noRedirectClient.R().
		SetContext(ctx).
		Get(url)

	if err != nil {
		// Check if it's a redirect (expected behavior)
		if resp != nil && resp.StatusCode() == http.StatusFound {
			location := resp.Header().Get("Location")
			if location != "" {
				return location, nil // Return the download URL
			}
		}
		return "", fmt.Errorf("failed to get workflow run logs: %w", err)
	}

	if resp.StatusCode() == http.StatusFound {
		return resp.Header().Get("Location"), nil
	}

	return "", fmt.Errorf("GitHub API error: %s", resp.Status())
}

// GetWorkflowFileContent retrieves the workflow YAML file content from GitHub
func (c *Client) GetWorkflowFileContent(ctx context.Context, owner, repo, path, ref string) (string, error) {
	url := fmt.Sprintf("%s/repos/%s/%s/contents/%s", c.baseURL, owner, repo, path)

	req := c.httpClient.R().
		SetContext(ctx).
		SetHeader("Accept", "application/vnd.github.raw+json")

	if ref != "" {
		req.SetQueryParam("ref", ref)
	}

	resp, err := req.Get(url)
	if err != nil {
		return "", fmt.Errorf("failed to get workflow file: %w", err)
	}

	if resp.StatusCode() != http.StatusOK {
		return "", fmt.Errorf("GitHub API error: %s", resp.Status())
	}

	return string(resp.Body()), nil
}

// WorkflowDefinition represents parsed workflow YAML structure
type WorkflowDefinition struct {
	Name string                        `yaml:"name"`
	Jobs map[string]WorkflowJobDef     `yaml:"jobs"`
}

// WorkflowJobDef represents a job definition in workflow YAML
type WorkflowJobDef struct {
	Name   string      `yaml:"name"`
	Needs  interface{} `yaml:"needs"` // Can be string or []string
	RunsOn interface{} `yaml:"runs-on"`
}

