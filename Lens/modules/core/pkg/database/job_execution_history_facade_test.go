package database

import (
	"context"
	"testing"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// mockJobExecutionHistoryFacade creates a JobExecutionHistoryFacade with the test database
type mockJobExecutionHistoryFacade struct {
	JobExecutionHistoryFacade
	db *gorm.DB
}

func (f *mockJobExecutionHistoryFacade) getDB() *gorm.DB {
	return f.db
}

// newTestJobExecutionHistoryFacade creates a test JobExecutionHistoryFacade
func newTestJobExecutionHistoryFacade(db *gorm.DB) JobExecutionHistoryFacadeInterface {
	return &mockJobExecutionHistoryFacade{
		db: db,
	}
}

// TestJobExecutionHistoryFacade_GetJobExecutionHistoryByID tests getting history by ID
func TestJobExecutionHistoryFacade_GetJobExecutionHistoryByID(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()
	
	facade := newTestJobExecutionHistoryFacade(helper.DB)
	ctx := helper.CreateTestContext()
	
	// Create a job execution history
	history := &model.JobExecutionHistory{
		JobName:         "test-job",
		JobType:         "sync",
		Status:          "success",
		StartedAt:       time.Now(),
		EndedAt:         time.Now().Add(5 * time.Minute),
		DurationSeconds: 300,
		ClusterName:     "test-cluster",
		Hostname:        "localhost",
	}
	err := helper.DB.Create(history).Error
	require.NoError(t, err)
	
	// Get the history
	result, err := facade.GetJobExecutionHistoryByID(ctx, history.ID)
	require.NoError(t, err)
	require.NotNil(t, result)
	
	assert.Equal(t, history.JobName, result.JobName)
	assert.Equal(t, history.Status, result.Status)
}

// TestJobExecutionHistoryFacade_GetJobExecutionHistoryByID_NotFound tests getting non-existent history
func TestJobExecutionHistoryFacade_GetJobExecutionHistoryByID_NotFound(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()
	
	facade := newTestJobExecutionHistoryFacade(helper.DB)
	ctx := helper.CreateTestContext()
	
	result, err := facade.GetJobExecutionHistoryByID(ctx, 99999)
	require.NoError(t, err)
	assert.Nil(t, result)
}

// TestJobExecutionHistoryFacade_ListJobExecutionHistories tests listing with filters
func TestJobExecutionHistoryFacade_ListJobExecutionHistories(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()
	
	facade := newTestJobExecutionHistoryFacade(helper.DB)
	ctx := helper.CreateTestContext()
	
	now := time.Now()
	
	// Create multiple job histories
	histories := []*model.JobExecutionHistory{
		{JobName: "job-1", JobType: "sync", Status: "success", StartedAt: now.Add(-3 * time.Hour), ClusterName: "cluster-1", Hostname: "host-1", DurationSeconds: 100},
		{JobName: "job-2", JobType: "cleanup", Status: "failed", StartedAt: now.Add(-2 * time.Hour), ClusterName: "cluster-1", Hostname: "host-1", DurationSeconds: 50},
		{JobName: "job-1", JobType: "sync", Status: "success", StartedAt: now.Add(-1 * time.Hour), ClusterName: "cluster-2", Hostname: "host-2", DurationSeconds: 120},
	}
	for _, h := range histories {
		h.EndedAt = h.StartedAt.Add(time.Duration(h.DurationSeconds) * time.Second)
		err := helper.DB.Create(h).Error
		require.NoError(t, err)
	}
	
	tests := []struct {
		name          string
		filter        *JobExecutionHistoryFilter
		expectedCount int
	}{
		{
			name:          "No filter",
			filter:        &JobExecutionHistoryFilter{},
			expectedCount: 3,
		},
		{
			name: "Filter by job name",
			filter: &JobExecutionHistoryFilter{
				JobName: stringPtr("job-1"),
			},
			expectedCount: 2,
		},
		{
			name: "Filter by status",
			filter: &JobExecutionHistoryFilter{
				Status: stringPtr("success"),
			},
			expectedCount: 2,
		},
		{
			name: "Filter by cluster",
			filter: &JobExecutionHistoryFilter{
				ClusterName: stringPtr("cluster-1"),
			},
			expectedCount: 2,
		},
		{
			name: "Filter by min duration",
			filter: &JobExecutionHistoryFilter{
				MinDuration: float64Ptr(100),
			},
			expectedCount: 2,
		},
		{
			name: "With pagination",
			filter: &JobExecutionHistoryFilter{
				Limit:  2,
				Offset: 0,
			},
			expectedCount: 2,
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			results, total, err := facade.ListJobExecutionHistories(ctx, tt.filter)
			require.NoError(t, err)
			assert.Len(t, results, tt.expectedCount)
			assert.Equal(t, int64(3), total)
		})
	}
}

// TestJobExecutionHistoryFacade_GetRecentFailures tests getting recent failed jobs
func TestJobExecutionHistoryFacade_GetRecentFailures(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()
	
	facade := newTestJobExecutionHistoryFacade(helper.DB)
	ctx := helper.CreateTestContext()
	
	now := time.Now()
	
	// Create job histories with different statuses
	histories := []*model.JobExecutionHistory{
		{JobName: "job-1", JobType: "sync", Status: "success", StartedAt: now.Add(-4 * time.Hour)},
		{JobName: "job-2", JobType: "cleanup", Status: "failed", StartedAt: now.Add(-3 * time.Hour)},
		{JobName: "job-3", JobType: "sync", Status: "timeout", StartedAt: now.Add(-2 * time.Hour)},
		{JobName: "job-4", JobType: "export", Status: "cancelled", StartedAt: now.Add(-1 * time.Hour)},
		{JobName: "job-5", JobType: "sync", Status: "success", StartedAt: now},
	}
	for _, h := range histories {
		err := helper.DB.Create(h).Error
		require.NoError(t, err)
	}
	
	// Get recent failures (limit 2)
	results, err := facade.GetRecentFailures(ctx, 2)
	require.NoError(t, err)
	assert.Len(t, results, 2)
	
	// Verify all are failures
	for _, r := range results {
		assert.Contains(t, []string{"failed", "timeout", "cancelled"}, r.Status)
	}
}

// TestJobExecutionHistoryFacade_GetJobStatistics tests getting job statistics
func TestJobExecutionHistoryFacade_GetJobStatistics(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()
	
	facade := newTestJobExecutionHistoryFacade(helper.DB)
	ctx := helper.CreateTestContext()
	
	jobName := "stat-test-job"
	now := time.Now()
	
	// Create multiple executions for the same job
	histories := []*model.JobExecutionHistory{
		{JobName: jobName, JobType: "sync", Status: "success", StartedAt: now.Add(-5 * time.Hour), DurationSeconds: 100},
		{JobName: jobName, JobType: "sync", Status: "success", StartedAt: now.Add(-4 * time.Hour), DurationSeconds: 120},
		{JobName: jobName, JobType: "sync", Status: "failed", StartedAt: now.Add(-3 * time.Hour), DurationSeconds: 50},
		{JobName: jobName, JobType: "sync", Status: "timeout", StartedAt: now.Add(-2 * time.Hour), DurationSeconds: 300},
		{JobName: jobName, JobType: "sync", Status: "cancelled", StartedAt: now.Add(-1 * time.Hour), DurationSeconds: 20},
	}
	for _, h := range histories {
		err := helper.DB.Create(h).Error
		require.NoError(t, err)
	}
	
	// Get statistics
	stats, err := facade.GetJobStatistics(ctx, jobName)
	require.NoError(t, err)
	require.NotNil(t, stats)
	
	assert.Equal(t, jobName, stats.JobName)
	assert.Equal(t, int64(5), stats.TotalRuns)
	assert.Equal(t, int64(2), stats.SuccessCount)
	assert.Equal(t, int64(1), stats.FailureCount)
	assert.Equal(t, int64(1), stats.TimeoutCount)
	assert.Equal(t, int64(1), stats.CancelledCount)
	assert.Greater(t, stats.AvgDuration, 0.0)
	assert.Equal(t, 20.0, stats.MinDuration)
	assert.Equal(t, 300.0, stats.MaxDuration)
	assert.Equal(t, "cancelled", stats.LastStatus)
}

// TestJobExecutionHistoryFacade_GetJobStatistics_NoHistory tests statistics for job with no history
func TestJobExecutionHistoryFacade_GetJobStatistics_NoHistory(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()
	
	facade := newTestJobExecutionHistoryFacade(helper.DB)
	ctx := helper.CreateTestContext()
	
	stats, err := facade.GetJobStatistics(ctx, "non-existent-job")
	require.NoError(t, err)
	require.NotNil(t, stats)
	
	assert.Equal(t, int64(0), stats.TotalRuns)
	assert.Equal(t, int64(0), stats.SuccessCount)
}

// TestJobExecutionHistoryFacade_GetDistinctJobTypes tests getting distinct job types
func TestJobExecutionHistoryFacade_GetDistinctJobTypes(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()
	
	facade := newTestJobExecutionHistoryFacade(helper.DB)
	ctx := helper.CreateTestContext()
	
	// Create histories with different job types
	histories := []*model.JobExecutionHistory{
		{JobName: "job-1", JobType: "sync", Status: "success", StartedAt: time.Now()},
		{JobName: "job-2", JobType: "cleanup", Status: "success", StartedAt: time.Now()},
		{JobName: "job-3", JobType: "sync", Status: "failed", StartedAt: time.Now()},
		{JobName: "job-4", JobType: "export", Status: "success", StartedAt: time.Now()},
	}
	for _, h := range histories {
		err := helper.DB.Create(h).Error
		require.NoError(t, err)
	}
	
	jobTypes, err := facade.GetDistinctJobTypes(ctx)
	require.NoError(t, err)
	
	assert.Len(t, jobTypes, 3)
	assert.Contains(t, jobTypes, "sync")
	assert.Contains(t, jobTypes, "cleanup")
	assert.Contains(t, jobTypes, "export")
}

// TestJobExecutionHistoryFacade_WithCluster tests the WithCluster method
func TestJobExecutionHistoryFacade_WithCluster(t *testing.T) {
	facade := NewJobExecutionHistoryFacade()
	
	clusterFacade := facade.WithCluster("test-cluster")
	
	require.NotNil(t, clusterFacade)
	assert.Implements(t, (*JobExecutionHistoryFacadeInterface)(nil), clusterFacade)
}

// ==================== Benchmark Tests ====================

func BenchmarkJobExecutionHistoryFacade_GetJobExecutionHistoryByID(b *testing.B) {
	helper := NewTestHelper(&testing.T{})
	defer helper.Cleanup()
	
	facade := newTestJobExecutionHistoryFacade(helper.DB)
	ctx := context.Background()
	
	// Pre-populate
	history := &model.JobExecutionHistory{
		JobName:   "bench-job",
		JobType:   "sync",
		Status:    "success",
		StartedAt: time.Now(),
	}
	_ = helper.DB.Create(history).Error
	
	b.ResetTimer()
	b.ReportAllocs()
	
	for i := 0; i < b.N; i++ {
		_, _ = facade.GetJobExecutionHistoryByID(ctx, history.ID)
	}
}

func BenchmarkJobExecutionHistoryFacade_ListJobExecutionHistories(b *testing.B) {
	helper := NewTestHelper(&testing.T{})
	defer helper.Cleanup()
	
	facade := newTestJobExecutionHistoryFacade(helper.DB)
	ctx := context.Background()
	
	// Pre-populate
	for i := 0; i < 50; i++ {
		history := &model.JobExecutionHistory{
			JobName:   "bench-job",
			JobType:   "sync",
			Status:    "success",
			StartedAt: time.Now().Add(-time.Duration(i) * time.Hour),
		}
		_ = helper.DB.Create(history).Error
	}
	
	filter := &JobExecutionHistoryFilter{Limit: 10}
	
	b.ResetTimer()
	b.ReportAllocs()
	
	for i := 0; i < b.N; i++ {
		_, _, _ = facade.ListJobExecutionHistories(ctx, filter)
	}
}

func BenchmarkJobExecutionHistoryFacade_GetJobStatistics(b *testing.B) {
	helper := NewTestHelper(&testing.T{})
	defer helper.Cleanup()
	
	facade := newTestJobExecutionHistoryFacade(helper.DB)
	ctx := context.Background()
	
	jobName := "bench-stat-job"
	
	// Pre-populate
	for i := 0; i < 20; i++ {
		status := "success"
		if i%3 == 0 {
			status = "failed"
		}
		history := &model.JobExecutionHistory{
			JobName:         jobName,
			JobType:         "sync",
			Status:          status,
			StartedAt:       time.Now().Add(-time.Duration(i) * time.Hour),
			DurationSeconds: float64(100 + i*10),
		}
		_ = helper.DB.Create(history).Error
	}
	
	b.ResetTimer()
	b.ReportAllocs()
	
	for i := 0; i < b.N; i++ {
		_, _ = facade.GetJobStatistics(ctx, jobName)
	}
}

