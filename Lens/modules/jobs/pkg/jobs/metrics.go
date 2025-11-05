package jobs

import (
	"context"
	"reflect"
	"time"

	"github.com/AMD-AGI/primus-lens/core/pkg/clientsets"
	"github.com/AMD-AGI/primus-lens/core/pkg/logger/log"
	"github.com/AMD-AGI/primus-lens/jobs/pkg/common"
	"github.com/prometheus/client_golang/prometheus"
)

var (
	// jobExecutionCount tracks the total number of executions for each job
	jobExecutionCount = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "primus_lens",
			Subsystem: "jobs",
			Name:      "execution_total",
			Help:      "Total number of job executions",
		},
		[]string{"job_name"},
	)

	// jobLastExecutionTimestamp tracks the timestamp of the last execution for each job
	jobLastExecutionTimestamp = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "primus_lens",
			Subsystem: "jobs",
			Name:      "last_execution_timestamp_seconds",
			Help:      "Timestamp of the last job execution in seconds since epoch",
		},
		[]string{"job_name"},
	)

	// jobExecutionDuration tracks the execution duration for each job (can calculate average and P90 automatically)
	jobExecutionDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: "primus_lens",
			Subsystem: "jobs",
			Name:      "execution_duration_seconds",
			Help:      "Duration of job execution in seconds",
			Buckets:   prometheus.ExponentialBuckets(0.1, 2, 15), // 0.1s ~ 1638s (27 minutes)
		},
		[]string{"job_name"},
	)

	// jobExecutionFailures tracks the number of failures for each job
	jobExecutionFailures = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "primus_lens",
			Subsystem: "jobs",
			Name:      "execution_failures_total",
			Help:      "Total number of job execution failures",
		},
		[]string{"job_name"},
	)
)

func init() {
	prometheus.MustRegister(jobExecutionCount)
	prometheus.MustRegister(jobLastExecutionTimestamp)
	prometheus.MustRegister(jobExecutionDuration)
	prometheus.MustRegister(jobExecutionFailures)
}

// getJobName returns the name of the job (using type name)
func getJobName(job Job) string {
	jobType := reflect.TypeOf(job)
	if jobType.Kind() == reflect.Ptr {
		jobType = jobType.Elem()
	}
	return jobType.Name()
}

// runJobWithMetrics executes the job and collects metrics
func runJobWithMetrics(ctx context.Context, job Job, k8sClient *clientsets.K8SClientSet, storageClient *clientsets.StorageClientSet) {
	jobName := getJobName(job)

	// Record start time
	startTime := time.Now()

	// Increment execution count
	jobExecutionCount.WithLabelValues(jobName).Inc()

	// Execute the job
	stats, err := job.Run(ctx, k8sClient, storageClient)

	// Record execution duration
	endTime := time.Now()
	duration := endTime.Sub(startTime)
	jobExecutionDuration.WithLabelValues(jobName).Observe(duration.Seconds())

	// Update last execution timestamp
	jobLastExecutionTimestamp.WithLabelValues(jobName).Set(float64(endTime.Unix()))

	// Build execution result
	result := &common.ExecutionResult{
		Success:   err == nil,
		Error:     err,
		Stats:     stats,
		StartTime: startTime,
		EndTime:   endTime,
		Duration:  duration.Seconds(),
	}

	// Record failure if execution failed
	if err != nil {
		jobExecutionFailures.WithLabelValues(jobName).Inc()
		log.Errorf("Job %s failed: %v (took %v)", jobName, err, duration)
	} else {
		log.Debugf("Job %s completed successfully (took %v)", jobName, duration)
		if stats != nil {
			log.Debugf("Job %s stats: processed=%d, created=%d, updated=%d, deleted=%d",
				jobName, stats.RecordsProcessed, stats.ItemsCreated, stats.ItemsUpdated, stats.ItemsDeleted)
		}
	}

	// Save execution history to database
	historyService := common.NewHistoryService()
	if err := historyService.RecordJobExecution(ctx, job, result); err != nil {
		log.Errorf("Failed to save execution history for job %s: %v", jobName, err)
	}
}
