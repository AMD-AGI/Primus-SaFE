package gpu_aggregation_backfill

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestGenerateAllHours(t *testing.T) {
	tests := []struct {
		name      string
		startTime time.Time
		endTime   time.Time
		expected  int
	}{
		{
			name:      "24 hours",
			startTime: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
			endTime:   time.Date(2025, 1, 1, 23, 0, 0, 0, time.UTC),
			expected:  24,
		},
		{
			name:      "single hour",
			startTime: time.Date(2025, 1, 1, 10, 0, 0, 0, time.UTC),
			endTime:   time.Date(2025, 1, 1, 10, 0, 0, 0, time.UTC),
			expected:  1,
		},
		{
			name:      "48 hours (2 days)",
			startTime: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
			endTime:   time.Date(2025, 1, 2, 23, 0, 0, 0, time.UTC),
			expected:  48,
		},
		{
			name:      "start time with minutes truncated",
			startTime: time.Date(2025, 1, 1, 10, 30, 45, 0, time.UTC),
			endTime:   time.Date(2025, 1, 1, 12, 15, 0, 0, time.UTC),
			expected:  3,
		},
		{
			name:      "end before start returns empty",
			startTime: time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC),
			endTime:   time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
			expected:  0,
		},
		{
			name:      "7 days",
			startTime: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
			endTime:   time.Date(2025, 1, 7, 23, 0, 0, 0, time.UTC),
			expected:  168,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := generateAllHours(tt.startTime, tt.endTime)
			assert.Equal(t, tt.expected, len(result), "Number of hours should match expected")

			if len(result) > 0 {
				assert.True(t, result[0].Equal(tt.startTime.Truncate(time.Hour)) || result[0].Before(tt.startTime.Truncate(time.Hour).Add(time.Hour)),
					"First hour should be truncated start time")
			}

			for i := 1; i < len(result); i++ {
				diff := result[i].Sub(result[i-1])
				assert.Equal(t, time.Hour, diff, "Hours should be consecutive")
			}
		})
	}
}

func TestGenerateAllHours_Truncation(t *testing.T) {
	startTime := time.Date(2025, 1, 1, 10, 45, 30, 123456789, time.UTC)
	endTime := time.Date(2025, 1, 1, 12, 15, 0, 0, time.UTC)

	result := generateAllHours(startTime, endTime)

	assert.Equal(t, 3, len(result))
	assert.Equal(t, time.Date(2025, 1, 1, 10, 0, 0, 0, time.UTC), result[0])
	assert.Equal(t, time.Date(2025, 1, 1, 11, 0, 0, 0, time.UTC), result[1])
	assert.Equal(t, time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC), result[2])
}

func TestClusterGpuAggregationBackfillConfig_DefaultValues(t *testing.T) {
	config := &ClusterGpuAggregationBackfillConfig{}

	assert.False(t, config.Enabled, "Default Enabled should be false")
	assert.Equal(t, 0, config.BackfillDays, "Default BackfillDays should be 0")
	assert.Equal(t, 0, config.BatchSize, "Default BatchSize should be 0")
}

func TestClusterGpuAggregationBackfillJob_GetConfig(t *testing.T) {
	config := &ClusterGpuAggregationBackfillConfig{
		Enabled:      true,
		BackfillDays: 7,
		BatchSize:    24,
	}

	job := &ClusterGpuAggregationBackfillJob{
		config: config,
	}

	result := job.GetConfig()
	assert.Equal(t, config, result, "GetConfig should return the config")
}

func TestClusterGpuAggregationBackfillJob_SetConfig(t *testing.T) {
	job := &ClusterGpuAggregationBackfillJob{
		config: &ClusterGpuAggregationBackfillConfig{
			Enabled: false,
		},
	}

	newConfig := &ClusterGpuAggregationBackfillConfig{
		Enabled:      true,
		BackfillDays: 14,
		BatchSize:    48,
	}

	job.SetConfig(newConfig)
	assert.Equal(t, newConfig, job.config, "SetConfig should update the config")
}

func TestClusterGpuAggregationBackfillJob_Schedule(t *testing.T) {
	job := &ClusterGpuAggregationBackfillJob{}
	assert.Equal(t, "@every 5m", job.Schedule(), "Schedule should return @every 5m")
}

func TestNamespaceGpuAggregationBackfillConfig_DefaultValues(t *testing.T) {
	config := &NamespaceGpuAggregationBackfillConfig{}

	assert.False(t, config.Enabled, "Default Enabled should be false")
	assert.Equal(t, 0, config.BackfillDays, "Default BackfillDays should be 0")
	assert.Equal(t, 0, config.BatchSize, "Default BatchSize should be 0")
	assert.Nil(t, config.ExcludeNamespaces, "Default ExcludeNamespaces should be nil")
	assert.False(t, config.IncludeSystemNamespaces, "Default IncludeSystemNamespaces should be false")
}

func TestNamespaceGpuAggregationBackfillJob_GetConfig(t *testing.T) {
	config := &NamespaceGpuAggregationBackfillConfig{
		Enabled:                 true,
		BackfillDays:            7,
		BatchSize:               24,
		ExcludeNamespaces:       []string{"dev", "test"},
		IncludeSystemNamespaces: false,
	}

	job := &NamespaceGpuAggregationBackfillJob{
		config: config,
	}

	result := job.GetConfig()
	assert.Equal(t, config, result, "GetConfig should return the config")
}

func TestNamespaceGpuAggregationBackfillJob_SetConfig(t *testing.T) {
	job := &NamespaceGpuAggregationBackfillJob{
		config: &NamespaceGpuAggregationBackfillConfig{
			Enabled: false,
		},
	}

	newConfig := &NamespaceGpuAggregationBackfillConfig{
		Enabled:                 true,
		BackfillDays:            14,
		BatchSize:               48,
		ExcludeNamespaces:       []string{"staging"},
		IncludeSystemNamespaces: true,
	}

	job.SetConfig(newConfig)
	assert.Equal(t, newConfig, job.config, "SetConfig should update the config")
}

func TestNamespaceGpuAggregationBackfillJob_Schedule(t *testing.T) {
	job := &NamespaceGpuAggregationBackfillJob{}
	assert.Equal(t, "@every 5m", job.Schedule(), "Schedule should return @every 5m")
}

func TestNamespaceGpuAggregationBackfillJob_ShouldExcludeNamespace(t *testing.T) {
	tests := []struct {
		name      string
		config    *NamespaceGpuAggregationBackfillConfig
		namespace string
		expected  bool
	}{
		{
			name: "namespace in exclusion list",
			config: &NamespaceGpuAggregationBackfillConfig{
				Enabled:                 true,
				ExcludeNamespaces:       []string{"test", "dev", "staging"},
				IncludeSystemNamespaces: true,
			},
			namespace: "dev",
			expected:  true,
		},
		{
			name: "system namespace excluded when flag is false",
			config: &NamespaceGpuAggregationBackfillConfig{
				Enabled:                 true,
				ExcludeNamespaces:       []string{},
				IncludeSystemNamespaces: false,
			},
			namespace: "kube-system",
			expected:  true,
		},
		{
			name: "kube-public excluded when flag is false",
			config: &NamespaceGpuAggregationBackfillConfig{
				Enabled:                 true,
				ExcludeNamespaces:       []string{},
				IncludeSystemNamespaces: false,
			},
			namespace: "kube-public",
			expected:  true,
		},
		{
			name: "kube-node-lease excluded when flag is false",
			config: &NamespaceGpuAggregationBackfillConfig{
				Enabled:                 true,
				ExcludeNamespaces:       []string{},
				IncludeSystemNamespaces: false,
			},
			namespace: "kube-node-lease",
			expected:  true,
		},
		{
			name: "system namespace included when flag is true",
			config: &NamespaceGpuAggregationBackfillConfig{
				Enabled:                 true,
				ExcludeNamespaces:       []string{},
				IncludeSystemNamespaces: true,
			},
			namespace: "kube-system",
			expected:  false,
		},
		{
			name: "regular namespace not excluded",
			config: &NamespaceGpuAggregationBackfillConfig{
				Enabled:                 true,
				ExcludeNamespaces:       []string{"test"},
				IncludeSystemNamespaces: false,
			},
			namespace: "production",
			expected:  false,
		},
		{
			name: "empty exclusion list",
			config: &NamespaceGpuAggregationBackfillConfig{
				Enabled:                 true,
				ExcludeNamespaces:       []string{},
				IncludeSystemNamespaces: true,
			},
			namespace: "any-namespace",
			expected:  false,
		},
		{
			name: "case sensitive match",
			config: &NamespaceGpuAggregationBackfillConfig{
				Enabled:                 true,
				ExcludeNamespaces:       []string{"Test"},
				IncludeSystemNamespaces: true,
			},
			namespace: "test",
			expected:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			job := &NamespaceGpuAggregationBackfillJob{
				config: tt.config,
			}
			result := job.shouldExcludeNamespace(tt.namespace)
			assert.Equal(t, tt.expected, result, "Exclusion result should match expected")
		})
	}
}

func TestLabelGpuAggregationBackfillConfig_DefaultValues(t *testing.T) {
	config := &LabelGpuAggregationBackfillConfig{}

	assert.False(t, config.Enabled, "Default Enabled should be false")
	assert.Equal(t, 0, config.BackfillDays, "Default BackfillDays should be 0")
	assert.Equal(t, 0, config.BatchSize, "Default BatchSize should be 0")
	assert.Nil(t, config.LabelKeys, "Default LabelKeys should be nil")
	assert.Nil(t, config.AnnotationKeys, "Default AnnotationKeys should be nil")
	assert.Empty(t, config.DefaultValue, "Default DefaultValue should be empty")
}

func TestLabelGpuAggregationBackfillJob_GetConfig(t *testing.T) {
	config := &LabelGpuAggregationBackfillConfig{
		Enabled:        true,
		BackfillDays:   7,
		BatchSize:      24,
		LabelKeys:      []string{"app", "team"},
		AnnotationKeys: []string{"project"},
		DefaultValue:   "unknown",
	}

	job := &LabelGpuAggregationBackfillJob{
		config: config,
	}

	result := job.GetConfig()
	assert.Equal(t, config, result, "GetConfig should return the config")
}

func TestLabelGpuAggregationBackfillJob_SetConfig(t *testing.T) {
	job := &LabelGpuAggregationBackfillJob{
		config: &LabelGpuAggregationBackfillConfig{
			Enabled: false,
		},
	}

	newConfig := &LabelGpuAggregationBackfillConfig{
		Enabled:        true,
		BackfillDays:   14,
		BatchSize:      48,
		LabelKeys:      []string{"env"},
		AnnotationKeys: []string{"cost-center"},
		DefaultValue:   "default",
	}

	job.SetConfig(newConfig)
	assert.Equal(t, newConfig, job.config, "SetConfig should update the config")
}

func TestLabelGpuAggregationBackfillJob_Schedule(t *testing.T) {
	job := &LabelGpuAggregationBackfillJob{}
	assert.Equal(t, "@every 5m", job.Schedule(), "Schedule should return @every 5m")
}

func TestLabelGpuAggregationSystemConfig_BasicStructure(t *testing.T) {
	config := LabelGpuAggregationSystemConfig{}

	config.Dimensions.Label.Enabled = true
	config.Dimensions.Label.LabelKeys = []string{"app", "team"}
	config.Dimensions.Label.AnnotationKeys = []string{"project", "cost-center"}
	config.Dimensions.Label.DefaultValue = "unknown"

	assert.True(t, config.Dimensions.Label.Enabled)
	assert.Equal(t, []string{"app", "team"}, config.Dimensions.Label.LabelKeys)
	assert.Equal(t, []string{"project", "cost-center"}, config.Dimensions.Label.AnnotationKeys)
	assert.Equal(t, "unknown", config.Dimensions.Label.DefaultValue)
}

func TestConstants(t *testing.T) {
	assert.Equal(t, 7, DefaultClusterBackfillDays)
	assert.Equal(t, 24, DefaultClusterBatchSize)
	assert.Equal(t, 7, DefaultNamespaceBackfillDays)
	assert.Equal(t, 24, DefaultNamespaceBatchSize)
	assert.Equal(t, 7, DefaultLabelBackfillDays)
	assert.Equal(t, 24, DefaultLabelBatchSize)
	assert.Equal(t, "job.gpu_aggregation.config", SystemConfigKeyGpuAggregationBackfill)
}

func TestClusterGpuAggregationBackfillJob_BuildClusterStatsFromResult(t *testing.T) {
	job := &ClusterGpuAggregationBackfillJob{}

	testHour := time.Date(2025, 1, 1, 10, 0, 0, 0, time.UTC)

	tests := []struct {
		name        string
		clusterName string
		hour        time.Time
	}{
		{
			name:        "normal result",
			clusterName: "test-cluster",
			hour:        testHour,
		},
		{
			name:        "empty cluster",
			clusterName: "empty-cluster",
			hour:        testHour,
		},
		{
			name:        "large cluster",
			clusterName: "large-cluster",
			hour:        testHour,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stats := job.createZeroClusterStats(tt.clusterName, tt.hour)

			assert.NotNil(t, stats)
			assert.Equal(t, tt.clusterName, stats.ClusterName)
			assert.Equal(t, tt.hour, stats.StatHour)
		})
	}
}

func TestClusterGpuAggregationBackfillJob_CreateZeroClusterStats(t *testing.T) {
	job := &ClusterGpuAggregationBackfillJob{}

	clusterName := "test-cluster"
	testHour := time.Date(2025, 1, 1, 10, 0, 0, 0, time.UTC)

	stats := job.createZeroClusterStats(clusterName, testHour)

	assert.NotNil(t, stats)
	assert.Equal(t, clusterName, stats.ClusterName)
	assert.Equal(t, testHour, stats.StatHour)
	assert.Equal(t, int32(0), stats.TotalGpuCapacity)
	assert.Equal(t, float64(0), stats.AllocatedGpuCount)
	assert.Equal(t, float64(0), stats.AllocationRate)
	assert.Equal(t, float64(0), stats.AvgUtilization)
	assert.Equal(t, float64(0), stats.MaxUtilization)
	assert.Equal(t, float64(0), stats.MinUtilization)
	assert.Equal(t, float64(0), stats.P50Utilization)
	assert.Equal(t, float64(0), stats.P95Utilization)
	assert.Equal(t, int32(0), stats.SampleCount)
}

func TestNamespaceGpuAggregationBackfillJob_CreateZeroNamespaceStats(t *testing.T) {
	job := &NamespaceGpuAggregationBackfillJob{}

	clusterName := "test-cluster"
	namespace := "test-namespace"
	testHour := time.Date(2025, 1, 1, 10, 0, 0, 0, time.UTC)

	stats := job.createZeroNamespaceStats(clusterName, namespace, testHour)

	assert.NotNil(t, stats)
	assert.Equal(t, clusterName, stats.ClusterName)
	assert.Equal(t, namespace, stats.Namespace)
	assert.Equal(t, testHour, stats.StatHour)
	assert.Equal(t, int32(0), stats.TotalGpuCapacity)
	assert.Equal(t, float64(0), stats.AllocatedGpuCount)
	assert.Equal(t, float64(0), stats.AllocationRate)
	assert.Equal(t, float64(0), stats.AvgUtilization)
	assert.Equal(t, float64(0), stats.MaxUtilization)
	assert.Equal(t, float64(0), stats.MinUtilization)
	assert.Equal(t, int32(0), stats.ActiveWorkloadCount)
}

func TestGenerateAllHours_TimezoneHandling(t *testing.T) {
	loc, _ := time.LoadLocation("America/New_York")
	startTime := time.Date(2025, 1, 1, 10, 0, 0, 0, loc)
	endTime := time.Date(2025, 1, 1, 12, 0, 0, 0, loc)

	result := generateAllHours(startTime, endTime)

	assert.Equal(t, 3, len(result))
	for _, hour := range result {
		assert.Equal(t, 0, hour.Minute(), "All hours should have 0 minutes")
		assert.Equal(t, 0, hour.Second(), "All hours should have 0 seconds")
		assert.Equal(t, 0, hour.Nanosecond(), "All hours should have 0 nanoseconds")
	}
}

func TestGenerateAllHours_DSTTransition(t *testing.T) {
	loc, _ := time.LoadLocation("America/New_York")
	startTime := time.Date(2025, 3, 9, 1, 0, 0, 0, loc)
	endTime := time.Date(2025, 3, 9, 4, 0, 0, 0, loc)

	result := generateAllHours(startTime, endTime)

	assert.True(t, len(result) >= 3, "Should generate hours even during DST transition")
	for i := 1; i < len(result); i++ {
		assert.True(t, result[i].After(result[i-1]), "Hours should be in ascending order")
	}
}

func TestClusterGpuAggregationBackfillJob_Structure(t *testing.T) {
	config := &ClusterGpuAggregationBackfillConfig{
		Enabled:      true,
		BackfillDays: 7,
		BatchSize:    24,
	}

	job := &ClusterGpuAggregationBackfillJob{
		config:      config,
		clusterName: "test-cluster",
	}

	assert.Equal(t, "test-cluster", job.clusterName)
	assert.True(t, job.config.Enabled)
	assert.Equal(t, 7, job.config.BackfillDays)
	assert.Equal(t, 24, job.config.BatchSize)
}

func TestNamespaceGpuAggregationBackfillJob_Structure(t *testing.T) {
	config := &NamespaceGpuAggregationBackfillConfig{
		Enabled:                 true,
		BackfillDays:            14,
		BatchSize:               48,
		ExcludeNamespaces:       []string{"dev", "test"},
		IncludeSystemNamespaces: false,
	}

	job := &NamespaceGpuAggregationBackfillJob{
		config:      config,
		clusterName: "prod-cluster",
	}

	assert.Equal(t, "prod-cluster", job.clusterName)
	assert.True(t, job.config.Enabled)
	assert.Equal(t, 14, job.config.BackfillDays)
	assert.Equal(t, 48, job.config.BatchSize)
	assert.Equal(t, []string{"dev", "test"}, job.config.ExcludeNamespaces)
	assert.False(t, job.config.IncludeSystemNamespaces)
}

func TestLabelGpuAggregationBackfillJob_Structure(t *testing.T) {
	config := &LabelGpuAggregationBackfillConfig{
		Enabled:        true,
		BackfillDays:   30,
		BatchSize:      72,
		LabelKeys:      []string{"app", "team", "env"},
		AnnotationKeys: []string{"project", "cost-center"},
		DefaultValue:   "not-set",
	}

	job := &LabelGpuAggregationBackfillJob{
		config:      config,
		clusterName: "staging-cluster",
	}

	assert.Equal(t, "staging-cluster", job.clusterName)
	assert.True(t, job.config.Enabled)
	assert.Equal(t, 30, job.config.BackfillDays)
	assert.Equal(t, 72, job.config.BatchSize)
	assert.Equal(t, []string{"app", "team", "env"}, job.config.LabelKeys)
	assert.Equal(t, []string{"project", "cost-center"}, job.config.AnnotationKeys)
	assert.Equal(t, "not-set", job.config.DefaultValue)
}

func TestGenerateAllHours_EdgeCases(t *testing.T) {
	tests := []struct {
		name        string
		startTime   time.Time
		endTime     time.Time
		expectedLen int
	}{
		{
			name:        "same hour",
			startTime:   time.Date(2025, 1, 1, 10, 0, 0, 0, time.UTC),
			endTime:     time.Date(2025, 1, 1, 10, 0, 0, 0, time.UTC),
			expectedLen: 1,
		},
		{
			name:        "one hour apart",
			startTime:   time.Date(2025, 1, 1, 10, 0, 0, 0, time.UTC),
			endTime:     time.Date(2025, 1, 1, 11, 0, 0, 0, time.UTC),
			expectedLen: 2,
		},
		{
			name:        "start with non-zero minutes",
			startTime:   time.Date(2025, 1, 1, 10, 30, 45, 0, time.UTC),
			endTime:     time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC),
			expectedLen: 3,
		},
		{
			name:        "end with non-zero minutes",
			startTime:   time.Date(2025, 1, 1, 10, 0, 0, 0, time.UTC),
			endTime:     time.Date(2025, 1, 1, 12, 45, 30, 0, time.UTC),
			expectedLen: 3,
		},
		{
			name:        "cross midnight",
			startTime:   time.Date(2025, 1, 1, 22, 0, 0, 0, time.UTC),
			endTime:     time.Date(2025, 1, 2, 2, 0, 0, 0, time.UTC),
			expectedLen: 5,
		},
		{
			name:        "cross month",
			startTime:   time.Date(2025, 1, 31, 22, 0, 0, 0, time.UTC),
			endTime:     time.Date(2025, 2, 1, 2, 0, 0, 0, time.UTC),
			expectedLen: 5,
		},
		{
			name:        "cross year",
			startTime:   time.Date(2024, 12, 31, 22, 0, 0, 0, time.UTC),
			endTime:     time.Date(2025, 1, 1, 2, 0, 0, 0, time.UTC),
			expectedLen: 5,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := generateAllHours(tt.startTime, tt.endTime)
			assert.Equal(t, tt.expectedLen, len(result), "Number of hours should match expected")
		})
	}
}

func TestGenerateAllHours_Ordering(t *testing.T) {
	startTime := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	endTime := time.Date(2025, 1, 1, 23, 0, 0, 0, time.UTC)

	result := generateAllHours(startTime, endTime)

	for i := 0; i < len(result)-1; i++ {
		assert.True(t, result[i].Before(result[i+1]), "Hours should be in ascending order")
		diff := result[i+1].Sub(result[i])
		assert.Equal(t, time.Hour, diff, "Difference between consecutive hours should be exactly 1 hour")
	}
}

func TestNamespaceGpuAggregationBackfillJob_ShouldExcludeNamespace_AllSystemNs(t *testing.T) {
	systemNamespaces := []string{"kube-system", "kube-public", "kube-node-lease"}

	config := &NamespaceGpuAggregationBackfillConfig{
		Enabled:                 true,
		ExcludeNamespaces:       []string{},
		IncludeSystemNamespaces: false,
	}

	job := &NamespaceGpuAggregationBackfillJob{config: config}

	for _, ns := range systemNamespaces {
		excluded := job.shouldExcludeNamespace(ns)
		assert.True(t, excluded, "System namespace %s should be excluded when IncludeSystemNamespaces is false", ns)
	}

	config.IncludeSystemNamespaces = true
	for _, ns := range systemNamespaces {
		excluded := job.shouldExcludeNamespace(ns)
		assert.False(t, excluded, "System namespace %s should not be excluded when IncludeSystemNamespaces is true", ns)
	}
}

func TestNamespaceGpuAggregationBackfillJob_ShouldExcludeNamespace_ExclusionListPriority(t *testing.T) {
	config := &NamespaceGpuAggregationBackfillConfig{
		Enabled:                 true,
		ExcludeNamespaces:       []string{"custom-exclude", "another-exclude"},
		IncludeSystemNamespaces: true,
	}

	job := &NamespaceGpuAggregationBackfillJob{config: config}

	assert.True(t, job.shouldExcludeNamespace("custom-exclude"))
	assert.True(t, job.shouldExcludeNamespace("another-exclude"))
	assert.False(t, job.shouldExcludeNamespace("kube-system"))
	assert.False(t, job.shouldExcludeNamespace("production"))
}

func TestNamespaceGpuAggregationBackfillJob_ShouldExcludeNamespace_EdgeCases(t *testing.T) {
	tests := []struct {
		name      string
		config    *NamespaceGpuAggregationBackfillConfig
		namespace string
		expected  bool
	}{
		{
			name: "empty namespace name in list",
			config: &NamespaceGpuAggregationBackfillConfig{
				Enabled:                 true,
				ExcludeNamespaces:       []string{""},
				IncludeSystemNamespaces: true,
			},
			namespace: "",
			expected:  true,
		},
		{
			name: "namespace with hyphens",
			config: &NamespaceGpuAggregationBackfillConfig{
				Enabled:                 true,
				ExcludeNamespaces:       []string{"my-namespace-1"},
				IncludeSystemNamespaces: true,
			},
			namespace: "my-namespace-1",
			expected:  true,
		},
		{
			name: "namespace prefix does not match",
			config: &NamespaceGpuAggregationBackfillConfig{
				Enabled:                 true,
				ExcludeNamespaces:       []string{"test"},
				IncludeSystemNamespaces: true,
			},
			namespace: "test-production",
			expected:  false,
		},
		{
			name: "case sensitive check",
			config: &NamespaceGpuAggregationBackfillConfig{
				Enabled:                 true,
				ExcludeNamespaces:       []string{"Production"},
				IncludeSystemNamespaces: true,
			},
			namespace: "production",
			expected:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			job := &NamespaceGpuAggregationBackfillJob{config: tt.config}
			result := job.shouldExcludeNamespace(tt.namespace)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestClusterGpuAggregationBackfillJob_CreateZeroClusterStats_Consistency(t *testing.T) {
	job := &ClusterGpuAggregationBackfillJob{}

	hours := []time.Time{
		time.Date(2025, 1, 1, 10, 0, 0, 0, time.UTC),
		time.Date(2025, 1, 1, 11, 0, 0, 0, time.UTC),
		time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC),
	}

	clusterName := "test-cluster"

	for _, hour := range hours {
		stats := job.createZeroClusterStats(clusterName, hour)

		assert.Equal(t, clusterName, stats.ClusterName)
		assert.Equal(t, hour, stats.StatHour)
		assert.Equal(t, int32(0), stats.TotalGpuCapacity)
		assert.Equal(t, float64(0), stats.AllocatedGpuCount)
		assert.Equal(t, float64(0), stats.AllocationRate)
		assert.Equal(t, float64(0), stats.AvgUtilization)
		assert.Equal(t, float64(0), stats.MaxUtilization)
		assert.Equal(t, float64(0), stats.MinUtilization)
		assert.Equal(t, float64(0), stats.P50Utilization)
		assert.Equal(t, float64(0), stats.P95Utilization)
		assert.Equal(t, int32(0), stats.SampleCount)
	}
}

func TestNamespaceGpuAggregationBackfillJob_CreateZeroNamespaceStats_Consistency(t *testing.T) {
	job := &NamespaceGpuAggregationBackfillJob{}

	namespaces := []string{"ns1", "ns2", "ns3"}
	clusterName := "test-cluster"
	testHour := time.Date(2025, 1, 1, 10, 0, 0, 0, time.UTC)

	for _, ns := range namespaces {
		stats := job.createZeroNamespaceStats(clusterName, ns, testHour)

		assert.Equal(t, clusterName, stats.ClusterName)
		assert.Equal(t, ns, stats.Namespace)
		assert.Equal(t, testHour, stats.StatHour)
		assert.Equal(t, int32(0), stats.TotalGpuCapacity)
		assert.Equal(t, float64(0), stats.AllocatedGpuCount)
		assert.Equal(t, float64(0), stats.AllocationRate)
		assert.Equal(t, float64(0), stats.AvgUtilization)
		assert.Equal(t, float64(0), stats.MaxUtilization)
		assert.Equal(t, float64(0), stats.MinUtilization)
		assert.Equal(t, int32(0), stats.ActiveWorkloadCount)
	}
}

func TestLabelGpuAggregationSystemConfig_ExtendedStructure(t *testing.T) {
	config := LabelGpuAggregationSystemConfig{}

	config.Dimensions.Label.Enabled = true
	config.Dimensions.Label.LabelKeys = []string{"app", "team", "environment"}
	config.Dimensions.Label.AnnotationKeys = []string{"project", "cost-center", "owner"}
	config.Dimensions.Label.DefaultValue = "unknown"

	assert.True(t, config.Dimensions.Label.Enabled)
	assert.Equal(t, 3, len(config.Dimensions.Label.LabelKeys))
	assert.Equal(t, 3, len(config.Dimensions.Label.AnnotationKeys))
	assert.Equal(t, "unknown", config.Dimensions.Label.DefaultValue)
}

func TestLabelGpuAggregationBackfillConfig_Validation(t *testing.T) {
	tests := []struct {
		name          string
		config        *LabelGpuAggregationBackfillConfig
		expectEnabled bool
		expectHasKeys bool
	}{
		{
			name: "enabled with label keys only",
			config: &LabelGpuAggregationBackfillConfig{
				Enabled:        true,
				BackfillDays:   7,
				BatchSize:      24,
				LabelKeys:      []string{"app"},
				AnnotationKeys: []string{},
				DefaultValue:   "unknown",
			},
			expectEnabled: true,
			expectHasKeys: true,
		},
		{
			name: "enabled with annotation keys only",
			config: &LabelGpuAggregationBackfillConfig{
				Enabled:        true,
				BackfillDays:   7,
				BatchSize:      24,
				LabelKeys:      []string{},
				AnnotationKeys: []string{"project"},
				DefaultValue:   "unknown",
			},
			expectEnabled: true,
			expectHasKeys: true,
		},
		{
			name: "enabled with both keys",
			config: &LabelGpuAggregationBackfillConfig{
				Enabled:        true,
				BackfillDays:   14,
				BatchSize:      48,
				LabelKeys:      []string{"app", "team"},
				AnnotationKeys: []string{"project", "cost-center"},
				DefaultValue:   "default",
			},
			expectEnabled: true,
			expectHasKeys: true,
		},
		{
			name: "enabled with no keys",
			config: &LabelGpuAggregationBackfillConfig{
				Enabled:        true,
				BackfillDays:   7,
				BatchSize:      24,
				LabelKeys:      []string{},
				AnnotationKeys: []string{},
				DefaultValue:   "unknown",
			},
			expectEnabled: true,
			expectHasKeys: false,
		},
		{
			name: "disabled",
			config: &LabelGpuAggregationBackfillConfig{
				Enabled:        false,
				BackfillDays:   7,
				BatchSize:      24,
				LabelKeys:      []string{"app"},
				AnnotationKeys: []string{"project"},
			},
			expectEnabled: false,
			expectHasKeys: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expectEnabled, tt.config.Enabled)
			hasKeys := len(tt.config.LabelKeys) > 0 || len(tt.config.AnnotationKeys) > 0
			assert.Equal(t, tt.expectHasKeys, hasKeys)
		})
	}
}

func TestBackfillDaysCalculation(t *testing.T) {
	tests := []struct {
		name         string
		backfillDays int
		expectedHrs  int
	}{
		{
			name:         "1 day",
			backfillDays: 1,
			expectedHrs:  24,
		},
		{
			name:         "7 days",
			backfillDays: 7,
			expectedHrs:  168,
		},
		{
			name:         "30 days",
			backfillDays: 30,
			expectedHrs:  720,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			calculatedHours := tt.backfillDays * 24
			assert.Equal(t, tt.expectedHrs, calculatedHours)
		})
	}
}

func TestTimeRangeBackfillCalculation(t *testing.T) {
	now := time.Now()
	endTime := now.Truncate(time.Hour).Add(-time.Hour)
	backfillDays := 7
	startTime := endTime.Add(-time.Duration(backfillDays) * 24 * time.Hour)

	assert.True(t, startTime.Before(endTime))
	assert.True(t, endTime.Before(now))

	expectedDuration := time.Duration(backfillDays) * 24 * time.Hour
	actualDuration := endTime.Sub(startTime)
	assert.Equal(t, expectedDuration, actualDuration)
}

func TestScheduleExpressions_Backfill(t *testing.T) {
	clusterJob := &ClusterGpuAggregationBackfillJob{}
	namespaceJob := &NamespaceGpuAggregationBackfillJob{}
	labelJob := &LabelGpuAggregationBackfillJob{}

	assert.Equal(t, "@every 5m", clusterJob.Schedule())
	assert.Equal(t, "@every 5m", namespaceJob.Schedule())
	assert.Equal(t, "@every 5m", labelJob.Schedule())

	assert.Equal(t, clusterJob.Schedule(), namespaceJob.Schedule())
	assert.Equal(t, namespaceJob.Schedule(), labelJob.Schedule())
}

func TestBackfillConfigDefaultConstants(t *testing.T) {
	assert.Equal(t, 7, DefaultClusterBackfillDays, "Default cluster backfill days should be 7")
	assert.Equal(t, 24, DefaultClusterBatchSize, "Default cluster batch size should be 24")
	assert.Equal(t, 7, DefaultNamespaceBackfillDays, "Default namespace backfill days should be 7")
	assert.Equal(t, 24, DefaultNamespaceBatchSize, "Default namespace batch size should be 24")
	assert.Equal(t, 7, DefaultLabelBackfillDays, "Default label backfill days should be 7")
	assert.Equal(t, 24, DefaultLabelBatchSize, "Default label batch size should be 24")
}

func TestGenerateAllHours_LargeRange(t *testing.T) {
	startTime := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	endTime := time.Date(2025, 1, 31, 23, 0, 0, 0, time.UTC)

	result := generateAllHours(startTime, endTime)

	expectedHours := 31 * 24
	assert.Equal(t, expectedHours, len(result), "Should generate correct number of hours for a month")

	assert.Equal(t, startTime, result[0], "First hour should be start time")
	assert.Equal(t, endTime, result[len(result)-1], "Last hour should be end time")
}

func TestGenerateAllHours_EmptyRange(t *testing.T) {
	startTime := time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC)
	endTime := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)

	result := generateAllHours(startTime, endTime)

	assert.Equal(t, 0, len(result), "Should return empty slice when end is before start")
}

func TestClusterGpuAggregationBackfillConfig_AllFields(t *testing.T) {
	config := ClusterGpuAggregationBackfillConfig{
		Enabled:      true,
		BackfillDays: 14,
		BatchSize:    48,
	}

	assert.True(t, config.Enabled)
	assert.Equal(t, 14, config.BackfillDays)
	assert.Equal(t, 48, config.BatchSize)
}

func TestNamespaceGpuAggregationBackfillConfig_AllFields(t *testing.T) {
	config := NamespaceGpuAggregationBackfillConfig{
		Enabled:                 true,
		BackfillDays:            14,
		BatchSize:               48,
		ExcludeNamespaces:       []string{"dev", "test", "staging"},
		IncludeSystemNamespaces: false,
	}

	assert.True(t, config.Enabled)
	assert.Equal(t, 14, config.BackfillDays)
	assert.Equal(t, 48, config.BatchSize)
	assert.Equal(t, 3, len(config.ExcludeNamespaces))
	assert.False(t, config.IncludeSystemNamespaces)
}

func TestLabelGpuAggregationBackfillConfig_AllFields(t *testing.T) {
	config := LabelGpuAggregationBackfillConfig{
		Enabled:        true,
		BackfillDays:   14,
		BatchSize:      48,
		LabelKeys:      []string{"app", "team", "env"},
		AnnotationKeys: []string{"project", "cost-center"},
		DefaultValue:   "default-value",
	}

	assert.True(t, config.Enabled)
	assert.Equal(t, 14, config.BackfillDays)
	assert.Equal(t, 48, config.BatchSize)
	assert.Equal(t, 3, len(config.LabelKeys))
	assert.Equal(t, 2, len(config.AnnotationKeys))
	assert.Equal(t, "default-value", config.DefaultValue)
}

func TestSystemConfigKeyGpuAggregationBackfill_Value(t *testing.T) {
	assert.Equal(t, "job.gpu_aggregation.config", SystemConfigKeyGpuAggregationBackfill)
}

// TestGenerateAllHours_ConsecutiveHours tests that all hours are consecutive
func TestGenerateAllHours_ConsecutiveHours(t *testing.T) {
	startTime := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	endTime := time.Date(2025, 1, 3, 23, 0, 0, 0, time.UTC)

	result := generateAllHours(startTime, endTime)

	for i := 1; i < len(result); i++ {
		diff := result[i].Sub(result[i-1])
		assert.Equal(t, time.Hour, diff, "Hours should be consecutive with 1 hour difference")
	}
}

// TestGenerateAllHours_AllHoursAreTruncated tests that all hours are properly truncated
func TestGenerateAllHours_AllHoursAreTruncated(t *testing.T) {
	startTime := time.Date(2025, 1, 1, 10, 45, 30, 123456, time.UTC)
	endTime := time.Date(2025, 1, 1, 15, 15, 45, 654321, time.UTC)

	result := generateAllHours(startTime, endTime)

	for _, hour := range result {
		assert.Equal(t, 0, hour.Minute(), "Minutes should be 0")
		assert.Equal(t, 0, hour.Second(), "Seconds should be 0")
		assert.Equal(t, 0, hour.Nanosecond(), "Nanoseconds should be 0")
	}
}

// TestClusterGpuAggregationBackfillJob_CreateZeroClusterStats_Fields tests all fields
func TestClusterGpuAggregationBackfillJob_CreateZeroClusterStats_Fields(t *testing.T) {
	job := &ClusterGpuAggregationBackfillJob{}

	clusters := []string{"cluster-1", "cluster-2", "prod-cluster", "dev-cluster"}
	testHour := time.Date(2025, 1, 1, 10, 0, 0, 0, time.UTC)

	for _, cluster := range clusters {
		stats := job.createZeroClusterStats(cluster, testHour)

		assert.Equal(t, cluster, stats.ClusterName)
		assert.Equal(t, testHour, stats.StatHour)
		assert.Zero(t, stats.TotalGpuCapacity)
		assert.Zero(t, stats.AllocatedGpuCount)
		assert.Zero(t, stats.AllocationRate)
		assert.Zero(t, stats.AvgUtilization)
		assert.Zero(t, stats.MaxUtilization)
		assert.Zero(t, stats.MinUtilization)
		assert.Zero(t, stats.P50Utilization)
		assert.Zero(t, stats.P95Utilization)
		assert.Zero(t, stats.SampleCount)
	}
}

// TestNamespaceGpuAggregationBackfillJob_CreateZeroNamespaceStats_Fields tests all fields
func TestNamespaceGpuAggregationBackfillJob_CreateZeroNamespaceStats_Fields(t *testing.T) {
	job := &NamespaceGpuAggregationBackfillJob{}

	namespaces := []string{"default", "production", "staging", "dev"}
	clusterName := "test-cluster"
	testHour := time.Date(2025, 1, 1, 10, 0, 0, 0, time.UTC)

	for _, ns := range namespaces {
		stats := job.createZeroNamespaceStats(clusterName, ns, testHour)

		assert.Equal(t, clusterName, stats.ClusterName)
		assert.Equal(t, ns, stats.Namespace)
		assert.Equal(t, testHour, stats.StatHour)
		assert.Zero(t, stats.TotalGpuCapacity)
		assert.Zero(t, stats.AllocatedGpuCount)
		assert.Zero(t, stats.AllocationRate)
		assert.Zero(t, stats.AvgUtilization)
		assert.Zero(t, stats.MaxUtilization)
		assert.Zero(t, stats.MinUtilization)
		assert.Zero(t, stats.ActiveWorkloadCount)
	}
}

// TestLabelGpuAggregationBackfillConfig_KeyValidation tests key validation
func TestLabelGpuAggregationBackfillConfig_KeyValidation(t *testing.T) {
	tests := []struct {
		name           string
		labelKeys      []string
		annotationKeys []string
		hasKeys        bool
	}{
		{
			name:           "no keys",
			labelKeys:      []string{},
			annotationKeys: []string{},
			hasKeys:        false,
		},
		{
			name:           "nil keys",
			labelKeys:      nil,
			annotationKeys: nil,
			hasKeys:        false,
		},
		{
			name:           "only label keys",
			labelKeys:      []string{"app"},
			annotationKeys: nil,
			hasKeys:        true,
		},
		{
			name:           "only annotation keys",
			labelKeys:      nil,
			annotationKeys: []string{"project"},
			hasKeys:        true,
		},
		{
			name:           "both keys",
			labelKeys:      []string{"app"},
			annotationKeys: []string{"project"},
			hasKeys:        true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := &LabelGpuAggregationBackfillConfig{
				LabelKeys:      tt.labelKeys,
				AnnotationKeys: tt.annotationKeys,
			}
			hasKeys := len(config.LabelKeys) > 0 || len(config.AnnotationKeys) > 0
			assert.Equal(t, tt.hasKeys, hasKeys)
		})
	}
}

// TestNamespaceGpuAggregationBackfillJob_ShouldExcludeNamespace_EmptyConfig tests empty config
func TestNamespaceGpuAggregationBackfillJob_ShouldExcludeNamespace_EmptyConfig(t *testing.T) {
	job := &NamespaceGpuAggregationBackfillJob{
		config: &NamespaceGpuAggregationBackfillConfig{
			ExcludeNamespaces:       nil,
			IncludeSystemNamespaces: true,
		},
	}

	assert.False(t, job.shouldExcludeNamespace("any-namespace"))
	assert.False(t, job.shouldExcludeNamespace("kube-system"))
}

// TestBackfillDaysToHours tests conversion from days to hours
func TestBackfillDaysToHours(t *testing.T) {
	tests := []struct {
		days          int
		expectedHours int
	}{
		{1, 24},
		{7, 168},
		{14, 336},
		{30, 720},
		{0, 0},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("%d_days", tt.days), func(t *testing.T) {
			hours := tt.days * 24
			assert.Equal(t, tt.expectedHours, hours)
		})
	}
}

// TestGenerateAllHours_MonthBoundary tests hour generation across month boundaries
func TestGenerateAllHours_MonthBoundary(t *testing.T) {
	startTime := time.Date(2025, 1, 31, 22, 0, 0, 0, time.UTC)
	endTime := time.Date(2025, 2, 1, 2, 0, 0, 0, time.UTC)

	result := generateAllHours(startTime, endTime)

	assert.Equal(t, 5, len(result))
	assert.Equal(t, time.January, result[0].Month())
	assert.Equal(t, time.February, result[len(result)-1].Month())
}

// TestGenerateAllHours_YearBoundary tests hour generation across year boundaries
func TestGenerateAllHours_YearBoundary(t *testing.T) {
	startTime := time.Date(2024, 12, 31, 22, 0, 0, 0, time.UTC)
	endTime := time.Date(2025, 1, 1, 2, 0, 0, 0, time.UTC)

	result := generateAllHours(startTime, endTime)

	assert.Equal(t, 5, len(result))
	assert.Equal(t, 2024, result[0].Year())
	assert.Equal(t, 2025, result[len(result)-1].Year())
}

// TestBackfillJobSchedules tests all backfill job schedules
func TestBackfillJobSchedules(t *testing.T) {
	clusterJob := &ClusterGpuAggregationBackfillJob{}
	namespaceJob := &NamespaceGpuAggregationBackfillJob{}
	labelJob := &LabelGpuAggregationBackfillJob{}

	assert.Equal(t, "@every 5m", clusterJob.Schedule())
	assert.Equal(t, "@every 5m", namespaceJob.Schedule())
	assert.Equal(t, "@every 5m", labelJob.Schedule())
}

// TestBackfillConfigSetterGetter tests config setter/getter for all backfill jobs
func TestBackfillConfigSetterGetter(t *testing.T) {
	t.Run("ClusterBackfillJob", func(t *testing.T) {
		job := &ClusterGpuAggregationBackfillJob{
			config: &ClusterGpuAggregationBackfillConfig{Enabled: false},
		}
		assert.False(t, job.GetConfig().Enabled)

		newConfig := &ClusterGpuAggregationBackfillConfig{Enabled: true, BackfillDays: 14}
		job.SetConfig(newConfig)
		assert.True(t, job.GetConfig().Enabled)
		assert.Equal(t, 14, job.GetConfig().BackfillDays)
	})

	t.Run("NamespaceBackfillJob", func(t *testing.T) {
		job := &NamespaceGpuAggregationBackfillJob{
			config: &NamespaceGpuAggregationBackfillConfig{Enabled: false},
		}
		assert.False(t, job.GetConfig().Enabled)

		newConfig := &NamespaceGpuAggregationBackfillConfig{
			Enabled:           true,
			ExcludeNamespaces: []string{"test"},
		}
		job.SetConfig(newConfig)
		assert.True(t, job.GetConfig().Enabled)
		assert.Equal(t, []string{"test"}, job.GetConfig().ExcludeNamespaces)
	})

	t.Run("LabelBackfillJob", func(t *testing.T) {
		job := &LabelGpuAggregationBackfillJob{
			config: &LabelGpuAggregationBackfillConfig{Enabled: false},
		}
		assert.False(t, job.GetConfig().Enabled)

		newConfig := &LabelGpuAggregationBackfillConfig{
			Enabled:   true,
			LabelKeys: []string{"app"},
		}
		job.SetConfig(newConfig)
		assert.True(t, job.GetConfig().Enabled)
		assert.Equal(t, []string{"app"}, job.GetConfig().LabelKeys)
	})
}

// TestLabelGpuAggregationSystemConfig_NilFields tests nil field handling
func TestLabelGpuAggregationSystemConfig_NilFields(t *testing.T) {
	config := LabelGpuAggregationSystemConfig{}

	assert.False(t, config.Dimensions.Label.Enabled)
	assert.Nil(t, config.Dimensions.Label.LabelKeys)
	assert.Nil(t, config.Dimensions.Label.AnnotationKeys)
	assert.Empty(t, config.Dimensions.Label.DefaultValue)
}

// TestGenerateAllHours_Performance tests performance with large ranges
func TestGenerateAllHours_Performance(t *testing.T) {
	startTime := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	endTime := time.Date(2024, 12, 31, 23, 0, 0, 0, time.UTC)

	result := generateAllHours(startTime, endTime)

	expectedHours := 366 * 24
	assert.Equal(t, expectedHours, len(result))
}

// TestNamespaceGpuAggregationBackfillJob_MultipleExclusions tests multiple exclusions
func TestNamespaceGpuAggregationBackfillJob_MultipleExclusions(t *testing.T) {
	config := &NamespaceGpuAggregationBackfillConfig{
		Enabled:                 true,
		ExcludeNamespaces:       []string{"dev", "test", "staging", "qa"},
		IncludeSystemNamespaces: false,
	}
	job := &NamespaceGpuAggregationBackfillJob{config: config}

	for _, ns := range config.ExcludeNamespaces {
		assert.True(t, job.shouldExcludeNamespace(ns))
	}

	assert.False(t, job.shouldExcludeNamespace("production"))
	assert.False(t, job.shouldExcludeNamespace("default"))

	assert.True(t, job.shouldExcludeNamespace("kube-system"))
}

// TestClusterGpuAggregationBackfillConfig_BatchSizeValidation tests batch size values
func TestClusterGpuAggregationBackfillConfig_BatchSizeValidation(t *testing.T) {
	tests := []struct {
		name      string
		batchSize int
		valid     bool
	}{
		{"default", DefaultClusterBatchSize, true},
		{"small", 1, true},
		{"large", 168, true},
		{"zero", 0, false},
		{"negative", -1, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := &ClusterGpuAggregationBackfillConfig{BatchSize: tt.batchSize}
			isValid := config.BatchSize > 0
			assert.Equal(t, tt.valid, isValid)
		})
	}
}

// TestNamespaceGpuAggregationBackfillConfig_BackfillDaysValidation tests backfill days values
func TestNamespaceGpuAggregationBackfillConfig_BackfillDaysValidation(t *testing.T) {
	tests := []struct {
		name         string
		backfillDays int
		valid        bool
	}{
		{"default", DefaultNamespaceBackfillDays, true},
		{"one day", 1, true},
		{"one month", 30, true},
		{"zero", 0, false},
		{"negative", -1, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := &NamespaceGpuAggregationBackfillConfig{BackfillDays: tt.backfillDays}
			isValid := config.BackfillDays > 0
			assert.Equal(t, tt.valid, isValid)
		})
	}
}

// TestLabelGpuAggregationBackfillConfig_DefaultValueHandling tests default value handling
func TestLabelGpuAggregationBackfillConfig_DefaultValueHandling(t *testing.T) {
	tests := []struct {
		name         string
		defaultValue string
	}{
		{"unknown", "unknown"},
		{"not-set", "not-set"},
		{"empty", ""},
		{"default", "default"},
		{"n/a", "n/a"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := &LabelGpuAggregationBackfillConfig{DefaultValue: tt.defaultValue}
			assert.Equal(t, tt.defaultValue, config.DefaultValue)
		})
	}
}

// TestGenerateAllHours_FirstAndLastHour tests first and last hour correctness
func TestGenerateAllHours_FirstAndLastHour(t *testing.T) {
	startTime := time.Date(2025, 6, 15, 8, 30, 45, 0, time.UTC)
	endTime := time.Date(2025, 6, 15, 16, 45, 30, 0, time.UTC)

	result := generateAllHours(startTime, endTime)

	assert.Equal(t, time.Date(2025, 6, 15, 8, 0, 0, 0, time.UTC), result[0])
	assert.Equal(t, time.Date(2025, 6, 15, 16, 0, 0, 0, time.UTC), result[len(result)-1])
}

// TestBackfillJobClusterName tests cluster name handling in backfill jobs
func TestBackfillJobClusterName(t *testing.T) {
	clusterNames := []string{"cluster-1", "prod-cluster", "test-cluster", ""}

	for _, name := range clusterNames {
		clusterJob := &ClusterGpuAggregationBackfillJob{clusterName: name}
		assert.Equal(t, name, clusterJob.clusterName)

		namespaceJob := &NamespaceGpuAggregationBackfillJob{clusterName: name}
		assert.Equal(t, name, namespaceJob.clusterName)

		labelJob := &LabelGpuAggregationBackfillJob{clusterName: name}
		assert.Equal(t, name, labelJob.clusterName)
	}
}

// TestDefaultConstants_Consistency tests default constant consistency
func TestDefaultConstants_Consistency(t *testing.T) {
	assert.Equal(t, DefaultClusterBackfillDays, DefaultNamespaceBackfillDays)
	assert.Equal(t, DefaultNamespaceBackfillDays, DefaultLabelBackfillDays)
	assert.Equal(t, DefaultClusterBatchSize, DefaultNamespaceBatchSize)
	assert.Equal(t, DefaultNamespaceBatchSize, DefaultLabelBatchSize)
}

// TestClusterGpuAggregationBackfillJob_StructInitialization tests struct initialization
func TestClusterGpuAggregationBackfillJob_StructInitialization(t *testing.T) {
	config := &ClusterGpuAggregationBackfillConfig{
		Enabled:      true,
		BackfillDays: 14,
		BatchSize:    48,
	}
	job := &ClusterGpuAggregationBackfillJob{
		config:      config,
		clusterName: "init-test-cluster",
	}

	assert.NotNil(t, job.config)
	assert.Equal(t, "init-test-cluster", job.clusterName)
	assert.True(t, job.config.Enabled)
	assert.Equal(t, 14, job.config.BackfillDays)
	assert.Equal(t, 48, job.config.BatchSize)
}

// TestNamespaceGpuAggregationBackfillJob_StructInitialization tests struct initialization
func TestNamespaceGpuAggregationBackfillJob_StructInitialization(t *testing.T) {
	config := &NamespaceGpuAggregationBackfillConfig{
		Enabled:                 true,
		BackfillDays:            7,
		BatchSize:               24,
		ExcludeNamespaces:       []string{"dev", "test"},
		IncludeSystemNamespaces: false,
	}
	job := &NamespaceGpuAggregationBackfillJob{
		config:      config,
		clusterName: "ns-test-cluster",
	}

	assert.NotNil(t, job.config)
	assert.Equal(t, "ns-test-cluster", job.clusterName)
	assert.Equal(t, 2, len(job.config.ExcludeNamespaces))
}

// TestLabelGpuAggregationBackfillJob_StructInitialization tests struct initialization
func TestLabelGpuAggregationBackfillJob_StructInitialization(t *testing.T) {
	config := &LabelGpuAggregationBackfillConfig{
		Enabled:        true,
		BackfillDays:   30,
		BatchSize:      72,
		LabelKeys:      []string{"app", "team"},
		AnnotationKeys: []string{"project"},
		DefaultValue:   "unknown",
	}
	job := &LabelGpuAggregationBackfillJob{
		config:      config,
		clusterName: "label-test-cluster",
	}

	assert.NotNil(t, job.config)
	assert.Equal(t, "label-test-cluster", job.clusterName)
	assert.Equal(t, 2, len(job.config.LabelKeys))
	assert.Equal(t, 1, len(job.config.AnnotationKeys))
	assert.Equal(t, "unknown", job.config.DefaultValue)
}

// TestGenerateAllHours_BoundaryConditions tests boundary conditions
func TestGenerateAllHours_BoundaryConditions(t *testing.T) {
	tests := []struct {
		name      string
		startTime time.Time
		endTime   time.Time
		expected  int
	}{
		{
			name:      "exactly 1 hour apart",
			startTime: time.Date(2025, 1, 1, 10, 0, 0, 0, time.UTC),
			endTime:   time.Date(2025, 1, 1, 11, 0, 0, 0, time.UTC),
			expected:  2,
		},
		{
			name:      "less than 1 hour apart same hour",
			startTime: time.Date(2025, 1, 1, 10, 15, 0, 0, time.UTC),
			endTime:   time.Date(2025, 1, 1, 10, 45, 0, 0, time.UTC),
			expected:  1,
		},
		{
			name:      "exactly at midnight",
			startTime: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
			endTime:   time.Date(2025, 1, 1, 23, 0, 0, 0, time.UTC),
			expected:  24,
		},
		{
			name:      "leap year feb 29",
			startTime: time.Date(2024, 2, 28, 22, 0, 0, 0, time.UTC),
			endTime:   time.Date(2024, 3, 1, 2, 0, 0, 0, time.UTC),
			expected:  29,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := generateAllHours(tt.startTime, tt.endTime)
			assert.Equal(t, tt.expected, len(result))
		})
	}
}

// TestNamespaceGpuAggregationBackfillJob_ShouldExcludeNamespace_Comprehensive tests comprehensive exclusion
func TestNamespaceGpuAggregationBackfillJob_ShouldExcludeNamespace_Comprehensive(t *testing.T) {
	config := &NamespaceGpuAggregationBackfillConfig{
		Enabled:                 true,
		ExcludeNamespaces:       []string{"exclude-1", "exclude-2"},
		IncludeSystemNamespaces: false,
	}
	job := &NamespaceGpuAggregationBackfillJob{config: config}

	testCases := []struct {
		namespace string
		excluded  bool
	}{
		{"exclude-1", true},
		{"exclude-2", true},
		{"kube-system", true},
		{"kube-public", true},
		{"kube-node-lease", true},
		{"production", false},
		{"staging", false},
		{"default", false},
	}

	for _, tc := range testCases {
		t.Run(tc.namespace, func(t *testing.T) {
			result := job.shouldExcludeNamespace(tc.namespace)
			assert.Equal(t, tc.excluded, result, "namespace %s", tc.namespace)
		})
	}
}

// TestClusterGpuAggregationBackfillJob_CreateZeroClusterStats_AllFieldsValidation tests zero stats all fields
func TestClusterGpuAggregationBackfillJob_CreateZeroClusterStats_AllFieldsValidation(t *testing.T) {
	job := &ClusterGpuAggregationBackfillJob{}
	testHour := time.Date(2025, 6, 15, 10, 0, 0, 0, time.UTC)

	stats := job.createZeroClusterStats("zero-test-cluster", testHour)

	assert.Equal(t, "zero-test-cluster", stats.ClusterName)
	assert.Equal(t, testHour, stats.StatHour)
	assert.Equal(t, int32(0), stats.TotalGpuCapacity)
	assert.Equal(t, float64(0), stats.AllocatedGpuCount)
	assert.Equal(t, float64(0), stats.AllocationRate)
	assert.Equal(t, float64(0), stats.AvgUtilization)
	assert.Equal(t, float64(0), stats.MaxUtilization)
	assert.Equal(t, float64(0), stats.MinUtilization)
	assert.Equal(t, float64(0), stats.P50Utilization)
	assert.Equal(t, float64(0), stats.P95Utilization)
	assert.Equal(t, int32(0), stats.SampleCount)
}

// TestNamespaceGpuAggregationBackfillJob_CreateZeroNamespaceStats_AllFieldsValidation tests zero stats all fields
func TestNamespaceGpuAggregationBackfillJob_CreateZeroNamespaceStats_AllFieldsValidation(t *testing.T) {
	job := &NamespaceGpuAggregationBackfillJob{}
	testHour := time.Date(2025, 6, 15, 10, 0, 0, 0, time.UTC)

	stats := job.createZeroNamespaceStats("zero-cluster", "zero-ns", testHour)

	assert.Equal(t, "zero-cluster", stats.ClusterName)
	assert.Equal(t, "zero-ns", stats.Namespace)
	assert.Equal(t, testHour, stats.StatHour)
	assert.Equal(t, int32(0), stats.TotalGpuCapacity)
	assert.Equal(t, float64(0), stats.AllocatedGpuCount)
	assert.Equal(t, float64(0), stats.AllocationRate)
	assert.Equal(t, float64(0), stats.AvgUtilization)
	assert.Equal(t, float64(0), stats.MaxUtilization)
	assert.Equal(t, float64(0), stats.MinUtilization)
	assert.Equal(t, int32(0), stats.ActiveWorkloadCount)
}

// TestLabelGpuAggregationSystemConfig_DefaultValues tests default values
func TestLabelGpuAggregationSystemConfig_DefaultValues(t *testing.T) {
	config := LabelGpuAggregationSystemConfig{}

	assert.False(t, config.Dimensions.Label.Enabled)
	assert.Nil(t, config.Dimensions.Label.LabelKeys)
	assert.Nil(t, config.Dimensions.Label.AnnotationKeys)
	assert.Empty(t, config.Dimensions.Label.DefaultValue)
}

// TestClusterGpuAggregationBackfillConfig_Validation tests config validation
func TestClusterGpuAggregationBackfillConfig_Validation(t *testing.T) {
	tests := []struct {
		name         string
		config       *ClusterGpuAggregationBackfillConfig
		isValidDays  bool
		isValidBatch bool
	}{
		{
			name: "valid config",
			config: &ClusterGpuAggregationBackfillConfig{
				Enabled:      true,
				BackfillDays: 7,
				BatchSize:    24,
			},
			isValidDays:  true,
			isValidBatch: true,
		},
		{
			name: "zero days",
			config: &ClusterGpuAggregationBackfillConfig{
				Enabled:      true,
				BackfillDays: 0,
				BatchSize:    24,
			},
			isValidDays:  false,
			isValidBatch: true,
		},
		{
			name: "zero batch",
			config: &ClusterGpuAggregationBackfillConfig{
				Enabled:      true,
				BackfillDays: 7,
				BatchSize:    0,
			},
			isValidDays:  true,
			isValidBatch: false,
		},
		{
			name: "negative days",
			config: &ClusterGpuAggregationBackfillConfig{
				Enabled:      true,
				BackfillDays: -1,
				BatchSize:    24,
			},
			isValidDays:  false,
			isValidBatch: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.isValidDays, tt.config.BackfillDays > 0)
			assert.Equal(t, tt.isValidBatch, tt.config.BatchSize > 0)
		})
	}
}

// TestNamespaceGpuAggregationBackfillConfig_Validation tests config validation
func TestNamespaceGpuAggregationBackfillConfig_Validation(t *testing.T) {
	tests := []struct {
		name   string
		config *NamespaceGpuAggregationBackfillConfig
		valid  bool
	}{
		{
			name: "valid with defaults",
			config: &NamespaceGpuAggregationBackfillConfig{
				Enabled:      true,
				BackfillDays: DefaultNamespaceBackfillDays,
				BatchSize:    DefaultNamespaceBatchSize,
			},
			valid: true,
		},
		{
			name: "disabled is valid",
			config: &NamespaceGpuAggregationBackfillConfig{
				Enabled:      false,
				BackfillDays: 0,
				BatchSize:    0,
			},
			valid: true,
		},
		{
			name: "with exclusions",
			config: &NamespaceGpuAggregationBackfillConfig{
				Enabled:           true,
				BackfillDays:      7,
				BatchSize:         24,
				ExcludeNamespaces: []string{"ns1", "ns2", "ns3"},
			},
			valid: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.NotNil(t, tt.config)
		})
	}
}

// TestLabelGpuAggregationBackfillConfig_KeysValidation tests keys validation
func TestLabelGpuAggregationBackfillConfig_KeysValidation(t *testing.T) {
	tests := []struct {
		name     string
		config   *LabelGpuAggregationBackfillConfig
		hasKeys  bool
		keyCount int
	}{
		{
			name: "no keys",
			config: &LabelGpuAggregationBackfillConfig{
				LabelKeys:      nil,
				AnnotationKeys: nil,
			},
			hasKeys:  false,
			keyCount: 0,
		},
		{
			name: "only label keys",
			config: &LabelGpuAggregationBackfillConfig{
				LabelKeys:      []string{"app", "team"},
				AnnotationKeys: nil,
			},
			hasKeys:  true,
			keyCount: 2,
		},
		{
			name: "only annotation keys",
			config: &LabelGpuAggregationBackfillConfig{
				LabelKeys:      nil,
				AnnotationKeys: []string{"project", "cost-center"},
			},
			hasKeys:  true,
			keyCount: 2,
		},
		{
			name: "both keys",
			config: &LabelGpuAggregationBackfillConfig{
				LabelKeys:      []string{"app"},
				AnnotationKeys: []string{"project"},
			},
			hasKeys:  true,
			keyCount: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hasKeys := len(tt.config.LabelKeys) > 0 || len(tt.config.AnnotationKeys) > 0
			assert.Equal(t, tt.hasKeys, hasKeys)
			keyCount := len(tt.config.LabelKeys) + len(tt.config.AnnotationKeys)
			assert.Equal(t, tt.keyCount, keyCount)
		})
	}
}

// TestGenerateAllHours_LongRange tests generating hours for long ranges
func TestGenerateAllHours_LongRange(t *testing.T) {
	startTime := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	endTime := time.Date(2025, 12, 31, 23, 0, 0, 0, time.UTC)

	result := generateAllHours(startTime, endTime)

	expectedHours := 365 * 24
	assert.Equal(t, expectedHours, len(result))

	assert.Equal(t, startTime, result[0])
	assert.Equal(t, endTime, result[len(result)-1])
}

// TestGenerateAllHours_HourTruncation tests that all hours are truncated
func TestGenerateAllHours_HourTruncation(t *testing.T) {
	startTime := time.Date(2025, 1, 1, 10, 30, 45, 123456789, time.UTC)
	endTime := time.Date(2025, 1, 1, 15, 15, 30, 987654321, time.UTC)

	result := generateAllHours(startTime, endTime)

	for _, hour := range result {
		assert.Equal(t, 0, hour.Minute())
		assert.Equal(t, 0, hour.Second())
		assert.Equal(t, 0, hour.Nanosecond())
	}
}

// TestGenerateAllHours_ConsecutiveHoursDiff tests consecutive hours difference
func TestGenerateAllHours_ConsecutiveHoursDiff(t *testing.T) {
	startTime := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	endTime := time.Date(2025, 1, 2, 23, 0, 0, 0, time.UTC)

	result := generateAllHours(startTime, endTime)

	for i := 1; i < len(result); i++ {
		diff := result[i].Sub(result[i-1])
		assert.Equal(t, time.Hour, diff, "Consecutive hours should differ by 1 hour")
	}
}

// TestSystemConfigKeyGpuAggregationBackfill_ValueAndContent tests config key value and content
func TestSystemConfigKeyGpuAggregationBackfill_ValueAndContent(t *testing.T) {
	assert.Equal(t, "job.gpu_aggregation.config", SystemConfigKeyGpuAggregationBackfill)
	assert.Contains(t, SystemConfigKeyGpuAggregationBackfill, "gpu_aggregation")
}

// TestBackfillJobSchedules tests all backfill job schedules are consistent
func TestBackfillJobSchedules_Consistent(t *testing.T) {
	clusterJob := &ClusterGpuAggregationBackfillJob{}
	namespaceJob := &NamespaceGpuAggregationBackfillJob{}
	labelJob := &LabelGpuAggregationBackfillJob{}

	schedules := []string{
		clusterJob.Schedule(),
		namespaceJob.Schedule(),
		labelJob.Schedule(),
	}

	expectedSchedule := "@every 5m"
	for i, schedule := range schedules {
		assert.Equal(t, expectedSchedule, schedule, "Job %d should have schedule %s", i, expectedSchedule)
	}
}

// TestClusterGpuAggregationBackfillJob_NilConfig tests nil config handling
func TestClusterGpuAggregationBackfillJob_NilConfig(t *testing.T) {
	job := &ClusterGpuAggregationBackfillJob{config: nil}
	assert.Nil(t, job.GetConfig())
}

// TestNamespaceGpuAggregationBackfillJob_NilConfig tests nil config handling
func TestNamespaceGpuAggregationBackfillJob_NilConfig(t *testing.T) {
	job := &NamespaceGpuAggregationBackfillJob{config: nil}
	assert.Nil(t, job.GetConfig())
}

// TestLabelGpuAggregationBackfillJob_NilConfig tests nil config handling
func TestLabelGpuAggregationBackfillJob_NilConfig(t *testing.T) {
	job := &LabelGpuAggregationBackfillJob{config: nil}
	assert.Nil(t, job.GetConfig())
}

// TestNamespaceGpuAggregationBackfillJob_ShouldExcludeNamespace_NilExclusions tests nil exclusions
func TestNamespaceGpuAggregationBackfillJob_ShouldExcludeNamespace_NilExclusions(t *testing.T) {
	job := &NamespaceGpuAggregationBackfillJob{
		config: &NamespaceGpuAggregationBackfillConfig{
			ExcludeNamespaces:       nil,
			IncludeSystemNamespaces: true,
		},
	}

	assert.False(t, job.shouldExcludeNamespace("any-namespace"))
	assert.False(t, job.shouldExcludeNamespace("kube-system"))
}

// TestGenerateAllHours_SingleMinute tests generation with times in same minute
func TestGenerateAllHours_SingleMinute(t *testing.T) {
	startTime := time.Date(2025, 1, 1, 10, 30, 0, 0, time.UTC)
	endTime := time.Date(2025, 1, 1, 10, 31, 0, 0, time.UTC)

	result := generateAllHours(startTime, endTime)

	assert.Equal(t, 1, len(result))
	assert.Equal(t, time.Date(2025, 1, 1, 10, 0, 0, 0, time.UTC), result[0])
}

// TestClusterGpuAggregationBackfillJob_EmptyClusterName tests empty cluster name
func TestClusterGpuAggregationBackfillJob_EmptyClusterName(t *testing.T) {
	job := &ClusterGpuAggregationBackfillJob{
		config:      &ClusterGpuAggregationBackfillConfig{Enabled: true},
		clusterName: "",
	}

	assert.Empty(t, job.clusterName)
}

// TestLabelGpuAggregationSystemConfig_NestedAccess tests nested field access
func TestLabelGpuAggregationSystemConfig_NestedAccess(t *testing.T) {
	config := LabelGpuAggregationSystemConfig{}

	config.Dimensions.Label.Enabled = true
	config.Dimensions.Label.LabelKeys = []string{"key1", "key2", "key3"}
	config.Dimensions.Label.AnnotationKeys = []string{"ann1", "ann2"}
	config.Dimensions.Label.DefaultValue = "default-val"

	assert.True(t, config.Dimensions.Label.Enabled)
	assert.Equal(t, 3, len(config.Dimensions.Label.LabelKeys))
	assert.Equal(t, 2, len(config.Dimensions.Label.AnnotationKeys))
	assert.Equal(t, "default-val", config.Dimensions.Label.DefaultValue)
}

// TestDefaultBackfillDays_Values tests default backfill days values
func TestDefaultBackfillDays_Values(t *testing.T) {
	assert.True(t, DefaultClusterBackfillDays > 0)
	assert.True(t, DefaultNamespaceBackfillDays > 0)
	assert.True(t, DefaultLabelBackfillDays > 0)

	assert.True(t, DefaultClusterBackfillDays <= 30)
	assert.True(t, DefaultNamespaceBackfillDays <= 30)
	assert.True(t, DefaultLabelBackfillDays <= 30)
}

// TestDefaultBatchSize_Values tests default batch size values
func TestDefaultBatchSize_Values(t *testing.T) {
	assert.True(t, DefaultClusterBatchSize > 0)
	assert.True(t, DefaultNamespaceBatchSize > 0)
	assert.True(t, DefaultLabelBatchSize > 0)

	assert.True(t, DefaultClusterBatchSize <= 168)
	assert.True(t, DefaultNamespaceBatchSize <= 168)
	assert.True(t, DefaultLabelBatchSize <= 168)
}

// TestClusterGpuAggregationBackfillJob_CreateZeroClusterStats_DifferentHours tests different hours
func TestClusterGpuAggregationBackfillJob_CreateZeroClusterStats_DifferentHours(t *testing.T) {
	job := &ClusterGpuAggregationBackfillJob{}

	hours := []time.Time{
		time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
		time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC),
		time.Date(2025, 1, 1, 23, 0, 0, 0, time.UTC),
		time.Date(2025, 6, 15, 10, 0, 0, 0, time.UTC),
		time.Date(2025, 12, 31, 23, 0, 0, 0, time.UTC),
	}

	for _, hour := range hours {
		stats := job.createZeroClusterStats("test-cluster", hour)
		assert.Equal(t, hour, stats.StatHour)
		assert.Equal(t, "test-cluster", stats.ClusterName)
	}
}

// TestNamespaceGpuAggregationBackfillJob_CreateZeroNamespaceStats_DifferentNamespaces tests different namespaces
func TestNamespaceGpuAggregationBackfillJob_CreateZeroNamespaceStats_DifferentNamespaces(t *testing.T) {
	job := &NamespaceGpuAggregationBackfillJob{}
	testHour := time.Date(2025, 1, 1, 10, 0, 0, 0, time.UTC)

	namespaces := []string{
		"default",
		"production",
		"staging",
		"dev",
		"ml-training",
		"kube-system",
	}

	for _, ns := range namespaces {
		stats := job.createZeroNamespaceStats("cluster", ns, testHour)
		assert.Equal(t, ns, stats.Namespace)
		assert.Equal(t, "cluster", stats.ClusterName)
	}
}

// TestBackfillTimeRangeCalculation tests time range calculation for backfill
func TestBackfillTimeRangeCalculation(t *testing.T) {
	backfillDays := 7
	now := time.Now()
	endTime := now.Truncate(time.Hour).Add(-time.Hour)
	startTime := endTime.Add(-time.Duration(backfillDays) * 24 * time.Hour)

	assert.True(t, startTime.Before(endTime))
	assert.True(t, endTime.Before(now))

	expectedDuration := time.Duration(backfillDays) * 24 * time.Hour
	actualDuration := endTime.Sub(startTime)
	assert.Equal(t, expectedDuration, actualDuration)
}

// TestGenerateAllHours_SpecificTimeZones tests with specific time zones
func TestGenerateAllHours_SpecificTimeZones(t *testing.T) {
	locations := []string{
		"UTC",
		"America/New_York",
		"Europe/London",
		"Asia/Tokyo",
	}

	for _, locName := range locations {
		t.Run(locName, func(t *testing.T) {
			loc, err := time.LoadLocation(locName)
			if err != nil {
				t.Skip("Location not available:", locName)
				return
			}

			startTime := time.Date(2025, 1, 1, 10, 0, 0, 0, loc)
			endTime := time.Date(2025, 1, 1, 15, 0, 0, 0, loc)

			result := generateAllHours(startTime, endTime)
			assert.Equal(t, 6, len(result))
		})
	}
}
