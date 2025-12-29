package background

import (
	"context"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/ai-gateway/pkg/config"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/airegistry"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/aitaskqueue"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
)

// Manager manages all background jobs
type Manager struct {
	config        *config.Config
	registry      airegistry.Registry
	taskQueue     *aitaskqueue.PGStore
	healthChecker *HealthCheckJob
	timeoutHandler *TimeoutJob
	cleanupJob    *CleanupJob
	stopCh        chan struct{}
}

// NewManager creates a new background job manager
func NewManager(registry airegistry.Registry, taskQueue *aitaskqueue.PGStore, cfg *config.Config) *Manager {
	return &Manager{
		config:    cfg,
		registry:  registry,
		taskQueue: taskQueue,
		stopCh:    make(chan struct{}),
	}
}

// Start starts all enabled background jobs
func (m *Manager) Start(ctx context.Context) {
	log.Info("Starting background jobs...")

	// Health check job
	if m.config.Background.HealthCheck.Enabled {
		m.healthChecker = NewHealthCheckJob(m.registry, m.config.Background.HealthCheck.Interval)
		go m.healthChecker.Run(ctx)
		log.Infof("Health check job started (interval: %v)", m.config.Background.HealthCheck.Interval)
	}

	// Timeout handler job
	if m.config.Background.Timeout.Enabled {
		m.timeoutHandler = NewTimeoutJob(m.taskQueue, m.config.Background.Timeout.Interval)
		go m.timeoutHandler.Run(ctx)
		log.Infof("Timeout handler job started (interval: %v)", m.config.Background.Timeout.Interval)
	}

	// Cleanup job
	if m.config.Background.Cleanup.Enabled {
		m.cleanupJob = NewCleanupJob(m.taskQueue, m.config.Background.Cleanup.Interval, m.config.Background.Cleanup.RetentionPeriod)
		go m.cleanupJob.Run(ctx)
		log.Infof("Cleanup job started (interval: %v, retention: %v)", m.config.Background.Cleanup.Interval, m.config.Background.Cleanup.RetentionPeriod)
	}
}

// Stop stops all background jobs
func (m *Manager) Stop() {
	log.Info("Stopping background jobs...")
	close(m.stopCh)

	if m.healthChecker != nil {
		m.healthChecker.Stop()
	}
	if m.timeoutHandler != nil {
		m.timeoutHandler.Stop()
	}
	if m.cleanupJob != nil {
		m.cleanupJob.Stop()
	}
}

// HealthCheckJob periodically checks agent health
type HealthCheckJob struct {
	registry airegistry.Registry
	checker  *airegistry.HealthChecker
	interval time.Duration
	stopCh   chan struct{}
}

// NewHealthCheckJob creates a new health check job
func NewHealthCheckJob(registry airegistry.Registry, interval time.Duration) *HealthCheckJob {
	return &HealthCheckJob{
		registry: registry,
		checker:  airegistry.NewHealthChecker(registry, 5*time.Second, 3),
		interval: interval,
		stopCh:   make(chan struct{}),
	}
}

// Run runs the health check job
func (j *HealthCheckJob) Run(ctx context.Context) {
	ticker := time.NewTicker(j.interval)
	defer ticker.Stop()

	// Run immediately on start
	j.check(ctx)

	for {
		select {
		case <-ctx.Done():
			return
		case <-j.stopCh:
			return
		case <-ticker.C:
			j.check(ctx)
		}
	}
}

// Stop stops the health check job
func (j *HealthCheckJob) Stop() {
	close(j.stopCh)
}

func (j *HealthCheckJob) check(ctx context.Context) {
	results := j.checker.CheckAll(ctx)
	healthyCount := 0
	unhealthyCount := 0

	for _, result := range results {
		if result.Healthy {
			healthyCount++
		} else {
			unhealthyCount++
			if result.Error != nil {
				log.Warnf("Agent %s health check failed: %v", result.AgentName, result.Error)
			}
		}
	}

	if len(results) > 0 {
		log.Debugf("Health check completed: %d healthy, %d unhealthy", healthyCount, unhealthyCount)
	}
}

// TimeoutJob handles timed-out tasks
type TimeoutJob struct {
	queue    *aitaskqueue.PGStore
	interval time.Duration
	stopCh   chan struct{}
}

// NewTimeoutJob creates a new timeout job
func NewTimeoutJob(queue *aitaskqueue.PGStore, interval time.Duration) *TimeoutJob {
	return &TimeoutJob{
		queue:    queue,
		interval: interval,
		stopCh:   make(chan struct{}),
	}
}

// Run runs the timeout job
func (j *TimeoutJob) Run(ctx context.Context) {
	ticker := time.NewTicker(j.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-j.stopCh:
			return
		case <-ticker.C:
			count, err := j.queue.HandleTimeouts(ctx)
			if err != nil {
				log.Warnf("Timeout handling failed: %v", err)
			} else if count > 0 {
				log.Infof("Handled %d timed-out tasks", count)
			}
		}
	}
}

// Stop stops the timeout job
func (j *TimeoutJob) Stop() {
	close(j.stopCh)
}

// CleanupJob cleans up old completed tasks
type CleanupJob struct {
	queue           *aitaskqueue.PGStore
	interval        time.Duration
	retentionPeriod time.Duration
	stopCh          chan struct{}
}

// NewCleanupJob creates a new cleanup job
func NewCleanupJob(queue *aitaskqueue.PGStore, interval, retentionPeriod time.Duration) *CleanupJob {
	return &CleanupJob{
		queue:           queue,
		interval:        interval,
		retentionPeriod: retentionPeriod,
		stopCh:          make(chan struct{}),
	}
}

// Run runs the cleanup job
func (j *CleanupJob) Run(ctx context.Context) {
	ticker := time.NewTicker(j.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-j.stopCh:
			return
		case <-ticker.C:
			count, err := j.queue.Cleanup(ctx, j.retentionPeriod)
			if err != nil {
				log.Warnf("Cleanup failed: %v", err)
			} else if count > 0 {
				log.Infof("Cleaned up %d old tasks", count)
			}
		}
	}
}

// Stop stops the cleanup job
func (j *CleanupJob) Stop() {
	close(j.stopCh)
}

