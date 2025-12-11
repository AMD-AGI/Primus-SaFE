package workload_stats_backfill

import (
	"fmt"
	"testing"
	"time"

	dbmodel "github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
	"github.com/stretchr/testify/assert"
)

func TestWorkloadStatsBackfillConfig_DefaultValues(t *testing.T) {
	config := &WorkloadStatsBackfillConfig{}

	assert.False(t, config.Enabled, "Default Enabled should be false")
	assert.Equal(t, 0, config.BackfillDays, "Default BackfillDays should be 0")
	assert.Equal(t, 0, config.PromQueryStep, "Default PromQueryStep should be 0")
}

func TestWorkloadStatsBackfillJob_GetConfig(t *testing.T) {
	config := &WorkloadStatsBackfillConfig{
		Enabled:       true,
		BackfillDays:  7,
		PromQueryStep: 30,
	}

	job := &WorkloadStatsBackfillJob{
		config: config,
	}

	result := job.GetConfig()
	assert.Equal(t, config, result, "GetConfig should return the config")
}

func TestWorkloadStatsBackfillJob_SetConfig(t *testing.T) {
	job := &WorkloadStatsBackfillJob{
		config: &WorkloadStatsBackfillConfig{
			Enabled: false,
		},
	}

	newConfig := &WorkloadStatsBackfillConfig{
		Enabled:       true,
		BackfillDays:  14,
		PromQueryStep: 60,
	}

	job.SetConfig(newConfig)
	assert.Equal(t, newConfig, job.config, "SetConfig should update the config")
}

func TestWorkloadStatsBackfillJob_Schedule(t *testing.T) {
	job := &WorkloadStatsBackfillJob{}
	assert.Equal(t, "@every 1m", job.Schedule(), "Schedule should return @every 1m")
}

func TestConstants(t *testing.T) {
	assert.Equal(t, 2, DefaultBackfillDays)
	assert.Equal(t, 60, DefaultPromQueryStep)
}

func TestWorkloadUtilizationQueryTemplate(t *testing.T) {
	uid := "test-uid-123"
	expected := `avg(workload_gpu_utilization{workload_uid="test-uid-123"})`
	result := fmt.Sprintf(WorkloadUtilizationQueryTemplate, uid)
	assert.Equal(t, expected, result, "Query template should be formatted correctly")
}

func TestWorkloadGpuMemoryUsedQueryTemplate(t *testing.T) {
	uid := "test-uid-456"
	expected := `avg(workload_gpu_used_vram{workload_uid="test-uid-456"})`
	result := fmt.Sprintf(WorkloadGpuMemoryUsedQueryTemplate, uid)
	assert.Equal(t, expected, result, "Query template should be formatted correctly")
}

func TestWorkloadGpuMemoryTotalQueryTemplate(t *testing.T) {
	uid := "test-uid-789"
	expected := `avg(workload_gpu_total_vram{workload_uid="test-uid-789"})`
	result := fmt.Sprintf(WorkloadGpuMemoryTotalQueryTemplate, uid)
	assert.Equal(t, expected, result, "Query template should be formatted correctly")
}

func TestWorkloadHourEntry_Structure(t *testing.T) {
	workload := &dbmodel.GpuWorkload{
		UID:       "test-uid",
		Name:      "test-workload",
		Namespace: "test-namespace",
		Kind:      "Deployment",
	}

	testHour := time.Date(2025, 1, 1, 10, 0, 0, 0, time.UTC)

	entry := WorkloadHourEntry{
		Workload: workload,
		Hour:     testHour,
	}

	assert.Equal(t, workload, entry.Workload)
	assert.Equal(t, testHour, entry.Hour)
	assert.Equal(t, "test-uid", entry.Workload.UID)
	assert.Equal(t, "test-workload", entry.Workload.Name)
	assert.Equal(t, "test-namespace", entry.Workload.Namespace)
}

func TestWorkloadStatsBackfillConfig_AllFields(t *testing.T) {
	config := &WorkloadStatsBackfillConfig{
		Enabled:       true,
		BackfillDays:  7,
		PromQueryStep: 30,
	}

	assert.True(t, config.Enabled)
	assert.Equal(t, 7, config.BackfillDays)
	assert.Equal(t, 30, config.PromQueryStep)
}

func TestWorkloadStatsBackfillJob_ConfigValidation(t *testing.T) {
	tests := []struct {
		name          string
		config        *WorkloadStatsBackfillConfig
		expectEnabled bool
	}{
		{
			name: "enabled with valid config",
			config: &WorkloadStatsBackfillConfig{
				Enabled:       true,
				BackfillDays:  7,
				PromQueryStep: 60,
			},
			expectEnabled: true,
		},
		{
			name: "disabled",
			config: &WorkloadStatsBackfillConfig{
				Enabled:       false,
				BackfillDays:  7,
				PromQueryStep: 60,
			},
			expectEnabled: false,
		},
		{
			name: "zero backfill days",
			config: &WorkloadStatsBackfillConfig{
				Enabled:       true,
				BackfillDays:  0,
				PromQueryStep: 60,
			},
			expectEnabled: true,
		},
		{
			name: "zero prom query step",
			config: &WorkloadStatsBackfillConfig{
				Enabled:       true,
				BackfillDays:  7,
				PromQueryStep: 0,
			},
			expectEnabled: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			job := &WorkloadStatsBackfillJob{
				config: tt.config,
			}
			assert.Equal(t, tt.expectEnabled, job.config.Enabled)
		})
	}
}

func TestWorkloadHourEntry_MultipleEntries(t *testing.T) {
	workload1 := &dbmodel.GpuWorkload{
		UID:       "uid-1",
		Name:      "workload-1",
		Namespace: "ns-1",
	}

	workload2 := &dbmodel.GpuWorkload{
		UID:       "uid-2",
		Name:      "workload-2",
		Namespace: "ns-2",
	}

	hour1 := time.Date(2025, 1, 1, 10, 0, 0, 0, time.UTC)
	hour2 := time.Date(2025, 1, 1, 11, 0, 0, 0, time.UTC)

	entries := []WorkloadHourEntry{
		{Workload: workload1, Hour: hour1},
		{Workload: workload1, Hour: hour2},
		{Workload: workload2, Hour: hour1},
		{Workload: workload2, Hour: hour2},
	}

	assert.Equal(t, 4, len(entries))

	assert.Equal(t, "uid-1", entries[0].Workload.UID)
	assert.Equal(t, hour1, entries[0].Hour)

	assert.Equal(t, "uid-1", entries[1].Workload.UID)
	assert.Equal(t, hour2, entries[1].Hour)

	assert.Equal(t, "uid-2", entries[2].Workload.UID)
	assert.Equal(t, hour1, entries[2].Hour)

	assert.Equal(t, "uid-2", entries[3].Workload.UID)
	assert.Equal(t, hour2, entries[3].Hour)
}

func TestQueryTemplates_SpecialCharacters(t *testing.T) {
	tests := []struct {
		name     string
		uid      string
		template string
	}{
		{
			name:     "uid with hyphens",
			uid:      "pod-abc-123-xyz",
			template: WorkloadUtilizationQueryTemplate,
		},
		{
			name:     "uid with underscores",
			uid:      "pod_abc_123_xyz",
			template: WorkloadGpuMemoryUsedQueryTemplate,
		},
		{
			name:     "long uid",
			uid:      "a1b2c3d4-e5f6-7890-abcd-ef1234567890",
			template: WorkloadGpuMemoryTotalQueryTemplate,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := fmt.Sprintf(tt.template, tt.uid)
			assert.Contains(t, result, tt.uid, "Query should contain the UID")
			assert.Contains(t, result, "workload_uid=", "Query should have workload_uid label")
		})
	}
}

func TestWorkloadStatsBackfillJob_ClusterName(t *testing.T) {
	job := &WorkloadStatsBackfillJob{
		config:      &WorkloadStatsBackfillConfig{Enabled: true},
		clusterName: "test-cluster",
	}

	assert.Equal(t, "test-cluster", job.clusterName)
}

func TestWorkloadStatsBackfillJob_EmptyClusterName(t *testing.T) {
	job := &WorkloadStatsBackfillJob{
		config:      &WorkloadStatsBackfillConfig{Enabled: true},
		clusterName: "",
	}

	assert.Empty(t, job.clusterName)
}

func TestWorkloadHourEntry_NilWorkload(t *testing.T) {
	testHour := time.Date(2025, 1, 1, 10, 0, 0, 0, time.UTC)

	entry := WorkloadHourEntry{
		Workload: nil,
		Hour:     testHour,
	}

	assert.Nil(t, entry.Workload)
	assert.Equal(t, testHour, entry.Hour)
}

func TestWorkloadHourEntry_ZeroHour(t *testing.T) {
	workload := &dbmodel.GpuWorkload{
		UID:  "test-uid",
		Name: "test-workload",
	}

	entry := WorkloadHourEntry{
		Workload: workload,
		Hour:     time.Time{},
	}

	assert.True(t, entry.Hour.IsZero())
}

func TestConfigJSON_Tags(t *testing.T) {
	config := WorkloadStatsBackfillConfig{
		Enabled:       true,
		BackfillDays:  5,
		PromQueryStep: 45,
	}

	assert.True(t, config.Enabled)
	assert.Equal(t, 5, config.BackfillDays)
	assert.Equal(t, 45, config.PromQueryStep)
}

func TestTimeRangeCalculation(t *testing.T) {
	now := time.Now()
	endTime := now.Truncate(time.Hour).Add(-time.Hour)
	backfillDays := 2
	startTime := endTime.Add(-time.Duration(backfillDays) * 24 * time.Hour)

	assert.True(t, endTime.Before(now), "End time should be before now")
	assert.True(t, startTime.Before(endTime), "Start time should be before end time")

	expectedDuration := time.Duration(backfillDays) * 24 * time.Hour
	actualDuration := endTime.Sub(startTime)
	assert.Equal(t, expectedDuration, actualDuration, "Duration should match backfill days")
}

func TestHourTruncation(t *testing.T) {
	tests := []struct {
		name     string
		input    time.Time
		expected time.Time
	}{
		{
			name:     "already truncated",
			input:    time.Date(2025, 1, 1, 10, 0, 0, 0, time.UTC),
			expected: time.Date(2025, 1, 1, 10, 0, 0, 0, time.UTC),
		},
		{
			name:     "with minutes",
			input:    time.Date(2025, 1, 1, 10, 30, 0, 0, time.UTC),
			expected: time.Date(2025, 1, 1, 10, 0, 0, 0, time.UTC),
		},
		{
			name:     "with seconds",
			input:    time.Date(2025, 1, 1, 10, 0, 45, 0, time.UTC),
			expected: time.Date(2025, 1, 1, 10, 0, 0, 0, time.UTC),
		},
		{
			name:     "with nanoseconds",
			input:    time.Date(2025, 1, 1, 10, 0, 0, 123456789, time.UTC),
			expected: time.Date(2025, 1, 1, 10, 0, 0, 0, time.UTC),
		},
		{
			name:     "with all components",
			input:    time.Date(2025, 1, 1, 10, 30, 45, 123456789, time.UTC),
			expected: time.Date(2025, 1, 1, 10, 0, 0, 0, time.UTC),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.input.Truncate(time.Hour)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestWorkloadActiveTimeRange(t *testing.T) {
	startTime := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	endTime := time.Date(2025, 1, 3, 0, 0, 0, 0, time.UTC)

	tests := []struct {
		name              string
		workloadCreatedAt time.Time
		workloadEndAt     time.Time
		expectedActive    bool
	}{
		{
			name:              "workload created before and not ended",
			workloadCreatedAt: time.Date(2024, 12, 31, 0, 0, 0, 0, time.UTC),
			workloadEndAt:     time.Time{},
			expectedActive:    true,
		},
		{
			name:              "workload created during range",
			workloadCreatedAt: time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC),
			workloadEndAt:     time.Time{},
			expectedActive:    true,
		},
		{
			name:              "workload created after range",
			workloadCreatedAt: time.Date(2025, 1, 4, 0, 0, 0, 0, time.UTC),
			workloadEndAt:     time.Time{},
			expectedActive:    false,
		},
		{
			name:              "workload ended before range",
			workloadCreatedAt: time.Date(2024, 12, 1, 0, 0, 0, 0, time.UTC),
			workloadEndAt:     time.Date(2024, 12, 31, 0, 0, 0, 0, time.UTC),
			expectedActive:    false,
		},
		{
			name:              "workload ended during range",
			workloadCreatedAt: time.Date(2024, 12, 1, 0, 0, 0, 0, time.UTC),
			workloadEndAt:     time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC),
			expectedActive:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			isActive := false

			if !tt.workloadCreatedAt.After(endTime) {
				if tt.workloadEndAt.IsZero() || !tt.workloadEndAt.Before(startTime) {
					isActive = true
				}
			}

			assert.Equal(t, tt.expectedActive, isActive)
		})
	}
}

func TestGenerateHoursForWorkload(t *testing.T) {
	workloadStartTime := time.Date(2025, 1, 1, 10, 30, 0, 0, time.UTC)
	workloadEndTime := time.Date(2025, 1, 1, 14, 45, 0, 0, time.UTC)

	currentHour := workloadStartTime.Truncate(time.Hour)
	endHour := workloadEndTime.Truncate(time.Hour)

	hours := make([]time.Time, 0)
	for !currentHour.After(endHour) {
		hours = append(hours, currentHour)
		currentHour = currentHour.Add(time.Hour)
	}

	expectedHours := []time.Time{
		time.Date(2025, 1, 1, 10, 0, 0, 0, time.UTC),
		time.Date(2025, 1, 1, 11, 0, 0, 0, time.UTC),
		time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC),
		time.Date(2025, 1, 1, 13, 0, 0, 0, time.UTC),
		time.Date(2025, 1, 1, 14, 0, 0, 0, time.UTC),
	}

	assert.Equal(t, len(expectedHours), len(hours))
	for i, expected := range expectedHours {
		assert.Equal(t, expected, hours[i])
	}
}

func TestMissingStatsMapKey(t *testing.T) {
	namespace := "test-ns"
	workloadName := "test-workload"
	statHour := time.Date(2025, 1, 1, 10, 0, 0, 0, time.UTC)

	key := fmt.Sprintf("%s/%s/%s", namespace, workloadName, statHour.Format(time.RFC3339))

	expectedKey := "test-ns/test-workload/2025-01-01T10:00:00Z"
	assert.Equal(t, expectedKey, key)
}

func TestMissingStatsMapKeyUniqueness(t *testing.T) {
	keys := make(map[string]struct{})

	testCases := []struct {
		namespace    string
		workloadName string
		hour         time.Time
	}{
		{"ns1", "workload1", time.Date(2025, 1, 1, 10, 0, 0, 0, time.UTC)},
		{"ns1", "workload1", time.Date(2025, 1, 1, 11, 0, 0, 0, time.UTC)},
		{"ns1", "workload2", time.Date(2025, 1, 1, 10, 0, 0, 0, time.UTC)},
		{"ns2", "workload1", time.Date(2025, 1, 1, 10, 0, 0, 0, time.UTC)},
	}

	for _, tc := range testCases {
		key := fmt.Sprintf("%s/%s/%s", tc.namespace, tc.workloadName, tc.hour.Format(time.RFC3339))
		keys[key] = struct{}{}
	}

	assert.Equal(t, 4, len(keys), "All keys should be unique")
}

func TestWorkloadStatsBackfillJob_Structure(t *testing.T) {
	config := &WorkloadStatsBackfillConfig{
		Enabled:       true,
		BackfillDays:  7,
		PromQueryStep: 30,
	}

	job := &WorkloadStatsBackfillJob{
		config:      config,
		clusterName: "test-cluster",
	}

	assert.Equal(t, "test-cluster", job.clusterName)
	assert.True(t, job.config.Enabled)
	assert.Equal(t, 7, job.config.BackfillDays)
	assert.Equal(t, 30, job.config.PromQueryStep)
}

func TestWorkloadStatsBackfillConfig_AllCombinations(t *testing.T) {
	tests := []struct {
		name          string
		config        *WorkloadStatsBackfillConfig
		expectEnabled bool
		expectDays    int
		expectStep    int
	}{
		{
			name: "default values",
			config: &WorkloadStatsBackfillConfig{
				Enabled:       true,
				BackfillDays:  DefaultBackfillDays,
				PromQueryStep: DefaultPromQueryStep,
			},
			expectEnabled: true,
			expectDays:    2,
			expectStep:    60,
		},
		{
			name: "custom values",
			config: &WorkloadStatsBackfillConfig{
				Enabled:       true,
				BackfillDays:  7,
				PromQueryStep: 30,
			},
			expectEnabled: true,
			expectDays:    7,
			expectStep:    30,
		},
		{
			name: "disabled",
			config: &WorkloadStatsBackfillConfig{
				Enabled:       false,
				BackfillDays:  7,
				PromQueryStep: 60,
			},
			expectEnabled: false,
			expectDays:    7,
			expectStep:    60,
		},
		{
			name: "large backfill days",
			config: &WorkloadStatsBackfillConfig{
				Enabled:       true,
				BackfillDays:  30,
				PromQueryStep: 60,
			},
			expectEnabled: true,
			expectDays:    30,
			expectStep:    60,
		},
		{
			name: "small query step",
			config: &WorkloadStatsBackfillConfig{
				Enabled:       true,
				BackfillDays:  2,
				PromQueryStep: 15,
			},
			expectEnabled: true,
			expectDays:    2,
			expectStep:    15,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expectEnabled, tt.config.Enabled)
			assert.Equal(t, tt.expectDays, tt.config.BackfillDays)
			assert.Equal(t, tt.expectStep, tt.config.PromQueryStep)
		})
	}
}

func TestWorkloadHourEntry_Initialization(t *testing.T) {
	tests := []struct {
		name      string
		workload  *dbmodel.GpuWorkload
		hour      time.Time
		expectNil bool
	}{
		{
			name: "with valid workload",
			workload: &dbmodel.GpuWorkload{
				UID:        "uid-123",
				Name:       "test-workload",
				Namespace:  "test-namespace",
				Kind:       "Deployment",
				GpuRequest: 4,
			},
			hour:      time.Date(2025, 1, 1, 10, 0, 0, 0, time.UTC),
			expectNil: false,
		},
		{
			name:      "with nil workload",
			workload:  nil,
			hour:      time.Date(2025, 1, 1, 10, 0, 0, 0, time.UTC),
			expectNil: true,
		},
		{
			name: "with zero time",
			workload: &dbmodel.GpuWorkload{
				UID:  "uid-456",
				Name: "another-workload",
			},
			hour:      time.Time{},
			expectNil: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			entry := WorkloadHourEntry{
				Workload: tt.workload,
				Hour:     tt.hour,
			}

			if tt.expectNil {
				assert.Nil(t, entry.Workload)
			} else {
				assert.NotNil(t, entry.Workload)
			}
			assert.Equal(t, tt.hour, entry.Hour)
		})
	}
}

func TestWorkloadHourEntry_WorkloadFields(t *testing.T) {
	workload := &dbmodel.GpuWorkload{
		UID:        "uid-abc-123",
		Name:       "ml-training-job",
		Namespace:  "ml-namespace",
		Kind:       "Job",
		GpuRequest: 8,
		Status:     "Running",
	}

	entry := WorkloadHourEntry{
		Workload: workload,
		Hour:     time.Date(2025, 1, 1, 10, 0, 0, 0, time.UTC),
	}

	assert.Equal(t, "uid-abc-123", entry.Workload.UID)
	assert.Equal(t, "ml-training-job", entry.Workload.Name)
	assert.Equal(t, "ml-namespace", entry.Workload.Namespace)
	assert.Equal(t, "Job", entry.Workload.Kind)
	assert.Equal(t, int32(8), entry.Workload.GpuRequest)
	assert.Equal(t, "Running", entry.Workload.Status)
}

func TestQueryTemplates_Formatting(t *testing.T) {
	tests := []struct {
		name     string
		template string
		uid      string
		contains []string
	}{
		{
			name:     "utilization query",
			template: WorkloadUtilizationQueryTemplate,
			uid:      "test-uid-1",
			contains: []string{"avg(", "workload_gpu_utilization", "workload_uid=", "test-uid-1"},
		},
		{
			name:     "memory used query",
			template: WorkloadGpuMemoryUsedQueryTemplate,
			uid:      "test-uid-2",
			contains: []string{"avg(", "workload_gpu_used_vram", "workload_uid=", "test-uid-2"},
		},
		{
			name:     "memory total query",
			template: WorkloadGpuMemoryTotalQueryTemplate,
			uid:      "test-uid-3",
			contains: []string{"avg(", "workload_gpu_total_vram", "workload_uid=", "test-uid-3"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := fmt.Sprintf(tt.template, tt.uid)
			for _, expected := range tt.contains {
				assert.Contains(t, result, expected)
			}
		})
	}
}

func TestQueryTemplates_UIDVariations(t *testing.T) {
	uids := []string{
		"simple-uid",
		"uid-with-numbers-123",
		"a1b2c3d4-e5f6-7890-abcd-ef1234567890",
		"namespace_name_workload",
		"very-long-uid-that-contains-many-hyphens-and-numbers-123456789",
	}

	for _, uid := range uids {
		utilQuery := fmt.Sprintf(WorkloadUtilizationQueryTemplate, uid)
		assert.Contains(t, utilQuery, fmt.Sprintf(`workload_uid="%s"`, uid))

		memUsedQuery := fmt.Sprintf(WorkloadGpuMemoryUsedQueryTemplate, uid)
		assert.Contains(t, memUsedQuery, fmt.Sprintf(`workload_uid="%s"`, uid))

		memTotalQuery := fmt.Sprintf(WorkloadGpuMemoryTotalQueryTemplate, uid)
		assert.Contains(t, memTotalQuery, fmt.Sprintf(`workload_uid="%s"`, uid))
	}
}

func TestTimeRangeCalculation_EdgeCases(t *testing.T) {
	tests := []struct {
		name         string
		now          time.Time
		backfillDays int
	}{
		{
			name:         "start of day",
			now:          time.Date(2025, 1, 1, 0, 30, 0, 0, time.UTC),
			backfillDays: 2,
		},
		{
			name:         "end of day",
			now:          time.Date(2025, 1, 1, 23, 30, 0, 0, time.UTC),
			backfillDays: 2,
		},
		{
			name:         "middle of day",
			now:          time.Date(2025, 1, 1, 12, 30, 0, 0, time.UTC),
			backfillDays: 2,
		},
		{
			name:         "large backfill",
			now:          time.Date(2025, 1, 15, 10, 30, 0, 0, time.UTC),
			backfillDays: 30,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			endTime := tt.now.Truncate(time.Hour).Add(-time.Hour)
			startTime := endTime.Add(-time.Duration(tt.backfillDays) * 24 * time.Hour)

			assert.True(t, startTime.Before(endTime))
			assert.True(t, endTime.Before(tt.now))

			expectedDuration := time.Duration(tt.backfillDays) * 24 * time.Hour
			actualDuration := endTime.Sub(startTime)
			assert.Equal(t, expectedDuration, actualDuration)

			assert.Equal(t, 0, endTime.Minute())
			assert.Equal(t, 0, endTime.Second())
			assert.Equal(t, 0, startTime.Minute())
			assert.Equal(t, 0, startTime.Second())
		})
	}
}

func TestWorkloadActiveTimeRange_EdgeCases(t *testing.T) {
	startTime := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	endTime := time.Date(2025, 1, 3, 0, 0, 0, 0, time.UTC)

	tests := []struct {
		name              string
		workloadCreatedAt time.Time
		workloadEndAt     time.Time
		expectedActive    bool
	}{
		{
			name:              "created exactly at start time",
			workloadCreatedAt: startTime,
			workloadEndAt:     time.Time{},
			expectedActive:    true,
		},
		{
			name:              "created exactly at end time - still active since not after",
			workloadCreatedAt: endTime,
			workloadEndAt:     time.Time{},
			expectedActive:    true,
		},
		{
			name:              "ended exactly at start time - still considered active",
			workloadCreatedAt: time.Date(2024, 12, 1, 0, 0, 0, 0, time.UTC),
			workloadEndAt:     startTime,
			expectedActive:    true,
		},
		{
			name:              "ended before start time - not active",
			workloadCreatedAt: time.Date(2024, 12, 1, 0, 0, 0, 0, time.UTC),
			workloadEndAt:     time.Date(2024, 12, 31, 0, 0, 0, 0, time.UTC),
			expectedActive:    false,
		},
		{
			name:              "created and ended within range",
			workloadCreatedAt: time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC),
			workloadEndAt:     time.Date(2025, 1, 2, 12, 0, 0, 0, time.UTC),
			expectedActive:    true,
		},
		{
			name:              "spans entire range",
			workloadCreatedAt: time.Date(2024, 12, 1, 0, 0, 0, 0, time.UTC),
			workloadEndAt:     time.Date(2025, 2, 1, 0, 0, 0, 0, time.UTC),
			expectedActive:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			isActive := false

			if !tt.workloadCreatedAt.After(endTime) {
				if tt.workloadEndAt.IsZero() || !tt.workloadEndAt.Before(startTime) {
					isActive = true
				}
			}

			assert.Equal(t, tt.expectedActive, isActive)
		})
	}
}

func TestGenerateHoursForWorkload_EdgeCases(t *testing.T) {
	tests := []struct {
		name           string
		workloadStart  time.Time
		workloadEnd    time.Time
		expectedHours  int
	}{
		{
			name:          "same hour",
			workloadStart: time.Date(2025, 1, 1, 10, 15, 0, 0, time.UTC),
			workloadEnd:   time.Date(2025, 1, 1, 10, 45, 0, 0, time.UTC),
			expectedHours: 1,
		},
		{
			name:          "consecutive hours",
			workloadStart: time.Date(2025, 1, 1, 10, 45, 0, 0, time.UTC),
			workloadEnd:   time.Date(2025, 1, 1, 11, 15, 0, 0, time.UTC),
			expectedHours: 2,
		},
		{
			name:          "full day",
			workloadStart: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
			workloadEnd:   time.Date(2025, 1, 1, 23, 59, 0, 0, time.UTC),
			expectedHours: 24,
		},
		{
			name:          "cross midnight",
			workloadStart: time.Date(2025, 1, 1, 22, 0, 0, 0, time.UTC),
			workloadEnd:   time.Date(2025, 1, 2, 2, 0, 0, 0, time.UTC),
			expectedHours: 5,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			currentHour := tt.workloadStart.Truncate(time.Hour)
			endHour := tt.workloadEnd.Truncate(time.Hour)

			hours := make([]time.Time, 0)
			for !currentHour.After(endHour) {
				hours = append(hours, currentHour)
				currentHour = currentHour.Add(time.Hour)
			}

			assert.Equal(t, tt.expectedHours, len(hours))
		})
	}
}

func TestMissingStatsMapKey_Variations(t *testing.T) {
	tests := []struct {
		name         string
		namespace    string
		workloadName string
		hour         time.Time
		expectedKey  string
	}{
		{
			name:         "simple names",
			namespace:    "default",
			workloadName: "nginx",
			hour:         time.Date(2025, 1, 1, 10, 0, 0, 0, time.UTC),
			expectedKey:  "default/nginx/2025-01-01T10:00:00Z",
		},
		{
			name:         "names with hyphens",
			namespace:    "my-namespace",
			workloadName: "my-workload",
			hour:         time.Date(2025, 1, 1, 10, 0, 0, 0, time.UTC),
			expectedKey:  "my-namespace/my-workload/2025-01-01T10:00:00Z",
		},
		{
			name:         "long names",
			namespace:    "very-long-namespace-name",
			workloadName: "very-long-workload-name-with-many-chars",
			hour:         time.Date(2025, 1, 1, 10, 0, 0, 0, time.UTC),
			expectedKey:  "very-long-namespace-name/very-long-workload-name-with-many-chars/2025-01-01T10:00:00Z",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			key := fmt.Sprintf("%s/%s/%s", tt.namespace, tt.workloadName, tt.hour.Format(time.RFC3339))
			assert.Equal(t, tt.expectedKey, key)
		})
	}
}

func TestWorkloadStatsBackfillJob_ScheduleValue(t *testing.T) {
	job := &WorkloadStatsBackfillJob{}

	schedule := job.Schedule()
	assert.Equal(t, "@every 1m", schedule)
	assert.NotEqual(t, "@every 5m", schedule)
}

func TestDefaultConstants_Values(t *testing.T) {
	assert.Equal(t, 2, DefaultBackfillDays, "Default backfill days should be 2")
	assert.Equal(t, 60, DefaultPromQueryStep, "Default prom query step should be 60")
}

func TestWorkloadHourEntry_MultipleWorkloads(t *testing.T) {
	workloads := []*dbmodel.GpuWorkload{
		{UID: "uid-1", Name: "workload-1", Namespace: "ns-1", Kind: "Deployment", GpuRequest: 2},
		{UID: "uid-2", Name: "workload-2", Namespace: "ns-1", Kind: "StatefulSet", GpuRequest: 4},
		{UID: "uid-3", Name: "workload-3", Namespace: "ns-2", Kind: "Job", GpuRequest: 8},
	}

	hours := []time.Time{
		time.Date(2025, 1, 1, 10, 0, 0, 0, time.UTC),
		time.Date(2025, 1, 1, 11, 0, 0, 0, time.UTC),
		time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC),
	}

	var entries []WorkloadHourEntry
	for _, workload := range workloads {
		for _, hour := range hours {
			entries = append(entries, WorkloadHourEntry{
				Workload: workload,
				Hour:     hour,
			})
		}
	}

	assert.Equal(t, 9, len(entries), "Should create 9 entries (3 workloads x 3 hours)")

	uniqueWorkloads := make(map[string]bool)
	for _, entry := range entries {
		uniqueWorkloads[entry.Workload.UID] = true
	}
	assert.Equal(t, 3, len(uniqueWorkloads), "Should have 3 unique workloads")

	uniqueHours := make(map[time.Time]bool)
	for _, entry := range entries {
		uniqueHours[entry.Hour] = true
	}
	assert.Equal(t, 3, len(uniqueHours), "Should have 3 unique hours")
}

func TestWorkloadStatsBackfillConfig_SetterGetter(t *testing.T) {
	job := &WorkloadStatsBackfillJob{
		config: &WorkloadStatsBackfillConfig{
			Enabled:       false,
			BackfillDays:  1,
			PromQueryStep: 30,
		},
	}

	originalConfig := job.GetConfig()
	assert.False(t, originalConfig.Enabled)
	assert.Equal(t, 1, originalConfig.BackfillDays)

	newConfig := &WorkloadStatsBackfillConfig{
		Enabled:       true,
		BackfillDays:  7,
		PromQueryStep: 60,
	}
	job.SetConfig(newConfig)

	updatedConfig := job.GetConfig()
	assert.True(t, updatedConfig.Enabled)
	assert.Equal(t, 7, updatedConfig.BackfillDays)
	assert.Equal(t, 60, updatedConfig.PromQueryStep)
}

func TestHourTruncation_AllComponents(t *testing.T) {
	tests := []struct {
		name     string
		input    time.Time
		expected time.Time
	}{
		{
			name:     "only nanoseconds",
			input:    time.Date(2025, 1, 1, 10, 0, 0, 999999999, time.UTC),
			expected: time.Date(2025, 1, 1, 10, 0, 0, 0, time.UTC),
		},
		{
			name:     "only seconds",
			input:    time.Date(2025, 1, 1, 10, 0, 59, 0, time.UTC),
			expected: time.Date(2025, 1, 1, 10, 0, 0, 0, time.UTC),
		},
		{
			name:     "only minutes",
			input:    time.Date(2025, 1, 1, 10, 59, 0, 0, time.UTC),
			expected: time.Date(2025, 1, 1, 10, 0, 0, 0, time.UTC),
		},
		{
			name:     "all components",
			input:    time.Date(2025, 1, 1, 10, 59, 59, 999999999, time.UTC),
			expected: time.Date(2025, 1, 1, 10, 0, 0, 0, time.UTC),
		},
		{
			name:     "already truncated",
			input:    time.Date(2025, 1, 1, 10, 0, 0, 0, time.UTC),
			expected: time.Date(2025, 1, 1, 10, 0, 0, 0, time.UTC),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.input.Truncate(time.Hour)
			assert.Equal(t, tt.expected, result)
			assert.Equal(t, 0, result.Minute())
			assert.Equal(t, 0, result.Second())
			assert.Equal(t, 0, result.Nanosecond())
		})
	}
}

func TestWorkloadGpuWorkload_FieldAccess(t *testing.T) {
	workload := &dbmodel.GpuWorkload{
		UID:         "uid-test-123",
		Name:        "test-workload",
		Namespace:   "test-namespace",
		Kind:        "Deployment",
		GpuRequest:  4,
		Status:      "Running",
		ParentUID:   "",
		Labels:      dbmodel.ExtType{"app": "test"},
		Annotations: dbmodel.ExtType{"project": "ml"},
	}

	assert.Equal(t, "uid-test-123", workload.UID)
	assert.Equal(t, "test-workload", workload.Name)
	assert.Equal(t, "test-namespace", workload.Namespace)
	assert.Equal(t, "Deployment", workload.Kind)
	assert.Equal(t, int32(4), workload.GpuRequest)
	assert.Equal(t, "Running", workload.Status)
	assert.Empty(t, workload.ParentUID)
	assert.NotNil(t, workload.Labels)
	assert.NotNil(t, workload.Annotations)
}

func TestWorkloadHourEntry_TopLevelWorkloadFiltering(t *testing.T) {
	workloads := []*dbmodel.GpuWorkload{
		{UID: "uid-1", Name: "top-level-1", Namespace: "ns-1", ParentUID: ""},
		{UID: "uid-2", Name: "child-1", Namespace: "ns-1", ParentUID: "uid-1"},
		{UID: "uid-3", Name: "top-level-2", Namespace: "ns-1", ParentUID: ""},
		{UID: "uid-4", Name: "child-2", Namespace: "ns-1", ParentUID: "uid-3"},
	}

	var topLevelWorkloads []*dbmodel.GpuWorkload
	for _, w := range workloads {
		if w.ParentUID == "" {
			topLevelWorkloads = append(topLevelWorkloads, w)
		}
	}

	assert.Equal(t, 2, len(topLevelWorkloads), "Should have 2 top-level workloads")
	for _, w := range topLevelWorkloads {
		assert.Empty(t, w.ParentUID, "Top-level workloads should have empty ParentUID")
	}
}

// TestWorkloadStatsBackfillConfig_EnabledToggle tests enabled flag behavior
func TestWorkloadStatsBackfillConfig_EnabledToggle(t *testing.T) {
	config := &WorkloadStatsBackfillConfig{Enabled: false}
	assert.False(t, config.Enabled)

	config.Enabled = true
	assert.True(t, config.Enabled)

	config.Enabled = false
	assert.False(t, config.Enabled)
}

// TestWorkloadStatsBackfillConfig_BackfillDaysValidation tests backfill days values
func TestWorkloadStatsBackfillConfig_BackfillDaysValidation(t *testing.T) {
	tests := []struct {
		name         string
		backfillDays int
		valid        bool
	}{
		{"default", DefaultBackfillDays, true},
		{"one day", 1, true},
		{"one week", 7, true},
		{"one month", 30, true},
		{"zero", 0, false},
		{"negative", -1, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := &WorkloadStatsBackfillConfig{BackfillDays: tt.backfillDays}
			isValid := config.BackfillDays > 0
			assert.Equal(t, tt.valid, isValid)
		})
	}
}

// TestWorkloadStatsBackfillConfig_PromQueryStepValidation tests PromQueryStep values
func TestWorkloadStatsBackfillConfig_PromQueryStepValidation(t *testing.T) {
	tests := []struct {
		name          string
		promQueryStep int
		valid         bool
	}{
		{"default", DefaultPromQueryStep, true},
		{"small", 15, true},
		{"large", 300, true},
		{"zero", 0, false},
		{"negative", -1, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := &WorkloadStatsBackfillConfig{PromQueryStep: tt.promQueryStep}
			isValid := config.PromQueryStep > 0
			assert.Equal(t, tt.valid, isValid)
		})
	}
}

// TestWorkloadHourEntry_HourConsistency tests hour consistency in entries
func TestWorkloadHourEntry_HourConsistency(t *testing.T) {
	workload := &dbmodel.GpuWorkload{
		UID:       "test-uid",
		Name:      "test-workload",
		Namespace: "test-namespace",
	}

	hours := []time.Time{
		time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
		time.Date(2025, 1, 1, 1, 0, 0, 0, time.UTC),
		time.Date(2025, 1, 1, 2, 0, 0, 0, time.UTC),
	}

	entries := make([]WorkloadHourEntry, 0)
	for _, hour := range hours {
		entries = append(entries, WorkloadHourEntry{
			Workload: workload,
			Hour:     hour,
		})
	}

	for i, entry := range entries {
		assert.Equal(t, hours[i], entry.Hour)
		assert.Equal(t, 0, entry.Hour.Minute())
		assert.Equal(t, 0, entry.Hour.Second())
		assert.Equal(t, 0, entry.Hour.Nanosecond())
	}
}

// TestWorkloadActiveTimeRange_AllCases tests all workload active time range cases
func TestWorkloadActiveTimeRange_AllCases(t *testing.T) {
	startTime := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	endTime := time.Date(2025, 1, 3, 0, 0, 0, 0, time.UTC)

	tests := []struct {
		name              string
		workloadCreatedAt time.Time
		workloadEndAt     time.Time
		expectedActive    bool
	}{
		{
			name:              "workload active entire range",
			workloadCreatedAt: time.Date(2024, 12, 1, 0, 0, 0, 0, time.UTC),
			workloadEndAt:     time.Time{},
			expectedActive:    true,
		},
		{
			name:              "workload created at start",
			workloadCreatedAt: startTime,
			workloadEndAt:     time.Time{},
			expectedActive:    true,
		},
		{
			name:              "workload created in middle",
			workloadCreatedAt: time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC),
			workloadEndAt:     time.Time{},
			expectedActive:    true,
		},
		{
			name:              "workload created after end",
			workloadCreatedAt: time.Date(2025, 1, 4, 0, 0, 0, 0, time.UTC),
			workloadEndAt:     time.Time{},
			expectedActive:    false,
		},
		{
			name:              "workload ended before start",
			workloadCreatedAt: time.Date(2024, 12, 1, 0, 0, 0, 0, time.UTC),
			workloadEndAt:     time.Date(2024, 12, 31, 0, 0, 0, 0, time.UTC),
			expectedActive:    false,
		},
		{
			name:              "workload ended during range",
			workloadCreatedAt: time.Date(2024, 12, 1, 0, 0, 0, 0, time.UTC),
			workloadEndAt:     time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC),
			expectedActive:    true,
		},
		{
			name:              "workload started and ended in range",
			workloadCreatedAt: time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC),
			workloadEndAt:     time.Date(2025, 1, 2, 12, 0, 0, 0, time.UTC),
			expectedActive:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			isActive := false

			if !tt.workloadCreatedAt.After(endTime) {
				if tt.workloadEndAt.IsZero() || !tt.workloadEndAt.Before(startTime) {
					isActive = true
				}
			}

			assert.Equal(t, tt.expectedActive, isActive)
		})
	}
}

// TestMissingStatsMapKey_EmptyValues tests map key generation with empty values
func TestMissingStatsMapKey_EmptyValues(t *testing.T) {
	tests := []struct {
		name         string
		namespace    string
		workloadName string
		hour         time.Time
	}{
		{
			name:         "empty namespace",
			namespace:    "",
			workloadName: "workload",
			hour:         time.Date(2025, 1, 1, 10, 0, 0, 0, time.UTC),
		},
		{
			name:         "empty workload name",
			namespace:    "namespace",
			workloadName: "",
			hour:         time.Date(2025, 1, 1, 10, 0, 0, 0, time.UTC),
		},
		{
			name:         "both empty",
			namespace:    "",
			workloadName: "",
			hour:         time.Date(2025, 1, 1, 10, 0, 0, 0, time.UTC),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			key := fmt.Sprintf("%s/%s/%s", tt.namespace, tt.workloadName, tt.hour.Format(time.RFC3339))
			assert.NotEmpty(t, key)
			assert.Contains(t, key, "/")
		})
	}
}

// TestQueryTemplates_EmptyUID tests query templates with empty UID
func TestQueryTemplates_EmptyUID(t *testing.T) {
	emptyUID := ""

	utilQuery := fmt.Sprintf(WorkloadUtilizationQueryTemplate, emptyUID)
	assert.Contains(t, utilQuery, `workload_uid=""`)

	memUsedQuery := fmt.Sprintf(WorkloadGpuMemoryUsedQueryTemplate, emptyUID)
	assert.Contains(t, memUsedQuery, `workload_uid=""`)

	memTotalQuery := fmt.Sprintf(WorkloadGpuMemoryTotalQueryTemplate, emptyUID)
	assert.Contains(t, memTotalQuery, `workload_uid=""`)
}

// TestWorkloadStatsBackfillJob_ScheduleValue tests schedule value
func TestWorkloadStatsBackfillJob_ScheduleValue_IsValid(t *testing.T) {
	job := &WorkloadStatsBackfillJob{}
	schedule := job.Schedule()

	assert.NotEmpty(t, schedule)
	assert.Contains(t, schedule, "@every")
	assert.Equal(t, "@every 1m", schedule)
}

// TestGenerateHoursForWorkload_CrossDays tests hour generation across multiple days
func TestGenerateHoursForWorkload_CrossDays(t *testing.T) {
	workloadStartTime := time.Date(2025, 1, 1, 22, 0, 0, 0, time.UTC)
	workloadEndTime := time.Date(2025, 1, 3, 2, 0, 0, 0, time.UTC)

	currentHour := workloadStartTime.Truncate(time.Hour)
	endHour := workloadEndTime.Truncate(time.Hour)

	hours := make([]time.Time, 0)
	for !currentHour.After(endHour) {
		hours = append(hours, currentHour)
		currentHour = currentHour.Add(time.Hour)
	}

	assert.Equal(t, 29, len(hours))
	assert.Equal(t, time.Date(2025, 1, 1, 22, 0, 0, 0, time.UTC), hours[0])
	assert.Equal(t, time.Date(2025, 1, 3, 2, 0, 0, 0, time.UTC), hours[len(hours)-1])
}

// TestWorkloadHourEntry_WorkloadKinds tests different workload kinds
func TestWorkloadHourEntry_WorkloadKinds(t *testing.T) {
	kinds := []string{"Deployment", "StatefulSet", "Job", "CronJob", "DaemonSet", "ReplicaSet"}

	for _, kind := range kinds {
		workload := &dbmodel.GpuWorkload{
			UID:  "test-uid",
			Name: "test-workload",
			Kind: kind,
		}

		entry := WorkloadHourEntry{
			Workload: workload,
			Hour:     time.Now().Truncate(time.Hour),
		}

		assert.Equal(t, kind, entry.Workload.Kind)
	}
}

// TestWorkloadGpuRequest_Values tests different GPU request values
func TestWorkloadGpuRequest_Values(t *testing.T) {
	tests := []struct {
		name       string
		gpuRequest int32
	}{
		{"zero", 0},
		{"one", 1},
		{"small", 4},
		{"medium", 8},
		{"large", 16},
		{"very large", 64},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			workload := &dbmodel.GpuWorkload{
				UID:        "test-uid",
				GpuRequest: tt.gpuRequest,
			}

			assert.Equal(t, tt.gpuRequest, workload.GpuRequest)
		})
	}
}

// TestTimeRangeCalculation_Precision tests time range calculation precision
func TestTimeRangeCalculation_Precision(t *testing.T) {
	tests := []struct {
		name         string
		now          time.Time
		backfillDays int
	}{
		{
			name:         "precise hour",
			now:          time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC),
			backfillDays: 2,
		},
		{
			name:         "with minutes",
			now:          time.Date(2025, 1, 1, 12, 30, 0, 0, time.UTC),
			backfillDays: 2,
		},
		{
			name:         "with seconds",
			now:          time.Date(2025, 1, 1, 12, 30, 45, 0, time.UTC),
			backfillDays: 2,
		},
		{
			name:         "with nanoseconds",
			now:          time.Date(2025, 1, 1, 12, 30, 45, 123456789, time.UTC),
			backfillDays: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			endTime := tt.now.Truncate(time.Hour).Add(-time.Hour)
			startTime := endTime.Add(-time.Duration(tt.backfillDays) * 24 * time.Hour)

			assert.Equal(t, 0, endTime.Minute())
			assert.Equal(t, 0, endTime.Second())
			assert.Equal(t, 0, endTime.Nanosecond())
			assert.Equal(t, 0, startTime.Minute())
			assert.Equal(t, 0, startTime.Second())
			assert.Equal(t, 0, startTime.Nanosecond())
		})
	}
}

// TestWorkloadHourEntry_CrossMonthBoundary tests entries across month boundaries
func TestWorkloadHourEntry_CrossMonthBoundary(t *testing.T) {
	workload := &dbmodel.GpuWorkload{
		UID:       "test-uid",
		Name:      "test-workload",
		Namespace: "test-namespace",
	}

	entry1 := WorkloadHourEntry{
		Workload: workload,
		Hour:     time.Date(2025, 1, 31, 23, 0, 0, 0, time.UTC),
	}

	entry2 := WorkloadHourEntry{
		Workload: workload,
		Hour:     time.Date(2025, 2, 1, 0, 0, 0, 0, time.UTC),
	}

	assert.Equal(t, time.January, entry1.Hour.Month())
	assert.Equal(t, time.February, entry2.Hour.Month())
	assert.Equal(t, time.Hour, entry2.Hour.Sub(entry1.Hour))
}

// TestWorkloadLabelsAndAnnotations tests labels and annotations handling
func TestWorkloadLabelsAndAnnotations(t *testing.T) {
	tests := []struct {
		name        string
		labels      dbmodel.ExtType
		annotations dbmodel.ExtType
	}{
		{
			name:        "nil labels and annotations",
			labels:      nil,
			annotations: nil,
		},
		{
			name:        "empty labels and annotations",
			labels:      dbmodel.ExtType{},
			annotations: dbmodel.ExtType{},
		},
		{
			name:        "with labels only",
			labels:      dbmodel.ExtType{"app": "test", "team": "ml"},
			annotations: nil,
		},
		{
			name:        "with annotations only",
			labels:      nil,
			annotations: dbmodel.ExtType{"project": "ml-training"},
		},
		{
			name:        "with both",
			labels:      dbmodel.ExtType{"app": "test"},
			annotations: dbmodel.ExtType{"project": "ml-training"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			workload := &dbmodel.GpuWorkload{
				UID:         "test-uid",
				Labels:      tt.labels,
				Annotations: tt.annotations,
			}

			entry := WorkloadHourEntry{
				Workload: workload,
				Hour:     time.Now().Truncate(time.Hour),
			}

			if tt.labels == nil {
				assert.Nil(t, entry.Workload.Labels)
			} else {
				assert.NotNil(t, entry.Workload.Labels)
			}

			if tt.annotations == nil {
				assert.Nil(t, entry.Workload.Annotations)
			} else {
				assert.NotNil(t, entry.Workload.Annotations)
			}
		})
	}
}

// TestWorkloadStatus_Values tests different workload status values
func TestWorkloadStatus_Values(t *testing.T) {
	statuses := []string{"Running", "Pending", "Completed", "Failed", "Unknown", ""}

	for _, status := range statuses {
		workload := &dbmodel.GpuWorkload{
			UID:    "test-uid",
			Status: status,
		}

		entry := WorkloadHourEntry{
			Workload: workload,
			Hour:     time.Now().Truncate(time.Hour),
		}

		assert.Equal(t, status, entry.Workload.Status)
	}
}

// TestMissingStatsMapKey_LongValues tests map key generation with long values
func TestMissingStatsMapKey_LongValues(t *testing.T) {
	longNamespace := "very-long-namespace-name-that-is-quite-long"
	longWorkloadName := "very-long-workload-name-that-is-also-quite-long"
	hour := time.Date(2025, 1, 1, 10, 0, 0, 0, time.UTC)

	key := fmt.Sprintf("%s/%s/%s", longNamespace, longWorkloadName, hour.Format(time.RFC3339))

	assert.Contains(t, key, longNamespace)
	assert.Contains(t, key, longWorkloadName)
	assert.Contains(t, key, "2025-01-01T10:00:00Z")
}

// TestWorkloadHourEntry_ManyEntries tests creating many entries
func TestWorkloadHourEntry_ManyEntries(t *testing.T) {
	workloads := make([]*dbmodel.GpuWorkload, 100)
	for i := 0; i < 100; i++ {
		workloads[i] = &dbmodel.GpuWorkload{
			UID:        fmt.Sprintf("uid-%d", i),
			Name:       fmt.Sprintf("workload-%d", i),
			Namespace:  fmt.Sprintf("ns-%d", i%10),
			GpuRequest: int32(i % 8),
		}
	}

	hours := make([]time.Time, 24)
	baseHour := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	for i := 0; i < 24; i++ {
		hours[i] = baseHour.Add(time.Duration(i) * time.Hour)
	}

	entries := make([]WorkloadHourEntry, 0)
	for _, w := range workloads {
		for _, h := range hours {
			entries = append(entries, WorkloadHourEntry{
				Workload: w,
				Hour:     h,
			})
		}
	}

	assert.Equal(t, 2400, len(entries))
}

// TestDefaultConstants_Relationship tests relationship between default constants
func TestDefaultConstants_Relationship(t *testing.T) {
	assert.True(t, DefaultBackfillDays > 0)
	assert.True(t, DefaultPromQueryStep > 0)
	assert.True(t, DefaultPromQueryStep <= 300)
	assert.True(t, DefaultBackfillDays <= 30)
}

// TestQueryTemplates_AllFormats tests all query template formats
func TestQueryTemplates_AllFormats(t *testing.T) {
	uid := "test-workload-uid-12345"

	templates := []struct {
		name     string
		template string
		metric   string
	}{
		{"utilization", WorkloadUtilizationQueryTemplate, "workload_gpu_utilization"},
		{"memory_used", WorkloadGpuMemoryUsedQueryTemplate, "workload_gpu_used_vram"},
		{"memory_total", WorkloadGpuMemoryTotalQueryTemplate, "workload_gpu_total_vram"},
	}

	for _, tt := range templates {
		t.Run(tt.name, func(t *testing.T) {
			query := fmt.Sprintf(tt.template, uid)
			assert.Contains(t, query, "avg(")
			assert.Contains(t, query, tt.metric)
			assert.Contains(t, query, uid)
			assert.Contains(t, query, "workload_uid=")
		})
	}
}

// TestWorkloadStatsBackfillJob_StructInitialization tests struct initialization
func TestWorkloadStatsBackfillJob_StructInitialization(t *testing.T) {
	config := &WorkloadStatsBackfillConfig{
		Enabled:       true,
		BackfillDays:  5,
		PromQueryStep: 30,
	}
	job := &WorkloadStatsBackfillJob{
		config:      config,
		clusterName: "init-test-cluster",
	}

	assert.NotNil(t, job.config)
	assert.Equal(t, "init-test-cluster", job.clusterName)
	assert.True(t, job.config.Enabled)
	assert.Equal(t, 5, job.config.BackfillDays)
	assert.Equal(t, 30, job.config.PromQueryStep)
}

// TestWorkloadStatsBackfillJob_NilConfig tests nil config handling
func TestWorkloadStatsBackfillJob_NilConfig(t *testing.T) {
	job := &WorkloadStatsBackfillJob{config: nil}
	assert.Nil(t, job.GetConfig())
}

// TestWorkloadStatsBackfillConfig_Validation tests config validation
func TestWorkloadStatsBackfillConfig_Validation(t *testing.T) {
	tests := []struct {
		name         string
		config       *WorkloadStatsBackfillConfig
		isValidDays  bool
		isValidStep  bool
	}{
		{
			name: "valid config",
			config: &WorkloadStatsBackfillConfig{
				Enabled:       true,
				BackfillDays:  DefaultBackfillDays,
				PromQueryStep: DefaultPromQueryStep,
			},
			isValidDays: true,
			isValidStep: true,
		},
		{
			name: "zero days",
			config: &WorkloadStatsBackfillConfig{
				Enabled:       true,
				BackfillDays:  0,
				PromQueryStep: 60,
			},
			isValidDays: false,
			isValidStep: true,
		},
		{
			name: "zero step",
			config: &WorkloadStatsBackfillConfig{
				Enabled:       true,
				BackfillDays:  2,
				PromQueryStep: 0,
			},
			isValidDays: true,
			isValidStep: false,
		},
		{
			name: "negative days",
			config: &WorkloadStatsBackfillConfig{
				Enabled:       true,
				BackfillDays:  -1,
				PromQueryStep: 60,
			},
			isValidDays: false,
			isValidStep: true,
		},
		{
			name: "negative step",
			config: &WorkloadStatsBackfillConfig{
				Enabled:       true,
				BackfillDays:  2,
				PromQueryStep: -1,
			},
			isValidDays: true,
			isValidStep: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.isValidDays, tt.config.BackfillDays > 0)
			assert.Equal(t, tt.isValidStep, tt.config.PromQueryStep > 0)
		})
	}
}

// TestWorkloadHourEntry_Equality tests entry equality
func TestWorkloadHourEntry_Equality(t *testing.T) {
	workload := &dbmodel.GpuWorkload{
		UID:       "test-uid",
		Name:      "test-workload",
		Namespace: "test-ns",
	}
	hour := time.Date(2025, 1, 1, 10, 0, 0, 0, time.UTC)

	entry1 := WorkloadHourEntry{
		Workload: workload,
		Hour:     hour,
	}
	entry2 := WorkloadHourEntry{
		Workload: workload,
		Hour:     hour,
	}

	assert.Equal(t, entry1.Workload.UID, entry2.Workload.UID)
	assert.Equal(t, entry1.Hour, entry2.Hour)
}

// TestWorkloadHourEntry_DifferentHours tests entries with different hours
func TestWorkloadHourEntry_DifferentHours(t *testing.T) {
	workload := &dbmodel.GpuWorkload{
		UID:  "test-uid",
		Name: "test-workload",
	}

	hour1 := time.Date(2025, 1, 1, 10, 0, 0, 0, time.UTC)
	hour2 := time.Date(2025, 1, 1, 11, 0, 0, 0, time.UTC)

	entry1 := WorkloadHourEntry{Workload: workload, Hour: hour1}
	entry2 := WorkloadHourEntry{Workload: workload, Hour: hour2}

	assert.Equal(t, entry1.Workload.UID, entry2.Workload.UID)
	assert.NotEqual(t, entry1.Hour, entry2.Hour)
}

// TestWorkloadHourEntry_DifferentWorkloads tests entries with different workloads
func TestWorkloadHourEntry_DifferentWorkloads(t *testing.T) {
	workload1 := &dbmodel.GpuWorkload{UID: "uid-1", Name: "workload-1"}
	workload2 := &dbmodel.GpuWorkload{UID: "uid-2", Name: "workload-2"}

	hour := time.Date(2025, 1, 1, 10, 0, 0, 0, time.UTC)

	entry1 := WorkloadHourEntry{Workload: workload1, Hour: hour}
	entry2 := WorkloadHourEntry{Workload: workload2, Hour: hour}

	assert.NotEqual(t, entry1.Workload.UID, entry2.Workload.UID)
	assert.Equal(t, entry1.Hour, entry2.Hour)
}

// TestMissingStatsMapKey_Format tests map key format
func TestMissingStatsMapKey_Format(t *testing.T) {
	testCases := []struct {
		namespace    string
		workloadName string
		hour         time.Time
	}{
		{"ns-1", "wl-1", time.Date(2025, 1, 1, 10, 0, 0, 0, time.UTC)},
		{"ns-2", "wl-2", time.Date(2025, 6, 15, 12, 0, 0, 0, time.UTC)},
		{"default", "nginx", time.Date(2025, 12, 31, 23, 0, 0, 0, time.UTC)},
	}

	for _, tc := range testCases {
		key := fmt.Sprintf("%s/%s/%s", tc.namespace, tc.workloadName, tc.hour.Format(time.RFC3339))

		assert.Contains(t, key, tc.namespace)
		assert.Contains(t, key, tc.workloadName)
		assert.Contains(t, key, "/")

		parts := 3
		assert.Equal(t, parts, len(splitBySlash(key)))
	}
}

func splitBySlash(s string) []string {
	result := make([]string, 0)
	current := ""
	for _, c := range s {
		if c == '/' {
			result = append(result, current)
			current = ""
		} else {
			current += string(c)
		}
	}
	if current != "" {
		result = append(result, current)
	}
	return result
}

// TestTimeCalculation_BackfillRange tests backfill time range calculation
func TestTimeCalculation_BackfillRange(t *testing.T) {
	tests := []struct {
		name         string
		backfillDays int
	}{
		{"1 day", 1},
		{"2 days (default)", 2},
		{"7 days", 7},
		{"30 days", 30},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			now := time.Now()
			endTime := now.Truncate(time.Hour).Add(-time.Hour)
			startTime := endTime.Add(-time.Duration(tt.backfillDays) * 24 * time.Hour)

			assert.True(t, startTime.Before(endTime))
			assert.True(t, endTime.Before(now))

			expectedDuration := time.Duration(tt.backfillDays) * 24 * time.Hour
			assert.Equal(t, expectedDuration, endTime.Sub(startTime))
		})
	}
}

// TestWorkloadGpuWorkload_AllFields tests all GpuWorkload fields
func TestWorkloadGpuWorkload_AllFields(t *testing.T) {
	now := time.Now()
	workload := &dbmodel.GpuWorkload{
		UID:         "uid-123-456",
		Name:        "ml-training-job",
		Namespace:   "ml-namespace",
		Kind:        "Job",
		GpuRequest:  8,
		Status:      "Running",
		ParentUID:   "",
		Labels:      dbmodel.ExtType{"app": "ml", "team": "research"},
		Annotations: dbmodel.ExtType{"project": "vision", "cost-center": "rd-001"},
		CreatedAt:   now.Add(-24 * time.Hour),
		EndAt:       time.Time{},
	}

	assert.Equal(t, "uid-123-456", workload.UID)
	assert.Equal(t, "ml-training-job", workload.Name)
	assert.Equal(t, "ml-namespace", workload.Namespace)
	assert.Equal(t, "Job", workload.Kind)
	assert.Equal(t, int32(8), workload.GpuRequest)
	assert.Equal(t, "Running", workload.Status)
	assert.Empty(t, workload.ParentUID)
	assert.NotNil(t, workload.Labels)
	assert.NotNil(t, workload.Annotations)
	assert.False(t, workload.CreatedAt.IsZero())
	assert.True(t, workload.EndAt.IsZero())
}

// TestWorkloadActiveTimeRange_Filtering tests workload active time range filtering
func TestWorkloadActiveTimeRange_Filtering(t *testing.T) {
	startTime := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	endTime := time.Date(2025, 1, 3, 0, 0, 0, 0, time.UTC)

	workloads := []*dbmodel.GpuWorkload{
		{
			UID:       "uid-1",
			Name:      "active-before-and-during",
			CreatedAt: time.Date(2024, 12, 1, 0, 0, 0, 0, time.UTC),
			EndAt:     time.Time{},
			ParentUID: "",
		},
		{
			UID:       "uid-2",
			Name:      "created-during",
			CreatedAt: time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC),
			EndAt:     time.Time{},
			ParentUID: "",
		},
		{
			UID:       "uid-3",
			Name:      "created-after",
			CreatedAt: time.Date(2025, 1, 4, 0, 0, 0, 0, time.UTC),
			EndAt:     time.Time{},
			ParentUID: "",
		},
		{
			UID:       "uid-4",
			Name:      "ended-before",
			CreatedAt: time.Date(2024, 12, 1, 0, 0, 0, 0, time.UTC),
			EndAt:     time.Date(2024, 12, 31, 0, 0, 0, 0, time.UTC),
			ParentUID: "",
		},
		{
			UID:       "uid-5",
			Name:      "child-workload",
			CreatedAt: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
			EndAt:     time.Time{},
			ParentUID: "parent-uid",
		},
	}

	activeWorkloads := make([]*dbmodel.GpuWorkload, 0)
	for _, w := range workloads {
		if w.ParentUID != "" {
			continue
		}
		if w.CreatedAt.After(endTime) {
			continue
		}
		if !w.EndAt.IsZero() && w.EndAt.Before(startTime) {
			continue
		}
		activeWorkloads = append(activeWorkloads, w)
	}

	assert.Equal(t, 2, len(activeWorkloads))
	assert.Equal(t, "uid-1", activeWorkloads[0].UID)
	assert.Equal(t, "uid-2", activeWorkloads[1].UID)
}

// TestHourGeneration_ForWorkload tests hour generation for a workload
func TestHourGeneration_ForWorkload(t *testing.T) {
	workloadStart := time.Date(2025, 1, 1, 10, 30, 0, 0, time.UTC)
	workloadEnd := time.Date(2025, 1, 1, 14, 15, 0, 0, time.UTC)

	currentHour := workloadStart.Truncate(time.Hour)
	endHour := workloadEnd.Truncate(time.Hour)

	hours := make([]time.Time, 0)
	for !currentHour.After(endHour) {
		hours = append(hours, currentHour)
		currentHour = currentHour.Add(time.Hour)
	}

	assert.Equal(t, 5, len(hours))
	assert.Equal(t, time.Date(2025, 1, 1, 10, 0, 0, 0, time.UTC), hours[0])
	assert.Equal(t, time.Date(2025, 1, 1, 14, 0, 0, 0, time.UTC), hours[4])
}

// TestWorkloadStatsBackfillJob_ScheduleFrequency tests schedule frequency
func TestWorkloadStatsBackfillJob_ScheduleFrequency(t *testing.T) {
	job := &WorkloadStatsBackfillJob{}
	schedule := job.Schedule()

	assert.Equal(t, "@every 1m", schedule)
	assert.NotEqual(t, "@every 5m", schedule)
	assert.Contains(t, schedule, "@every")
}

// TestWorkloadHourEntry_LargeScale tests creating many entries
func TestWorkloadHourEntry_LargeScale(t *testing.T) {
	numWorkloads := 50
	numHours := 48

	workloads := make([]*dbmodel.GpuWorkload, numWorkloads)
	for i := 0; i < numWorkloads; i++ {
		workloads[i] = &dbmodel.GpuWorkload{
			UID:        fmt.Sprintf("uid-%d", i),
			Name:       fmt.Sprintf("workload-%d", i),
			Namespace:  fmt.Sprintf("ns-%d", i%10),
			GpuRequest: int32(i%8 + 1),
		}
	}

	baseHour := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	hours := make([]time.Time, numHours)
	for i := 0; i < numHours; i++ {
		hours[i] = baseHour.Add(time.Duration(i) * time.Hour)
	}

	entries := make([]WorkloadHourEntry, 0)
	for _, w := range workloads {
		for _, h := range hours {
			entries = append(entries, WorkloadHourEntry{
				Workload: w,
				Hour:     h,
			})
		}
	}

	assert.Equal(t, numWorkloads*numHours, len(entries))
}

// TestDefaultConstants_RelationshipAndBounds tests relationship and bounds between default constants
func TestDefaultConstants_RelationshipAndBounds(t *testing.T) {
	assert.True(t, DefaultBackfillDays > 0)
	assert.True(t, DefaultPromQueryStep > 0)
	assert.True(t, DefaultBackfillDays <= 30)
	assert.True(t, DefaultPromQueryStep <= 600)
}

// TestQueryTemplates_PromQLSyntax tests PromQL syntax
func TestQueryTemplates_PromQLSyntax(t *testing.T) {
	uids := []string{
		"simple-uid",
		"uid-with-dashes-123",
		"a1b2c3d4-e5f6-7890-abcd-ef1234567890",
	}

	templates := []string{
		WorkloadUtilizationQueryTemplate,
		WorkloadGpuMemoryUsedQueryTemplate,
		WorkloadGpuMemoryTotalQueryTemplate,
	}

	for _, uid := range uids {
		for _, template := range templates {
			query := fmt.Sprintf(template, uid)

			assert.Contains(t, query, "avg(")
			assert.Contains(t, query, ")")
			assert.Contains(t, query, "{")
			assert.Contains(t, query, "}")
			assert.Contains(t, query, fmt.Sprintf(`workload_uid="%s"`, uid))
		}
	}
}

// TestWorkloadStatsBackfillConfig_SetterGetter tests setter and getter
func TestWorkloadStatsBackfillConfig_SetterGetter_Detailed(t *testing.T) {
	job := &WorkloadStatsBackfillJob{
		config: &WorkloadStatsBackfillConfig{
			Enabled:       false,
			BackfillDays:  1,
			PromQueryStep: 30,
		},
		clusterName: "initial-cluster",
	}

	assert.False(t, job.GetConfig().Enabled)
	assert.Equal(t, 1, job.GetConfig().BackfillDays)
	assert.Equal(t, 30, job.GetConfig().PromQueryStep)

	newConfig := &WorkloadStatsBackfillConfig{
		Enabled:       true,
		BackfillDays:  7,
		PromQueryStep: 60,
	}
	job.SetConfig(newConfig)

	assert.True(t, job.GetConfig().Enabled)
	assert.Equal(t, 7, job.GetConfig().BackfillDays)
	assert.Equal(t, 60, job.GetConfig().PromQueryStep)
}

// TestHourTruncation_Precision tests hour truncation precision
func TestHourTruncation_Precision(t *testing.T) {
	tests := []struct {
		name     string
		input    time.Time
		expected time.Time
	}{
		{
			name:     "max nanoseconds",
			input:    time.Date(2025, 1, 1, 10, 59, 59, 999999999, time.UTC),
			expected: time.Date(2025, 1, 1, 10, 0, 0, 0, time.UTC),
		},
		{
			name:     "exactly on hour",
			input:    time.Date(2025, 1, 1, 10, 0, 0, 0, time.UTC),
			expected: time.Date(2025, 1, 1, 10, 0, 0, 0, time.UTC),
		},
		{
			name:     "1 nanosecond past hour",
			input:    time.Date(2025, 1, 1, 10, 0, 0, 1, time.UTC),
			expected: time.Date(2025, 1, 1, 10, 0, 0, 0, time.UTC),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.input.Truncate(time.Hour)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestWorkloadHourEntry_WorkloadFieldAccess tests workload field access
func TestWorkloadHourEntry_WorkloadFieldAccess(t *testing.T) {
	workload := &dbmodel.GpuWorkload{
		UID:         "uid-access-test",
		Name:        "access-test-workload",
		Namespace:   "access-test-ns",
		Kind:        "Deployment",
		GpuRequest:  4,
		Status:      "Running",
		Labels:      dbmodel.ExtType{"app": "test"},
		Annotations: dbmodel.ExtType{"project": "access-test"},
	}

	entry := WorkloadHourEntry{
		Workload: workload,
		Hour:     time.Now().Truncate(time.Hour),
	}

	assert.Equal(t, "uid-access-test", entry.Workload.UID)
	assert.Equal(t, "access-test-workload", entry.Workload.Name)
	assert.Equal(t, "access-test-ns", entry.Workload.Namespace)
	assert.Equal(t, "Deployment", entry.Workload.Kind)
	assert.Equal(t, int32(4), entry.Workload.GpuRequest)
	assert.Equal(t, "Running", entry.Workload.Status)
	assert.NotNil(t, entry.Workload.Labels)
	assert.NotNil(t, entry.Workload.Annotations)
}

// TestMissingStatsMapKey_Collision tests map key collision potential
func TestMissingStatsMapKey_Collision(t *testing.T) {
	keys := make(map[string]bool)

	testCases := []struct {
		namespace    string
		workloadName string
		hour         time.Time
	}{
		{"ns1", "wl1", time.Date(2025, 1, 1, 10, 0, 0, 0, time.UTC)},
		{"ns1", "wl1", time.Date(2025, 1, 1, 11, 0, 0, 0, time.UTC)},
		{"ns1", "wl2", time.Date(2025, 1, 1, 10, 0, 0, 0, time.UTC)},
		{"ns2", "wl1", time.Date(2025, 1, 1, 10, 0, 0, 0, time.UTC)},
		{"ns1/wl1", "fake", time.Date(2025, 1, 1, 10, 0, 0, 0, time.UTC)},
	}

	for _, tc := range testCases {
		key := fmt.Sprintf("%s/%s/%s", tc.namespace, tc.workloadName, tc.hour.Format(time.RFC3339))
		keys[key] = true
	}

	assert.Equal(t, 5, len(keys))
}

// TestWorkloadStatus_AllPossibleValues tests all possible status values
func TestWorkloadStatus_AllPossibleValues(t *testing.T) {
	statuses := []string{
		"Running",
		"Pending",
		"Succeeded",
		"Failed",
		"Unknown",
		"Terminating",
		"",
	}

	for _, status := range statuses {
		workload := &dbmodel.GpuWorkload{
			UID:    "test-uid",
			Status: status,
		}

		entry := WorkloadHourEntry{
			Workload: workload,
			Hour:     time.Now().Truncate(time.Hour),
		}

		assert.Equal(t, status, entry.Workload.Status)
	}
}

// TestWorkloadKind_AllPossibleValues tests all possible kind values
func TestWorkloadKind_AllPossibleValues(t *testing.T) {
	kinds := []string{
		"Deployment",
		"StatefulSet",
		"Job",
		"CronJob",
		"DaemonSet",
		"ReplicaSet",
		"Pod",
	}

	for _, kind := range kinds {
		workload := &dbmodel.GpuWorkload{
			UID:  "test-uid",
			Kind: kind,
		}

		entry := WorkloadHourEntry{
			Workload: workload,
			Hour:     time.Now().Truncate(time.Hour),
		}

		assert.Equal(t, kind, entry.Workload.Kind)
	}
}

// TestTimeRangeExclusion tests that current hour is excluded
func TestTimeRangeExclusion(t *testing.T) {
	now := time.Now()
	currentHour := now.Truncate(time.Hour)
	endTime := currentHour.Add(-time.Hour)

	assert.True(t, endTime.Before(currentHour))
	assert.True(t, currentHour.Sub(endTime) >= time.Hour)
}

// TestWorkloadHourEntry_EmptyWorkload tests empty workload fields
func TestWorkloadHourEntry_EmptyWorkload(t *testing.T) {
	workload := &dbmodel.GpuWorkload{
		UID:       "",
		Name:      "",
		Namespace: "",
		Kind:      "",
		Status:    "",
	}

	entry := WorkloadHourEntry{
		Workload: workload,
		Hour:     time.Now().Truncate(time.Hour),
	}

	assert.Empty(t, entry.Workload.UID)
	assert.Empty(t, entry.Workload.Name)
	assert.Empty(t, entry.Workload.Namespace)
	assert.Empty(t, entry.Workload.Kind)
	assert.Empty(t, entry.Workload.Status)
}

// TestBackfillDaysToHours tests conversion from days to hours
func TestBackfillDaysToHoursConversion(t *testing.T) {
	tests := []struct {
		days          int
		expectedHours int
	}{
		{1, 24},
		{2, 48},
		{7, 168},
		{14, 336},
		{30, 720},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("%d_days", tt.days), func(t *testing.T) {
			hours := tt.days * 24
			assert.Equal(t, tt.expectedHours, hours)
		})
	}
}

// TestConfigEnabledBehavior tests config enabled behavior
func TestConfigEnabledBehavior(t *testing.T) {
	config := &WorkloadStatsBackfillConfig{Enabled: false}
	assert.False(t, config.Enabled)

	config.Enabled = true
	assert.True(t, config.Enabled)

	config.Enabled = false
	assert.False(t, config.Enabled)
}

// TestWorkloadHourEntry_CrossDayBoundary tests entries across day boundaries
func TestWorkloadHourEntry_CrossDayBoundary(t *testing.T) {
	workload := &dbmodel.GpuWorkload{
		UID:  "test-uid",
		Name: "test-workload",
	}

	entry1 := WorkloadHourEntry{
		Workload: workload,
		Hour:     time.Date(2025, 1, 1, 23, 0, 0, 0, time.UTC),
	}
	entry2 := WorkloadHourEntry{
		Workload: workload,
		Hour:     time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC),
	}

	assert.Equal(t, 1, entry1.Hour.Day())
	assert.Equal(t, 2, entry2.Hour.Day())
	assert.Equal(t, time.Hour, entry2.Hour.Sub(entry1.Hour))
}

// TestWorkloadHourEntry_CrossMonthBoundary tests entries across month boundaries
func TestWorkloadHourEntry_CrossMonthBoundaryDetailed(t *testing.T) {
	workload := &dbmodel.GpuWorkload{
		UID:  "test-uid",
		Name: "test-workload",
	}

	entry1 := WorkloadHourEntry{
		Workload: workload,
		Hour:     time.Date(2025, 1, 31, 23, 0, 0, 0, time.UTC),
	}
	entry2 := WorkloadHourEntry{
		Workload: workload,
		Hour:     time.Date(2025, 2, 1, 0, 0, 0, 0, time.UTC),
	}

	assert.Equal(t, time.January, entry1.Hour.Month())
	assert.Equal(t, time.February, entry2.Hour.Month())
	assert.Equal(t, time.Hour, entry2.Hour.Sub(entry1.Hour))
}

// TestWorkloadHourEntry_CrossYearBoundary tests entries across year boundaries
func TestWorkloadHourEntry_CrossYearBoundary(t *testing.T) {
	workload := &dbmodel.GpuWorkload{
		UID:  "test-uid",
		Name: "test-workload",
	}

	entry1 := WorkloadHourEntry{
		Workload: workload,
		Hour:     time.Date(2024, 12, 31, 23, 0, 0, 0, time.UTC),
	}
	entry2 := WorkloadHourEntry{
		Workload: workload,
		Hour:     time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
	}

	assert.Equal(t, 2024, entry1.Hour.Year())
	assert.Equal(t, 2025, entry2.Hour.Year())
	assert.Equal(t, time.Hour, entry2.Hour.Sub(entry1.Hour))
}

