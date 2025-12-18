package task

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// TaskQueuePendingTotal represents total pending tasks in the queue
	TaskQueuePendingTotal = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "lens",
			Subsystem: "task_scheduler",
			Name:      "pending_tasks_total",
			Help:      "Total number of pending tasks in the queue",
		},
		[]string{"task_type"},
	)

	// TaskQueueRunningTotal represents currently running tasks
	TaskQueueRunningTotal = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "lens",
			Subsystem: "task_scheduler",
			Name:      "running_tasks_total",
			Help:      "Total number of currently running tasks",
		},
		[]string{"task_type"},
	)

	// TaskQueueCapacity represents the max concurrent tasks allowed
	TaskQueueCapacity = promauto.NewGauge(
		prometheus.GaugeOpts{
			Namespace: "lens",
			Subsystem: "task_scheduler",
			Name:      "queue_capacity",
			Help:      "Maximum number of concurrent tasks allowed",
		},
	)

	// TaskQueueUtilization represents the utilization ratio (running/capacity)
	TaskQueueUtilization = promauto.NewGauge(
		prometheus.GaugeOpts{
			Namespace: "lens",
			Subsystem: "task_scheduler",
			Name:      "queue_utilization",
			Help:      "Queue utilization ratio (running tasks / capacity)",
		},
	)

	// TaskExecutionsTotal represents total task executions
	TaskExecutionsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "lens",
			Subsystem: "task_scheduler",
			Name:      "executions_total",
			Help:      "Total number of task executions",
		},
		[]string{"task_type", "status"},
	)

	// TaskExecutionDuration represents task execution duration
	TaskExecutionDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: "lens",
			Subsystem: "task_scheduler",
			Name:      "execution_duration_seconds",
			Help:      "Task execution duration in seconds",
			Buckets:   []float64{0.1, 0.5, 1, 5, 10, 30, 60, 120, 300, 600},
		},
		[]string{"task_type"},
	)

	// TaskQueueWaitingDuration represents how long tasks wait in queue
	TaskQueueWaitingDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: "lens",
			Subsystem: "task_scheduler",
			Name:      "queue_waiting_seconds",
			Help:      "Time tasks spend waiting in queue before execution",
			Buckets:   []float64{1, 5, 10, 30, 60, 120, 300, 600, 1800, 3600},
		},
		[]string{"task_type"},
	)

	// TaskLockAcquisitionFailures represents lock acquisition failures
	TaskLockAcquisitionFailures = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "lens",
			Subsystem: "task_scheduler",
			Name:      "lock_acquisition_failures_total",
			Help:      "Total number of lock acquisition failures",
		},
		[]string{"task_type"},
	)

	// TaskStaleLocksCleaned represents stale locks cleaned up
	TaskStaleLocksCleaned = promauto.NewCounter(
		prometheus.CounterOpts{
			Namespace: "lens",
			Subsystem: "task_scheduler",
			Name:      "stale_locks_cleaned_total",
			Help:      "Total number of stale locks cleaned up",
		},
	)
)

// updateQueueMetrics updates queue metrics based on current state
func updateQueueMetrics(running int, capacity int, pendingByType map[string]int, runningByType map[string]int) {
	// Update capacity
	TaskQueueCapacity.Set(float64(capacity))

	// Update utilization
	if capacity > 0 {
		TaskQueueUtilization.Set(float64(running) / float64(capacity))
	}

	// Update pending by type
	for taskType, count := range pendingByType {
		TaskQueuePendingTotal.WithLabelValues(taskType).Set(float64(count))
	}

	// Update running by type
	for taskType, count := range runningByType {
		TaskQueueRunningTotal.WithLabelValues(taskType).Set(float64(count))
	}
}
