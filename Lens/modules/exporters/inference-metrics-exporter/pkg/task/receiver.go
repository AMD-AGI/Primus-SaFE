// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package task

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/constant"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	"github.com/AMD-AGI/Primus-SaFE/Lens/inference-metrics-exporter/pkg/config"
	"github.com/AMD-AGI/Primus-SaFE/Lens/inference-metrics-exporter/pkg/exporter"
	"github.com/AMD-AGI/Primus-SaFE/Lens/inference-metrics-exporter/pkg/scraper"
)

// TaskReceiver polls database for scrape tasks and manages their lifecycle
type TaskReceiver struct {
	cfg           *config.ExporterConfig
	facade        *database.WorkloadTaskFacade
	exporter      *exporter.MetricsExporter
	scrapeManager *scraper.ScrapeManager

	// Active tasks managed by this instance
	activeTasks map[string]*ScrapeTask // workloadUID -> task
	tasksMu     sync.RWMutex

	// Lifecycle
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// NewTaskReceiver creates a new task receiver
func NewTaskReceiver(cfg *config.ExporterConfig, exp *exporter.MetricsExporter) *TaskReceiver {
	return &TaskReceiver{
		cfg:           cfg,
		facade:        database.NewWorkloadTaskFacade(),
		exporter:      exp,
		scrapeManager: scraper.NewScrapeManager(exp),
		activeTasks:   make(map[string]*ScrapeTask),
	}
}

// Start begins the task receiver
func (r *TaskReceiver) Start(ctx context.Context) error {
	r.ctx, r.cancel = context.WithCancel(ctx)

	log.Infof("Starting task receiver with instanceID=%s", r.cfg.InstanceID)

	// Start scrape manager
	if err := r.scrapeManager.Start(ctx); err != nil {
		return fmt.Errorf("start scrape manager: %w", err)
	}

	// Recover tasks owned by this instance from previous run
	if err := r.recoverOwnedTasks(); err != nil {
		log.Errorf("Failed to recover owned tasks: %v", err)
		// Continue anyway, not fatal
	}

	// Start background goroutines
	r.wg.Add(3)
	go r.pollAndAcquireTasks()
	go r.renewLocks()
	go r.cleanupStaleLocks()

	return nil
}

// Stop stops the task receiver
func (r *TaskReceiver) Stop() error {
	log.Info("Stopping task receiver...")
	r.cancel()

	// Stop scrape manager (stops all targets)
	if err := r.scrapeManager.Stop(); err != nil {
		log.Errorf("Failed to stop scrape manager: %v", err)
	}

	// Release all locks
	r.tasksMu.RLock()
	for uid := range r.activeTasks {
		if err := r.facade.ReleaseLock(context.Background(), uid, config.TaskTypeInferenceMetricsScrape, r.cfg.InstanceID); err != nil {
			log.Errorf("Failed to release lock for %s: %v", uid, err)
		}
	}
	r.tasksMu.RUnlock()

	r.wg.Wait()
	log.Info("Task receiver stopped")
	return nil
}

// recoverOwnedTasks recovers tasks that were owned by this instance
func (r *TaskReceiver) recoverOwnedTasks() error {
	tasks, err := r.facade.ListTasksByStatus(r.ctx, constant.TaskStatusRunning)
	if err != nil {
		return err
	}

	for _, m := range tasks {
		if m.TaskType != config.TaskTypeInferenceMetricsScrape {
			continue
		}
		if m.LockOwner != r.cfg.InstanceID {
			continue
		}

		task, err := FromModel(m)
		if err != nil {
			log.Errorf("Failed to parse task %s: %v", m.WorkloadUID, err)
			continue
		}

		log.Infof("Recovering task %s", task.WorkloadUID)
		r.addActiveTask(task)
	}

	return nil
}

// pollAndAcquireTasks polls for available tasks and tries to acquire them
func (r *TaskReceiver) pollAndAcquireTasks() {
	defer r.wg.Done()

	ticker := time.NewTicker(r.cfg.TaskPollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-r.ctx.Done():
			return
		case <-ticker.C:
			r.tryAcquireNewTasks()
			r.checkForCompletedTasks()
		}
	}
}

// tryAcquireNewTasks finds and acquires available tasks
func (r *TaskReceiver) tryAcquireNewTasks() {
	// Check capacity
	r.tasksMu.RLock()
	currentCount := len(r.activeTasks)
	r.tasksMu.RUnlock()

	if currentCount >= r.cfg.MaxConcurrentScrapes {
		log.Debugf("At max capacity (%d tasks), skipping acquisition", currentCount)
		return
	}

	// Find pending tasks
	tasks, err := r.facade.ListTasksByStatus(r.ctx, constant.TaskStatusPending)
	if err != nil {
		log.Errorf("Failed to list pending tasks: %v", err)
		return
	}

	for _, m := range tasks {
		if m.TaskType != config.TaskTypeInferenceMetricsScrape {
			continue
		}

		// Check if already owned
		r.tasksMu.RLock()
		_, exists := r.activeTasks[m.WorkloadUID]
		r.tasksMu.RUnlock()
		if exists {
			continue
		}

		// Try to acquire
		acquired, err := r.facade.TryAcquireLock(r.ctx, m.WorkloadUID, config.TaskTypeInferenceMetricsScrape, r.cfg.InstanceID, r.cfg.LockDuration)
		if err != nil {
			log.Errorf("Failed to acquire lock for %s: %v", m.WorkloadUID, err)
			continue
		}

		if acquired {
			task, err := FromModel(m)
			if err != nil {
				log.Errorf("Failed to parse task %s: %v", m.WorkloadUID, err)
				continue
			}

			log.Infof("Acquired task %s (framework=%s)", task.WorkloadUID, task.Ext.Framework)
			r.addActiveTask(task)

			// Check capacity again
			r.tasksMu.RLock()
			currentCount = len(r.activeTasks)
			r.tasksMu.RUnlock()
			if currentCount >= r.cfg.MaxConcurrentScrapes {
				break
			}
		}
	}
}

// checkForCompletedTasks checks if any active tasks have been marked as completed
func (r *TaskReceiver) checkForCompletedTasks() {
	r.tasksMu.RLock()
	workloadUIDs := make([]string, 0, len(r.activeTasks))
	for uid := range r.activeTasks {
		workloadUIDs = append(workloadUIDs, uid)
	}
	r.tasksMu.RUnlock()

	for _, uid := range workloadUIDs {
		task, err := r.facade.GetTask(r.ctx, uid, config.TaskTypeInferenceMetricsScrape)
		if err != nil {
			log.Errorf("Failed to get task %s: %v", uid, err)
			continue
		}

		if task == nil || task.Status == constant.TaskStatusCompleted || task.Status == constant.TaskStatusCancelled {
			log.Infof("Task %s is completed/cancelled, removing", uid)
			r.removeActiveTask(uid)
		}
	}
}

// renewLocks periodically renews locks for active tasks
func (r *TaskReceiver) renewLocks() {
	defer r.wg.Done()

	ticker := time.NewTicker(r.cfg.LockRenewInterval)
	defer ticker.Stop()

	for {
		select {
		case <-r.ctx.Done():
			return
		case <-ticker.C:
			r.tasksMu.RLock()
			tasks := make([]*ScrapeTask, 0, len(r.activeTasks))
			for _, t := range r.activeTasks {
				tasks = append(tasks, t)
			}
			r.tasksMu.RUnlock()

			for _, task := range tasks {
				extended, err := r.facade.ExtendLock(r.ctx, task.WorkloadUID, config.TaskTypeInferenceMetricsScrape, r.cfg.InstanceID, r.cfg.LockDuration)
				if err != nil {
					log.Errorf("Failed to extend lock for %s: %v", task.WorkloadUID, err)
					continue
				}

				if !extended {
					log.Warnf("Lock lost for task %s, removing", task.WorkloadUID)
					r.removeActiveTask(task.WorkloadUID)
				}
			}
		}
	}
}

// cleanupStaleLocks periodically cleans up stale locks
func (r *TaskReceiver) cleanupStaleLocks() {
	defer r.wg.Done()

	// Run less frequently
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-r.ctx.Done():
			return
		case <-ticker.C:
			count, err := r.facade.ReleaseStaleLocks(r.ctx)
			if err != nil {
				log.Errorf("Failed to cleanup stale locks: %v", err)
			} else if count > 0 {
				log.Infof("Released %d stale locks", count)
			}
		}
	}
}

// addActiveTask adds a task and starts its scrape loop via ScrapeManager
func (r *TaskReceiver) addActiveTask(task *ScrapeTask) {
	r.tasksMu.Lock()
	r.activeTasks[task.WorkloadUID] = task
	r.tasksMu.Unlock()

	// Create target config from task
	cfg := &scraper.TargetConfig{
		WorkloadUID:    task.WorkloadUID,
		Framework:      task.Ext.Framework,
		Namespace:      task.Ext.Namespace,
		PodName:        task.Ext.PodName,
		PodIP:          task.Ext.PodIP,
		MetricsURL:     task.GetMetricsURL(),
		Labels:         task.Ext.Labels,
		ScrapeInterval: task.GetScrapeInterval(),
		ScrapeTimeout:  task.GetScrapeTimeout(),
	}

	// Add to scrape manager (starts scraping automatically)
	if err := r.scrapeManager.AddTarget(cfg); err != nil {
		log.Errorf("Failed to add scrape target for %s: %v", task.WorkloadUID, err)
	}

	// Update metrics
	exporter.UpdateScrapeTargets(len(r.activeTasks))
}

// removeActiveTask removes a task and stops its scrape loop via ScrapeManager
func (r *TaskReceiver) removeActiveTask(workloadUID string) {
	// Remove from scrape manager (stops scraping)
	if err := r.scrapeManager.RemoveTarget(workloadUID); err != nil {
		log.Errorf("Failed to remove scrape target for %s: %v", workloadUID, err)
	}

	// Remove from active tasks
	r.tasksMu.Lock()
	delete(r.activeTasks, workloadUID)
	r.tasksMu.Unlock()

	// Remove metrics for this workload
	r.exporter.RemoveMetrics(workloadUID)

	// Update metrics
	exporter.UpdateScrapeTargets(len(r.activeTasks))
}

// GetActiveTasks returns a copy of active tasks
func (r *TaskReceiver) GetActiveTasks() []*ScrapeTask {
	r.tasksMu.RLock()
	defer r.tasksMu.RUnlock()

	tasks := make([]*ScrapeTask, 0, len(r.activeTasks))
	for _, t := range r.activeTasks {
		tasks = append(tasks, t)
	}
	return tasks
}

// GetTask returns a specific active task
func (r *TaskReceiver) GetTask(workloadUID string) (*ScrapeTask, bool) {
	r.tasksMu.RLock()
	defer r.tasksMu.RUnlock()
	t, ok := r.activeTasks[workloadUID]
	return t, ok
}

// GetStats returns receiver statistics
func (r *TaskReceiver) GetStats() ReceiverStats {
	r.tasksMu.RLock()
	defer r.tasksMu.RUnlock()

	stats := ReceiverStats{
		InstanceID:   r.cfg.InstanceID,
		ActiveTasks:  len(r.activeTasks),
		MaxTasks:     r.cfg.MaxConcurrentScrapes,
		TasksByFramework: make(map[string]int),
	}

	for _, t := range r.activeTasks {
		stats.TasksByFramework[t.Ext.Framework]++
	}

	return stats
}

// ReceiverStats contains statistics about the receiver
type ReceiverStats struct {
	InstanceID       string         `json:"instance_id"`
	ActiveTasks      int            `json:"active_tasks"`
	MaxTasks         int            `json:"max_tasks"`
	TasksByFramework map[string]int `json:"tasks_by_framework"`
}

// GetScrapeManager returns the scrape manager for direct access
func (r *TaskReceiver) GetScrapeManager() *scraper.ScrapeManager {
	return r.scrapeManager
}

// GetScrapeTarget returns the ScrapeTarget for a specific workload
func (r *TaskReceiver) GetScrapeTarget(workloadUID string) (*scraper.ScrapeTarget, bool) {
	return r.scrapeManager.GetTarget(workloadUID)
}

// Ensure interface compliance
var _ fmt.Stringer = (*ScrapeTask)(nil)

func (t *ScrapeTask) String() string {
	return fmt.Sprintf("ScrapeTask{workloadUID=%s, framework=%s, status=%s}", t.WorkloadUID, t.Ext.Framework, t.Status)
}

