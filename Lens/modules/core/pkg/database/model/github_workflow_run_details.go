package model

import (
	"time"
)

const TableNameGithubWorkflowRunDetails = "github_workflow_run_details"

// GithubWorkflowRunDetails represents workflow run details from GitHub API
type GithubWorkflowRunDetails struct {
	ID                     int64     `gorm:"column:id;primaryKey;autoIncrement:true" json:"id"`
	RunID                  int64     `gorm:"column:run_id;not null" json:"run_id"`
	GithubRunID            int64     `gorm:"column:github_run_id;not null" json:"github_run_id"`
	GithubRunNumber        int       `gorm:"column:github_run_number" json:"github_run_number"`
	GithubRunAttempt       int       `gorm:"column:github_run_attempt;default:1" json:"github_run_attempt"`
	WorkflowID             int64     `gorm:"column:workflow_id" json:"workflow_id"`
	WorkflowName           string    `gorm:"column:workflow_name" json:"workflow_name"`
	WorkflowPath           string    `gorm:"column:workflow_path" json:"workflow_path"`
	Status                 string    `gorm:"column:status" json:"status"`
	Conclusion             string    `gorm:"column:conclusion" json:"conclusion"`
	HTMLURL                string    `gorm:"column:html_url" json:"html_url"`
	JobsURL                string    `gorm:"column:jobs_url" json:"jobs_url"`
	LogsURL                string    `gorm:"column:logs_url" json:"logs_url"`
	ArtifactsURL           string    `gorm:"column:artifacts_url" json:"artifacts_url"`
	CreatedAtGithub        time.Time `gorm:"column:created_at_github" json:"created_at_github"`
	UpdatedAtGithub        time.Time `gorm:"column:updated_at_github" json:"updated_at_github"`
	RunStartedAt           time.Time `gorm:"column:run_started_at" json:"run_started_at"`
	RunCompletedAt         time.Time `gorm:"column:run_completed_at" json:"run_completed_at"`
	DurationSeconds        int       `gorm:"column:duration_seconds" json:"duration_seconds"`
	Event                  string    `gorm:"column:event" json:"event"`
	TriggerActor           string    `gorm:"column:trigger_actor" json:"trigger_actor"`
	TriggerActorID         int64     `gorm:"column:trigger_actor_id" json:"trigger_actor_id"`
	HeadSHA                string    `gorm:"column:head_sha" json:"head_sha"`
	HeadBranch             string    `gorm:"column:head_branch" json:"head_branch"`
	HeadRepositoryFullName string    `gorm:"column:head_repository_full_name" json:"head_repository_full_name"`
	BaseSHA                string    `gorm:"column:base_sha" json:"base_sha"`
	BaseBranch             string    `gorm:"column:base_branch" json:"base_branch"`
	PullRequestNumber      int       `gorm:"column:pull_request_number" json:"pull_request_number"`
	PullRequestTitle       string    `gorm:"column:pull_request_title" json:"pull_request_title"`
	PullRequestURL         string    `gorm:"column:pull_request_url" json:"pull_request_url"`
	Jobs                   ExtJSON   `gorm:"column:jobs" json:"jobs"`
	CreatedAt              time.Time `gorm:"column:created_at;not null;default:now()" json:"created_at"`
}

func (*GithubWorkflowRunDetails) TableName() string {
	return TableNameGithubWorkflowRunDetails
}

// WorkflowRunStatus constants
const (
	WorkflowRunStatusQueued     = "queued"
	WorkflowRunStatusInProgress = "in_progress"
	WorkflowRunStatusCompleted  = "completed"
)

// WorkflowRunConclusion constants
const (
	WorkflowRunConclusionSuccess        = "success"
	WorkflowRunConclusionFailure        = "failure"
	WorkflowRunConclusionCancelled      = "cancelled"
	WorkflowRunConclusionSkipped        = "skipped"
	WorkflowRunConclusionTimedOut       = "timed_out"
	WorkflowRunConclusionActionRequired = "action_required"
)

// WorkflowRunEvent constants
const (
	WorkflowRunEventPush             = "push"
	WorkflowRunEventPullRequest      = "pull_request"
	WorkflowRunEventSchedule         = "schedule"
	WorkflowRunEventWorkflowDispatch = "workflow_dispatch"
)
