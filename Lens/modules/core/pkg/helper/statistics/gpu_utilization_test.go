package statistics

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// ==================== ClusterGpuUtilizationStats Tests ====================

func TestClusterGpuUtilizationStats_Initialization(t *testing.T) {
	stats := &ClusterGpuUtilizationStats{
		AvgUtilization: 75.5,
		MaxUtilization: 95.0,
		MinUtilization: 50.0,
		P50Utilization: 73.0,
		P95Utilization: 92.0,
	}

	assert.Equal(t, 75.5, stats.AvgUtilization)
	assert.Equal(t, 95.0, stats.MaxUtilization)
	assert.Equal(t, 50.0, stats.MinUtilization)
	assert.Equal(t, 73.0, stats.P50Utilization)
	assert.Equal(t, 92.0, stats.P95Utilization)
}

func TestClusterGpuUtilizationStats_ZeroValues(t *testing.T) {
	stats := &ClusterGpuUtilizationStats{}

	assert.Equal(t, 0.0, stats.AvgUtilization)
	assert.Equal(t, 0.0, stats.MaxUtilization)
	assert.Equal(t, 0.0, stats.MinUtilization)
	assert.Equal(t, 0.0, stats.P50Utilization)
	assert.Equal(t, 0.0, stats.P95Utilization)
}

func TestClusterGpuUtilizationStats_HighUtilization(t *testing.T) {
	stats := &ClusterGpuUtilizationStats{
		AvgUtilization: 92.3,
		MaxUtilization: 99.8,
		MinUtilization: 85.1,
		P50Utilization: 93.5,
		P95Utilization: 98.2,
	}

	// Verify high utilization scenario
	assert.Greater(t, stats.AvgUtilization, 90.0)
	assert.Greater(t, stats.MaxUtilization, 95.0)
	assert.Greater(t, stats.MinUtilization, 80.0)
	assert.Greater(t, stats.P50Utilization, 90.0)
	assert.Greater(t, stats.P95Utilization, 95.0)
}

func TestClusterGpuUtilizationStats_LowUtilization(t *testing.T) {
	stats := &ClusterGpuUtilizationStats{
		AvgUtilization: 15.5,
		MaxUtilization: 30.0,
		MinUtilization: 5.0,
		P50Utilization: 14.0,
		P95Utilization: 28.0,
	}

	// Verify low utilization scenario
	assert.Less(t, stats.AvgUtilization, 20.0)
	assert.Less(t, stats.MaxUtilization, 40.0)
	assert.Less(t, stats.MinUtilization, 10.0)
	assert.Less(t, stats.P50Utilization, 20.0)
	assert.Less(t, stats.P95Utilization, 30.0)
}

func TestClusterGpuUtilizationStats_PercentileOrdering(t *testing.T) {
	stats := &ClusterGpuUtilizationStats{
		AvgUtilization: 60.0,
		MaxUtilization: 95.0,
		MinUtilization: 20.0,
		P50Utilization: 58.0,
		P95Utilization: 90.0,
	}

	// Verify logical ordering: min <= avg <= max
	assert.LessOrEqual(t, stats.MinUtilization, stats.AvgUtilization)
	assert.LessOrEqual(t, stats.AvgUtilization, stats.MaxUtilization)

	// P50 should typically be close to average
	assert.LessOrEqual(t, stats.P50Utilization, stats.MaxUtilization)
	assert.GreaterOrEqual(t, stats.P50Utilization, stats.MinUtilization)

	// P95 should be between average and max
	assert.LessOrEqual(t, stats.P95Utilization, stats.MaxUtilization)
	assert.GreaterOrEqual(t, stats.P95Utilization, stats.AvgUtilization)
}

func TestClusterGpuUtilizationStats_UniformDistribution(t *testing.T) {
	// Test case where all GPUs have similar utilization
	stats := &ClusterGpuUtilizationStats{
		AvgUtilization: 70.0,
		MaxUtilization: 72.0,
		MinUtilization: 68.0,
		P50Utilization: 70.0,
		P95Utilization: 71.5,
	}

	// In uniform distribution, all values should be close
	delta := 5.0
	assert.InDelta(t, stats.AvgUtilization, stats.P50Utilization, delta)
	assert.InDelta(t, stats.MaxUtilization, stats.MinUtilization, delta)
}

func TestClusterGpuUtilizationStats_SkewedDistribution(t *testing.T) {
	// Test case where utilization is skewed (some GPUs very busy, others idle)
	stats := &ClusterGpuUtilizationStats{
		AvgUtilization: 50.0,
		MaxUtilization: 98.0,
		MinUtilization: 2.0,
		P50Utilization: 45.0,
		P95Utilization: 95.0,
	}

	// Verify wide distribution
	distributionRange := stats.MaxUtilization - stats.MinUtilization
	assert.Greater(t, distributionRange, 80.0)

	// P95 should be much higher than average in right-skewed distribution
	assert.Greater(t, stats.P95Utilization, stats.AvgUtilization+20.0)
}

// ==================== Edge Cases Tests ====================

func TestClusterGpuUtilizationStats_AllGpusIdle(t *testing.T) {
	stats := &ClusterGpuUtilizationStats{
		AvgUtilization: 0.0,
		MaxUtilization: 0.0,
		MinUtilization: 0.0,
		P50Utilization: 0.0,
		P95Utilization: 0.0,
	}

	assert.Equal(t, 0.0, stats.AvgUtilization)
	assert.Equal(t, 0.0, stats.MaxUtilization)
	assert.Equal(t, 0.0, stats.MinUtilization)
}

func TestClusterGpuUtilizationStats_AllGpusFull(t *testing.T) {
	stats := &ClusterGpuUtilizationStats{
		AvgUtilization: 100.0,
		MaxUtilization: 100.0,
		MinUtilization: 100.0,
		P50Utilization: 100.0,
		P95Utilization: 100.0,
	}

	assert.Equal(t, 100.0, stats.AvgUtilization)
	assert.Equal(t, 100.0, stats.MaxUtilization)
	assert.Equal(t, 100.0, stats.MinUtilization)
}

func TestClusterGpuUtilizationStats_SingleGpu(t *testing.T) {
	// When there's only one GPU, all stats should be the same
	utilizationValue := 65.5
	stats := &ClusterGpuUtilizationStats{
		AvgUtilization: utilizationValue,
		MaxUtilization: utilizationValue,
		MinUtilization: utilizationValue,
		P50Utilization: utilizationValue,
		P95Utilization: utilizationValue,
	}

	assert.Equal(t, utilizationValue, stats.AvgUtilization)
	assert.Equal(t, utilizationValue, stats.MaxUtilization)
	assert.Equal(t, utilizationValue, stats.MinUtilization)
	assert.Equal(t, utilizationValue, stats.P50Utilization)
	assert.Equal(t, utilizationValue, stats.P95Utilization)
}

// ==================== GpuUtilizationResult Tests ====================

func TestGpuUtilizationResult_Initialization(t *testing.T) {
	result := &GpuUtilizationResult{
		AvgUtilization: 75.5,
	}

	assert.Equal(t, 75.5, result.AvgUtilization)
}

func TestGpuUtilizationResult_ZeroValue(t *testing.T) {
	result := &GpuUtilizationResult{}
	assert.Equal(t, 0.0, result.AvgUtilization)
}

