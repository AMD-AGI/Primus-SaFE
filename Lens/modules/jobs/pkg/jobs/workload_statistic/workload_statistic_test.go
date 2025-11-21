package workload_statistic

import (
	"encoding/json"
	"testing"
	"time"

	dbModel "github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
	"github.com/stretchr/testify/assert"
)

// Helper function to convert histogram JSON to ExtType
func histogramToExtType(hist *Histogram) dbModel.ExtType {
	histJSON, _ := hist.ToJSON()
	var histMap map[string]interface{}
	if err := json.Unmarshal(histJSON, &histMap); err != nil {
		return make(dbModel.ExtType)
	}
	return dbModel.ExtType(histMap)
}

func TestSchedule(t *testing.T) {
	job := NewWorkloadStatisticJob()
	schedule := job.Schedule()
	assert.Equal(t, "@every 30s", schedule, "Schedule should be every 30 seconds")
}

func TestCalculateIncrementalStartTime(t *testing.T) {
	job := NewWorkloadStatisticJob()
	endTime := time.Now()

	t.Run("New record - use workload creation time", func(t *testing.T) {
		workload := &dbModel.GpuWorkload{
			CreatedAt: endTime.Add(-2 * time.Hour),
		}
		record := &dbModel.WorkloadStatistic{
			LastQueryTime: time.Time{}, // Zero time indicates new record
		}

		startTime := job.calculateIncrementalStartTime(record, workload, endTime)
		duration := endTime.Sub(startTime)

		assert.InDelta(t, 2*time.Hour.Seconds(), duration.Seconds(), 1.0,
			"Should use workload creation time for new record")
	})

	t.Run("Existing record - use last query time", func(t *testing.T) {
		lastQueryTime := endTime.Add(-5 * time.Minute)
		workload := &dbModel.GpuWorkload{
			CreatedAt: endTime.Add(-2 * time.Hour),
		}
		record := &dbModel.WorkloadStatistic{
			LastQueryTime: lastQueryTime,
		}

		startTime := job.calculateIncrementalStartTime(record, workload, endTime)

		assert.Equal(t, lastQueryTime, startTime,
			"Should use last query time for existing record")
	})

	t.Run("Old workload - limit to max window", func(t *testing.T) {
		workload := &dbModel.GpuWorkload{
			CreatedAt: endTime.Add(-48 * time.Hour), // Very old
		}
		record := &dbModel.WorkloadStatistic{
			LastQueryTime: time.Time{},
		}

		startTime := job.calculateIncrementalStartTime(record, workload, endTime)
		duration := endTime.Sub(startTime)

		assert.InDelta(t, MaxQueryWindow.Seconds(), duration.Seconds(), 1.0,
			"Should limit to max window for very old workload")
	})
}

func TestUpdateStatisticsIncremental(t *testing.T) {
	job := NewWorkloadStatisticJob()

	t.Run("First update - new record", func(t *testing.T) {
		// Initialize empty histogram
		hist := NewHistogram()

		record := &dbModel.WorkloadStatistic{
			SampleCount:       0,
			TotalSum:          0,
			AvgGpuUtilization: 0,
			Histogram:         histogramToExtType(hist),
		}

		newValues := []float64{10, 20, 30, 40, 50}
		startTime := time.Now().Add(-1 * time.Hour)
		endTime := time.Now()

		job.updateStatisticsIncremental(record, newValues, startTime, endTime)

		assert.Equal(t, int32(5), record.SampleCount)
		assert.Equal(t, 150.0, record.TotalSum) // 10+20+30+40+50
		assert.Equal(t, 30.0, record.AvgGpuUtilization)
		assert.Equal(t, 10.0, record.MinGpuUtilization)
		assert.Equal(t, 50.0, record.MaxGpuUtilization)
		assert.Greater(t, record.P50GpuUtilization, 0.0)
	})

	t.Run("Incremental update - existing record", func(t *testing.T) {
		// Initialize record with existing data
		hist := NewHistogram()
		hist.AddValues([]float64{10, 20, 30, 40, 50})

		record := &dbModel.WorkloadStatistic{
			SampleCount:       5,
			TotalSum:          150, // 10+20+30+40+50
			AvgGpuUtilization: 30,
			MinGpuUtilization: 10,
			MaxGpuUtilization: 50,
			Histogram:         histogramToExtType(hist),
		}

		// Add new data
		newValues := []float64{60, 70, 80, 90, 100}
		startTime := time.Now().Add(-30 * time.Minute)
		endTime := time.Now()

		job.updateStatisticsIncremental(record, newValues, startTime, endTime)

		// Verify incremental update
		assert.Equal(t, int32(10), record.SampleCount) // 5 + 5
		assert.Equal(t, 550.0, record.TotalSum)        // 150 + 400
		assert.Equal(t, 55.0, record.AvgGpuUtilization)
		assert.Equal(t, 10.0, record.MinGpuUtilization)  // Keep original value
		assert.Equal(t, 100.0, record.MaxGpuUtilization) // Updated to new max
	})

	t.Run("Empty new values", func(t *testing.T) {
		hist := NewHistogram()

		record := &dbModel.WorkloadStatistic{
			SampleCount: 5,
			TotalSum:    150,
			Histogram:   histogramToExtType(hist),
		}

		originalCount := record.SampleCount
		job.updateStatisticsIncremental(record, []float64{}, time.Now(), time.Now())

		// Should have no changes
		assert.Equal(t, originalCount, record.SampleCount)
	})

	t.Run("Min/Max update", func(t *testing.T) {
		hist := NewHistogram()
		hist.AddValues([]float64{50})

		record := &dbModel.WorkloadStatistic{
			SampleCount:       1,
			TotalSum:          50,
			MinGpuUtilization: 50,
			MaxGpuUtilization: 50,
			Histogram:         histogramToExtType(hist),
		}

		// Add smaller and larger values
		newValues := []float64{10, 90}
		job.updateStatisticsIncremental(record, newValues, time.Now(), time.Now())

		assert.Equal(t, 10.0, record.MinGpuUtilization, "Min should be updated")
		assert.Equal(t, 90.0, record.MaxGpuUtilization, "Max should be updated")
	})
}

// TestCalculateQueryStartTime tests the old query start time calculation (deprecated)
// Kept for regression testing
func TestCalculateQueryStartTime(t *testing.T) {
	job := NewWorkloadStatisticJob()
	endTime := time.Now()

	tests := []struct {
		name              string
		workloadCreatedAt time.Time
		expectedDuration  time.Duration
		description       string
	}{
		{
			name:              "Recent workload - use creation time",
			workloadCreatedAt: endTime.Add(-2 * time.Hour),
			expectedDuration:  2 * time.Hour,
			description:       "Workload created 2 hours ago, should use creation time",
		},
		{
			name:              "Very old workload - use max window",
			workloadCreatedAt: endTime.Add(-48 * time.Hour),
			expectedDuration:  MaxQueryWindow,
			description:       "Workload created 48 hours ago, should limit to max window (24h)",
		},
		{
			name:              "Exactly at max window",
			workloadCreatedAt: endTime.Add(-MaxQueryWindow),
			expectedDuration:  MaxQueryWindow,
			description:       "Workload created exactly 24 hours ago",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			workload := &dbModel.GpuWorkload{
				CreatedAt: tt.workloadCreatedAt,
			}

			startTime := job.calculateQueryStartTime(workload, endTime)
			actualDuration := endTime.Sub(startTime)

			assert.InDelta(t, tt.expectedDuration.Seconds(), actualDuration.Seconds(), 1.0,
				"Duration mismatch for: %s", tt.description)
		})
	}
}
