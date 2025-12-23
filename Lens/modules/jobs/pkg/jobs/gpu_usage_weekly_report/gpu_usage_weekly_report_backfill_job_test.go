package gpu_usage_weekly_report

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/config"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database"
	dbmodel "github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// setupMockDependencies creates mock dependencies for testing
func setupMockDependencies() (*Dependencies, *database.MockFacade, *MockClusterManager) {
	mockFacade := database.NewMockFacade()
	mockClusterManager := NewMockClusterManager()

	deps := &Dependencies{
		ClusterManager: mockClusterManager,
		DatabaseFacade: mockFacade,
		DBConnectionProvider: &MockDBConnectionProvider{
			GetDBForClusterFunc: func(clusterName string) (*gorm.DB, error) {
				return nil, nil
			},
		},
		GeneratorFactory: func() ReportGeneratorInterface {
			return &MockReportGenerator{}
		},
		RendererFactory: func() ReportRendererInterface {
			return &MockReportRenderer{}
		},
	}

	return deps, mockFacade, mockClusterManager
}

// TestNewGpuUsageWeeklyReportBackfillJob tests the constructor
func TestNewGpuUsageWeeklyReportBackfillJob(t *testing.T) {
	t.Run("creates job with nil config", func(t *testing.T) {
		job := NewGpuUsageWeeklyReportBackfillJob(nil)
		assert.NotNil(t, job)
		assert.Nil(t, job.config)
	})

	t.Run("creates job with config", func(t *testing.T) {
		cfg := &GpuUsageWeeklyReportBackfillConfig{
			Enabled: true,
			Cron:    "0 4 * * *",
		}
		job := NewGpuUsageWeeklyReportBackfillJob(cfg)
		assert.NotNil(t, job)
		assert.Equal(t, cfg, job.config)
	})
}

// TestNewGpuUsageWeeklyReportBackfillJobWithDeps tests constructor with dependencies
func TestNewGpuUsageWeeklyReportBackfillJobWithDeps(t *testing.T) {
	t.Run("creates job with dependencies", func(t *testing.T) {
		mockClusterManager := NewMockClusterManager()
		deps := &Dependencies{
			ClusterManager: mockClusterManager,
		}
		cfg := &GpuUsageWeeklyReportBackfillConfig{
			Enabled: true,
		}

		job := NewGpuUsageWeeklyReportBackfillJobWithDeps(cfg, deps)
		assert.NotNil(t, job)
		assert.Equal(t, mockClusterManager, job.clusterManager)
	})

	t.Run("creates job with nil dependencies", func(t *testing.T) {
		cfg := &GpuUsageWeeklyReportBackfillConfig{
			Enabled: true,
		}

		job := NewGpuUsageWeeklyReportBackfillJobWithDeps(cfg, nil)
		assert.NotNil(t, job)
		assert.Nil(t, job.clusterManager)
	})
}

// TestSchedule tests the Schedule method
func TestSchedule(t *testing.T) {
	t.Run("returns default schedule when config is nil", func(t *testing.T) {
		job := NewGpuUsageWeeklyReportBackfillJob(nil)
		assert.Equal(t, "0 3 * * *", job.Schedule())
	})

	t.Run("returns default schedule when cron is empty", func(t *testing.T) {
		cfg := &GpuUsageWeeklyReportBackfillConfig{
			Enabled: true,
		}
		job := NewGpuUsageWeeklyReportBackfillJob(cfg)
		assert.Equal(t, "0 3 * * *", job.Schedule())
	})

	t.Run("returns configured schedule", func(t *testing.T) {
		cfg := &GpuUsageWeeklyReportBackfillConfig{
			Cron: "0 4 * * 1",
		}
		job := NewGpuUsageWeeklyReportBackfillJob(cfg)
		assert.Equal(t, "0 4 * * 1", job.Schedule())
	})
}

// TestShouldRenderPDF tests the shouldRenderPDF method
func TestShouldRenderPDF(t *testing.T) {
	t.Run("returns true when config is nil", func(t *testing.T) {
		job := NewGpuUsageWeeklyReportBackfillJob(nil)
		assert.True(t, job.shouldRenderPDF())
	})

	t.Run("returns true when WeeklyReportConfig is nil", func(t *testing.T) {
		cfg := &GpuUsageWeeklyReportBackfillConfig{
			Enabled: true,
		}
		job := NewGpuUsageWeeklyReportBackfillJob(cfg)
		assert.True(t, job.shouldRenderPDF())
	})

	t.Run("returns true when output formats include pdf", func(t *testing.T) {
		cfg := &GpuUsageWeeklyReportBackfillConfig{
			WeeklyReportConfig: &config.WeeklyReportConfig{
				OutputFormats: []string{"html", "pdf"},
			},
		}
		job := NewGpuUsageWeeklyReportBackfillJob(cfg)
		assert.True(t, job.shouldRenderPDF())
	})

	t.Run("returns false when output formats exclude pdf", func(t *testing.T) {
		cfg := &GpuUsageWeeklyReportBackfillConfig{
			WeeklyReportConfig: &config.WeeklyReportConfig{
				OutputFormats: []string{"html"},
			},
		}
		job := NewGpuUsageWeeklyReportBackfillJob(cfg)
		assert.False(t, job.shouldRenderPDF())
	})

	t.Run("returns true when output formats is empty", func(t *testing.T) {
		cfg := &GpuUsageWeeklyReportBackfillConfig{
			WeeklyReportConfig: &config.WeeklyReportConfig{
				OutputFormats: []string{},
			},
		}
		job := NewGpuUsageWeeklyReportBackfillJob(cfg)
		assert.True(t, job.shouldRenderPDF())
	})
}

// TestGetNextMonday tests the getNextMonday method
func TestGetNextMonday(t *testing.T) {
	job := NewGpuUsageWeeklyReportBackfillJob(nil)

	testCases := []struct {
		name     string
		input    time.Time
		expected time.Time
	}{
		{
			name:     "Monday at midnight",
			input:    time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), // Monday
			expected: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
		},
		{
			name:     "Monday at noon",
			input:    time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC), // Monday
			expected: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
		},
		{
			name:     "Tuesday",
			input:    time.Date(2024, 1, 2, 10, 0, 0, 0, time.UTC), // Tuesday
			expected: time.Date(2024, 1, 8, 0, 0, 0, 0, time.UTC),  // Next Monday
		},
		{
			name:     "Wednesday",
			input:    time.Date(2024, 1, 3, 10, 0, 0, 0, time.UTC), // Wednesday
			expected: time.Date(2024, 1, 8, 0, 0, 0, 0, time.UTC),  // Next Monday
		},
		{
			name:     "Sunday",
			input:    time.Date(2024, 1, 7, 23, 59, 59, 0, time.UTC), // Sunday
			expected: time.Date(2024, 1, 8, 0, 0, 0, 0, time.UTC),    // Next Monday
		},
		{
			name:     "Saturday",
			input:    time.Date(2024, 1, 6, 15, 30, 0, 0, time.UTC), // Saturday
			expected: time.Date(2024, 1, 8, 0, 0, 0, 0, time.UTC),   // Next Monday
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := job.getNextMonday(tc.input)
			assert.Equal(t, tc.expected, result)
		})
	}
}

// TestCalculateNaturalWeeks tests the calculateNaturalWeeks method
func TestCalculateNaturalWeeks(t *testing.T) {
	job := NewGpuUsageWeeklyReportBackfillJob(nil)

	t.Run("returns empty when no complete week", func(t *testing.T) {
		// Only 3 days of data
		minTime := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)  // Monday
		maxTime := time.Date(2024, 1, 3, 23, 59, 59, 0, time.UTC) // Wednesday

		weeks := job.calculateNaturalWeeks(minTime, maxTime)
		assert.Empty(t, weeks)
	})

	t.Run("returns one week when exactly one complete week", func(t *testing.T) {
		minTime := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)  // Monday
		maxTime := time.Date(2024, 1, 7, 23, 59, 59, 999999999, time.UTC) // Sunday

		weeks := job.calculateNaturalWeeks(minTime, maxTime)
		assert.Len(t, weeks, 1)
		assert.Equal(t, time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), weeks[0].StartTime)
	})

	t.Run("returns multiple weeks", func(t *testing.T) {
		minTime := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)   // Monday
		maxTime := time.Date(2024, 1, 21, 23, 59, 59, 999999999, time.UTC) // 3rd Sunday

		weeks := job.calculateNaturalWeeks(minTime, maxTime)
		assert.Len(t, weeks, 3)
	})

	t.Run("starts from first Monday after minTime", func(t *testing.T) {
		minTime := time.Date(2024, 1, 3, 0, 0, 0, 0, time.UTC)  // Wednesday
		maxTime := time.Date(2024, 1, 21, 23, 59, 59, 999999999, time.UTC) // 3rd Sunday

		weeks := job.calculateNaturalWeeks(minTime, maxTime)
		assert.Len(t, weeks, 2) // Only 2 complete weeks starting from Jan 8
		assert.Equal(t, time.Date(2024, 1, 8, 0, 0, 0, 0, time.UTC), weeks[0].StartTime)
	})
}

// TestFindMissingWeeks tests the findMissingWeeks method
func TestFindMissingWeeks(t *testing.T) {
	job := NewGpuUsageWeeklyReportBackfillJob(nil)

	t.Run("returns all weeks when no existing reports", func(t *testing.T) {
		weeks := []ReportPeriod{
			{StartTime: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), EndTime: time.Date(2024, 1, 7, 23, 59, 59, 0, time.UTC)},
			{StartTime: time.Date(2024, 1, 8, 0, 0, 0, 0, time.UTC), EndTime: time.Date(2024, 1, 14, 23, 59, 59, 0, time.UTC)},
		}
		existingReports := []*dbmodel.GpuUsageWeeklyReports{}

		missing := job.findMissingWeeks(weeks, existingReports)
		assert.Len(t, missing, 2)
	})

	t.Run("returns empty when all weeks have reports", func(t *testing.T) {
		weeks := []ReportPeriod{
			{StartTime: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), EndTime: time.Date(2024, 1, 7, 23, 59, 59, 0, time.UTC)},
		}
		existingReports := []*dbmodel.GpuUsageWeeklyReports{
			{PeriodStart: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), Status: "generated"},
		}

		missing := job.findMissingWeeks(weeks, existingReports)
		assert.Empty(t, missing)
	})

	t.Run("ignores failed reports", func(t *testing.T) {
		weeks := []ReportPeriod{
			{StartTime: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), EndTime: time.Date(2024, 1, 7, 23, 59, 59, 0, time.UTC)},
		}
		existingReports := []*dbmodel.GpuUsageWeeklyReports{
			{PeriodStart: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), Status: "failed"},
		}

		missing := job.findMissingWeeks(weeks, existingReports)
		assert.Len(t, missing, 1)
	})

	t.Run("considers sent status as completed", func(t *testing.T) {
		weeks := []ReportPeriod{
			{StartTime: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), EndTime: time.Date(2024, 1, 7, 23, 59, 59, 0, time.UTC)},
		}
		existingReports := []*dbmodel.GpuUsageWeeklyReports{
			{PeriodStart: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), Status: "sent"},
		}

		missing := job.findMissingWeeks(weeks, existingReports)
		assert.Empty(t, missing)
	})
}

// TestRunWithDisabledConfig tests Run when job is disabled
func TestRunWithDisabledConfig(t *testing.T) {
	t.Run("skips when config is nil", func(t *testing.T) {
		job := NewGpuUsageWeeklyReportBackfillJob(nil)
		stats, err := job.Run(context.Background(), nil, nil)

		require.NoError(t, err)
		assert.NotNil(t, stats)
	})

	t.Run("skips when disabled", func(t *testing.T) {
		cfg := &GpuUsageWeeklyReportBackfillConfig{
			Enabled: false,
		}
		job := NewGpuUsageWeeklyReportBackfillJob(cfg)
		stats, err := job.Run(context.Background(), nil, nil)

		require.NoError(t, err)
		assert.NotNil(t, stats)
	})
}

// TestRunWithMockedDependencies tests Run with mocked dependencies
func TestRunWithMockedDependencies(t *testing.T) {
	t.Run("returns early when no clusters found", func(t *testing.T) {
		mockClusterManager := NewMockClusterManager()
		// No clusters added

		deps := &Dependencies{
			ClusterManager: mockClusterManager,
		}

		cfg := &GpuUsageWeeklyReportBackfillConfig{
			Enabled: true,
		}

		job := NewGpuUsageWeeklyReportBackfillJobWithDeps(cfg, deps)
		stats, err := job.Run(context.Background(), nil, nil)

		require.NoError(t, err)
		assert.NotNil(t, stats)
		assert.Equal(t, int64(0), stats.ItemsCreated)
	})
}

// TestSetDependencies tests the SetDependencies method
func TestSetDependencies(t *testing.T) {
	t.Run("sets all dependencies", func(t *testing.T) {
		job := NewGpuUsageWeeklyReportBackfillJob(nil)

		mockClusterManager := NewMockClusterManager()
		mockFacade := database.NewMockFacade()

		deps := &Dependencies{
			ClusterManager: mockClusterManager,
			DatabaseFacade: mockFacade,
		}

		job.SetDependencies(deps)

		assert.Equal(t, mockClusterManager, job.clusterManager)
		assert.Equal(t, mockFacade, job.databaseFacade)
	})

	t.Run("handles nil dependencies", func(t *testing.T) {
		job := NewGpuUsageWeeklyReportBackfillJob(nil)
		job.SetDependencies(nil)
		// Should not panic
		assert.Nil(t, job.clusterManager)
	})
}

// TestMockReportGenerator tests the MockReportGenerator
func TestMockReportGenerator(t *testing.T) {
	t.Run("returns default report when GenerateFunc is nil", func(t *testing.T) {
		mock := &MockReportGenerator{}
		period := ReportPeriod{
			StartTime: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
			EndTime:   time.Date(2024, 1, 7, 23, 59, 59, 0, time.UTC),
		}

		data, err := mock.Generate(context.Background(), "test-cluster", period)

		require.NoError(t, err)
		assert.NotNil(t, data)
		assert.Equal(t, "test-cluster", data.ClusterName)
		assert.NotNil(t, data.Summary)
	})

	t.Run("uses custom GenerateFunc when provided", func(t *testing.T) {
		expectedData := &ReportData{
			ClusterName: "custom-cluster",
		}
		mock := &MockReportGenerator{
			GenerateFunc: func(ctx context.Context, clusterName string, period ReportPeriod) (*ReportData, error) {
				return expectedData, nil
			},
		}

		data, err := mock.Generate(context.Background(), "any", ReportPeriod{})

		require.NoError(t, err)
		assert.Equal(t, expectedData, data)
	})

	t.Run("returns error from custom GenerateFunc", func(t *testing.T) {
		expectedErr := errors.New("generation failed")
		mock := &MockReportGenerator{
			GenerateFunc: func(ctx context.Context, clusterName string, period ReportPeriod) (*ReportData, error) {
				return nil, expectedErr
			},
		}

		data, err := mock.Generate(context.Background(), "any", ReportPeriod{})

		assert.Nil(t, data)
		assert.Equal(t, expectedErr, err)
	})
}

// TestMockReportRenderer tests the MockReportRenderer
func TestMockReportRenderer(t *testing.T) {
	t.Run("returns default HTML when RenderHTMLFunc is nil", func(t *testing.T) {
		mock := &MockReportRenderer{}
		data := &ReportData{}

		html, err := mock.RenderHTML(context.Background(), data)

		require.NoError(t, err)
		assert.Contains(t, string(html), "Mock HTML Report")
	})

	t.Run("returns default PDF when RenderPDFFunc is nil", func(t *testing.T) {
		mock := &MockReportRenderer{}

		pdf, err := mock.RenderPDF(context.Background(), []byte("<html></html>"))

		require.NoError(t, err)
		assert.Equal(t, []byte("mock pdf content"), pdf)
	})

	t.Run("uses custom RenderHTMLFunc when provided", func(t *testing.T) {
		expectedHTML := []byte("<html>custom</html>")
		mock := &MockReportRenderer{
			RenderHTMLFunc: func(ctx context.Context, data *ReportData) ([]byte, error) {
				return expectedHTML, nil
			},
		}

		html, err := mock.RenderHTML(context.Background(), nil)

		require.NoError(t, err)
		assert.Equal(t, expectedHTML, html)
	})
}

// TestMockClusterManager tests the MockClusterManager
func TestMockClusterManager(t *testing.T) {
	t.Run("returns empty cluster names when no clusters", func(t *testing.T) {
		mock := NewMockClusterManager()

		names := mock.GetClusterNames()

		assert.Empty(t, names)
	})

	t.Run("returns cluster names after adding clusters", func(t *testing.T) {
		mock := NewMockClusterManager()
		mock.AddCluster("cluster-1")
		mock.AddCluster("cluster-2")

		names := mock.GetClusterNames()

		assert.Len(t, names, 2)
		assert.Contains(t, names, "cluster-1")
		assert.Contains(t, names, "cluster-2")
	})

	t.Run("uses custom GetClusterNamesFunc when provided", func(t *testing.T) {
		mock := NewMockClusterManager()
		mock.GetClusterNamesFunc = func() []string {
			return []string{"custom-cluster"}
		}

		names := mock.GetClusterNames()

		assert.Equal(t, []string{"custom-cluster"}, names)
	})

	t.Run("GetClientSetByClusterName returns nil for unknown cluster", func(t *testing.T) {
		mock := NewMockClusterManager()

		cs, err := mock.GetClientSetByClusterName("unknown")

		assert.Nil(t, cs)
		assert.Nil(t, err)
	})

	t.Run("GetClientSetByClusterName returns clientset for known cluster", func(t *testing.T) {
		mock := NewMockClusterManager()
		mock.AddCluster("test-cluster")

		cs, err := mock.GetClientSetByClusterName("test-cluster")

		assert.NotNil(t, cs)
		assert.Nil(t, err)
		assert.Equal(t, "test-cluster", cs.ClusterName)
	})
}

// TestReportDataToExtType tests the ToExtType method
func TestReportDataToExtType(t *testing.T) {
	t.Run("converts report data to ExtType", func(t *testing.T) {
		data := &ReportData{
			ClusterName:    "test-cluster",
			MarkdownReport: "# Test Report",
			Summary: &ReportSummary{
				TotalGPUs:      100,
				AvgUtilization: 50.5,
			},
		}

		extType := data.ToExtType()

		assert.NotNil(t, extType)
	})
}

// TestReportDataGenerateMetadata tests the GenerateMetadata method
func TestReportDataGenerateMetadata(t *testing.T) {
	t.Run("generates metadata without summary", func(t *testing.T) {
		data := &ReportData{
			ClusterName: "test-cluster",
		}

		metadata := data.GenerateMetadata()

		assert.NotNil(t, metadata)
		assert.Equal(t, "test-cluster", metadata["cluster_name"])
	})

	t.Run("generates metadata with summary", func(t *testing.T) {
		data := &ReportData{
			ClusterName: "test-cluster",
			Summary: &ReportSummary{
				TotalGPUs:      100,
				AvgUtilization: 50.5,
				AvgAllocation:  60.0,
				LowUtilCount:   5,
				WastedGpuDays:  10.5,
			},
		}

		metadata := data.GenerateMetadata()

		assert.NotNil(t, metadata)
		assert.Equal(t, "test-cluster", metadata["cluster_name"])
		assert.Equal(t, float64(100), metadata["total_gpus"])
		assert.Equal(t, 50.5, metadata["avg_utilization"])
		assert.Equal(t, 60.0, metadata["avg_allocation"])
		assert.Equal(t, float64(5), metadata["low_util_count"])
		assert.Equal(t, 10.5, metadata["wasted_gpu_days"])
	})
}

// TestReportPeriod tests the ReportPeriod struct
func TestReportPeriod(t *testing.T) {
	t.Run("creates report period", func(t *testing.T) {
		start := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
		end := time.Date(2024, 1, 7, 23, 59, 59, 0, time.UTC)

		period := ReportPeriod{
			StartTime: start,
			EndTime:   end,
		}

		assert.Equal(t, start, period.StartTime)
		assert.Equal(t, end, period.EndTime)
	})
}

// TestGpuUsageWeeklyReportBackfillConfig tests the config struct
func TestGpuUsageWeeklyReportBackfillConfig(t *testing.T) {
	t.Run("creates config with all fields", func(t *testing.T) {
		cfg := &GpuUsageWeeklyReportBackfillConfig{
			Enabled:            true,
			Cron:               "0 5 * * *",
			MaxWeeksToBackfill: 10,
			WeeklyReportConfig: &config.WeeklyReportConfig{
				TimeRangeDays: 7,
			},
		}

		assert.True(t, cfg.Enabled)
		assert.Equal(t, "0 5 * * *", cfg.Cron)
		assert.Equal(t, 10, cfg.MaxWeeksToBackfill)
		assert.NotNil(t, cfg.WeeklyReportConfig)
	})
}

// TestDependencies tests the Dependencies struct
func TestDependencies(t *testing.T) {
	t.Run("creates dependencies", func(t *testing.T) {
		mockCM := NewMockClusterManager()
		mockFacade := database.NewMockFacade()

		deps := &Dependencies{
			ClusterManager: mockCM,
			DatabaseFacade: mockFacade,
		}

		assert.Equal(t, mockCM, deps.ClusterManager)
		assert.Equal(t, mockFacade, deps.DatabaseFacade)
	})
}

// TestDefaultDBConnectionProvider tests the DefaultDBConnectionProvider
func TestDefaultDBConnectionProvider(t *testing.T) {
	t.Run("creates provider", func(t *testing.T) {
		mockCM := NewMockClusterManager()
		provider := NewDefaultDBConnectionProvider(mockCM, nil)

		assert.NotNil(t, provider)
	})
}

// TestMockDBConnectionProvider tests the MockDBConnectionProvider
func TestMockDBConnectionProvider(t *testing.T) {
	t.Run("returns nil when GetDBForClusterFunc is nil", func(t *testing.T) {
		mock := &MockDBConnectionProvider{}

		db, err := mock.GetDBForCluster("any")

		assert.Nil(t, db)
		assert.Nil(t, err)
	})

	t.Run("uses custom GetDBForClusterFunc when provided", func(t *testing.T) {
		expectedErr := errors.New("db error")
		mock := &MockDBConnectionProvider{
			GetDBForClusterFunc: func(clusterName string) (*gorm.DB, error) {
				return nil, expectedErr
			},
		}

		db, err := mock.GetDBForCluster("any")

		assert.Nil(t, db)
		assert.Equal(t, expectedErr, err)
	})
}

// TestTimeRangeResult tests the TimeRangeResult struct
func TestTimeRangeResult(t *testing.T) {
	t.Run("creates time range result", func(t *testing.T) {
		minTime := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
		maxTime := time.Date(2024, 1, 31, 23, 59, 59, 0, time.UTC)

		result := TimeRangeResult{
			MinTime: minTime,
			MaxTime: maxTime,
		}

		assert.Equal(t, minTime, result.MinTime)
		assert.Equal(t, maxTime, result.MaxTime)
	})
}

// TestGetExistingReports tests the getExistingReports method
func TestGetExistingReports(t *testing.T) {
	t.Run("returns reports from database", func(t *testing.T) {
		mockFacade := database.NewMockFacade()
		mockReportFacade := mockFacade.GpuUsageWeeklyReportMock.(*database.MockGpuUsageWeeklyReportFacade)

		// Add test reports
		mockReportFacade.Reports["rpt-1"] = &dbmodel.GpuUsageWeeklyReports{
			ID:          "rpt-1",
			ClusterName: "test-cluster",
			Status:      "generated",
		}
		mockReportFacade.Reports["rpt-2"] = &dbmodel.GpuUsageWeeklyReports{
			ID:          "rpt-2",
			ClusterName: "test-cluster",
			Status:      "generated",
		}

		deps := &Dependencies{
			ClusterManager: NewMockClusterManager(),
			DatabaseFacade: mockFacade,
		}

		cfg := &GpuUsageWeeklyReportBackfillConfig{
			Enabled: true,
		}

		job := NewGpuUsageWeeklyReportBackfillJobWithDeps(cfg, deps)

		reports, err := job.getExistingReports(context.Background(), "test-cluster")

		require.NoError(t, err)
		assert.Len(t, reports, 2)
	})
}

// TestGetDistinctClusters tests the getDistinctClusters method
func TestGetDistinctClusters(t *testing.T) {
	t.Run("returns clusters from cluster manager", func(t *testing.T) {
		mockClusterManager := NewMockClusterManager()
		mockClusterManager.AddCluster("cluster-1")
		mockClusterManager.AddCluster("cluster-2")

		deps := &Dependencies{
			ClusterManager: mockClusterManager,
		}

		cfg := &GpuUsageWeeklyReportBackfillConfig{
			Enabled: true,
		}

		job := NewGpuUsageWeeklyReportBackfillJobWithDeps(cfg, deps)

		clusters, err := job.getDistinctClusters(context.Background())

		require.NoError(t, err)
		assert.Len(t, clusters, 2)
	})

	t.Run("returns nil when no clusters", func(t *testing.T) {
		mockClusterManager := NewMockClusterManager()

		deps := &Dependencies{
			ClusterManager: mockClusterManager,
		}

		cfg := &GpuUsageWeeklyReportBackfillConfig{
			Enabled: true,
		}

		job := NewGpuUsageWeeklyReportBackfillJobWithDeps(cfg, deps)

		clusters, err := job.getDistinctClusters(context.Background())

		require.NoError(t, err)
		assert.Nil(t, clusters)
	})
}

// TestGetGenerator tests the getGenerator method
func TestGetGenerator(t *testing.T) {
	t.Run("uses factory when provided", func(t *testing.T) {
		customGenerator := &MockReportGenerator{}
		deps := &Dependencies{
			GeneratorFactory: func() ReportGeneratorInterface {
				return customGenerator
			},
		}

		cfg := &GpuUsageWeeklyReportBackfillConfig{
			Enabled: true,
		}

		job := NewGpuUsageWeeklyReportBackfillJobWithDeps(cfg, deps)

		generator := job.getGenerator()

		assert.Equal(t, customGenerator, generator)
	})
}

// TestGetRenderer tests the getRenderer method
func TestGetRenderer(t *testing.T) {
	t.Run("uses factory when provided", func(t *testing.T) {
		customRenderer := &MockReportRenderer{}
		deps := &Dependencies{
			RendererFactory: func() ReportRendererInterface {
				return customRenderer
			},
		}

		cfg := &GpuUsageWeeklyReportBackfillConfig{
			Enabled: true,
		}

		job := NewGpuUsageWeeklyReportBackfillJobWithDeps(cfg, deps)

		renderer := job.getRenderer()

		assert.Equal(t, customRenderer, renderer)
	})
}

// TestGetDBForCluster tests the getDBForCluster method
func TestGetDBForCluster(t *testing.T) {
	t.Run("uses DBConnectionProvider when provided", func(t *testing.T) {
		mockProvider := &MockDBConnectionProvider{
			GetDBForClusterFunc: func(clusterName string) (*gorm.DB, error) {
				return nil, nil
			},
		}

		deps := &Dependencies{
			DBConnectionProvider: mockProvider,
		}

		cfg := &GpuUsageWeeklyReportBackfillConfig{
			Enabled: true,
		}

		job := NewGpuUsageWeeklyReportBackfillJobWithDeps(cfg, deps)

		db, err := job.getDBForCluster("test-cluster")

		assert.Nil(t, db)
		require.NoError(t, err)
	})

	t.Run("returns error from DBConnectionProvider", func(t *testing.T) {
		expectedErr := errors.New("db connection error")
		mockProvider := &MockDBConnectionProvider{
			GetDBForClusterFunc: func(clusterName string) (*gorm.DB, error) {
				return nil, expectedErr
			},
		}

		deps := &Dependencies{
			DBConnectionProvider: mockProvider,
		}

		cfg := &GpuUsageWeeklyReportBackfillConfig{
			Enabled: true,
		}

		job := NewGpuUsageWeeklyReportBackfillJobWithDeps(cfg, deps)

		db, err := job.getDBForCluster("test-cluster")

		assert.Nil(t, db)
		assert.Equal(t, expectedErr, err)
	})
}

// TestDefaultDBConnectionProviderGetDBForCluster tests the GetDBForCluster method
func TestDefaultDBConnectionProviderGetDBForCluster(t *testing.T) {
	t.Run("returns default DB when cluster not found", func(t *testing.T) {
		mockCM := NewMockClusterManager()
		provider := NewDefaultDBConnectionProvider(mockCM, nil)

		db, err := provider.GetDBForCluster("unknown-cluster")

		assert.Nil(t, db) // default DB is nil in this test
		require.NoError(t, err)
	})

	t.Run("returns default DB when clientset has no storage", func(t *testing.T) {
		mockCM := NewMockClusterManager()
		mockCM.AddCluster("test-cluster") // adds cluster without storage

		provider := NewDefaultDBConnectionProvider(mockCM, nil)

		db, err := provider.GetDBForCluster("test-cluster")

		assert.Nil(t, db)
		require.NoError(t, err)
	})
}

// TestReportSummary tests the ReportSummary struct
func TestReportSummary(t *testing.T) {
	t.Run("creates report summary with all fields", func(t *testing.T) {
		summary := &ReportSummary{
			TotalGPUs:      100,
			AvgUtilization: 75.5,
			AvgAllocation:  80.0,
			TotalGpuHours:  1000.5,
			LowUtilCount:   10,
			WastedGpuDays:  50.5,
		}

		assert.Equal(t, 100, summary.TotalGPUs)
		assert.Equal(t, 75.5, summary.AvgUtilization)
		assert.Equal(t, 80.0, summary.AvgAllocation)
		assert.Equal(t, 1000.5, summary.TotalGpuHours)
		assert.Equal(t, 10, summary.LowUtilCount)
		assert.Equal(t, 50.5, summary.WastedGpuDays)
	})
}

// TestChartData tests the ChartData struct
func TestChartData(t *testing.T) {
	t.Run("creates chart data", func(t *testing.T) {
		chartData := &ChartData{
			ClusterUsageTrend: &EChartsData{
				Title: "GPU Usage Trend",
				XAxis: []string{"Mon", "Tue", "Wed"},
			},
			NamespaceUsage: []NamespaceData{
				{Name: "ns1", GpuHours: 100},
			},
		}

		assert.NotNil(t, chartData.ClusterUsageTrend)
		assert.Equal(t, "GPU Usage Trend", chartData.ClusterUsageTrend.Title)
		assert.Len(t, chartData.NamespaceUsage, 1)
	})
}

// TestEChartsData tests the EChartsData struct
func TestEChartsData(t *testing.T) {
	t.Run("creates echarts data", func(t *testing.T) {
		data := &EChartsData{
			XAxis:         []string{"Mon", "Tue"},
			Title:         "Test Chart",
			Cluster:       "test-cluster",
			TimeRangeDays: 7,
			Series: []EChartsSeries{
				{Name: "Series1", Type: "line", Data: []interface{}{1, 2, 3}},
			},
		}

		assert.Equal(t, "Test Chart", data.Title)
		assert.Equal(t, "test-cluster", data.Cluster)
		assert.Equal(t, 7, data.TimeRangeDays)
		assert.Len(t, data.Series, 1)
	})
}

// TestEChartsSeries tests the EChartsSeries struct
func TestEChartsSeries(t *testing.T) {
	t.Run("creates echarts series", func(t *testing.T) {
		series := EChartsSeries{
			Name:   "Utilization",
			Type:   "line",
			Data:   []interface{}{10.5, 20.5, 30.5},
			Smooth: true,
		}

		assert.Equal(t, "Utilization", series.Name)
		assert.Equal(t, "line", series.Type)
		assert.Len(t, series.Data, 3)
		assert.True(t, series.Smooth)
	})
}

// TestTimeSeriesPoint tests the TimeSeriesPoint struct
func TestTimeSeriesPoint(t *testing.T) {
	t.Run("creates time series point", func(t *testing.T) {
		point := TimeSeriesPoint{
			Timestamp: 1704067200,
			Value:     75.5,
		}

		assert.Equal(t, int64(1704067200), point.Timestamp)
		assert.Equal(t, 75.5, point.Value)
	})
}

// TestNamespaceData tests the NamespaceData struct
func TestNamespaceData(t *testing.T) {
	t.Run("creates namespace data", func(t *testing.T) {
		ns := NamespaceData{
			Name:        "production",
			GpuHours:    500.5,
			Utilization: 85.0,
		}

		assert.Equal(t, "production", ns.Name)
		assert.Equal(t, 500.5, ns.GpuHours)
		assert.Equal(t, 85.0, ns.Utilization)
	})
}

// TestUserData tests the UserData struct
func TestUserData(t *testing.T) {
	t.Run("creates user data", func(t *testing.T) {
		user := UserData{
			Username:       "john",
			Namespace:      "dev",
			AvgUtilization: 20.0,
			GpuCount:       4,
			WastedGpuHours: 100.0,
		}

		assert.Equal(t, "john", user.Username)
		assert.Equal(t, "dev", user.Namespace)
		assert.Equal(t, 20.0, user.AvgUtilization)
		assert.Equal(t, 4, user.GpuCount)
		assert.Equal(t, 100.0, user.WastedGpuHours)
	})
}

// TestConductorReportRequest tests the ConductorReportRequest struct
func TestConductorReportRequest(t *testing.T) {
	t.Run("creates conductor report request", func(t *testing.T) {
		req := ConductorReportRequest{
			Cluster:              "test-cluster",
			TimeRangeDays:        7,
			StartTime:            "2024-01-01T00:00:00Z",
			EndTime:              "2024-01-07T23:59:59Z",
			UtilizationThreshold: 30,
			MinGpuCount:          1,
			TopN:                 20,
		}

		assert.Equal(t, "test-cluster", req.Cluster)
		assert.Equal(t, 7, req.TimeRangeDays)
		assert.Equal(t, "2024-01-01T00:00:00Z", req.StartTime)
		assert.Equal(t, "2024-01-07T23:59:59Z", req.EndTime)
		assert.Equal(t, 30, req.UtilizationThreshold)
		assert.Equal(t, 1, req.MinGpuCount)
		assert.Equal(t, 20, req.TopN)
	})
}

// TestConductorReportResponse tests the ConductorReportResponse struct
func TestConductorReportResponse(t *testing.T) {
	t.Run("creates conductor report response", func(t *testing.T) {
		resp := ConductorReportResponse{
			Status:         "success",
			Report:         "# Weekly Report",
			MarkdownReport: "# Weekly Report Alt",
			ChartData: map[string]interface{}{
				"cluster_usage_trend": map[string]interface{}{},
			},
			Summary: map[string]interface{}{
				"total_gpus": 100,
			},
			Metadata: map[string]interface{}{
				"version": "1.0",
			},
			Timestamp: "2024-01-07T00:00:00Z",
		}

		assert.Equal(t, "success", resp.Status)
		assert.Equal(t, "# Weekly Report", resp.Report)
		assert.NotNil(t, resp.ChartData)
		assert.NotNil(t, resp.Summary)
		assert.NotNil(t, resp.Metadata)
	})
}

// TestMockGpuUsageWeeklyReportFacade tests the MockGpuUsageWeeklyReportFacade
func TestMockGpuUsageWeeklyReportFacade(t *testing.T) {
	t.Run("Create stores report", func(t *testing.T) {
		mock := database.NewMockGpuUsageWeeklyReportFacade()
		report := &dbmodel.GpuUsageWeeklyReports{
			ID:          "rpt-1",
			ClusterName: "test",
			Status:      "pending",
		}

		err := mock.Create(context.Background(), report)

		require.NoError(t, err)
		assert.Len(t, mock.Reports, 1)
	})

	t.Run("GetByID returns report", func(t *testing.T) {
		mock := database.NewMockGpuUsageWeeklyReportFacade()
		mock.Reports["rpt-1"] = &dbmodel.GpuUsageWeeklyReports{
			ID:          "rpt-1",
			ClusterName: "test",
		}

		report, err := mock.GetByID(context.Background(), "rpt-1")

		require.NoError(t, err)
		assert.NotNil(t, report)
		assert.Equal(t, "rpt-1", report.ID)
	})

	t.Run("Update modifies report", func(t *testing.T) {
		mock := database.NewMockGpuUsageWeeklyReportFacade()
		mock.Reports["rpt-1"] = &dbmodel.GpuUsageWeeklyReports{
			ID:     "rpt-1",
			Status: "pending",
		}

		report := mock.Reports["rpt-1"]
		report.Status = "generated"
		err := mock.Update(context.Background(), report)

		require.NoError(t, err)
		assert.Equal(t, "generated", mock.Reports["rpt-1"].Status)
	})

	t.Run("UpdateStatus updates status only", func(t *testing.T) {
		mock := database.NewMockGpuUsageWeeklyReportFacade()
		mock.Reports["rpt-1"] = &dbmodel.GpuUsageWeeklyReports{
			ID:     "rpt-1",
			Status: "pending",
		}

		err := mock.UpdateStatus(context.Background(), "rpt-1", "generated")

		require.NoError(t, err)
		assert.Equal(t, "generated", mock.Reports["rpt-1"].Status)
	})

	t.Run("List filters by cluster", func(t *testing.T) {
		mock := database.NewMockGpuUsageWeeklyReportFacade()
		mock.Reports["rpt-1"] = &dbmodel.GpuUsageWeeklyReports{
			ID:          "rpt-1",
			ClusterName: "cluster-1",
			Status:      "generated",
		}
		mock.Reports["rpt-2"] = &dbmodel.GpuUsageWeeklyReports{
			ID:          "rpt-2",
			ClusterName: "cluster-2",
			Status:      "generated",
		}

		reports, count, err := mock.List(context.Background(), "cluster-1", "", 0, 100)

		require.NoError(t, err)
		assert.Len(t, reports, 1)
		assert.Equal(t, int64(1), count)
	})

	t.Run("GetLatestByCluster returns most recent", func(t *testing.T) {
		mock := database.NewMockGpuUsageWeeklyReportFacade()
		mock.Reports["rpt-1"] = &dbmodel.GpuUsageWeeklyReports{
			ID:          "rpt-1",
			ClusterName: "test",
			GeneratedAt: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
		}
		mock.Reports["rpt-2"] = &dbmodel.GpuUsageWeeklyReports{
			ID:          "rpt-2",
			ClusterName: "test",
			GeneratedAt: time.Date(2024, 1, 8, 0, 0, 0, 0, time.UTC),
		}

		report, err := mock.GetLatestByCluster(context.Background(), "test")

		require.NoError(t, err)
		assert.NotNil(t, report)
		assert.Equal(t, "rpt-2", report.ID)
	})

	t.Run("DeleteOlderThan removes old reports", func(t *testing.T) {
		mock := database.NewMockGpuUsageWeeklyReportFacade()
		mock.Reports["rpt-1"] = &dbmodel.GpuUsageWeeklyReports{
			ID:          "rpt-1",
			GeneratedAt: time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC),
		}
		mock.Reports["rpt-2"] = &dbmodel.GpuUsageWeeklyReports{
			ID:          "rpt-2",
			GeneratedAt: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
		}

		deleted, err := mock.DeleteOlderThan(context.Background(), time.Date(2023, 6, 1, 0, 0, 0, 0, time.UTC))

		require.NoError(t, err)
		assert.Equal(t, int64(1), deleted)
		assert.Len(t, mock.Reports, 1)
	})

	t.Run("WithCluster returns same mock", func(t *testing.T) {
		mock := database.NewMockGpuUsageWeeklyReportFacade()

		result := mock.WithCluster("any-cluster")

		assert.Equal(t, mock, result)
	})
}

// TestMockGpuAggregationFacade tests the MockGpuAggregationFacade
func TestMockGpuAggregationFacade(t *testing.T) {
	t.Run("SaveClusterHourlyStats stores stats", func(t *testing.T) {
		mock := database.NewMockGpuAggregationFacade()
		stats := &dbmodel.ClusterGpuHourlyStats{
			TotalGpuCapacity: 100,
		}

		err := mock.SaveClusterHourlyStats(context.Background(), stats)

		require.NoError(t, err)
		assert.Len(t, mock.ClusterHourlyStats, 1)
	})

	t.Run("BatchSaveClusterHourlyStats stores multiple", func(t *testing.T) {
		mock := database.NewMockGpuAggregationFacade()
		stats := []*dbmodel.ClusterGpuHourlyStats{
			{TotalGpuCapacity: 100},
			{TotalGpuCapacity: 200},
		}

		err := mock.BatchSaveClusterHourlyStats(context.Background(), stats)

		require.NoError(t, err)
		assert.Len(t, mock.ClusterHourlyStats, 2)
	})

	t.Run("GetClusterHourlyStats filters by time range", func(t *testing.T) {
		mock := database.NewMockGpuAggregationFacade()
		mock.ClusterHourlyStats = []*dbmodel.ClusterGpuHourlyStats{
			{StatHour: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), TotalGpuCapacity: 100},
			{StatHour: time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC), TotalGpuCapacity: 100},
			{StatHour: time.Date(2024, 1, 10, 0, 0, 0, 0, time.UTC), TotalGpuCapacity: 100},
		}

		stats, err := mock.GetClusterHourlyStats(context.Background(),
			time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
			time.Date(2024, 1, 5, 0, 0, 0, 0, time.UTC))

		require.NoError(t, err)
		assert.Len(t, stats, 2)
	})

	t.Run("WithCluster returns same mock", func(t *testing.T) {
		mock := database.NewMockGpuAggregationFacade()

		result := mock.WithCluster("any-cluster")

		assert.Equal(t, mock, result)
	})
}

// TestMockFacade tests the MockFacade
func TestMockFacade(t *testing.T) {
	t.Run("creates mock facade with default mocks", func(t *testing.T) {
		mock := database.NewMockFacade()

		assert.NotNil(t, mock)
		assert.NotNil(t, mock.GetGpuUsageWeeklyReport())
		assert.NotNil(t, mock.GetGpuAggregation())
	})

	t.Run("WithCluster returns same mock", func(t *testing.T) {
		mock := database.NewMockFacade()

		result := mock.WithCluster("any-cluster")

		assert.Equal(t, mock, result)
	})

	t.Run("other getters return nil", func(t *testing.T) {
		mock := database.NewMockFacade()

		assert.Nil(t, mock.GetNode())
		assert.Nil(t, mock.GetPod())
		assert.Nil(t, mock.GetWorkload())
		assert.Nil(t, mock.GetContainer())
		assert.Nil(t, mock.GetTraining())
		assert.Nil(t, mock.GetStorage())
		assert.Nil(t, mock.GetAlert())
	})
}
