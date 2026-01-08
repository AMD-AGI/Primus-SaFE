package github_workflow_scanner

import (
	"context"
	"fmt"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/clientsets"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	"github.com/AMD-AGI/Primus-SaFE/Lens/modules/jobs/pkg/common"
)

const (
	// EphemeralRunnerKind is the Kubernetes kind for GitHub Actions runner pods
	EphemeralRunnerKind = "EphemeralRunner"

	// Annotations on EphemeralRunner that contain GitHub info
	AnnotationRunID      = "actions.github.com/run-id"
	AnnotationRunNumber  = "actions.github.com/run-number"
	AnnotationJobID      = "actions.github.com/job-id"
	AnnotationWorkflow   = "actions.github.com/workflow"
	AnnotationRepository = "actions.github.com/repository"
	AnnotationBranch     = "actions.github.com/branch"
	AnnotationSHA        = "actions.github.com/sha"
)

// GithubWorkflowScannerJob scans for completed EphemeralRunners and creates run records
type GithubWorkflowScannerJob struct {
	// ScanLookbackDuration defines how far back to look for completed runners
	ScanLookbackDuration time.Duration
	// MaxRunnersPerScan limits the number of runners processed per scan
	MaxRunnersPerScan int
}

// NewGithubWorkflowScannerJob creates a new GithubWorkflowScannerJob instance
func NewGithubWorkflowScannerJob() *GithubWorkflowScannerJob {
	return &GithubWorkflowScannerJob{
		ScanLookbackDuration: 24 * time.Hour, // Look back 24 hours by default
		MaxRunnersPerScan:    100,            // Process up to 100 runners per scan
	}
}

// Run executes the GitHub workflow scanner job
func (j *GithubWorkflowScannerJob) Run(ctx context.Context, clientSets *clientsets.K8SClientSet, storageClientSet *clientsets.StorageClientSet) (*common.ExecutionStats, error) {
	startTime := time.Now()
	stats := common.NewExecutionStats()

	cm := clientsets.GetClusterManager()
	clusterName := cm.GetCurrentClusterName()

	log.Infof("GithubWorkflowScannerJob: starting scan for cluster %s", clusterName)

	// Get all enabled configs for this cluster
	configFacade := database.GetFacade().GetGithubWorkflowConfig()
	configs, err := configFacade.ListEnabled(ctx)
	if err != nil {
		log.Errorf("GithubWorkflowScannerJob: failed to list enabled configs: %v", err)
		stats.ErrorCount++
		return stats, err
	}

	if len(configs) == 0 {
		log.Debug("GithubWorkflowScannerJob: no enabled configs found, skipping")
		stats.AddMessage("No enabled configs found")
		return stats, nil
	}

	log.Infof("GithubWorkflowScannerJob: found %d enabled configs", len(configs))

	workloadFacade := database.GetFacade().GetWorkload()
	runFacade := database.GetFacade().GetGithubWorkflowRun()

	totalNewRuns := 0
	totalSkipped := 0

	for _, config := range configs {
		if config.ClusterName != "" && config.ClusterName != clusterName {
			// Skip configs for other clusters
			continue
		}

		newRuns, skipped, err := j.scanConfigRunners(ctx, config, workloadFacade, runFacade, configFacade)
		if err != nil {
			log.Errorf("GithubWorkflowScannerJob: error scanning config %s: %v", config.Name, err)
			stats.ErrorCount++
			continue
		}

		totalNewRuns += newRuns
		totalSkipped += skipped
	}

	stats.RecordsProcessed = int64(totalNewRuns + totalSkipped)
	stats.ItemsCreated = int64(totalNewRuns)
	stats.ProcessDuration = time.Since(startTime).Seconds()
	stats.AddMessage(fmt.Sprintf("Scanned %d configs, created %d new run records, skipped %d existing",
		len(configs), totalNewRuns, totalSkipped))

	log.Infof("GithubWorkflowScannerJob: completed - new runs: %d, skipped: %d, errors: %d",
		totalNewRuns, totalSkipped, stats.ErrorCount)

	return stats, nil
}

// scanConfigRunners scans completed EphemeralRunners for a single config
func (j *GithubWorkflowScannerJob) scanConfigRunners(
	ctx context.Context,
	config *model.GithubWorkflowConfigs,
	workloadFacade database.WorkloadFacadeInterface,
	runFacade database.GithubWorkflowRunFacadeInterface,
	configFacade database.GithubWorkflowConfigFacadeInterface,
) (newRuns int, skipped int, err error) {
	since := time.Now().Add(-j.ScanLookbackDuration)

	// List completed EphemeralRunners in the config's namespace
	completedRunners, err := workloadFacade.ListCompletedWorkloadsByKindAndNamespace(
		ctx,
		EphemeralRunnerKind,
		config.RunnerSetNamespace,
		since,
		j.MaxRunnersPerScan,
	)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to list completed runners: %w", err)
	}

	if len(completedRunners) == 0 {
		log.Debugf("GithubWorkflowScannerJob: no completed runners found for config %s", config.Name)
		return 0, 0, nil
	}

	log.Infof("GithubWorkflowScannerJob: found %d completed runners for config %s", len(completedRunners), config.Name)

	for _, runner := range completedRunners {
		// Check if we already have a run record for this workload
		existingRun, err := runFacade.GetByConfigAndWorkload(ctx, config.ID, runner.UID)
		if err != nil {
			log.Errorf("GithubWorkflowScannerJob: error checking existing run for %s: %v", runner.UID, err)
			continue
		}

		if existingRun != nil {
			// Already processed
			skipped++
			continue
		}

		// Extract GitHub info from annotations
		githubRunID, githubRunNumber, githubJobID, headSHA, headBranch, workflowName := extractGitHubInfo(runner)

		// Check workflow filter if set
		if config.WorkflowFilter != "" && workflowName != "" && workflowName != config.WorkflowFilter {
			log.Debugf("GithubWorkflowScannerJob: skipping runner %s - workflow %s doesn't match filter %s",
				runner.Name, workflowName, config.WorkflowFilter)
			skipped++
			continue
		}

		// Check branch filter if set
		if config.BranchFilter != "" && headBranch != "" && headBranch != config.BranchFilter {
			log.Debugf("GithubWorkflowScannerJob: skipping runner %s - branch %s doesn't match filter %s",
				runner.Name, headBranch, config.BranchFilter)
			skipped++
			continue
		}

		// Create a new run record
		run := &model.GithubWorkflowRuns{
			ConfigID:            config.ID,
			WorkloadUID:         runner.UID,
			WorkloadName:        runner.Name,
			WorkloadNamespace:   runner.Namespace,
			GithubRunID:         githubRunID,
			GithubRunNumber:     githubRunNumber,
			GithubJobID:         githubJobID,
			HeadSha:             headSHA,
			HeadBranch:          headBranch,
			WorkflowName:        workflowName,
			Status:              database.WorkflowRunStatusPending,
			TriggerSource:       database.WorkflowRunTriggerRealtime,
			WorkloadStartedAt:   runner.CreatedAt,
			WorkloadCompletedAt: runner.EndAt,
		}

		if err := runFacade.Create(ctx, run); err != nil {
			log.Errorf("GithubWorkflowScannerJob: failed to create run record for %s: %v", runner.Name, err)
			continue
		}

		log.Infof("GithubWorkflowScannerJob: created run record %d for runner %s (GitHub run: %d)",
			run.ID, runner.Name, githubRunID)
		newRuns++
	}

	// Update last checked timestamp
	if err := configFacade.UpdateLastChecked(ctx, config.ID); err != nil {
		log.Warnf("GithubWorkflowScannerJob: failed to update last_checked_at for config %d: %v", config.ID, err)
	}

	return newRuns, skipped, nil
}

// extractGitHubInfo extracts GitHub workflow information from workload annotations
func extractGitHubInfo(workload *model.GpuWorkload) (runID int64, runNumber int32, jobID int64, sha, branch, workflow string) {
	if workload.Annotations == nil {
		return
	}

	// ExtType is map[string]interface{}, get string values directly
	getStringAnnotation := func(key string) string {
		if v, ok := workload.Annotations[key]; ok {
			if s, ok := v.(string); ok {
				return s
			}
		}
		return ""
	}

	if v := getStringAnnotation(AnnotationRunID); v != "" {
		fmt.Sscanf(v, "%d", &runID)
	}
	if v := getStringAnnotation(AnnotationRunNumber); v != "" {
		fmt.Sscanf(v, "%d", &runNumber)
	}
	if v := getStringAnnotation(AnnotationJobID); v != "" {
		fmt.Sscanf(v, "%d", &jobID)
	}
	sha = getStringAnnotation(AnnotationSHA)
	branch = getStringAnnotation(AnnotationBranch)
	workflow = getStringAnnotation(AnnotationWorkflow)

	return
}

// Schedule returns the cron schedule for this job
// Runs every 2 minutes to scan for completed runners
func (j *GithubWorkflowScannerJob) Schedule() string {
	return "@every 2m"
}

