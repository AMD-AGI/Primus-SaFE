package github

import (
	"time"
)

// CommitInfo represents commit details from GitHub API
type CommitInfo struct {
	SHA       string       `json:"sha"`
	Message   string       `json:"message"`
	HTMLURL   string       `json:"html_url"`
	Author    *GitUser     `json:"author"`
	Committer *GitUser     `json:"committer"`
	Stats     *CommitStats `json:"stats,omitempty"`
	Files     []FileChange `json:"files,omitempty"`
	Parents   []string     `json:"parents"`
}

// GitUser represents a git user (author or committer)
type GitUser struct {
	Name  string    `json:"name"`
	Email string    `json:"email"`
	Date  time.Time `json:"date"`
}

// CommitStats represents commit statistics
type CommitStats struct {
	Additions int `json:"additions"`
	Deletions int `json:"deletions"`
	Total     int `json:"total"`
}

// FileChange represents a file changed in a commit
type FileChange struct {
	Filename         string `json:"filename"`
	Status           string `json:"status"` // added, removed, modified, renamed, copied, changed
	Additions        int    `json:"additions"`
	Deletions        int    `json:"deletions"`
	Changes          int    `json:"changes"`
	Patch            string `json:"patch,omitempty"`
	PreviousFilename string `json:"previous_filename,omitempty"`
}

// WorkflowRunInfo represents workflow run details from GitHub API
type WorkflowRunInfo struct {
	ID              int64            `json:"id"`
	RunNumber       int              `json:"run_number"`
	RunAttempt      int              `json:"run_attempt"`
	WorkflowID      int64            `json:"workflow_id"`
	WorkflowName    string           `json:"workflow_name"`
	WorkflowPath    string           `json:"workflow_path"`
	Status          string           `json:"status"`     // queued, in_progress, completed
	Conclusion      string           `json:"conclusion"` // success, failure, cancelled, skipped, timed_out, action_required
	HTMLURL         string           `json:"html_url"`
	JobsURL         string           `json:"jobs_url"`
	LogsURL         string           `json:"logs_url"`
	ArtifactsURL    string           `json:"artifacts_url"`
	CreatedAt       time.Time        `json:"created_at"`
	UpdatedAt       time.Time        `json:"updated_at"`
	RunStartedAt    *time.Time       `json:"run_started_at"`
	RunCompletedAt  *time.Time       `json:"run_completed_at,omitempty"`
	DurationSeconds int              `json:"duration_seconds,omitempty"`
	Event           string           `json:"event"` // push, pull_request, schedule, workflow_dispatch
	TriggerActor    *Actor           `json:"trigger_actor,omitempty"`
	HeadSHA         string           `json:"head_sha"`
	HeadBranch      string           `json:"head_branch"`
	HeadRepository  string           `json:"head_repository,omitempty"`
	BaseSHA         string           `json:"base_sha,omitempty"`
	BaseBranch      string           `json:"base_branch,omitempty"`
	PullRequest     *PullRequestInfo `json:"pull_request,omitempty"`
	Jobs            []JobInfo        `json:"jobs,omitempty"`
}

// Actor represents a GitHub user
type Actor struct {
	Login string `json:"login"`
	ID    int64  `json:"id"`
}

// PullRequestInfo represents pull request info for workflow runs
type PullRequestInfo struct {
	Number int    `json:"number"`
	Title  string `json:"title"`
	URL    string `json:"url"`
}

// JobInfo represents a job within a workflow run
type JobInfo struct {
	ID          int64      `json:"id"`
	Name        string     `json:"name"`
	Status      string     `json:"status"`     // queued, in_progress, completed
	Conclusion  string     `json:"conclusion"` // success, failure, cancelled, skipped
	StartedAt   *time.Time `json:"started_at"`
	CompletedAt *time.Time `json:"completed_at"`
	RunnerID    int64      `json:"runner_id,omitempty"`
	RunnerName  string     `json:"runner_name,omitempty"`
	Steps       []StepInfo `json:"steps,omitempty"`
}

// StepInfo represents a step within a job
type StepInfo struct {
	Name        string     `json:"name"`
	Status      string     `json:"status"`
	Conclusion  string     `json:"conclusion"`
	Number      int        `json:"number"`
	StartedAt   *time.Time `json:"started_at"`
	CompletedAt *time.Time `json:"completed_at"`
}

// CommitComparison represents a comparison between two commits
type CommitComparison struct {
	BaseCommit   string       `json:"base_commit"`
	HeadCommit   string       `json:"head_commit"`
	Status       string       `json:"status"` // ahead, behind, diverged, identical
	AheadBy      int          `json:"ahead_by"`
	BehindBy     int          `json:"behind_by"`
	TotalCommits int          `json:"total_commits"`
	Commits      []CommitInfo `json:"commits,omitempty"`
	Files        []FileChange `json:"files,omitempty"`
}

// RunnerSetInfo represents discovered AutoScalingRunnerSet information
type RunnerSetInfo struct {
	UID                string `json:"uid"`
	Name               string `json:"name"`
	Namespace          string `json:"namespace"`
	GitHubConfigURL    string `json:"github_config_url"`
	GitHubConfigSecret string `json:"github_config_secret"`
	RunnerGroup        string `json:"runner_group,omitempty"`
	GitHubOwner        string `json:"github_owner,omitempty"`
	GitHubRepo         string `json:"github_repo,omitempty"`
	MinRunners         int    `json:"min_runners"`
	MaxRunners         int    `json:"max_runners"`
	CurrentRunners     int    `json:"current_runners"`
	DesiredRunners     int    `json:"desired_runners"`
}
