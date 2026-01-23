// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package task

import (
	"context"
	"fmt"
	"strings"
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
	podFacade     database.PodFacadeInterface
	workloadFacade database.WorkloadFacadeInterface
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
		cfg:            cfg,
		facade:         database.NewWorkloadTaskFacade(),
		podFacade:      database.GetFacade().GetPod(),
		workloadFacade: database.GetFacade().GetWorkload(),
		exporter:       exp,
		scrapeManager:  scraper.NewScrapeManager(exp),
		activeTasks:    make(map[string]*ScrapeTask),
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
	// Enrich pod info if missing (fallback mechanism)
	if task.Ext.PodIP == "" || task.Ext.MetricsPort == 0 {
		log.Infof("Task %s missing pod info (PodIP=%s, MetricsPort=%d), attempting to enrich from database",
			task.WorkloadUID, task.Ext.PodIP, task.Ext.MetricsPort)
		if err := r.enrichPodInfo(task); err != nil {
			log.Warnf("Failed to enrich pod info for task %s: %v", task.WorkloadUID, err)
		}
	}

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

// enrichPodInfo tries to fill in missing pod information from database
// This is a fallback mechanism when ai-advisor didn't populate pod info
func (r *TaskReceiver) enrichPodInfo(task *ScrapeTask) error {
	ctx := r.ctx
	workloadUID := task.WorkloadUID

	// Try to find pods through workload_pod_reference table
	podRefs, err := r.workloadFacade.ListWorkloadPodReferenceByWorkloadUid(ctx, workloadUID)
	if err != nil {
		log.Warnf("Failed to query workload_pod_reference for workload %s: %v", workloadUID, err)
	}

	var podIP, podName, namespace, workloadName string

	if len(podRefs) > 0 {
		// Query pod details through pod UID list
		for _, ref := range podRefs {
			pod, err := r.podFacade.GetGpuPodsByPodUid(ctx, ref.PodUID)
			if err != nil || pod == nil {
				continue
			}
			if !pod.Deleted && pod.Running && pod.IP != "" {
				podIP = pod.IP
				podName = pod.Name
				namespace = pod.Namespace
				break
			}
		}
	}

	// If no pods found through references, try child workloads
	if podIP == "" {
		childWorkloads, err := r.workloadFacade.ListChildrenWorkloadByParentUid(ctx, workloadUID)
		if err != nil {
			log.Debugf("Failed to query child workloads for %s: %v", workloadUID, err)
		} else {
			for _, child := range childWorkloads {
				childPodRefs, err := r.workloadFacade.ListWorkloadPodReferenceByWorkloadUid(ctx, child.UID)
				if err != nil {
					continue
				}
				for _, ref := range childPodRefs {
					pod, err := r.podFacade.GetGpuPodsByPodUid(ctx, ref.PodUID)
					if err != nil || pod == nil {
						continue
					}
					if !pod.Deleted && pod.Running && pod.IP != "" {
						podIP = pod.IP
						podName = pod.Name
						namespace = pod.Namespace
						break
					}
				}
				if podIP != "" {
					break
				}
			}
		}
	}

	if podIP == "" {
		return fmt.Errorf("no running pod with IP found for workload %s", workloadUID)
	}

	// Get workload name
	workload, err := r.workloadFacade.GetGpuWorkloadByUid(ctx, workloadUID)
	if err == nil && workload != nil {
		workloadName = workload.Name
	}

	// Update task ext with pod info
	task.Ext.PodIP = podIP
	task.Ext.PodName = podName
	task.Ext.Namespace = namespace

	// Set default metrics port if not set
	if task.Ext.MetricsPort == 0 {
		task.Ext.MetricsPort = r.getDefaultMetricsPort(task.Ext.Framework)
	}

	// Set default metrics path if not set
	if task.Ext.MetricsPath == "" {
		task.Ext.MetricsPath = "/metrics"
	}

	// Update labels
	if task.Ext.Labels == nil {
		task.Ext.Labels = make(map[string]string)
	}
	task.Ext.Labels["namespace"] = namespace
	task.Ext.Labels["pod_name"] = podName
	task.Ext.Labels["workload_uid"] = workloadUID
	task.Ext.Labels["workload_name"] = workloadName
	task.Ext.Labels["framework"] = task.Ext.Framework

	log.Infof("Enriched pod info for task %s: pod=%s/%s, ip=%s, port=%d",
		workloadUID, namespace, podName, podIP, task.Ext.MetricsPort)

	// Update the task in database so we don't need to re-enrich next time
	if err := r.updateTaskExtInDB(task); err != nil {
		log.Warnf("Failed to update task ext in database for %s: %v", workloadUID, err)
		// Continue anyway - we have the info in memory
	}

	return nil
}

// getDefaultMetricsPort returns the default metrics port for a framework
func (r *TaskReceiver) getDefaultMetricsPort(framework string) int {
	switch {
	case contains(framework, "vllm"):
		return 8000
	case contains(framework, "tgi"):
		return 8080
	case contains(framework, "triton"):
		return 8002
	case contains(framework, "tensorrt"):
		return 8000
	case contains(framework, "tei"), contains(framework, "text-embeddings"):
		return 8000
	default:
		return 8000
	}
}

// contains checks if s contains substr (case-insensitive)
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsLower(s, substr))
}

func containsLower(s, substr string) bool {
	s = strings.ToLower(s)
	substr = strings.ToLower(substr)
	return strings.Contains(s, substr)
}

// updateTaskExtInDB updates the task's ext field in database
func (r *TaskReceiver) updateTaskExtInDB(task *ScrapeTask) error {
	extMap := task.Ext.ToExtMap()
	return r.facade.UpdateTaskExt(r.ctx, task.WorkloadUID, config.TaskTypeInferenceMetricsScrape, extMap)
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

