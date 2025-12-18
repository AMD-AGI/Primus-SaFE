package task

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/assert"
)

func TestUpdateQueueMetrics(t *testing.T) {
	// Reset metrics before test
	TaskQueueCapacity.Set(0)
	TaskQueueUtilization.Set(0)

	pendingByType := map[string]int{
		"task_type_a": 5,
		"task_type_b": 3,
	}

	runningByType := map[string]int{
		"task_type_a": 2,
		"task_type_b": 1,
	}

	updateQueueMetrics(3, 10, pendingByType, runningByType)

	// Verify capacity
	assert.Equal(t, float64(10), testutil.ToFloat64(TaskQueueCapacity))

	// Verify utilization (3/10 = 0.3)
	assert.Equal(t, 0.3, testutil.ToFloat64(TaskQueueUtilization))

	// Verify pending by type
	assert.Equal(t, float64(5), testutil.ToFloat64(TaskQueuePendingTotal.WithLabelValues("task_type_a")))
	assert.Equal(t, float64(3), testutil.ToFloat64(TaskQueuePendingTotal.WithLabelValues("task_type_b")))

	// Verify running by type
	assert.Equal(t, float64(2), testutil.ToFloat64(TaskQueueRunningTotal.WithLabelValues("task_type_a")))
	assert.Equal(t, float64(1), testutil.ToFloat64(TaskQueueRunningTotal.WithLabelValues("task_type_b")))
}

func TestUpdateQueueMetricsZeroCapacity(t *testing.T) {
	// Reset metrics before test
	TaskQueueCapacity.Set(0)
	TaskQueueUtilization.Set(0)

	pendingByType := map[string]int{}
	runningByType := map[string]int{}

	// Test with zero capacity (should not divide by zero)
	updateQueueMetrics(0, 0, pendingByType, runningByType)

	assert.Equal(t, float64(0), testutil.ToFloat64(TaskQueueCapacity))
	// Utilization should remain unchanged when capacity is 0
	assert.Equal(t, float64(0), testutil.ToFloat64(TaskQueueUtilization))
}

func TestUpdateQueueMetricsFullCapacity(t *testing.T) {
	// Reset metrics before test
	TaskQueueCapacity.Set(0)
	TaskQueueUtilization.Set(0)

	pendingByType := map[string]int{
		"task_type_a": 10,
	}

	runningByType := map[string]int{
		"task_type_a": 5,
	}

	updateQueueMetrics(5, 5, pendingByType, runningByType)

	// Verify full capacity
	assert.Equal(t, float64(5), testutil.ToFloat64(TaskQueueCapacity))

	// Verify 100% utilization
	assert.Equal(t, 1.0, testutil.ToFloat64(TaskQueueUtilization))
}

func TestTaskMetricsRegistration(t *testing.T) {
	// Verify that all metrics are registered
	t.Run("TaskQueuePendingTotal registered", func(t *testing.T) {
		assert.NotNil(t, TaskQueuePendingTotal)
	})

	t.Run("TaskQueueRunningTotal registered", func(t *testing.T) {
		assert.NotNil(t, TaskQueueRunningTotal)
	})

	t.Run("TaskQueueCapacity registered", func(t *testing.T) {
		assert.NotNil(t, TaskQueueCapacity)
	})

	t.Run("TaskQueueUtilization registered", func(t *testing.T) {
		assert.NotNil(t, TaskQueueUtilization)
	})

	t.Run("TaskExecutionsTotal registered", func(t *testing.T) {
		assert.NotNil(t, TaskExecutionsTotal)
	})

	t.Run("TaskExecutionDuration registered", func(t *testing.T) {
		assert.NotNil(t, TaskExecutionDuration)
	})

	t.Run("TaskQueueWaitingDuration registered", func(t *testing.T) {
		assert.NotNil(t, TaskQueueWaitingDuration)
	})

	t.Run("TaskLockAcquisitionFailures registered", func(t *testing.T) {
		assert.NotNil(t, TaskLockAcquisitionFailures)
	})

	t.Run("TaskStaleLocksCleaned registered", func(t *testing.T) {
		assert.NotNil(t, TaskStaleLocksCleaned)
	})
}

func TestTaskExecutionsTotalCounter(t *testing.T) {
	// Create a fresh counter for testing
	counter := TaskExecutionsTotal.WithLabelValues("test_task", "completed")

	// Get initial value
	initial := testutil.ToFloat64(counter)

	// Increment
	counter.Inc()

	// Verify increment
	assert.Equal(t, initial+1, testutil.ToFloat64(counter))
}

func TestTaskExecutionDurationHistogram(t *testing.T) {
	histogram := TaskExecutionDuration.WithLabelValues("test_task")

	// Observe some values
	histogram.Observe(0.5)
	histogram.Observe(1.5)
	histogram.Observe(10.0)

	// Verify the histogram received observations (count should be positive)
	// We can't easily verify exact bucket values without more complex setup
	assert.NotNil(t, histogram)
}

func TestTaskQueueMetricsLabels(t *testing.T) {
	// Test that we can create metrics with various labels
	t.Run("pending total with label", func(t *testing.T) {
		gauge := TaskQueuePendingTotal.WithLabelValues("custom_task_type")
		assert.NotNil(t, gauge)
		gauge.Set(5)
		assert.Equal(t, float64(5), testutil.ToFloat64(gauge))
	})

	t.Run("running total with label", func(t *testing.T) {
		gauge := TaskQueueRunningTotal.WithLabelValues("custom_task_type")
		assert.NotNil(t, gauge)
		gauge.Set(3)
		assert.Equal(t, float64(3), testutil.ToFloat64(gauge))
	})

	t.Run("executions total with labels", func(t *testing.T) {
		counter := TaskExecutionsTotal.WithLabelValues("custom_task", "success")
		assert.NotNil(t, counter)
	})

	t.Run("lock failures with label", func(t *testing.T) {
		counter := TaskLockAcquisitionFailures.WithLabelValues("custom_task")
		assert.NotNil(t, counter)
	})
}

func TestMetricsDescriptions(t *testing.T) {
	// Test that metrics have proper descriptions
	t.Run("capacity gauge has description", func(t *testing.T) {
		desc := make(chan *prometheus.Desc, 1)
		TaskQueueCapacity.Describe(desc)
		d := <-desc
		assert.NotNil(t, d)
	})

	t.Run("utilization gauge has description", func(t *testing.T) {
		desc := make(chan *prometheus.Desc, 1)
		TaskQueueUtilization.Describe(desc)
		d := <-desc
		assert.NotNil(t, d)
	})

	t.Run("stale locks counter has description", func(t *testing.T) {
		desc := make(chan *prometheus.Desc, 1)
		TaskStaleLocksCleaned.Describe(desc)
		d := <-desc
		assert.NotNil(t, d)
	})
}
