package api

import (
	"testing"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/model"
	"github.com/stretchr/testify/assert"
)

func TestGetGpuUsageHistoryCacheKey(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name        string
		startTime   time.Time
		endTime     time.Time
		expected    string
		description string
	}{
		{
			name:        "1 hour query - recent data",
			startTime:   now.Add(-1 * time.Hour),
			endTime:     now,
			expected:    "cluster:gpu:usage_history:1h",
			description: "Query data for the last 1 hour",
		},
		{
			name:        "6 hours query - recent data",
			startTime:   now.Add(-6 * time.Hour),
			endTime:     now,
			expected:    "cluster:gpu:usage_history:6h",
			description: "Query data for the last 6 hours",
		},
		{
			name:        "24 hours query - recent data",
			startTime:   now.Add(-24 * time.Hour),
			endTime:     now,
			expected:    "cluster:gpu:usage_history:24h",
			description: "Query data for the last 24 hours",
		},
		{
			name:        "55 minutes query - within 1 hour range",
			startTime:   now.Add(-55 * time.Minute),
			endTime:     now,
			expected:    "cluster:gpu:usage_history:1h",
			description: "55 minutes is close to 1 hour, should match 1 hour cache",
		},
		{
			name:        "65 minutes query - within 1 hour range",
			startTime:   now.Add(-65 * time.Minute),
			endTime:     now,
			expected:    "cluster:gpu:usage_history:1h",
			description: "65 minutes is close to 1 hour, should match 1 hour cache",
		},
		{
			name:        "5 hours 45 minutes - within 6 hours range",
			startTime:   now.Add(-5*time.Hour - 45*time.Minute),
			endTime:     now,
			expected:    "cluster:gpu:usage_history:6h",
			description: "Close to 6 hours, should match 6 hours cache",
		},
		{
			name:        "6 hours 15 minutes - within 6 hours range",
			startTime:   now.Add(-6*time.Hour - 15*time.Minute),
			endTime:     now,
			expected:    "cluster:gpu:usage_history:6h",
			description: "Close to 6 hours, should match 6 hours cache",
		},
		{
			name:        "23 hours 30 minutes - within 24 hours range",
			startTime:   now.Add(-23*time.Hour - 30*time.Minute),
			endTime:     now,
			expected:    "cluster:gpu:usage_history:24h",
			description: "Close to 24 hours, should match 24 hours cache",
		},
		{
			name:        "24 hours 30 minutes - within 24 hours range",
			startTime:   now.Add(-24*time.Hour - 30*time.Minute),
			endTime:     now,
			expected:    "cluster:gpu:usage_history:24h",
			description: "Close to 24 hours, should match 24 hours cache",
		},
		{
			name:        "2 hours query - no cache match",
			startTime:   now.Add(-2 * time.Hour),
			endTime:     now,
			expected:    "",
			description: "2 hours is not in any cache range",
		},
		{
			name:        "3 hours query - no cache match",
			startTime:   now.Add(-3 * time.Hour),
			endTime:     now,
			expected:    "",
			description: "3 hours is not in any cache range",
		},
		{
			name:        "12 hours query - no cache match",
			startTime:   now.Add(-12 * time.Hour),
			endTime:     now,
			expected:    "",
			description: "12 hours is not in any cache range",
		},
		{
			name:        "30 minutes query - too short",
			startTime:   now.Add(-30 * time.Minute),
			endTime:     now,
			expected:    "",
			description: "30 minutes is less than 1 hour range",
		},
		{
			name:        "45 minutes query - too short",
			startTime:   now.Add(-45 * time.Minute),
			endTime:     now,
			expected:    "",
			description: "45 minutes is less than 1 hour range",
		},
		{
			name:        "historical data - ended 10 minutes ago",
			startTime:   now.Add(-2 * time.Hour),
			endTime:     now.Add(-10 * time.Minute),
			expected:    "",
			description: "End time exceeds 5 minute tolerance, do not use cache",
		},
		{
			name:        "historical data - ended 1 hour ago",
			startTime:   now.Add(-2 * time.Hour),
			endTime:     now.Add(-1 * time.Hour),
			expected:    "",
			description: "Not a recent query, do not use cache",
		},
		{
			name:        "historical data - yesterday's 1 hour",
			startTime:   now.Add(-25 * time.Hour),
			endTime:     now.Add(-24 * time.Hour),
			expected:    "",
			description: "Yesterday's historical data, do not use cache",
		},
		{
			name:        "boundary test - ended exactly 5 minutes ago",
			startTime:   now.Add(-1*time.Hour - 5*time.Minute),
			endTime:     now.Add(-5 * time.Minute),
			expected:    "cluster:gpu:usage_history:1h", // timeSinceEnd > tolerance check, 5 minutes is not greater than, so it matches
			description: "Exactly within 5 minute tolerance boundary",
		},
		{
			name:        "boundary test - ended 4 minutes ago",
			startTime:   now.Add(-1*time.Hour - 4*time.Minute),
			endTime:     now.Add(-4 * time.Minute),
			expected:    "cluster:gpu:usage_history:1h",
			description: "Within 5 minute tolerance range",
		},
		{
			name:        "boundary test - ended 6 minutes ago",
			startTime:   now.Add(-1*time.Hour - 6*time.Minute),
			endTime:     now.Add(-6 * time.Minute),
			expected:    "",
			description: "Exceeds 5 minute tolerance, do not use cache",
		},
		{
			name:        "future time - do not use cache",
			startTime:   now.Add(1 * time.Hour),
			endTime:     now.Add(2 * time.Hour),
			expected:    "cluster:gpu:usage_history:1h", // Future time duration is also 1 hour, may match
			description: "Future time (may match duration)",
		},
		{
			name:        "boundary test - 50 minutes",
			startTime:   now.Add(-50 * time.Minute),
			endTime:     now,
			expected:    "cluster:gpu:usage_history:1h",
			description: "Exactly 50 minutes, matches 1 hour cache lower bound",
		},
		{
			name:        "boundary test - 70 minutes",
			startTime:   now.Add(-70 * time.Minute),
			endTime:     now,
			expected:    "cluster:gpu:usage_history:1h",
			description: "Exactly 70 minutes, matches 1 hour cache upper bound",
		},
		{
			name:        "boundary test - 49 minutes",
			startTime:   now.Add(-49 * time.Minute),
			endTime:     now,
			expected:    "",
			description: "49 minutes is less than 50 minute lower bound",
		},
		{
			name:        "boundary test - 71 minutes",
			startTime:   now.Add(-71 * time.Minute),
			endTime:     now,
			expected:    "",
			description: "71 minutes exceeds 70 minute upper bound",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getGpuUsageHistoryCacheKey(tt.startTime, tt.endTime)
			assert.Equal(t, tt.expected, result, tt.description)
		})
	}
}

func TestFilterTimePoints(t *testing.T) {
	tests := []struct {
		name      string
		points    []model.TimePoint
		startUnix int64
		endUnix   int64
		expected  []model.TimePoint
	}{
		{
			name: "all points within range",
			points: []model.TimePoint{
				{Timestamp: 100, Value: 10.0},
				{Timestamp: 200, Value: 20.0},
				{Timestamp: 300, Value: 30.0},
			},
			startUnix: 50,
			endUnix:   350,
			expected: []model.TimePoint{
				{Timestamp: 100, Value: 10.0},
				{Timestamp: 200, Value: 20.0},
				{Timestamp: 300, Value: 30.0},
			},
		},
		{
			name: "some points within range",
			points: []model.TimePoint{
				{Timestamp: 100, Value: 10.0},
				{Timestamp: 200, Value: 20.0},
				{Timestamp: 300, Value: 30.0},
				{Timestamp: 400, Value: 40.0},
				{Timestamp: 500, Value: 50.0},
			},
			startUnix: 200,
			endUnix:   400,
			expected: []model.TimePoint{
				{Timestamp: 200, Value: 20.0},
				{Timestamp: 300, Value: 30.0},
				{Timestamp: 400, Value: 40.0},
			},
		},
		{
			name: "no points within range",
			points: []model.TimePoint{
				{Timestamp: 100, Value: 10.0},
				{Timestamp: 200, Value: 20.0},
				{Timestamp: 300, Value: 30.0},
			},
			startUnix: 400,
			endUnix:   500,
			expected:  []model.TimePoint{},
		},
		{
			name:      "empty slice",
			points:    []model.TimePoint{},
			startUnix: 100,
			endUnix:   200,
			expected:  []model.TimePoint{},
		},
		{
			name: "single point - within range",
			points: []model.TimePoint{
				{Timestamp: 150, Value: 15.0},
			},
			startUnix: 100,
			endUnix:   200,
			expected: []model.TimePoint{
				{Timestamp: 150, Value: 15.0},
			},
		},
		{
			name: "single point - outside range",
			points: []model.TimePoint{
				{Timestamp: 250, Value: 25.0},
			},
			startUnix: 100,
			endUnix:   200,
			expected:  []model.TimePoint{},
		},
		{
			name: "boundary test - exactly at start point",
			points: []model.TimePoint{
				{Timestamp: 100, Value: 10.0},
				{Timestamp: 200, Value: 20.0},
			},
			startUnix: 100,
			endUnix:   200,
			expected: []model.TimePoint{
				{Timestamp: 100, Value: 10.0},
				{Timestamp: 200, Value: 20.0},
			},
		},
		{
			name: "boundary test - exactly at end point",
			points: []model.TimePoint{
				{Timestamp: 100, Value: 10.0},
				{Timestamp: 200, Value: 20.0},
			},
			startUnix: 50,
			endUnix:   200,
			expected: []model.TimePoint{
				{Timestamp: 100, Value: 10.0},
				{Timestamp: 200, Value: 20.0},
			},
		},
		{
			name: "boundary test - just beyond start point",
			points: []model.TimePoint{
				{Timestamp: 99, Value: 9.9},
				{Timestamp: 100, Value: 10.0},
				{Timestamp: 200, Value: 20.0},
			},
			startUnix: 100,
			endUnix:   300,
			expected: []model.TimePoint{
				{Timestamp: 100, Value: 10.0},
				{Timestamp: 200, Value: 20.0},
			},
		},
		{
			name: "boundary test - just beyond end point",
			points: []model.TimePoint{
				{Timestamp: 100, Value: 10.0},
				{Timestamp: 200, Value: 20.0},
				{Timestamp: 201, Value: 20.1},
			},
			startUnix: 50,
			endUnix:   200,
			expected: []model.TimePoint{
				{Timestamp: 100, Value: 10.0},
				{Timestamp: 200, Value: 20.0},
			},
		},
		{
			name: "negative timestamps",
			points: []model.TimePoint{
				{Timestamp: -200, Value: -20.0},
				{Timestamp: -100, Value: -10.0},
				{Timestamp: 0, Value: 0.0},
				{Timestamp: 100, Value: 10.0},
			},
			startUnix: -150,
			endUnix:   50,
			expected: []model.TimePoint{
				{Timestamp: -100, Value: -10.0},
				{Timestamp: 0, Value: 0.0},
			},
		},
		{
			name: "timestamp is zero",
			points: []model.TimePoint{
				{Timestamp: 0, Value: 0.0},
			},
			startUnix: -100,
			endUnix:   100,
			expected: []model.TimePoint{
				{Timestamp: 0, Value: 0.0},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := filterTimePoints(tt.points, tt.startUnix, tt.endUnix)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestFilterGpuUsageHistoryByTimeRange(t *testing.T) {
	now := time.Now()
	startTime := now.Add(-1 * time.Hour)
	endTime := now

	startUnix := startTime.Unix()
	endUnix := endTime.Unix()

	tests := []struct {
		name     string
		history  model.GpuUtilizationHistory
		expected model.GpuUtilizationHistory
	}{
		{
			name: "complete history data - all fields have data",
			history: model.GpuUtilizationHistory{
				AllocationRate: []model.TimePoint{
					{Timestamp: startUnix - 100, Value: 50.0},
					{Timestamp: startUnix + 100, Value: 60.0},
					{Timestamp: startUnix + 200, Value: 70.0},
					{Timestamp: endUnix + 100, Value: 80.0},
				},
				Utilization: []model.TimePoint{
					{Timestamp: startUnix - 100, Value: 40.0},
					{Timestamp: startUnix + 100, Value: 50.0},
					{Timestamp: startUnix + 200, Value: 60.0},
					{Timestamp: endUnix + 100, Value: 70.0},
				},
				VramUtilization: []model.TimePoint{
					{Timestamp: startUnix - 100, Value: 30.0},
					{Timestamp: startUnix + 100, Value: 40.0},
					{Timestamp: startUnix + 200, Value: 50.0},
					{Timestamp: endUnix + 100, Value: 60.0},
				},
			},
			expected: model.GpuUtilizationHistory{
				AllocationRate: []model.TimePoint{
					{Timestamp: startUnix + 100, Value: 60.0},
					{Timestamp: startUnix + 200, Value: 70.0},
				},
				Utilization: []model.TimePoint{
					{Timestamp: startUnix + 100, Value: 50.0},
					{Timestamp: startUnix + 200, Value: 60.0},
				},
				VramUtilization: []model.TimePoint{
					{Timestamp: startUnix + 100, Value: 40.0},
					{Timestamp: startUnix + 200, Value: 50.0},
				},
			},
		},
		{
			name: "empty history data",
			history: model.GpuUtilizationHistory{
				AllocationRate:  []model.TimePoint{},
				Utilization:     []model.TimePoint{},
				VramUtilization: []model.TimePoint{},
			},
			expected: model.GpuUtilizationHistory{
				AllocationRate:  []model.TimePoint{},
				Utilization:     []model.TimePoint{},
				VramUtilization: []model.TimePoint{},
			},
		},
		{
			name: "some fields have data",
			history: model.GpuUtilizationHistory{
				AllocationRate: []model.TimePoint{
					{Timestamp: startUnix + 100, Value: 60.0},
				},
				Utilization:     []model.TimePoint{},
				VramUtilization: []model.TimePoint{},
			},
			expected: model.GpuUtilizationHistory{
				AllocationRate: []model.TimePoint{
					{Timestamp: startUnix + 100, Value: 60.0},
				},
				Utilization:     []model.TimePoint{},
				VramUtilization: []model.TimePoint{},
			},
		},
		{
			name: "all data outside range",
			history: model.GpuUtilizationHistory{
				AllocationRate: []model.TimePoint{
					{Timestamp: startUnix - 200, Value: 50.0},
					{Timestamp: startUnix - 100, Value: 60.0},
				},
				Utilization: []model.TimePoint{
					{Timestamp: endUnix + 100, Value: 70.0},
					{Timestamp: endUnix + 200, Value: 80.0},
				},
				VramUtilization: []model.TimePoint{
					{Timestamp: startUnix - 300, Value: 30.0},
				},
			},
			expected: model.GpuUtilizationHistory{
				AllocationRate:  []model.TimePoint{},
				Utilization:     []model.TimePoint{},
				VramUtilization: []model.TimePoint{},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := filterGpuUsageHistoryByTimeRange(tt.history, startTime, endTime)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestFilterTimePoints_Performance(t *testing.T) {
	// Create large amount of data points
	largePoints := make([]model.TimePoint, 10000)
	for i := 0; i < 10000; i++ {
		largePoints[i] = model.TimePoint{
			Timestamp: int64(i * 60), // One point per minute
			Value:     float64(i),
		}
	}

	// Test filtering performance
	startUnix := int64(100 * 60)
	endUnix := int64(200 * 60)

	result := filterTimePoints(largePoints, startUnix, endUnix)

	// Verify result
	assert.Len(t, result, 101) // Should contain 100 to 200, a total of 101 points
	assert.Equal(t, startUnix, result[0].Timestamp)
	assert.Equal(t, endUnix, result[len(result)-1].Timestamp)
}

func BenchmarkFilterTimePoints(b *testing.B) {
	// Create test data
	points := make([]model.TimePoint, 1000)
	for i := 0; i < 1000; i++ {
		points[i] = model.TimePoint{
			Timestamp: int64(i * 60),
			Value:     float64(i),
		}
	}

	startUnix := int64(100 * 60)
	endUnix := int64(900 * 60)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = filterTimePoints(points, startUnix, endUnix)
	}
}

func BenchmarkFilterGpuUsageHistoryByTimeRange(b *testing.B) {
	now := time.Now()
	startTime := now.Add(-1 * time.Hour)
	endTime := now

	// Create test data
	points := make([]model.TimePoint, 100)
	for i := 0; i < 100; i++ {
		points[i] = model.TimePoint{
			Timestamp: startTime.Unix() + int64(i*60),
			Value:     float64(i),
		}
	}

	history := model.GpuUtilizationHistory{
		AllocationRate:  points,
		Utilization:     points,
		VramUtilization: points,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = filterGpuUsageHistoryByTimeRange(history, startTime, endTime)
	}
}

