package detection

import (
	"context"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
)

// EvidenceCleanupConfig holds configuration for evidence cleanup
type EvidenceCleanupConfig struct {
	// Interval between cleanup runs
	Interval time.Duration

	// DefaultExpiration is the default expiration time for evidence records
	// that don't have an explicit expires_at set
	DefaultExpiration time.Duration

	// KeepProcessedFor is how long to keep processed evidence
	KeepProcessedFor time.Duration

	// MaxEvidencePerWorkload is the maximum number of evidence records to keep per workload
	MaxEvidencePerWorkload int

	// BatchSize is the number of records to delete in each batch
	BatchSize int
}

// DefaultEvidenceCleanupConfig returns the default cleanup configuration
func DefaultEvidenceCleanupConfig() *EvidenceCleanupConfig {
	return &EvidenceCleanupConfig{
		Interval:               1 * time.Hour,
		DefaultExpiration:      7 * 24 * time.Hour, // 7 days
		KeepProcessedFor:       3 * 24 * time.Hour, // 3 days
		MaxEvidencePerWorkload: 100,
		BatchSize:              1000,
	}
}

// EvidenceCleanupJob handles cleanup of expired and old evidence records
type EvidenceCleanupJob struct {
	config         *EvidenceCleanupConfig
	evidenceFacade database.WorkloadDetectionEvidenceFacadeInterface
	stopCh         chan struct{}
	running        bool
}

// NewEvidenceCleanupJob creates a new evidence cleanup job
func NewEvidenceCleanupJob(config *EvidenceCleanupConfig) *EvidenceCleanupJob {
	if config == nil {
		config = DefaultEvidenceCleanupConfig()
	}

	return &EvidenceCleanupJob{
		config:         config,
		evidenceFacade: database.NewWorkloadDetectionEvidenceFacade(),
		stopCh:         make(chan struct{}),
	}
}

// NewEvidenceCleanupJobWithFacade creates a new evidence cleanup job with custom facade
func NewEvidenceCleanupJobWithFacade(config *EvidenceCleanupConfig, facade database.WorkloadDetectionEvidenceFacadeInterface) *EvidenceCleanupJob {
	if config == nil {
		config = DefaultEvidenceCleanupConfig()
	}

	return &EvidenceCleanupJob{
		config:         config,
		evidenceFacade: facade,
		stopCh:         make(chan struct{}),
	}
}

// Start starts the cleanup job
func (j *EvidenceCleanupJob) Start(ctx context.Context) {
	if j.running {
		log.Warn("Evidence cleanup job is already running")
		return
	}

	j.running = true
	log.Infof("Starting evidence cleanup job with interval %v", j.config.Interval)

	go j.run(ctx)
}

// Stop stops the cleanup job
func (j *EvidenceCleanupJob) Stop() {
	if !j.running {
		return
	}

	log.Info("Stopping evidence cleanup job")
	close(j.stopCh)
	j.running = false
}

// run is the main loop for the cleanup job
func (j *EvidenceCleanupJob) run(ctx context.Context) {
	ticker := time.NewTicker(j.config.Interval)
	defer ticker.Stop()

	// Run immediately on start
	j.cleanup(ctx)

	for {
		select {
		case <-ctx.Done():
			log.Info("Evidence cleanup job context cancelled")
			return
		case <-j.stopCh:
			log.Info("Evidence cleanup job stopped")
			return
		case <-ticker.C:
			j.cleanup(ctx)
		}
	}
}

// cleanup performs the cleanup operation
func (j *EvidenceCleanupJob) cleanup(ctx context.Context) {
	startTime := time.Now()
	log.Debug("Starting evidence cleanup run")

	var totalDeleted int64

	// 1. Delete expired evidence (those with explicit expires_at)
	expiredCount, err := j.evidenceFacade.DeleteExpiredEvidence(ctx)
	if err != nil {
		log.Errorf("Failed to delete expired evidence: %v", err)
	} else {
		totalDeleted += expiredCount
		if expiredCount > 0 {
			log.Infof("Deleted %d expired evidence records", expiredCount)
		}
	}

	// Note: Additional cleanup operations can be added here:
	// - Delete old processed evidence (beyond KeepProcessedFor)
	// - Trim evidence per workload to MaxEvidencePerWorkload
	// These would require additional database methods

	duration := time.Since(startTime)
	log.Debugf("Evidence cleanup completed in %v, deleted %d records total", duration, totalDeleted)
}

// RunOnce runs the cleanup once (for testing or manual trigger)
func (j *EvidenceCleanupJob) RunOnce(ctx context.Context) (int64, error) {
	startTime := time.Now()
	log.Info("Running one-time evidence cleanup")

	expiredCount, err := j.evidenceFacade.DeleteExpiredEvidence(ctx)
	if err != nil {
		return 0, err
	}

	duration := time.Since(startTime)
	log.Infof("One-time evidence cleanup completed in %v, deleted %d records", duration, expiredCount)

	return expiredCount, nil
}
