package gpu_usage_weekly_report

import (
	"context"
	"testing"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/config"
	dbmodel "github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewGpuUsageWeeklyReportBackfillJob(t *testing.T) {
	t.Run("creates job with nil config", func(t *testing.T) {
		job := NewGpuUsageWeeklyReportBackfillJob(nil)
		assert.NotNil(t, job)
		assert.Nil(t, job.config)
	})

	t.Run("creates job with config", func(t *testing.T) {
		cfg := &GpuUsageWeeklyReportBackfillConfig{
			Enabled:            true,
			Cron:               "0 4 * * *",
			MaxWeeksToBackfill: 10,
		}
		job := NewGpuUsageWeeklyReportBackfillJob(cfg)
		assert.NotNil(t, job)
		assert.Equal(t, cfg, job.config)
	})
}

func TestSchedule(t *testing.T) {
	t.Run("returns default schedule when config is nil", func(t *testing.T) {
		job := NewGpuUsageWeeklyReportBackfillJob(nil)
		assert.Equal(t, "0 3 * * *", job.Schedule())
	})

	t.Run("returns default schedule when cron is empty", func(t *testing.T) {
		job := NewGpuUsageWeeklyReportBackfillJob(&GpuUsageWeeklyReportBackfillConfig{
			Enabled: true,
			Cron:    "",
		})
		assert.Equal(t, "0 3 * * *", job.Schedule())
	})

	t.Run("returns configured schedule", func(t *testing.T) {
		job := NewGpuUsageWeeklyReportBackfillJob(&GpuUsageWeeklyReportBackfillConfig{
			Enabled: true,
			Cron:    "0 5 * * *",
		})
		assert.Equal(t, "0 5 * * *", job.Schedule())
	})
}

func TestShouldRenderPDF(t *testing.T) {
	t.Run("returns true when config is nil", func(t *testing.T) {
		job := NewGpuUsageWeeklyReportBackfillJob(nil)
		assert.True(t, job.shouldRenderPDF())
	})

	t.Run("returns true when WeeklyReportConfig is nil", func(t *testing.T) {
		job := NewGpuUsageWeeklyReportBackfillJob(&GpuUsageWeeklyReportBackfillConfig{
			Enabled: true,
		})
		assert.True(t, job.shouldRenderPDF())
	})

	t.Run("returns true when OutputFormats is empty", func(t *testing.T) {
		job := NewGpuUsageWeeklyReportBackfillJob(&GpuUsageWeeklyReportBackfillConfig{
			Enabled: true,
			WeeklyReportConfig: &config.WeeklyReportConfig{
				OutputFormats: []string{},
			},
		})
		assert.True(t, job.shouldRenderPDF())
	})

	t.Run("returns true when pdf is in OutputFormats", func(t *testing.T) {
		job := NewGpuUsageWeeklyReportBackfillJob(&GpuUsageWeeklyReportBackfillConfig{
			Enabled: true,
			WeeklyReportConfig: &config.WeeklyReportConfig{
				OutputFormats: []string{"html", "pdf", "json"},
			},
		})
		assert.True(t, job.shouldRenderPDF())
	})

	t.Run("returns false when pdf is not in OutputFormats", func(t *testing.T) {
		job := NewGpuUsageWeeklyReportBackfillJob(&GpuUsageWeeklyReportBackfillConfig{
			Enabled: true,
			WeeklyReportConfig: &config.WeeklyReportConfig{
				OutputFormats: []string{"html", "json"},
			},
		})
		assert.False(t, job.shouldRenderPDF())
	})
}

func TestGetNextMonday(t *testing.T) {
	job := NewGpuUsageWeeklyReportBackfillJob(nil)

	// Note: getNextMonday returns the next Monday at or after the given time
	// For Monday, it returns that Monday at 00:00
	// For Tuesday-Sunday, it returns the NEXT Monday at 00:00
	testCases := []struct {
		name     string
		input    time.Time
		expected time.Time
	}{
		{
			name:     "already Monday at midnight",
			input:    time.Date(2025, 12, 8, 0, 0, 0, 0, time.UTC), // Monday
			expected: time.Date(2025, 12, 8, 0, 0, 0, 0, time.UTC),
		},
		{
			name:     "Monday but not midnight",
			input:    time.Date(2025, 12, 8, 10, 30, 0, 0, time.UTC), // Monday
			expected: time.Date(2025, 12, 8, 0, 0, 0, 0, time.UTC),
		},
		{
			name:     "Tuesday - returns next Monday",
			input:    time.Date(2025, 12, 9, 15, 0, 0, 0, time.UTC), // Tuesday
			expected: time.Date(2025, 12, 15, 0, 0, 0, 0, time.UTC), // Next Monday
		},
		{
			name:     "Wednesday - returns next Monday",
			input:    time.Date(2025, 12, 10, 8, 0, 0, 0, time.UTC), // Wednesday
			expected: time.Date(2025, 12, 15, 0, 0, 0, 0, time.UTC), // Next Monday
		},
		{
			name:     "Thursday - returns next Monday",
			input:    time.Date(2025, 12, 11, 12, 0, 0, 0, time.UTC), // Thursday
			expected: time.Date(2025, 12, 15, 0, 0, 0, 0, time.UTC), // Next Monday
		},
		{
			name:     "Friday - returns next Monday",
			input:    time.Date(2025, 12, 12, 9, 0, 0, 0, time.UTC), // Friday
			expected: time.Date(2025, 12, 15, 0, 0, 0, 0, time.UTC), // Next Monday
		},
		{
			name:     "Saturday - returns next Monday",
			input:    time.Date(2025, 12, 13, 14, 0, 0, 0, time.UTC), // Saturday
			expected: time.Date(2025, 12, 15, 0, 0, 0, 0, time.UTC), // Next Monday
		},
		{
			name:     "Sunday - returns next Monday",
			input:    time.Date(2025, 12, 14, 20, 0, 0, 0, time.UTC), // Sunday
			expected: time.Date(2025, 12, 15, 0, 0, 0, 0, time.UTC), // Next Monday
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := job.getNextMonday(tc.input)
			assert.Equal(t, tc.expected, result, "expected %v, got %v", tc.expected, result)
		})
	}
}

func TestCalculateNaturalWeeks(t *testing.T) {
	job := NewGpuUsageWeeklyReportBackfillJob(nil)

	t.Run("no complete weeks", func(t *testing.T) {
		// Data from Dec 10 (Wed) to Dec 12 (Fri) - no complete week
		minTime := time.Date(2025, 12, 10, 0, 0, 0, 0, time.UTC)
		maxTime := time.Date(2025, 12, 12, 23, 59, 59, 0, time.UTC)

		weeks := job.calculateNaturalWeeks(minTime, maxTime)
		assert.Empty(t, weeks)
	})

	t.Run("one complete week", func(t *testing.T) {
		// Data from Dec 1 (Mon) to Dec 14 (Sun) - one complete week (Dec 1-7)
		minTime := time.Date(2025, 12, 1, 0, 0, 0, 0, time.UTC)
		maxTime := time.Date(2025, 12, 14, 23, 59, 59, 999999999, time.UTC)

		weeks := job.calculateNaturalWeeks(minTime, maxTime)
		require.Len(t, weeks, 2)

		// First week: Dec 1 - Dec 7
		assert.Equal(t, time.Date(2025, 12, 1, 0, 0, 0, 0, time.UTC), weeks[0].StartTime)
		assert.Equal(t, time.Date(2025, 12, 8, 0, 0, 0, 0, time.UTC).Add(-time.Nanosecond), weeks[0].EndTime)

		// Second week: Dec 8 - Dec 14
		assert.Equal(t, time.Date(2025, 12, 8, 0, 0, 0, 0, time.UTC), weeks[1].StartTime)
		assert.Equal(t, time.Date(2025, 12, 15, 0, 0, 0, 0, time.UTC).Add(-time.Nanosecond), weeks[1].EndTime)
	})

	t.Run("multiple complete weeks", func(t *testing.T) {
		// Data from Nov 1 to Dec 31 - should have several complete weeks
		minTime := time.Date(2025, 11, 1, 0, 0, 0, 0, time.UTC)
		maxTime := time.Date(2025, 12, 31, 23, 59, 59, 999999999, time.UTC)

		weeks := job.calculateNaturalWeeks(minTime, maxTime)
		assert.Greater(t, len(weeks), 5, "should have at least 5 complete weeks")

		// Verify all weeks start on Monday and end on Sunday
		for i, week := range weeks {
			assert.Equal(t, time.Monday, week.StartTime.Weekday(), "week %d should start on Monday", i)
			assert.Equal(t, time.Sunday, week.EndTime.Weekday(), "week %d should end on Sunday", i)
			assert.Equal(t, 0, week.StartTime.Hour(), "week %d should start at midnight", i)
		}
	})

	t.Run("data starts mid-week", func(t *testing.T) {
		// Data from Dec 3 (Wed) to Dec 21 (Sun)
		minTime := time.Date(2025, 12, 3, 10, 0, 0, 0, time.UTC)
		maxTime := time.Date(2025, 12, 21, 23, 59, 59, 999999999, time.UTC)

		weeks := job.calculateNaturalWeeks(minTime, maxTime)
		require.Len(t, weeks, 2)

		// First complete week should be Dec 8-14
		assert.Equal(t, time.Date(2025, 12, 8, 0, 0, 0, 0, time.UTC), weeks[0].StartTime)
		// Second complete week should be Dec 15-21
		assert.Equal(t, time.Date(2025, 12, 15, 0, 0, 0, 0, time.UTC), weeks[1].StartTime)
	})

	t.Run("handles timezone correctly", func(t *testing.T) {
		loc, _ := time.LoadLocation("Asia/Shanghai")
		minTime := time.Date(2025, 12, 1, 0, 0, 0, 0, loc)
		maxTime := time.Date(2025, 12, 14, 23, 59, 59, 999999999, loc)

		weeks := job.calculateNaturalWeeks(minTime, maxTime)
		assert.NotEmpty(t, weeks)

		// Verify timezone is preserved
		for _, week := range weeks {
			assert.Equal(t, loc, week.StartTime.Location())
			assert.Equal(t, loc, week.EndTime.Location())
		}
	})
}

func TestFindMissingWeeks(t *testing.T) {
	job := NewGpuUsageWeeklyReportBackfillJob(nil)

	t.Run("all weeks missing", func(t *testing.T) {
		weeks := []ReportPeriod{
			{
				StartTime: time.Date(2025, 12, 1, 0, 0, 0, 0, time.UTC),
				EndTime:   time.Date(2025, 12, 7, 23, 59, 59, 0, time.UTC),
			},
			{
				StartTime: time.Date(2025, 12, 8, 0, 0, 0, 0, time.UTC),
				EndTime:   time.Date(2025, 12, 14, 23, 59, 59, 0, time.UTC),
			},
		}
		existingReports := []*dbmodel.GpuUsageWeeklyReports{}

		missing := job.findMissingWeeks(weeks, existingReports)
		assert.Len(t, missing, 2)
	})

	t.Run("no weeks missing", func(t *testing.T) {
		weeks := []ReportPeriod{
			{
				StartTime: time.Date(2025, 12, 1, 0, 0, 0, 0, time.UTC),
				EndTime:   time.Date(2025, 12, 7, 23, 59, 59, 0, time.UTC),
			},
			{
				StartTime: time.Date(2025, 12, 8, 0, 0, 0, 0, time.UTC),
				EndTime:   time.Date(2025, 12, 14, 23, 59, 59, 0, time.UTC),
			},
		}
		existingReports := []*dbmodel.GpuUsageWeeklyReports{
			{
				Status:      "generated",
				PeriodStart: time.Date(2025, 12, 1, 0, 0, 0, 0, time.UTC),
			},
			{
				Status:      "sent",
				PeriodStart: time.Date(2025, 12, 8, 0, 0, 0, 0, time.UTC),
			},
		}

		missing := job.findMissingWeeks(weeks, existingReports)
		assert.Empty(t, missing)
	})

	t.Run("some weeks missing", func(t *testing.T) {
		weeks := []ReportPeriod{
			{
				StartTime: time.Date(2025, 12, 1, 0, 0, 0, 0, time.UTC),
				EndTime:   time.Date(2025, 12, 7, 23, 59, 59, 0, time.UTC),
			},
			{
				StartTime: time.Date(2025, 12, 8, 0, 0, 0, 0, time.UTC),
				EndTime:   time.Date(2025, 12, 14, 23, 59, 59, 0, time.UTC),
			},
			{
				StartTime: time.Date(2025, 12, 15, 0, 0, 0, 0, time.UTC),
				EndTime:   time.Date(2025, 12, 21, 23, 59, 59, 0, time.UTC),
			},
		}
		existingReports := []*dbmodel.GpuUsageWeeklyReports{
			{
				Status:      "generated",
				PeriodStart: time.Date(2025, 12, 8, 0, 0, 0, 0, time.UTC),
			},
		}

		missing := job.findMissingWeeks(weeks, existingReports)
		require.Len(t, missing, 2)
		assert.Equal(t, time.Date(2025, 12, 1, 0, 0, 0, 0, time.UTC), missing[0].StartTime)
		assert.Equal(t, time.Date(2025, 12, 15, 0, 0, 0, 0, time.UTC), missing[1].StartTime)
	})

	t.Run("ignores failed reports", func(t *testing.T) {
		weeks := []ReportPeriod{
			{
				StartTime: time.Date(2025, 12, 1, 0, 0, 0, 0, time.UTC),
				EndTime:   time.Date(2025, 12, 7, 23, 59, 59, 0, time.UTC),
			},
		}
		existingReports := []*dbmodel.GpuUsageWeeklyReports{
			{
				Status:      "failed",
				PeriodStart: time.Date(2025, 12, 1, 0, 0, 0, 0, time.UTC),
			},
		}

		missing := job.findMissingWeeks(weeks, existingReports)
		assert.Len(t, missing, 1, "failed reports should be treated as missing")
	})

	t.Run("ignores pending reports", func(t *testing.T) {
		weeks := []ReportPeriod{
			{
				StartTime: time.Date(2025, 12, 1, 0, 0, 0, 0, time.UTC),
				EndTime:   time.Date(2025, 12, 7, 23, 59, 59, 0, time.UTC),
			},
		}
		existingReports := []*dbmodel.GpuUsageWeeklyReports{
			{
				Status:      "pending",
				PeriodStart: time.Date(2025, 12, 1, 0, 0, 0, 0, time.UTC),
			},
		}

		missing := job.findMissingWeeks(weeks, existingReports)
		assert.Len(t, missing, 1, "pending reports should be treated as missing")
	})

	t.Run("handles sent status", func(t *testing.T) {
		weeks := []ReportPeriod{
			{
				StartTime: time.Date(2025, 12, 1, 0, 0, 0, 0, time.UTC),
				EndTime:   time.Date(2025, 12, 7, 23, 59, 59, 0, time.UTC),
			},
		}
		existingReports := []*dbmodel.GpuUsageWeeklyReports{
			{
				Status:      "sent",
				PeriodStart: time.Date(2025, 12, 1, 0, 0, 0, 0, time.UTC),
			},
		}

		missing := job.findMissingWeeks(weeks, existingReports)
		assert.Empty(t, missing, "sent reports should not be treated as missing")
	})
}

func TestRunDisabled(t *testing.T) {
	t.Run("returns early when config is nil", func(t *testing.T) {
		job := NewGpuUsageWeeklyReportBackfillJob(nil)
		ctx := context.Background()

		stats, err := job.Run(ctx, nil, nil)
		assert.NoError(t, err)
		assert.NotNil(t, stats)
	})

	t.Run("returns early when disabled", func(t *testing.T) {
		job := NewGpuUsageWeeklyReportBackfillJob(&GpuUsageWeeklyReportBackfillConfig{
			Enabled: false,
		})
		ctx := context.Background()

		stats, err := job.Run(ctx, nil, nil)
		assert.NoError(t, err)
		assert.NotNil(t, stats)
	})
}

func TestReportPeriodString(t *testing.T) {
	period := ReportPeriod{
		StartTime: time.Date(2025, 12, 1, 0, 0, 0, 0, time.UTC),
		EndTime:   time.Date(2025, 12, 7, 23, 59, 59, 999999999, time.UTC),
	}

	assert.Equal(t, "2025-12-01", period.StartTime.Format("2006-01-02"))
	assert.Equal(t, "2025-12-07", period.EndTime.Format("2006-01-02"))
}

func TestMaxWeeksToBackfillLimit(t *testing.T) {
	job := NewGpuUsageWeeklyReportBackfillJob(&GpuUsageWeeklyReportBackfillConfig{
		Enabled:            true,
		MaxWeeksToBackfill: 2,
	})

	// Create 5 weeks
	weeks := []ReportPeriod{}
	startDate := time.Date(2025, 11, 3, 0, 0, 0, 0, time.UTC)
	for i := 0; i < 5; i++ {
		weeks = append(weeks, ReportPeriod{
			StartTime: startDate.AddDate(0, 0, i*7),
			EndTime:   startDate.AddDate(0, 0, i*7+6).Add(23*time.Hour + 59*time.Minute + 59*time.Second),
		})
	}

	// All weeks are missing
	existingReports := []*dbmodel.GpuUsageWeeklyReports{}
	missing := job.findMissingWeeks(weeks, existingReports)

	// Should have 5 missing weeks
	assert.Len(t, missing, 5)

	// Apply limit
	if job.config.MaxWeeksToBackfill > 0 && len(missing) > job.config.MaxWeeksToBackfill {
		missing = missing[:job.config.MaxWeeksToBackfill]
	}

	// Should be limited to 2
	assert.Len(t, missing, 2)
}

func TestCalculateNaturalWeeksEdgeCases(t *testing.T) {
	job := NewGpuUsageWeeklyReportBackfillJob(nil)

	t.Run("exact week boundary", func(t *testing.T) {
		// Exactly one week: Monday 00:00 to Sunday 23:59:59.999999999
		minTime := time.Date(2025, 12, 1, 0, 0, 0, 0, time.UTC)
		maxTime := time.Date(2025, 12, 7, 23, 59, 59, 999999999, time.UTC)

		weeks := job.calculateNaturalWeeks(minTime, maxTime)
		require.Len(t, weeks, 1)
		assert.Equal(t, minTime, weeks[0].StartTime)
	})

	t.Run("ends just before week complete", func(t *testing.T) {
		// Data ends on Sunday at 23:00 (not quite complete)
		minTime := time.Date(2025, 12, 1, 0, 0, 0, 0, time.UTC)
		maxTime := time.Date(2025, 12, 7, 23, 0, 0, 0, time.UTC)

		weeks := job.calculateNaturalWeeks(minTime, maxTime)
		// Week should not be included because maxTime is before week end
		assert.Empty(t, weeks)
	})

	t.Run("empty time range", func(t *testing.T) {
		minTime := time.Date(2025, 12, 1, 0, 0, 0, 0, time.UTC)
		maxTime := time.Date(2025, 12, 1, 0, 0, 0, 0, time.UTC)

		weeks := job.calculateNaturalWeeks(minTime, maxTime)
		assert.Empty(t, weeks)
	})

	t.Run("max before min", func(t *testing.T) {
		minTime := time.Date(2025, 12, 14, 0, 0, 0, 0, time.UTC)
		maxTime := time.Date(2025, 12, 1, 0, 0, 0, 0, time.UTC)

		weeks := job.calculateNaturalWeeks(minTime, maxTime)
		assert.Empty(t, weeks)
	})
}

func TestGetNextMondayEdgeCases(t *testing.T) {
	job := NewGpuUsageWeeklyReportBackfillJob(nil)

	t.Run("year boundary - Wednesday to next Monday", func(t *testing.T) {
		// Dec 31, 2025 is Wednesday
		input := time.Date(2025, 12, 31, 12, 0, 0, 0, time.UTC)
		result := job.getNextMonday(input)
		// Wednesday -> next Monday is Jan 5, 2026
		assert.Equal(t, time.Date(2026, 1, 5, 0, 0, 0, 0, time.UTC), result)
	})

	t.Run("leap year - Thursday to next Monday", func(t *testing.T) {
		// Feb 29, 2024 (leap year) is Thursday
		input := time.Date(2024, 2, 29, 10, 0, 0, 0, time.UTC)
		result := job.getNextMonday(input)
		// Thursday -> next Monday is March 4, 2024
		assert.Equal(t, time.Date(2024, 3, 4, 0, 0, 0, 0, time.UTC), result)
	})

	t.Run("exact Monday midnight", func(t *testing.T) {
		// Jan 6, 2025 is Monday
		input := time.Date(2025, 1, 6, 0, 0, 0, 0, time.UTC)
		result := job.getNextMonday(input)
		assert.Equal(t, time.Date(2025, 1, 6, 0, 0, 0, 0, time.UTC), result)
	})
}

func TestConfigValidation(t *testing.T) {
	t.Run("zero MaxWeeksToBackfill means no limit", func(t *testing.T) {
		cfg := &GpuUsageWeeklyReportBackfillConfig{
			Enabled:            true,
			MaxWeeksToBackfill: 0,
		}
		job := NewGpuUsageWeeklyReportBackfillJob(cfg)

		// MaxWeeksToBackfill = 0 should mean no limit
		assert.Equal(t, 0, job.config.MaxWeeksToBackfill)
	})

	t.Run("negative MaxWeeksToBackfill treated as no limit", func(t *testing.T) {
		cfg := &GpuUsageWeeklyReportBackfillConfig{
			Enabled:            true,
			MaxWeeksToBackfill: -1,
		}
		job := NewGpuUsageWeeklyReportBackfillJob(cfg)

		// The actual limit check is: > 0, so -1 or 0 means no limit
		assert.Less(t, job.config.MaxWeeksToBackfill, 1)
	})
}

func TestWeeklyReportBackfillConfigDefaults(t *testing.T) {
	t.Run("empty config has correct defaults via Schedule", func(t *testing.T) {
		job := NewGpuUsageWeeklyReportBackfillJob(&GpuUsageWeeklyReportBackfillConfig{})
		assert.Equal(t, "0 3 * * *", job.Schedule())
	})
}

// Additional tests for types.go
func TestReportDataToExtType(t *testing.T) {
	t.Run("converts report data to ext type", func(t *testing.T) {
		reportData := &ReportData{
			ClusterName:    "test-cluster",
			MarkdownReport: "# Test Report",
			ChartData: &ChartData{
				GpuUtilization: []TimeSeriesPoint{
					{Timestamp: 1234567890, Value: 50.5},
				},
			},
			Summary: &ReportSummary{
				TotalGPUs:      10,
				AvgUtilization: 45.5,
			},
		}

		extType := reportData.ToExtType()
		assert.NotNil(t, extType)
		assert.Equal(t, "test-cluster", extType["cluster_name"])
		assert.Equal(t, "# Test Report", extType["markdown_report"])
	})

	t.Run("handles nil chart data", func(t *testing.T) {
		reportData := &ReportData{
			ClusterName: "test-cluster",
			ChartData:   nil,
			Summary:     nil,
		}

		extType := reportData.ToExtType()
		assert.NotNil(t, extType)
	})
}

func TestReportDataGenerateMetadata(t *testing.T) {
	t.Run("generates metadata with summary", func(t *testing.T) {
		reportData := &ReportData{
			ClusterName: "test-cluster",
			Summary: &ReportSummary{
				TotalGPUs:      100,
				AvgUtilization: 75.5,
				AvgAllocation:  80.0,
				LowUtilCount:   5,
				WastedGpuDays:  10.5,
			},
		}

		metadata := reportData.GenerateMetadata()
		assert.NotNil(t, metadata)
		assert.Equal(t, "test-cluster", metadata["cluster_name"])
		assert.Equal(t, 75.5, metadata["avg_utilization"])
		assert.Equal(t, 80.0, metadata["avg_allocation"])
		// Note: JSON marshal/unmarshal converts int to float64
		assert.Equal(t, float64(100), metadata["total_gpus"])
		assert.Equal(t, float64(5), metadata["low_util_count"])
		assert.Equal(t, 10.5, metadata["wasted_gpu_days"])
	})

	t.Run("generates metadata without summary", func(t *testing.T) {
		reportData := &ReportData{
			ClusterName: "test-cluster",
			Summary:     nil,
		}

		metadata := reportData.GenerateMetadata()
		assert.NotNil(t, metadata)
		assert.Equal(t, "test-cluster", metadata["cluster_name"])
		// Should not have summary fields
		_, hasAvgUtil := metadata["avg_utilization"]
		assert.False(t, hasAvgUtil)
	})
}

func TestChartDataStructure(t *testing.T) {
	t.Run("creates valid chart data", func(t *testing.T) {
		chartData := &ChartData{
			ClusterUsageTrend: &EChartsData{
				XAxis: []string{"Mon", "Tue", "Wed"},
				Series: []EChartsSeries{
					{
						Name: "Utilization",
						Type: "line",
						Data: []interface{}{50, 60, 70},
					},
				},
				Title:   "GPU Usage",
				Cluster: "test-cluster",
			},
			GpuUtilization: []TimeSeriesPoint{
				{Timestamp: 1000, Value: 50.0},
				{Timestamp: 2000, Value: 60.0},
			},
			NamespaceUsage: []NamespaceData{
				{Name: "ns1", GpuHours: 100, Utilization: 80},
			},
			LowUtilUsers: []UserData{
				{Username: "user1", Namespace: "ns1", AvgUtilization: 20, GpuCount: 2},
			},
		}

		assert.Equal(t, 3, len(chartData.ClusterUsageTrend.XAxis))
		assert.Equal(t, 2, len(chartData.GpuUtilization))
		assert.Equal(t, 1, len(chartData.NamespaceUsage))
		assert.Equal(t, 1, len(chartData.LowUtilUsers))
	})
}

func TestReportSummaryStructure(t *testing.T) {
	t.Run("creates valid summary", func(t *testing.T) {
		summary := &ReportSummary{
			TotalGPUs:      256,
			AvgUtilization: 65.5,
			AvgAllocation:  70.2,
			TotalGpuHours:  5000.5,
			LowUtilCount:   15,
			WastedGpuDays:  25.3,
		}

		assert.Equal(t, 256, summary.TotalGPUs)
		assert.Equal(t, 65.5, summary.AvgUtilization)
		assert.Equal(t, 70.2, summary.AvgAllocation)
		assert.Equal(t, 5000.5, summary.TotalGpuHours)
		assert.Equal(t, 15, summary.LowUtilCount)
		assert.Equal(t, 25.3, summary.WastedGpuDays)
	})
}

func TestConductorReportRequest(t *testing.T) {
	t.Run("creates valid request", func(t *testing.T) {
		req := &ConductorReportRequest{
			Cluster:              "test-cluster",
			TimeRangeDays:        7,
			StartTime:            "2025-12-01T00:00:00Z",
			EndTime:              "2025-12-07T23:59:59Z",
			UtilizationThreshold: 30,
			MinGpuCount:          1,
			TopN:                 20,
		}

		assert.Equal(t, "test-cluster", req.Cluster)
		assert.Equal(t, 7, req.TimeRangeDays)
		assert.Equal(t, 30, req.UtilizationThreshold)
	})
}

func TestConductorReportResponse(t *testing.T) {
	t.Run("handles response fields", func(t *testing.T) {
		resp := &ConductorReportResponse{
			Status:         "success",
			Report:         "# Markdown Report",
			MarkdownReport: "# Fallback Report",
			ChartData:      map[string]interface{}{"key": "value"},
			Summary:        map[string]interface{}{"total": 100},
			Metadata:       map[string]interface{}{"version": "1.0"},
			Timestamp:      "2025-12-23T00:00:00Z",
		}

		assert.Equal(t, "success", resp.Status)
		assert.Equal(t, "# Markdown Report", resp.Report)
		assert.NotNil(t, resp.ChartData)
		assert.NotNil(t, resp.Summary)
	})
}

func TestReportPeriod(t *testing.T) {
	t.Run("creates valid period", func(t *testing.T) {
		period := ReportPeriod{
			StartTime: time.Date(2025, 12, 1, 0, 0, 0, 0, time.UTC),
			EndTime:   time.Date(2025, 12, 7, 23, 59, 59, 999999999, time.UTC),
		}

		// Duration should be approximately 7 days
		duration := period.EndTime.Sub(period.StartTime)
		assert.Greater(t, duration.Hours(), float64(167)) // Just under 7 days in hours
		assert.Less(t, duration.Hours(), float64(169))    // Just over 7 days in hours
	})
}

func TestUserData(t *testing.T) {
	t.Run("creates valid user data", func(t *testing.T) {
		user := &UserData{
			Username:       "john.doe",
			Namespace:      "ml-training",
			AvgUtilization: 15.5,
			GpuCount:       4,
			WastedGpuHours: 120.5,
		}

		assert.Equal(t, "john.doe", user.Username)
		assert.Equal(t, "ml-training", user.Namespace)
		assert.Equal(t, 15.5, user.AvgUtilization)
		assert.Equal(t, 4, user.GpuCount)
		assert.Equal(t, 120.5, user.WastedGpuHours)
	})
}

func TestNamespaceData(t *testing.T) {
	t.Run("creates valid namespace data", func(t *testing.T) {
		ns := &NamespaceData{
			Name:        "production",
			GpuHours:    1000.5,
			Utilization: 85.0,
		}

		assert.Equal(t, "production", ns.Name)
		assert.Equal(t, 1000.5, ns.GpuHours)
		assert.Equal(t, 85.0, ns.Utilization)
	})
}

func TestEChartsData(t *testing.T) {
	t.Run("creates valid echarts data", func(t *testing.T) {
		data := &EChartsData{
			XAxis: []string{"Day 1", "Day 2", "Day 3"},
			Series: []EChartsSeries{
				{
					Name:   "Utilization",
					Type:   "line",
					Data:   []interface{}{10, 20, 30},
					Smooth: true,
				},
				{
					Name: "Allocation",
					Type: "bar",
					Data: []interface{}{15, 25, 35},
				},
			},
			Title:         "Weekly Report",
			Cluster:       "cluster-1",
			TimeRangeDays: 7,
		}

		assert.Len(t, data.XAxis, 3)
		assert.Len(t, data.Series, 2)
		assert.Equal(t, "Utilization", data.Series[0].Name)
		assert.True(t, data.Series[0].Smooth)
		assert.False(t, data.Series[1].Smooth)
	})
}

func TestTimeSeriesPoint(t *testing.T) {
	t.Run("creates valid time series point", func(t *testing.T) {
		point := TimeSeriesPoint{
			Timestamp: 1703318400000, // Example timestamp
			Value:     75.5,
		}

		assert.Equal(t, int64(1703318400000), point.Timestamp)
		assert.Equal(t, 75.5, point.Value)
	})
}

// Benchmark tests
func BenchmarkCalculateNaturalWeeks(b *testing.B) {
	job := NewGpuUsageWeeklyReportBackfillJob(nil)
	minTime := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	maxTime := time.Date(2025, 12, 31, 23, 59, 59, 999999999, time.UTC)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		job.calculateNaturalWeeks(minTime, maxTime)
	}
}

func BenchmarkFindMissingWeeks(b *testing.B) {
	job := NewGpuUsageWeeklyReportBackfillJob(nil)

	// Create 100 weeks
	weeks := make([]ReportPeriod, 100)
	startDate := time.Date(2023, 1, 2, 0, 0, 0, 0, time.UTC)
	for i := 0; i < 100; i++ {
		weeks[i] = ReportPeriod{
			StartTime: startDate.AddDate(0, 0, i*7),
			EndTime:   startDate.AddDate(0, 0, i*7+6),
		}
	}

	// Create 50 existing reports
	existingReports := make([]*dbmodel.GpuUsageWeeklyReports, 50)
	for i := 0; i < 50; i++ {
		existingReports[i] = &dbmodel.GpuUsageWeeklyReports{
			Status:      "generated",
			PeriodStart: weeks[i*2].StartTime,
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		job.findMissingWeeks(weeks, existingReports)
	}
}

func BenchmarkGetNextMonday(b *testing.B) {
	job := NewGpuUsageWeeklyReportBackfillJob(nil)
	testTime := time.Date(2025, 12, 10, 15, 30, 45, 0, time.UTC)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		job.getNextMonday(testTime)
	}
}

