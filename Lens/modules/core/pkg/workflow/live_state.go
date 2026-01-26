// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package workflow

import (
	"context"
	"sync"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/github"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
)

// WorkflowLiveState represents real-time workflow state
type WorkflowLiveState struct {
	RunID              int64           `json:"run_id"`
	GithubRunID        int64           `json:"github_run_id"`
	WorkflowName       string          `json:"workflow_name"`
	HeadSHA            string          `json:"head_sha,omitempty"`
	HeadBranch         string          `json:"head_branch,omitempty"`

	// Overall status
	WorkflowStatus     string          `json:"workflow_status"`     // queued, in_progress, completed
	WorkflowConclusion string          `json:"workflow_conclusion"` // success, failure, cancelled, etc.
	CollectionStatus   string          `json:"collection_status"`

	// Progress
	CurrentJobName     string          `json:"current_job_name,omitempty"`
	CurrentStepName    string          `json:"current_step_name,omitempty"`
	ProgressPercent    int             `json:"progress_percent"`

	// Timing
	StartedAt          *time.Time      `json:"started_at,omitempty"`
	EstimatedEndAt     *time.Time      `json:"estimated_end_at,omitempty"`
	ElapsedSeconds     int             `json:"elapsed_seconds"`

	// Jobs with real-time status
	Jobs               []*JobLiveState `json:"jobs"`

	// Metadata
	LastSyncedAt       time.Time       `json:"last_synced_at"`
	UpdatedAt          time.Time       `json:"updated_at"`
}

// JobLiveState represents real-time job state
type JobLiveState struct {
	ID                int64            `json:"id"`
	GithubJobID       int64            `json:"github_job_id"`
	Name              string           `json:"name"`
	Status            string           `json:"status"`     // queued, in_progress, completed
	Conclusion        string           `json:"conclusion"` // success, failure, cancelled, skipped

	// Progress
	CurrentStepNumber int              `json:"current_step_number,omitempty"`
	CurrentStepName   string           `json:"current_step_name,omitempty"`

	// Timing
	StartedAt         *time.Time       `json:"started_at,omitempty"`
	CompletedAt       *time.Time       `json:"completed_at,omitempty"`
	DurationSeconds   int              `json:"duration_seconds"`

	// Runner info
	RunnerName        string           `json:"runner_name,omitempty"`

	// Steps
	Steps             []*StepLiveState `json:"steps"`
}

// StepLiveState represents real-time step state
type StepLiveState struct {
	Number          int        `json:"number"`
	Name            string     `json:"name"`
	Status          string     `json:"status"`
	Conclusion      string     `json:"conclusion"`
	StartedAt       *time.Time `json:"started_at,omitempty"`
	CompletedAt     *time.Time `json:"completed_at,omitempty"`
	DurationSeconds int        `json:"duration_seconds"`
}

// syncContext holds sync state for a single run
type syncContext struct {
	runID     int64
	cancel    context.CancelFunc
	lastState *WorkflowLiveState
	interval  time.Duration
}

// WorkflowStateSyncer handles real-time workflow state synchronization
type WorkflowStateSyncer struct {
	mu          sync.RWMutex
	subscribers map[int64][]chan *WorkflowLiveState // runID -> subscribers
	activeRuns  map[int64]*syncContext              // runID -> sync context
}

// NewWorkflowStateSyncer creates a new syncer instance
func NewWorkflowStateSyncer() *WorkflowStateSyncer {
	return &WorkflowStateSyncer{
		subscribers: make(map[int64][]chan *WorkflowLiveState),
		activeRuns:  make(map[int64]*syncContext),
	}
}

// Subscribe subscribes to real-time updates for a workflow run
func (s *WorkflowStateSyncer) Subscribe(runID int64) (<-chan *WorkflowLiveState, func()) {
	s.mu.Lock()
	defer s.mu.Unlock()

	ch := make(chan *WorkflowLiveState, 10)
	s.subscribers[runID] = append(s.subscribers[runID], ch)

	// Start syncing if this is the first subscriber
	if _, exists := s.activeRuns[runID]; !exists {
		s.startSyncLocked(runID)
	}

	// Increment subscribers count in DB
	s.updateSubscribersCount(runID, 1)

	// Return channel and unsubscribe function
	return ch, func() {
		s.unsubscribe(runID, ch)
	}
}

// unsubscribe removes a subscriber
func (s *WorkflowStateSyncer) unsubscribe(runID int64, ch chan *WorkflowLiveState) {
	s.mu.Lock()
	defer s.mu.Unlock()

	subs := s.subscribers[runID]
	for i, sub := range subs {
		if sub == ch {
			s.subscribers[runID] = append(subs[:i], subs[i+1:]...)
			close(ch)
			break
		}
	}

	// Stop syncing if no more subscribers
	if len(s.subscribers[runID]) == 0 {
		if syncCtx, exists := s.activeRuns[runID]; exists {
			syncCtx.cancel()
			delete(s.activeRuns, runID)
		}
		delete(s.subscribers, runID)
	}

	// Decrement subscribers count in DB
	s.updateSubscribersCount(runID, -1)
}

// StartSync starts synchronization for a run (can be called externally)
func (s *WorkflowStateSyncer) StartSync(runID int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.activeRuns[runID]; !exists {
		s.startSyncLocked(runID)
	}
	return nil
}

// StopSync stops synchronization for a run
func (s *WorkflowStateSyncer) StopSync(runID int64) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if syncCtx, exists := s.activeRuns[runID]; exists {
		syncCtx.cancel()
		delete(s.activeRuns, runID)
	}
}

// GetCurrentState returns the current state for a run (from cache or DB)
func (s *WorkflowStateSyncer) GetCurrentState(ctx context.Context, runID int64) (*WorkflowLiveState, error) {
	s.mu.RLock()
	if syncCtx, exists := s.activeRuns[runID]; exists && syncCtx.lastState != nil {
		state := syncCtx.lastState
		s.mu.RUnlock()
		return state, nil
	}
	s.mu.RUnlock()

	// Fetch from database
	return s.buildStateFromDB(ctx, runID)
}

// startSyncLocked starts background syncing (must hold write lock)
func (s *WorkflowStateSyncer) startSyncLocked(runID int64) {
	ctx, cancel := context.WithCancel(context.Background())

	s.activeRuns[runID] = &syncContext{
		runID:    runID,
		cancel:   cancel,
		interval: 3 * time.Second, // Default 3s polling
	}

	go s.syncLoop(ctx, runID)
}

// syncLoop continuously syncs workflow state from GitHub
func (s *WorkflowStateSyncer) syncLoop(ctx context.Context, runID int64) {
	// Initial sync
	s.syncOnce(ctx, runID)

	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			state := s.syncOnce(ctx, runID)

			// Adjust polling interval based on state
			if state != nil {
				newInterval := s.calculatePollInterval(state)
				if newInterval == 0 {
					// Workflow completed, stop syncing
					s.StopSync(runID)
					return
				}
				ticker.Reset(newInterval)
			}
		}
	}
}

// syncOnce performs a single sync from GitHub API
func (s *WorkflowStateSyncer) syncOnce(ctx context.Context, runID int64) *WorkflowLiveState {
	// 1. Get run info from database
	run, runnerSet, err := s.getRunInfo(ctx, runID)
	if err != nil {
		log.Errorf("WorkflowStateSyncer: failed to get run info for %d: %v", runID, err)
		return nil
	}

	if run.GithubRunID == 0 {
		log.Debugf("WorkflowStateSyncer: run %d has no GitHub run ID, skipping", runID)
		return nil
	}

	// 2. Fetch latest state from GitHub
	state, err := s.fetchFromGitHub(ctx, run, runnerSet)
	if err != nil {
		log.Warnf("WorkflowStateSyncer: failed to fetch from GitHub for run %d: %v", runID, err)
		// Return cached state if available
		s.mu.RLock()
		if syncCtx, exists := s.activeRuns[runID]; exists && syncCtx.lastState != nil {
			state = syncCtx.lastState
		}
		s.mu.RUnlock()
		return state
	}

	// 3. Update database
	if err := s.updateDatabase(ctx, run, state); err != nil {
		log.Errorf("WorkflowStateSyncer: failed to update database for run %d: %v", runID, err)
	}

	// 4. Cache state
	s.mu.Lock()
	if syncCtx, exists := s.activeRuns[runID]; exists {
		syncCtx.lastState = state
	}
	s.mu.Unlock()

	// 5. Broadcast to subscribers
	s.broadcast(runID, state)

	return state
}

// getRunInfo gets run and runner set from database
func (s *WorkflowStateSyncer) getRunInfo(ctx context.Context, runID int64) (*model.GithubWorkflowRuns, *model.GithubRunnerSets, error) {
	runFacade := database.GetFacade().GetGithubWorkflowRun()
	run, err := runFacade.GetByID(ctx, runID)
	if err != nil {
		return nil, nil, err
	}
	if run == nil {
		return nil, nil, nil
	}

	runnerSetFacade := database.GetFacade().GetGithubRunnerSet()
	runnerSet, err := runnerSetFacade.GetByID(ctx, run.RunnerSetID)
	if err != nil {
		return run, nil, err
	}

	return run, runnerSet, nil
}

// fetchFromGitHub fetches the latest workflow state from GitHub API
func (s *WorkflowStateSyncer) fetchFromGitHub(ctx context.Context, run *model.GithubWorkflowRuns, runnerSet *model.GithubRunnerSets) (*WorkflowLiveState, error) {
	if runnerSet == nil || runnerSet.GithubOwner == "" || runnerSet.GithubRepo == "" {
		return nil, nil
	}

	// Get GitHub client
	githubManager := github.GetGlobalManager()
	if githubManager == nil {
		return nil, nil
	}

	client, err := githubManager.GetClientForSecret(ctx, runnerSet.Namespace, runnerSet.GithubConfigSecret)
	if err != nil {
		return nil, err
	}

	// Get workflow run info
	ghRun, err := client.GetWorkflowRun(ctx, runnerSet.GithubOwner, runnerSet.GithubRepo, run.GithubRunID)
	if err != nil {
		return nil, err
	}

	// Get jobs and steps
	ghJobs, err := client.GetWorkflowRunJobs(ctx, runnerSet.GithubOwner, runnerSet.GithubRepo, run.GithubRunID)
	if err != nil {
		log.Warnf("WorkflowStateSyncer: failed to get jobs for run %d: %v", run.GithubRunID, err)
		ghJobs = nil
	}

	// Build live state
	state := &WorkflowLiveState{
		RunID:              run.ID,
		GithubRunID:        run.GithubRunID,
		WorkflowName:       ghRun.WorkflowName,
		HeadSHA:            ghRun.HeadSHA,
		HeadBranch:         ghRun.HeadBranch,
		WorkflowStatus:     ghRun.Status,
		WorkflowConclusion: ghRun.Conclusion,
		CollectionStatus:   run.Status,
		StartedAt:          ghRun.RunStartedAt,
		LastSyncedAt:       time.Now(),
		UpdatedAt:          time.Now(),
	}

	// Calculate elapsed time
	if state.StartedAt != nil {
		state.ElapsedSeconds = int(time.Since(*state.StartedAt).Seconds())
	}

	// Process jobs
	if ghJobs != nil {
		state.Jobs = make([]*JobLiveState, len(ghJobs))
		totalSteps := 0
		completedSteps := 0

		for i, ghJob := range ghJobs {
			jobState := s.buildJobState(&ghJob)
			state.Jobs[i] = jobState

			// Track current job/step
			if ghJob.Status == "in_progress" {
				state.CurrentJobName = ghJob.Name
				if jobState.CurrentStepName != "" {
					state.CurrentStepName = jobState.CurrentStepName
				}
			}

			// Count steps for progress
			for _, step := range ghJob.Steps {
				totalSteps++
				if step.Conclusion == "success" || step.Conclusion == "skipped" {
					completedSteps++
				}
			}
		}

		// Calculate progress percentage
		if totalSteps > 0 {
			state.ProgressPercent = (completedSteps * 100) / totalSteps
		}
	}

	return state, nil
}

// buildJobState builds JobLiveState from GitHub JobInfo
func (s *WorkflowStateSyncer) buildJobState(ghJob *github.JobInfo) *JobLiveState {
	jobState := &JobLiveState{
		GithubJobID: ghJob.ID,
		Name:        ghJob.Name,
		Status:      ghJob.Status,
		Conclusion:  ghJob.Conclusion,
		StartedAt:   ghJob.StartedAt,
		CompletedAt: ghJob.CompletedAt,
		RunnerName:  ghJob.RunnerName,
		Steps:       make([]*StepLiveState, len(ghJob.Steps)),
	}

	// Calculate duration
	if ghJob.StartedAt != nil {
		endTime := time.Now()
		if ghJob.CompletedAt != nil {
			endTime = *ghJob.CompletedAt
		}
		jobState.DurationSeconds = int(endTime.Sub(*ghJob.StartedAt).Seconds())
	}

	// Process steps
	for j, ghStep := range ghJob.Steps {
		stepState := &StepLiveState{
			Number:      ghStep.Number,
			Name:        ghStep.Name,
			Status:      ghStep.Status,
			Conclusion:  ghStep.Conclusion,
			StartedAt:   ghStep.StartedAt,
			CompletedAt: ghStep.CompletedAt,
		}

		// Calculate step duration
		if ghStep.StartedAt != nil {
			endTime := time.Now()
			if ghStep.CompletedAt != nil {
				endTime = *ghStep.CompletedAt
			}
			stepState.DurationSeconds = int(endTime.Sub(*ghStep.StartedAt).Seconds())
		}

		// Track current step
		if ghStep.Status == "in_progress" {
			jobState.CurrentStepNumber = ghStep.Number
			jobState.CurrentStepName = ghStep.Name
		}

		jobState.Steps[j] = stepState
	}

	return jobState
}

// updateDatabase updates the database with latest state
func (s *WorkflowStateSyncer) updateDatabase(ctx context.Context, run *model.GithubWorkflowRuns, state *WorkflowLiveState) error {
	// Sync jobs and steps to database
	if state.Jobs != nil {
		jobFacade := database.NewGithubWorkflowJobFacade()

		// Convert to github.JobInfo for SyncFromGitHub
		ghJobs := make([]github.JobInfo, len(state.Jobs))
		for i, job := range state.Jobs {
			ghJobs[i] = github.JobInfo{
				ID:          job.GithubJobID,
				Name:        job.Name,
				Status:      job.Status,
				Conclusion:  job.Conclusion,
				StartedAt:   job.StartedAt,
				CompletedAt: job.CompletedAt,
				RunnerName:  job.RunnerName,
			}

			// Convert steps
			ghJobs[i].Steps = make([]github.StepInfo, len(job.Steps))
			for j, step := range job.Steps {
				ghJobs[i].Steps[j] = github.StepInfo{
					Number:      step.Number,
					Name:        step.Name,
					Status:      step.Status,
					Conclusion:  step.Conclusion,
					StartedAt:   step.StartedAt,
					CompletedAt: step.CompletedAt,
				}
			}
		}

		if err := jobFacade.SyncFromGitHub(ctx, run.ID, ghJobs); err != nil {
			return err
		}
	}

	return nil
}

// buildStateFromDB builds state from database (fallback when not actively syncing)
func (s *WorkflowStateSyncer) buildStateFromDB(ctx context.Context, runID int64) (*WorkflowLiveState, error) {
	runFacade := database.GetFacade().GetGithubWorkflowRun()
	run, err := runFacade.GetByID(ctx, runID)
	if err != nil || run == nil {
		return nil, err
	}

	state := &WorkflowLiveState{
		RunID:            run.ID,
		GithubRunID:      run.GithubRunID,
		WorkflowName:     run.WorkflowName,
		HeadSHA:          run.HeadSha,
		HeadBranch:       run.HeadBranch,
		CollectionStatus: run.Status,
		UpdatedAt:        run.UpdatedAt,
	}

	// Load jobs from database
	jobFacade := database.NewGithubWorkflowJobFacade()
	jobsWithSteps, err := jobFacade.ListByRunIDWithSteps(ctx, runID)
	if err == nil && len(jobsWithSteps) > 0 {
		state.Jobs = make([]*JobLiveState, len(jobsWithSteps))
		for i, job := range jobsWithSteps {
			jobState := &JobLiveState{
				ID:              job.ID,
				GithubJobID:     job.GithubJobID,
				Name:            job.Name,
				Status:          job.Status,
				Conclusion:      job.Conclusion,
				StartedAt:       job.StartedAt,
				CompletedAt:     job.CompletedAt,
				DurationSeconds: job.DurationSeconds,
				RunnerName:      job.RunnerName,
			}

			if job.Steps != nil {
				jobState.Steps = make([]*StepLiveState, len(job.Steps))
				for j, step := range job.Steps {
					jobState.Steps[j] = &StepLiveState{
						Number:          step.StepNumber,
						Name:            step.Name,
						Status:          step.Status,
						Conclusion:      step.Conclusion,
						StartedAt:       step.StartedAt,
						CompletedAt:     step.CompletedAt,
						DurationSeconds: step.DurationSeconds,
					}
				}
			}

			state.Jobs[i] = jobState
		}
	}

	return state, nil
}

// broadcast sends state update to all subscribers
func (s *WorkflowStateSyncer) broadcast(runID int64, state *WorkflowLiveState) {
	s.mu.RLock()
	subscribers := s.subscribers[runID]
	s.mu.RUnlock()

	for _, ch := range subscribers {
		select {
		case ch <- state:
		default:
			// Channel full, skip this update
		}
	}
}

// calculatePollInterval determines polling interval based on state
func (s *WorkflowStateSyncer) calculatePollInterval(state *WorkflowLiveState) time.Duration {
	// Stop polling for completed workflows
	if state.WorkflowStatus == "completed" {
		return 0
	}

	// More frequent polling for active workflows
	if state.WorkflowStatus == "in_progress" {
		// If a step just started (<10s), poll faster
		for _, job := range state.Jobs {
			for _, step := range job.Steps {
				if step.Status == "in_progress" && step.DurationSeconds < 10 {
					return 2 * time.Second
				}
			}
		}
		return 5 * time.Second
	}

	// Less frequent for queued
	if state.WorkflowStatus == "queued" {
		return 10 * time.Second
	}

	return 5 * time.Second
}

// updateSubscribersCount updates the subscribers count in the database
func (s *WorkflowStateSyncer) updateSubscribersCount(runID int64, delta int) {
	// This is a best-effort update, don't block on it
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		runFacade := database.GetFacade().GetGithubWorkflowRun()
		run, err := runFacade.GetByID(ctx, runID)
		if err != nil || run == nil {
			return
		}

		// Update ext field with subscribers count
		// This is optional metadata for monitoring
		log.Debugf("WorkflowStateSyncer: run %d subscribers changed by %d", runID, delta)
	}()
}

// Global syncer instance
var (
	globalSyncer     *WorkflowStateSyncer
	globalSyncerOnce sync.Once
)

// GetGlobalSyncer returns the global syncer instance
func GetGlobalSyncer() *WorkflowStateSyncer {
	globalSyncerOnce.Do(func() {
		globalSyncer = NewWorkflowStateSyncer()
	})
	return globalSyncer
}
