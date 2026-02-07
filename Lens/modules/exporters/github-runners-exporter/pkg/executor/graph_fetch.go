// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package executor

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/clientsets"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/constant"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/github"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/task"
	"gopkg.in/yaml.v3"
)

const (
	// ExtKeyRunSummaryID is the key for run summary ID in task ext
	ExtKeyRunSummaryID = "run_summary_id"
	// ExtKeyGithubRunID is the key for GitHub run ID in task ext
	ExtKeyGithubRunID = "github_run_id"
	// ExtKeyOwner is the key for GitHub owner in task ext
	ExtKeyOwner = "owner"
	// ExtKeyRepo is the key for GitHub repo in task ext
	ExtKeyRepo = "repo"
	// ExtKeyRunnerSetNamespace is the key for runner set namespace in task ext
	ExtKeyRunnerSetNamespace = "runner_set_namespace"
	// ExtKeyRunnerSetName is the key for runner set name in task ext
	ExtKeyRunnerSetName = "runner_set_name"
)

// GraphFetchExecutor implements task.TaskExecutor for fetching workflow graph from GitHub
type GraphFetchExecutor struct {
	task.BaseExecutor
	clientSets    *clientsets.K8SClientSet
	clientManager *github.ClientManager
}

// NewGraphFetchExecutor creates a new GraphFetchExecutor
func NewGraphFetchExecutor(clientSets *clientsets.K8SClientSet) *GraphFetchExecutor {
	return &GraphFetchExecutor{
		clientSets:    clientSets,
		clientManager: github.GetGlobalManager(),
	}
}

// GetTaskType returns the task type this executor handles
func (e *GraphFetchExecutor) GetTaskType() string {
	return constant.TaskTypeGithubGraphFetch
}

// Validate validates task parameters
func (e *GraphFetchExecutor) Validate(taskState *model.WorkloadTaskState) error {
	summaryID := e.GetExtInt(taskState, ExtKeyRunSummaryID)
	githubRunID := e.GetExtInt(taskState, ExtKeyGithubRunID)
	if summaryID == 0 && githubRunID == 0 {
		return fmt.Errorf("missing required parameter: %s or %s", ExtKeyRunSummaryID, ExtKeyGithubRunID)
	}
	return nil
}

// Cancel cancels task execution
func (e *GraphFetchExecutor) Cancel(ctx context.Context, taskState *model.WorkloadTaskState) error {
	log.Infof("GraphFetchExecutor: cancelling task for workload %s", taskState.WorkloadUID)
	return nil
}

// Execute executes the graph fetch task
func (e *GraphFetchExecutor) Execute(ctx context.Context, execCtx *task.ExecutionContext) (*task.ExecutionResult, error) {
	taskState := execCtx.Task
	summaryID := int64(e.GetExtInt(taskState, ExtKeyRunSummaryID))
	githubRunID := int64(e.GetExtInt(taskState, ExtKeyGithubRunID))
	owner := e.GetExtString(taskState, ExtKeyOwner)
	repo := e.GetExtString(taskState, ExtKeyRepo)
	runnerSetNamespace := e.GetExtString(taskState, ExtKeyRunnerSetNamespace)
	runnerSetName := e.GetExtString(taskState, ExtKeyRunnerSetName)

	log.Infof("GraphFetchExecutor: starting graph fetch for run summary %d (github_run_id: %d)",
		summaryID, githubRunID)

	summaryFacade := database.GetFacade().GetGithubWorkflowRunSummary()

	// Get or create summary
	var summary *model.GithubWorkflowRunSummaries
	var err error

	if summaryID > 0 {
		summary, err = summaryFacade.GetByID(ctx, summaryID)
	} else if githubRunID > 0 {
		summary, err = summaryFacade.GetByGithubRunID(ctx, githubRunID)
	}

	if err != nil {
		return task.FailureResult(
			fmt.Sprintf("failed to get run summary: %v", err),
			map[string]interface{}{ExtKeyErrorMessage: err.Error()},
		), nil
	}

	if summary == nil {
		// Create new summary if not exists
		if githubRunID > 0 && owner != "" && repo != "" {
			// runnerSetID is 0 here as this is a fallback path; the reconciler
			// normally creates summaries with the correct PrimaryRunnerSetID.
			summary, _, err = summaryFacade.GetOrCreateByRunID(ctx, githubRunID, owner, repo, 0)
			if err != nil {
				return task.FailureResult(
					fmt.Sprintf("failed to create run summary: %v", err),
					map[string]interface{}{ExtKeyErrorMessage: err.Error()},
				), nil
			}
		} else {
			return task.FailureResult(
				"run summary not found and cannot create without github_run_id/owner/repo",
				nil,
			), nil
		}
	}

	// Check if already fetched
	if summary.GraphFetched {
		log.Infof("GraphFetchExecutor: graph already fetched for run summary %d, skipping", summary.ID)
		return task.SuccessResult(map[string]interface{}{
			"status": "already_fetched",
		}), nil
	}

	// Get GitHub client
	client, err := e.getGitHubClient(ctx, runnerSetNamespace, runnerSetName)
	if err != nil {
		log.Warnf("GraphFetchExecutor: failed to get GitHub client: %v", err)
		// Don't fail, just mark as fetched without data
		if updateErr := summaryFacade.UpdateGraphFetched(ctx, summary.ID, true); updateErr != nil {
			log.Errorf("GraphFetchExecutor: failed to update graph_fetched: %v", updateErr)
		}
		return task.SuccessResult(map[string]interface{}{
			"status": "skipped_no_client",
		}), nil
	}

	// Use owner/repo from summary if not provided
	if owner == "" {
		owner = summary.Owner
	}
	if repo == "" {
		repo = summary.Repo
	}

	if owner == "" || repo == "" {
		return task.FailureResult(
			"missing owner or repo for GitHub API call",
			nil,
		), nil
	}

	// Fetch workflow run details from GitHub
	runInfo, err := client.GetWorkflowRunWithJobs(ctx, owner, repo, summary.GithubRunID)
	if err != nil {
		return task.FailureResult(
			fmt.Sprintf("failed to fetch workflow run from GitHub: %v", err),
			map[string]interface{}{ExtKeyErrorMessage: err.Error()},
		), nil
	}

	// Update summary with GitHub data
	var runCompletedAt *time.Time
	if runInfo.RunCompletedAt != nil {
		runCompletedAt = runInfo.RunCompletedAt
	}

	githubData := &database.GitHubRunData{
		RunNumber:       int32(runInfo.RunNumber),
		RunAttempt:      int32(runInfo.RunAttempt),
		WorkflowName:    runInfo.WorkflowName,
		WorkflowPath:    runInfo.WorkflowPath,
		WorkflowID:      runInfo.WorkflowID,
		DisplayTitle:    runInfo.DisplayTitle,
		HeadSha:         runInfo.HeadSHA,
		HeadBranch:      runInfo.HeadBranch,
		BaseBranch:      runInfo.BaseBranch,
		EventName:       runInfo.Event,
		Status:          runInfo.Status,
		Conclusion:      runInfo.Conclusion,
		RunCompletedAt:  runCompletedAt,
	}

	if runInfo.Actor != nil && runInfo.Actor.Login != "" {
		githubData.Actor = runInfo.Actor.Login
	} else if runInfo.TriggerActor != nil && runInfo.TriggerActor.Login != "" {
		githubData.Actor = runInfo.TriggerActor.Login
	}
	if runInfo.TriggerActor != nil && runInfo.TriggerActor.Login != "" {
		githubData.TriggeringActor = runInfo.TriggerActor.Login
	} else if runInfo.Actor != nil && runInfo.Actor.Login != "" {
		githubData.TriggeringActor = runInfo.Actor.Login
	}
	if runInfo.RunStartedAt != nil {
		githubData.RunStartedAt = *runInfo.RunStartedAt
	}

	if err := summaryFacade.UpdateFromGitHub(ctx, summary.ID, githubData); err != nil {
		log.Errorf("GraphFetchExecutor: failed to update summary from GitHub: %v", err)
	}

	// Update total jobs count
	summary.TotalJobs = int32(len(runInfo.Jobs))
	if err := summaryFacade.Update(ctx, summary); err != nil {
		log.Errorf("GraphFetchExecutor: failed to update total_jobs: %v", err)
	}

	// Fetch workflow file to get job dependencies (needs)
	jobNeeds := make(map[string][]string)
	if runInfo.WorkflowPath != "" {
		workflowContent, err := client.GetWorkflowFileContent(ctx, owner, repo, runInfo.WorkflowPath, runInfo.HeadSHA)
		if err != nil {
			log.Warnf("GraphFetchExecutor: failed to get workflow file: %v", err)
		} else {
			// Parse workflow YAML to extract job dependencies
			jobNeeds = e.parseWorkflowNeeds(workflowContent)
			log.Infof("GraphFetchExecutor: parsed job dependencies from workflow: %v", jobNeeds)
		}
	}

	// Sync jobs and steps
	if len(runInfo.Jobs) > 0 {
		if err := e.syncJobsAndSteps(ctx, summary.ID, runInfo.Jobs, jobNeeds); err != nil {
			log.Errorf("GraphFetchExecutor: failed to sync jobs and steps: %v", err)
		}
	}

	// Mark graph as fetched
	if err := summaryFacade.UpdateGraphFetched(ctx, summary.ID, true); err != nil {
		log.Errorf("GraphFetchExecutor: failed to update graph_fetched: %v", err)
	}

	log.Infof("GraphFetchExecutor: graph fetch completed for run summary %d (jobs: %d)",
		summary.ID, len(runInfo.Jobs))

	return task.SuccessResult(map[string]interface{}{
		"status":     "success",
		"total_jobs": len(runInfo.Jobs),
	}), nil
}

// getGitHubClient returns a GitHub client for the given runner set
func (e *GraphFetchExecutor) getGitHubClient(ctx context.Context, namespace, name string) (*github.Client, error) {
	if e.clientManager == nil {
		return nil, fmt.Errorf("GitHub client manager not initialized")
	}

	if namespace == "" || name == "" {
		return nil, fmt.Errorf("runner set namespace/name not provided")
	}

	// Get runner set info to find the secret
	runnerSetFacade := database.GetFacade().GetGithubRunnerSet()
	runnerSet, err := runnerSetFacade.GetByNamespaceName(ctx, namespace, name)
	if err != nil || runnerSet == nil {
		return nil, fmt.Errorf("failed to get runner set: %v", err)
	}

	if runnerSet.GithubConfigSecret == "" {
		return nil, fmt.Errorf("no GitHub secret configured for runner set")
	}

	return e.clientManager.GetClientForSecret(ctx, namespace, runnerSet.GithubConfigSecret)
}

// parseWorkflowNeeds parses workflow YAML content and extracts job dependencies
func (e *GraphFetchExecutor) parseWorkflowNeeds(content string) map[string][]string {
	result := make(map[string][]string)

	var workflow struct {
		Jobs map[string]struct {
			Name  string      `yaml:"name"`
			Needs interface{} `yaml:"needs"`
		} `yaml:"jobs"`
	}

	if err := yaml.Unmarshal([]byte(content), &workflow); err != nil {
		log.Warnf("GraphFetchExecutor: failed to parse workflow YAML: %v", err)
		return result
	}

	for jobID, jobDef := range workflow.Jobs {
		if jobDef.Needs == nil {
			continue
		}

		// needs can be a string or []string
		switch needs := jobDef.Needs.(type) {
		case string:
			result[jobID] = []string{needs}
		case []interface{}:
			var needsList []string
			for _, n := range needs {
				if s, ok := n.(string); ok {
					needsList = append(needsList, s)
				}
			}
			result[jobID] = needsList
		}
	}

	return result
}

// syncJobsAndSteps syncs job and step data to the database
func (e *GraphFetchExecutor) syncJobsAndSteps(ctx context.Context, runSummaryID int64, jobs []github.JobInfo, jobNeeds map[string][]string) error {
	jobFacade := database.NewGithubWorkflowJobFacade()
	stepFacade := database.NewGithubWorkflowStepFacade()

	// Find any run under this summary to use as the target run_id.
	// github_workflow_runs are per-launcher, not per-GitHub-job,
	// so we pick the first available run as the anchor.
	runFacade := database.GetFacade().GetGithubWorkflowRun()
	runs, err := runFacade.ListByRunSummaryID(ctx, runSummaryID)
	if err != nil || len(runs) == 0 {
		return fmt.Errorf("no runs found for summary %d: %v", runSummaryID, err)
	}
	targetRunID := runs[0].ID

	// Build a reverse map: YAML job key -> needs list (already provided)
	// We need to match GitHub display names to YAML keys for needs lookup.
	// GitHub display names:
	//   - Non-matrix: same as YAML key or "name" field (e.g. "wait_for_build")
	//   - Matrix: "yaml_key (matrix_values)" (e.g. "integration_tests_mi325 (primus_pyt_train_llama-3.1-8b)")

	for _, ghJob := range jobs {
		// Calculate duration
		var duration int
		if ghJob.StartedAt != nil && ghJob.CompletedAt != nil {
			duration = int(ghJob.CompletedAt.Sub(*ghJob.StartedAt).Seconds())
		}

		// Count steps
		stepsCompleted := 0
		stepsFailed := 0
		for _, step := range ghJob.Steps {
			if step.Conclusion == "success" {
				stepsCompleted++
			} else if step.Conclusion == "failure" {
				stepsFailed++
			}
		}

		// Find needs for this job by matching against YAML job keys
		needsJSON := e.resolveNeeds(ghJob.Name, jobNeeds)

		job := &model.GithubWorkflowJobs{
			RunID:           targetRunID,
			GithubJobID:     ghJob.ID,
			Name:            ghJob.Name,
			Needs:           needsJSON,
			Status:          ghJob.Status,
			Conclusion:      ghJob.Conclusion,
			StartedAt:       ghJob.StartedAt,
			CompletedAt:     ghJob.CompletedAt,
			DurationSeconds: duration,
			RunnerID:        ghJob.RunnerID,
			RunnerName:      ghJob.RunnerName,
			HTMLURL:         ghJob.HTMLURL,
			StepsCount:      len(ghJob.Steps),
			StepsCompleted:  stepsCompleted,
			StepsFailed:     stepsFailed,
		}

		if err := jobFacade.Upsert(ctx, job); err != nil {
			log.Errorf("GraphFetchExecutor: failed to upsert job %d: %v", ghJob.ID, err)
			continue
		}

		// Get saved job to get its ID
		savedJob, err := jobFacade.GetByGithubJobID(ctx, targetRunID, ghJob.ID)
		if err != nil || savedJob == nil {
			continue
		}

		// Sync steps
		for _, ghStep := range ghJob.Steps {
			var stepDuration int
			if ghStep.StartedAt != nil && ghStep.CompletedAt != nil {
				stepDuration = int(ghStep.CompletedAt.Sub(*ghStep.StartedAt).Seconds())
			}

			step := &model.GithubWorkflowSteps{
				JobID:           savedJob.ID,
				StepNumber:      ghStep.Number,
				Name:            ghStep.Name,
				Status:          ghStep.Status,
				Conclusion:      ghStep.Conclusion,
				StartedAt:       ghStep.StartedAt,
				CompletedAt:     ghStep.CompletedAt,
				DurationSeconds: stepDuration,
			}

			if err := stepFacade.Upsert(ctx, step); err != nil {
				log.Errorf("GraphFetchExecutor: failed to upsert step %d: %v", ghStep.Number, err)
			}
		}
	}

	return nil
}

// resolveNeeds matches a GitHub job display name to YAML job keys and returns
// the serialized needs JSON. Handles both exact matches and matrix prefix matches.
func (e *GraphFetchExecutor) resolveNeeds(ghJobName string, jobNeeds map[string][]string) string {
	// 1. Exact match: "wait_for_build" == "wait_for_build"
	if needs, ok := jobNeeds[ghJobName]; ok {
		if needsBytes, err := json.Marshal(needs); err == nil {
			return string(needsBytes)
		}
	}

	// 2. Prefix match for matrix jobs: "integration_tests_mi325 (variant)" starts with "integration_tests_mi325"
	// Find the longest matching prefix to avoid false positives
	bestMatch := ""
	for yamlKey := range jobNeeds {
		if len(yamlKey) > len(bestMatch) && len(ghJobName) > len(yamlKey) {
			prefix := ghJobName[:len(yamlKey)]
			nextChar := ghJobName[len(yamlKey)]
			if prefix == yamlKey && (nextChar == ' ' || nextChar == '(') {
				bestMatch = yamlKey
			}
		}
	}

	if bestMatch != "" {
		if needs, ok := jobNeeds[bestMatch]; ok {
			if needsBytes, err := json.Marshal(needs); err == nil {
				return string(needsBytes)
			}
		}
	}

	return ""
}
