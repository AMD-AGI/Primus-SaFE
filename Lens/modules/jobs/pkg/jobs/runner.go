package jobs

import (
	"context"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/clientsets"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	"github.com/robfig/cron/v3"
)

func Start(ctx context.Context) error {
	InitJobs()
	// Use SkipIfStillRunning to prevent concurrent execution of the same job
	// If a job is still running when the next scheduled time arrives, the new execution will be skipped
	c := cron.New(cron.WithChain(
		cron.SkipIfStillRunning(cron.DefaultLogger),
	))
	cm := clientsets.GetClusterManager()
	currentCluster := cm.GetCurrentClusterClients()

	// Register all jobs with metrics collection
	for _, job := range jobs {
		// Capture job variable for closure
		jobToRun := job
		jobName := getJobName(jobToRun)

		_, err := c.AddFunc(job.Schedule(), func() {
			runJobWithMetrics(ctx, jobToRun, currentCluster.K8SClientSet, currentCluster.StorageClientSet)
		})

		if err != nil {
			log.Errorf("Failed to schedule job %s: %v", jobName, err)
			return err
		}

		log.Infof("Registered job: %s with schedule: %s", jobName, job.Schedule())
	}

	c.Start()
	log.Infof("Started %d jobs", len(jobs))
	return nil
}
