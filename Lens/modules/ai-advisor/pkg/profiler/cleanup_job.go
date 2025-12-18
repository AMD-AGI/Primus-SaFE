package profiler

import (
	"context"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
)

// ProfilerCleanupJob profiler file cleanup job
type ProfilerCleanupJob struct {
	lifecycleMgr *LifecycleManager
	interval     time.Duration
	ticker       *time.Ticker
	stopChan     chan struct{}
}

// NewProfilerCleanupJob creates cleanup job
func NewProfilerCleanupJob(
	lifecycleMgr *LifecycleManager,
	schedule string, // For now, just store it (cron support can be added later)
) *ProfilerCleanupJob {
	// Default: run every 24 hours
	interval := 24 * time.Hour

	return &ProfilerCleanupJob{
		lifecycleMgr: lifecycleMgr,
		interval:     interval,
		stopChan:     make(chan struct{}),
	}
}

// Start starts cleanup job
func (j *ProfilerCleanupJob) Start(ctx context.Context) error {
	log.Infof("Starting profiler cleanup job with interval: %v", j.interval)

	j.ticker = time.NewTicker(j.interval)

	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-j.stopChan:
				return
			case <-j.ticker.C:
				j.runCleanup(ctx)
			}
		}
	}()

	return nil
}

// Stop stops cleanup job
func (j *ProfilerCleanupJob) Stop() {
	log.Info("Stopping profiler cleanup job")
	if j.ticker != nil {
		j.ticker.Stop()
	}
	close(j.stopChan)
}

// runCleanup executes cleanup
func (j *ProfilerCleanupJob) runCleanup(ctx context.Context) {
	log.Info("Running scheduled profiler file cleanup")

	// 1. Cleanup expired files
	result, err := j.lifecycleMgr.CleanupExpiredFiles(ctx)
	if err != nil {
		log.Errorf("Failed to cleanup expired files: %v", err)
		return
	}

	log.Infof("Cleanup completed: deleted %d files, freed %d MB",
		result.DeletedCount, result.FreedSpace/(1024*1024))

	// 2. Cleanup marked files (safe delete)
	markedResult, err := j.lifecycleMgr.DeleteMarkedFiles(ctx)
	if err != nil {
		log.Errorf("Failed to delete marked files: %v", err)
		return
	}

	if markedResult.DeletedCount > 0 {
		log.Infof("Deleted %d marked files, freed %d MB",
			markedResult.DeletedCount, markedResult.FreedSpace/(1024*1024))
	}
}

// RunOnce manually triggers cleanup once
func (j *ProfilerCleanupJob) RunOnce(ctx context.Context) (*CleanupResult, error) {
	return j.lifecycleMgr.CleanupExpiredFiles(ctx)
}
