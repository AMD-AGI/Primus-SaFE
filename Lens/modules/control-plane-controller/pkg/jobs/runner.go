// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package jobs

import (
	"context"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/clientsets"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/config"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	"github.com/robfig/cron/v3"
)

// Start initializes and starts all control plane jobs
func Start(ctx context.Context, cfg *config.JobsConfig) error {
	// Initialize job registry with configuration
	InitJobs(cfg)

	jobs := GetJobs()
	if len(jobs) == 0 {
		log.Warn("No control plane jobs to run")
		return nil
	}

	// Create cron scheduler with SkipIfStillRunning to prevent overlapping executions
	c := cron.New(cron.WithChain(
		cron.SkipIfStillRunning(cron.DefaultLogger),
	))

	// Get cluster manager for K8S and Storage client access
	cm := clientsets.GetClusterManager()

	// Register all jobs
	for _, job := range jobs {
		jobToRun := job
		jobName := getJobName(jobToRun)

		_, err := c.AddFunc(job.Schedule(), func() {
			// Get current cluster's clients
			currentCluster := cm.GetCurrentClusterClients()
			if currentCluster == nil {
				log.Warnf("Skipping job %s: current cluster not initialized", getJobName(jobToRun))
				return
			}

			// Get K8S client (required for all jobs)
			k8sClient := currentCluster.K8SClientSet
			if k8sClient == nil {
				log.Warnf("Skipping job %s: K8S client not available", getJobName(jobToRun))
				return
			}

			// Get storage client (optional - may be nil for pure CP jobs)
			// Multi-cluster jobs will handle nil storage gracefully
			storageClient := currentCluster.StorageClientSet

			// Run job with metrics
			runJobWithMetrics(ctx, jobToRun, k8sClient, storageClient)
		})

		if err != nil {
			log.Errorf("Failed to schedule job %s: %v", jobName, err)
			return err
		}

		log.Infof("Scheduled job: %s with schedule: %s", jobName, job.Schedule())
	}

	// Start scheduler
	c.Start()
	log.Infof("Control plane controller started with %d jobs", len(jobs))

	// Handle graceful shutdown
	go func() {
		<-ctx.Done()
		log.Info("Stopping control plane job scheduler...")
		c.Stop()
	}()

	return nil
}
