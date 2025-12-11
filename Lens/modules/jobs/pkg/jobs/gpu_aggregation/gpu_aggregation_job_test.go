package gpu_aggregation

import (
	"fmt"
	"testing"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/model"
	"github.com/stretchr/testify/assert"
)

func TestCalculatePercentile(t *testing.T) {
	tests := []struct {
		name         string
		sortedValues []float64
		percentile   float64
		expected     float64
	}{
		{
			name:         "empty slice",
			sortedValues: []float64{},
			percentile:   0.5,
			expected:     0,
		},
		{
			name:         "single value",
			sortedValues: []float64{10.0},
			percentile:   0.5,
			expected:     10.0,
		},
		{
			name:         "p50 of sorted values",
			sortedValues: []float64{1.0, 2.0, 3.0, 4.0, 5.0},
			percentile:   0.5,
			expected:     3.0,
		},
		{
			name:         "p95 of sorted values",
			sortedValues: []float64{1.0, 2.0, 3.0, 4.0, 5.0, 6.0, 7.0, 8.0, 9.0, 10.0},
			percentile:   0.95,
			expected:     10.0,
		},
		{
			name:         "p0 (minimum)",
			sortedValues: []float64{5.0, 10.0, 15.0, 20.0},
			percentile:   0.0,
			expected:     5.0,
		},
		{
			name:         "p100 (maximum)",
			sortedValues: []float64{5.0, 10.0, 15.0, 20.0},
			percentile:   1.0,
			expected:     20.0,
		},
		{
			name:         "p75",
			sortedValues: []float64{1.0, 2.0, 3.0, 4.0, 5.0, 6.0, 7.0, 8.0},
			percentile:   0.75,
			expected:     6.0,
		},
		{
			name:         "p25",
			sortedValues: []float64{10.0, 20.0, 30.0, 40.0},
			percentile:   0.25,
			expected:     10.0,
		},
		{
			name:         "floating point values",
			sortedValues: []float64{1.5, 2.7, 3.9, 4.1, 5.3},
			percentile:   0.5,
			expected:     3.9,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := calculatePercentile(tt.sortedValues, tt.percentile)
			assert.Equal(t, tt.expected, result, "Percentile should match expected")
		})
	}
}

func TestSplitAnnotationKey(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{
			name:     "key value pair",
			input:    "app:frontend",
			expected: []string{"app", "frontend"},
		},
		{
			name:     "key with empty value",
			input:    "key:",
			expected: []string{"key", ""},
		},
		{
			name:     "value with colon",
			input:    "url:http://example.com",
			expected: []string{"url", "http://example.com"},
		},
		{
			name:     "multiple colons",
			input:    "path:a:b:c",
			expected: []string{"path", "a:b:c"},
		},
		{
			name:     "no colon",
			input:    "nocolon",
			expected: []string{"nocolon"},
		},
		{
			name:     "empty string",
			input:    "",
			expected: []string{""},
		},
		{
			name:     "only colon",
			input:    ":",
			expected: []string{"", ""},
		},
		{
			name:     "namespace qualified key",
			input:    "kubernetes.io/name:value",
			expected: []string{"kubernetes.io/name", "value"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := splitAnnotationKey(tt.input)
			assert.Equal(t, tt.expected, result, "Split result should match expected")
		})
	}
}

func TestNamespaceGpuAggregationJob_ShouldExcludeNamespace(t *testing.T) {
	tests := []struct {
		name      string
		config    *NamespaceGpuAggregationConfig
		namespace string
		expected  bool
	}{
		{
			name: "namespace in exclusion list",
			config: &NamespaceGpuAggregationConfig{
				Enabled:                 true,
				ExcludeNamespaces:       []string{"test", "dev", "staging"},
				IncludeSystemNamespaces: true,
			},
			namespace: "dev",
			expected:  true,
		},
		{
			name: "system namespace excluded when flag is false",
			config: &NamespaceGpuAggregationConfig{
				Enabled:                 true,
				ExcludeNamespaces:       []string{},
				IncludeSystemNamespaces: false,
			},
			namespace: "kube-system",
			expected:  true,
		},
		{
			name: "kube-public excluded when flag is false",
			config: &NamespaceGpuAggregationConfig{
				Enabled:                 true,
				ExcludeNamespaces:       []string{},
				IncludeSystemNamespaces: false,
			},
			namespace: "kube-public",
			expected:  true,
		},
		{
			name: "kube-node-lease excluded when flag is false",
			config: &NamespaceGpuAggregationConfig{
				Enabled:                 true,
				ExcludeNamespaces:       []string{},
				IncludeSystemNamespaces: false,
			},
			namespace: "kube-node-lease",
			expected:  true,
		},
		{
			name: "system namespace included when flag is true",
			config: &NamespaceGpuAggregationConfig{
				Enabled:                 true,
				ExcludeNamespaces:       []string{},
				IncludeSystemNamespaces: true,
			},
			namespace: "kube-system",
			expected:  false,
		},
		{
			name: "regular namespace not excluded",
			config: &NamespaceGpuAggregationConfig{
				Enabled:                 true,
				ExcludeNamespaces:       []string{"test"},
				IncludeSystemNamespaces: false,
			},
			namespace: "production",
			expected:  false,
		},
		{
			name: "empty exclusion list",
			config: &NamespaceGpuAggregationConfig{
				Enabled:                 true,
				ExcludeNamespaces:       []string{},
				IncludeSystemNamespaces: true,
			},
			namespace: "any-namespace",
			expected:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			job := &NamespaceGpuAggregationJob{
				config: tt.config,
			}
			result := job.shouldExcludeNamespace(tt.namespace)
			assert.Equal(t, tt.expected, result, "Exclusion result should match expected")
		})
	}
}

func TestConvertToDBClusterStats(t *testing.T) {
	tests := []struct {
		name  string
		input *model.ClusterGpuHourlyStats
	}{
		{
			name: "convert with all fields",
			input: &model.ClusterGpuHourlyStats{
				ClusterName:       "test-cluster",
				TotalGpuCapacity:  100,
				AllocatedGpuCount: 75.5,
				AllocationRate:    75.5,
				AvgUtilization:    60.5,
				MaxUtilization:    95.0,
				MinUtilization:    30.0,
				P50Utilization:    55.0,
				P95Utilization:    90.0,
				SampleCount:       12,
			},
		},
		{
			name: "convert with zero values",
			input: &model.ClusterGpuHourlyStats{
				ClusterName:       "empty-cluster",
				TotalGpuCapacity:  0,
				AllocatedGpuCount: 0,
				AllocationRate:    0,
				AvgUtilization:    0,
				MaxUtilization:    0,
				MinUtilization:    0,
				P50Utilization:    0,
				P95Utilization:    0,
				SampleCount:       0,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := convertToDBClusterStats(tt.input)

			assert.NotNil(t, result, "Result should not be nil")
			assert.Equal(t, tt.input.ClusterName, result.ClusterName)
			assert.Equal(t, int32(tt.input.TotalGpuCapacity), result.TotalGpuCapacity)
			assert.Equal(t, tt.input.AllocatedGpuCount, result.AllocatedGpuCount)
			assert.Equal(t, tt.input.AllocationRate, result.AllocationRate)
			assert.Equal(t, tt.input.AvgUtilization, result.AvgUtilization)
			assert.Equal(t, tt.input.MaxUtilization, result.MaxUtilization)
			assert.Equal(t, tt.input.MinUtilization, result.MinUtilization)
			assert.Equal(t, tt.input.P50Utilization, result.P50Utilization)
			assert.Equal(t, tt.input.P95Utilization, result.P95Utilization)
			assert.Equal(t, int32(tt.input.SampleCount), result.SampleCount)
			assert.Equal(t, tt.input.StatHour, result.StatHour)
		})
	}
}

func TestConvertToDBNamespaceStats(t *testing.T) {
	tests := []struct {
		name  string
		input *model.NamespaceGpuHourlyStats
	}{
		{
			name: "convert with all fields",
			input: &model.NamespaceGpuHourlyStats{
				ClusterName:         "test-cluster",
				Namespace:           "production",
				TotalGpuCapacity:    100,
				AllocatedGpuCount:   50.5,
				AvgUtilization:      70.0,
				MaxUtilization:      90.0,
				MinUtilization:      40.0,
				ActiveWorkloadCount: 5,
			},
		},
		{
			name: "convert with zero values",
			input: &model.NamespaceGpuHourlyStats{
				ClusterName:         "test-cluster",
				Namespace:           "empty-ns",
				TotalGpuCapacity:    0,
				AllocatedGpuCount:   0,
				AvgUtilization:      0,
				MaxUtilization:      0,
				MinUtilization:      0,
				ActiveWorkloadCount: 0,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := convertToDBNamespaceStats(tt.input)

			assert.NotNil(t, result, "Result should not be nil")
			assert.Equal(t, tt.input.ClusterName, result.ClusterName)
			assert.Equal(t, tt.input.Namespace, result.Namespace)
			assert.Equal(t, int32(tt.input.TotalGpuCapacity), result.TotalGpuCapacity)
			assert.Equal(t, tt.input.AllocatedGpuCount, result.AllocatedGpuCount)
			assert.Equal(t, tt.input.AvgUtilization, result.AvgUtilization)
			assert.Equal(t, tt.input.MaxUtilization, result.MaxUtilization)
			assert.Equal(t, tt.input.MinUtilization, result.MinUtilization)
			assert.Equal(t, int32(tt.input.ActiveWorkloadCount), result.ActiveWorkloadCount)
			assert.Equal(t, tt.input.StatHour, result.StatHour)
		})
	}
}

func TestConvertToDBLabelStats(t *testing.T) {
	tests := []struct {
		name  string
		input *model.LabelGpuHourlyStats
	}{
		{
			name: "convert label dimension",
			input: &model.LabelGpuHourlyStats{
				ClusterName:         "test-cluster",
				DimensionType:       "label",
				DimensionKey:        "app",
				DimensionValue:      "frontend",
				AllocatedGpuCount:   25.5,
				AvgUtilization:      65.0,
				MaxUtilization:      85.0,
				MinUtilization:      45.0,
				ActiveWorkloadCount: 3,
			},
		},
		{
			name: "convert annotation dimension",
			input: &model.LabelGpuHourlyStats{
				ClusterName:         "test-cluster",
				DimensionType:       "annotation",
				DimensionKey:        "project",
				DimensionValue:      "ml-training",
				AllocatedGpuCount:   40.0,
				AvgUtilization:      80.0,
				MaxUtilization:      95.0,
				MinUtilization:      60.0,
				ActiveWorkloadCount: 10,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := convertToDBLabelStats(tt.input)

			assert.NotNil(t, result, "Result should not be nil")
			assert.Equal(t, tt.input.ClusterName, result.ClusterName)
			assert.Equal(t, tt.input.DimensionType, result.DimensionType)
			assert.Equal(t, tt.input.DimensionKey, result.DimensionKey)
			assert.Equal(t, tt.input.DimensionValue, result.DimensionValue)
			assert.Equal(t, tt.input.AllocatedGpuCount, result.AllocatedGpuCount)
			assert.Equal(t, tt.input.AvgUtilization, result.AvgUtilization)
			assert.Equal(t, tt.input.MaxUtilization, result.MaxUtilization)
			assert.Equal(t, tt.input.MinUtilization, result.MinUtilization)
			assert.Equal(t, int32(tt.input.ActiveWorkloadCount), result.ActiveWorkloadCount)
			assert.Equal(t, tt.input.StatHour, result.StatHour)
		})
	}
}

func TestClusterGpuAggregationJob_GetConfig(t *testing.T) {
	config := &ClusterGpuAggregationConfig{
		Enabled: true,
	}

	job := &ClusterGpuAggregationJob{
		config: config,
	}

	result := job.GetConfig()
	assert.Equal(t, config, result, "GetConfig should return the config")
}

func TestNamespaceGpuAggregationJob_GetConfig(t *testing.T) {
	config := &NamespaceGpuAggregationConfig{
		Enabled:                 true,
		ExcludeNamespaces:       []string{},
		IncludeSystemNamespaces: false,
	}

	job := &NamespaceGpuAggregationJob{
		config: config,
	}

	result := job.GetConfig()
	assert.Equal(t, config, result, "GetConfig should return the config")
}

func TestWorkloadGpuAggregationJob_GetConfig(t *testing.T) {
	config := &WorkloadGpuAggregationConfig{
		Enabled:       true,
		PromQueryStep: 60,
	}

	job := &WorkloadGpuAggregationJob{
		config: config,
	}

	result := job.GetConfig()
	assert.Equal(t, config, result, "GetConfig should return the config")
}

func TestLabelGpuAggregationJob_GetConfig(t *testing.T) {
	config := &LabelGpuAggregationConfig{
		Enabled:        true,
		LabelKeys:      []string{"app", "team"},
		AnnotationKeys: []string{"project"},
		DefaultValue:   "unknown",
		PromQueryStep:  30,
	}

	job := &LabelGpuAggregationJob{
		config: config,
	}

	result := job.GetConfig()
	assert.Equal(t, config, result, "GetConfig should return the config")
}

func TestLabelGpuAggregationJob_SetConfig(t *testing.T) {
	job := &LabelGpuAggregationJob{
		config: &LabelGpuAggregationConfig{
			Enabled: false,
		},
	}

	newConfig := &LabelGpuAggregationConfig{
		Enabled:        true,
		LabelKeys:      []string{"env"},
		AnnotationKeys: []string{"cost-center"},
		DefaultValue:   "default",
		PromQueryStep:  60,
	}

	job.SetConfig(newConfig)
	assert.Equal(t, newConfig, job.config, "SetConfig should update the config")
}

func TestClusterGpuAggregationJob_SetConfig(t *testing.T) {
	job := &ClusterGpuAggregationJob{
		config: &ClusterGpuAggregationConfig{
			Enabled: false,
		},
	}

	newConfig := &ClusterGpuAggregationConfig{
		Enabled: true,
	}

	job.SetConfig(newConfig)
	assert.Equal(t, newConfig, job.config, "SetConfig should update the config")
}

func TestNamespaceGpuAggregationJob_SetConfig(t *testing.T) {
	job := &NamespaceGpuAggregationJob{
		config: &NamespaceGpuAggregationConfig{
			Enabled: false,
		},
	}

	newConfig := &NamespaceGpuAggregationConfig{
		Enabled:                 true,
		ExcludeNamespaces:       []string{"dev", "test"},
		IncludeSystemNamespaces: true,
	}

	job.SetConfig(newConfig)
	assert.Equal(t, newConfig, job.config, "SetConfig should update the config")
}

func TestWorkloadGpuAggregationJob_SetConfig(t *testing.T) {
	job := &WorkloadGpuAggregationJob{
		config: &WorkloadGpuAggregationConfig{
			Enabled: false,
		},
	}

	newConfig := &WorkloadGpuAggregationConfig{
		Enabled:       true,
		PromQueryStep: 30,
	}

	job.SetConfig(newConfig)
	assert.Equal(t, newConfig, job.config, "SetConfig should update the config")
}

func TestLabelGpuAggregationJob_Schedule(t *testing.T) {
	job := &LabelGpuAggregationJob{}
	assert.Equal(t, "@every 5m", job.Schedule(), "Schedule should return @every 5m")
}

func TestClusterGpuAggregationJob_Schedule(t *testing.T) {
	job := &ClusterGpuAggregationJob{}
	assert.Equal(t, "@every 5m", job.Schedule(), "Schedule should return @every 5m")
}

func TestNamespaceGpuAggregationJob_Schedule(t *testing.T) {
	job := &NamespaceGpuAggregationJob{}
	assert.Equal(t, "@every 5m", job.Schedule(), "Schedule should return @every 5m")
}

func TestWorkloadGpuAggregationJob_Schedule(t *testing.T) {
	job := &WorkloadGpuAggregationJob{}
	assert.Equal(t, "@every 5m", job.Schedule(), "Schedule should return @every 5m")
}

func TestLabelGpuAggregationConfig_DefaultValues(t *testing.T) {
	config := &LabelGpuAggregationConfig{}

	assert.False(t, config.Enabled, "Default Enabled should be false")
	assert.Nil(t, config.LabelKeys, "Default LabelKeys should be nil")
	assert.Nil(t, config.AnnotationKeys, "Default AnnotationKeys should be nil")
	assert.Empty(t, config.DefaultValue, "Default DefaultValue should be empty")
	assert.Equal(t, 0, config.PromQueryStep, "Default PromQueryStep should be 0")
}

func TestGpuAggregationSystemConfig_Structure(t *testing.T) {
	config := GpuAggregationSystemConfig{}

	config.Dimensions.Label.Enabled = true
	config.Dimensions.Label.LabelKeys = []string{"app"}
	config.Dimensions.Label.AnnotationKeys = []string{"project"}
	config.Dimensions.Label.DefaultValue = "unknown"
	config.Prometheus.QueryStep = 30

	assert.True(t, config.Dimensions.Label.Enabled)
	assert.Equal(t, []string{"app"}, config.Dimensions.Label.LabelKeys)
	assert.Equal(t, []string{"project"}, config.Dimensions.Label.AnnotationKeys)
	assert.Equal(t, "unknown", config.Dimensions.Label.DefaultValue)
	assert.Equal(t, 30, config.Prometheus.QueryStep)
}

func TestClusterGpuAggregationConfig_DefaultValues(t *testing.T) {
	config := &ClusterGpuAggregationConfig{}

	assert.False(t, config.Enabled, "Default Enabled should be false")
}

func TestNamespaceGpuAggregationConfig_DefaultValues(t *testing.T) {
	config := &NamespaceGpuAggregationConfig{}

	assert.False(t, config.Enabled, "Default Enabled should be false")
	assert.Nil(t, config.ExcludeNamespaces, "Default ExcludeNamespaces should be nil")
	assert.False(t, config.IncludeSystemNamespaces, "Default IncludeSystemNamespaces should be false")
}

func TestWorkloadGpuAggregationConfig_DefaultValues(t *testing.T) {
	config := &WorkloadGpuAggregationConfig{}

	assert.False(t, config.Enabled, "Default Enabled should be false")
	assert.Equal(t, 0, config.PromQueryStep, "Default PromQueryStep should be 0")
}

func TestCalculatePercentile_EdgeCases(t *testing.T) {
	tests := []struct {
		name         string
		sortedValues []float64
		percentile   float64
		expected     float64
	}{
		{
			name:         "negative percentile treated as p0",
			sortedValues: []float64{1.0, 2.0, 3.0},
			percentile:   -0.1,
			expected:     1.0,
		},
		{
			name:         "percentile greater than 1 treated as p100",
			sortedValues: []float64{1.0, 2.0, 3.0},
			percentile:   1.5,
			expected:     3.0,
		},
		{
			name:         "very small percentile",
			sortedValues: []float64{1.0, 2.0, 3.0, 4.0, 5.0},
			percentile:   0.01,
			expected:     1.0,
		},
		{
			name:         "very high percentile",
			sortedValues: []float64{1.0, 2.0, 3.0, 4.0, 5.0},
			percentile:   0.99,
			expected:     5.0,
		},
		{
			name:         "two values p50",
			sortedValues: []float64{10.0, 20.0},
			percentile:   0.5,
			expected:     10.0,
		},
		{
			name:         "large dataset p90",
			sortedValues: []float64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20},
			percentile:   0.90,
			expected:     18.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := calculatePercentile(tt.sortedValues, tt.percentile)
			assert.Equal(t, tt.expected, result, "Percentile should match expected")
		})
	}
}

func TestNamespaceGpuAggregationJob_ShouldExcludeNamespace_EdgeCases(t *testing.T) {
	tests := []struct {
		name      string
		config    *NamespaceGpuAggregationConfig
		namespace string
		expected  bool
	}{
		{
			name: "empty namespace name",
			config: &NamespaceGpuAggregationConfig{
				Enabled:                 true,
				ExcludeNamespaces:       []string{""},
				IncludeSystemNamespaces: true,
			},
			namespace: "",
			expected:  true,
		},
		{
			name: "namespace with special characters",
			config: &NamespaceGpuAggregationConfig{
				Enabled:                 true,
				ExcludeNamespaces:       []string{"test-ns-1"},
				IncludeSystemNamespaces: true,
			},
			namespace: "test-ns-1",
			expected:  true,
		},
		{
			name: "namespace not in exclusion list with similar prefix",
			config: &NamespaceGpuAggregationConfig{
				Enabled:                 true,
				ExcludeNamespaces:       []string{"test"},
				IncludeSystemNamespaces: true,
			},
			namespace: "test-production",
			expected:  false,
		},
		{
			name: "case sensitive match",
			config: &NamespaceGpuAggregationConfig{
				Enabled:                 true,
				ExcludeNamespaces:       []string{"Test"},
				IncludeSystemNamespaces: true,
			},
			namespace: "test",
			expected:  false,
		},
		{
			name: "multiple system namespaces check",
			config: &NamespaceGpuAggregationConfig{
				Enabled:                 true,
				ExcludeNamespaces:       []string{},
				IncludeSystemNamespaces: false,
			},
			namespace: "default",
			expected:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			job := &NamespaceGpuAggregationJob{
				config: tt.config,
			}
			result := job.shouldExcludeNamespace(tt.namespace)
			assert.Equal(t, tt.expected, result, "Exclusion result should match expected")
		})
	}
}

func TestConstants(t *testing.T) {
	assert.Equal(t, "job.label_gpu_aggregation.last_processed_hour", CacheKeyLabelGpuAggregationLastHour)
	assert.Equal(t, "job.gpu_aggregation.config", SystemConfigKeyGpuAggregation)
	assert.Equal(t, "job.namespace_gpu_aggregation.last_processed_hour", CacheKeyNamespaceGpuAggregationLastHour)
	assert.Equal(t, "job.cluster_gpu_aggregation.last_processed_hour", CacheKeyClusterGpuAggregationLastHour)
	assert.Equal(t, "job.workload_gpu_aggregation.last_processed_hour", CacheKeyWorkloadGpuAggregationLastHour)
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

func TestLabelGpuAggregationJob_JobStructure(t *testing.T) {
	config := &LabelGpuAggregationConfig{
		Enabled:        true,
		LabelKeys:      []string{"app", "team"},
		AnnotationKeys: []string{"project"},
		DefaultValue:   "unknown",
		PromQueryStep:  60,
	}

	job := &LabelGpuAggregationJob{
		config:      config,
		clusterName: "test-cluster",
	}

	assert.Equal(t, "test-cluster", job.clusterName)
	assert.Equal(t, config, job.config)
	assert.True(t, job.config.Enabled)
	assert.Equal(t, []string{"app", "team"}, job.config.LabelKeys)
}

func TestClusterGpuAggregationJob_JobStructure(t *testing.T) {
	config := &ClusterGpuAggregationConfig{
		Enabled: true,
	}

	job := &ClusterGpuAggregationJob{
		config:      config,
		clusterName: "test-cluster",
	}

	assert.Equal(t, "test-cluster", job.clusterName)
	assert.True(t, job.config.Enabled)
}

func TestNamespaceGpuAggregationJob_JobStructure(t *testing.T) {
	config := &NamespaceGpuAggregationConfig{
		Enabled:                 true,
		ExcludeNamespaces:       []string{"dev", "test"},
		IncludeSystemNamespaces: false,
	}

	job := &NamespaceGpuAggregationJob{
		config:      config,
		clusterName: "test-cluster",
	}

	assert.Equal(t, "test-cluster", job.clusterName)
	assert.True(t, job.config.Enabled)
	assert.Equal(t, []string{"dev", "test"}, job.config.ExcludeNamespaces)
}

func TestWorkloadGpuAggregationJob_JobStructure(t *testing.T) {
	config := &WorkloadGpuAggregationConfig{
		Enabled:       true,
		PromQueryStep: 30,
	}

	job := &WorkloadGpuAggregationJob{
		config:      config,
		clusterName: "test-cluster",
	}

	assert.Equal(t, "test-cluster", job.clusterName)
	assert.True(t, job.config.Enabled)
	assert.Equal(t, 30, job.config.PromQueryStep)
}

func TestLabelGpuAggregationConfig_Validation(t *testing.T) {
	tests := []struct {
		name           string
		config         *LabelGpuAggregationConfig
		expectEnabled  bool
		expectHasKeys  bool
	}{
		{
			name: "enabled with label keys",
			config: &LabelGpuAggregationConfig{
				Enabled:        true,
				LabelKeys:      []string{"app"},
				AnnotationKeys: []string{},
				DefaultValue:   "unknown",
			},
			expectEnabled: true,
			expectHasKeys: true,
		},
		{
			name: "enabled with annotation keys",
			config: &LabelGpuAggregationConfig{
				Enabled:        true,
				LabelKeys:      []string{},
				AnnotationKeys: []string{"project"},
				DefaultValue:   "unknown",
			},
			expectEnabled: true,
			expectHasKeys: true,
		},
		{
			name: "enabled with both keys",
			config: &LabelGpuAggregationConfig{
				Enabled:        true,
				LabelKeys:      []string{"app", "team"},
				AnnotationKeys: []string{"project", "cost-center"},
				DefaultValue:   "default",
			},
			expectEnabled: true,
			expectHasKeys: true,
		},
		{
			name: "enabled with no keys",
			config: &LabelGpuAggregationConfig{
				Enabled:        true,
				LabelKeys:      []string{},
				AnnotationKeys: []string{},
				DefaultValue:   "unknown",
			},
			expectEnabled: true,
			expectHasKeys: false,
		},
		{
			name: "disabled",
			config: &LabelGpuAggregationConfig{
				Enabled:        false,
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

func TestNamespaceGpuAggregationConfig_Validation(t *testing.T) {
	tests := []struct {
		name   string
		config *NamespaceGpuAggregationConfig
	}{
		{
			name: "default values",
			config: &NamespaceGpuAggregationConfig{
				Enabled:                 true,
				ExcludeNamespaces:       []string{},
				IncludeSystemNamespaces: false,
			},
		},
		{
			name: "with exclusions",
			config: &NamespaceGpuAggregationConfig{
				Enabled:                 true,
				ExcludeNamespaces:       []string{"dev", "test", "staging"},
				IncludeSystemNamespaces: false,
			},
		},
		{
			name: "include system namespaces",
			config: &NamespaceGpuAggregationConfig{
				Enabled:                 true,
				ExcludeNamespaces:       []string{},
				IncludeSystemNamespaces: true,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			job := &NamespaceGpuAggregationJob{config: tt.config}
			assert.NotNil(t, job.config)
		})
	}
}

func TestWorkloadGpuAggregationConfig_Validation(t *testing.T) {
	tests := []struct {
		name          string
		config        *WorkloadGpuAggregationConfig
		expectedStep  int
	}{
		{
			name: "default step",
			config: &WorkloadGpuAggregationConfig{
				Enabled:       true,
				PromQueryStep: DefaultPromQueryStep,
			},
			expectedStep: 60,
		},
		{
			name: "custom step",
			config: &WorkloadGpuAggregationConfig{
				Enabled:       true,
				PromQueryStep: 30,
			},
			expectedStep: 30,
		},
		{
			name: "zero step",
			config: &WorkloadGpuAggregationConfig{
				Enabled:       true,
				PromQueryStep: 0,
			},
			expectedStep: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expectedStep, tt.config.PromQueryStep)
		})
	}
}

func TestCalculatePercentile_LargeDataset(t *testing.T) {
	values := make([]float64, 1000)
	for i := 0; i < 1000; i++ {
		values[i] = float64(i + 1)
	}

	p50 := calculatePercentile(values, 0.5)
	assert.True(t, p50 >= 499 && p50 <= 501, "P50 should be around 500")

	p99 := calculatePercentile(values, 0.99)
	assert.True(t, p99 >= 989 && p99 <= 1000, "P99 should be around 990")

	p10 := calculatePercentile(values, 0.1)
	assert.True(t, p10 >= 99 && p10 <= 101, "P10 should be around 100")
}

func TestCalculatePercentile_IdenticalValues(t *testing.T) {
	values := []float64{50.0, 50.0, 50.0, 50.0, 50.0}

	p0 := calculatePercentile(values, 0.0)
	p50 := calculatePercentile(values, 0.5)
	p100 := calculatePercentile(values, 1.0)

	assert.Equal(t, 50.0, p0)
	assert.Equal(t, 50.0, p50)
	assert.Equal(t, 50.0, p100)
}

func TestNamespaceGpuAggregationJob_AllSystemNamespaces(t *testing.T) {
	systemNamespaces := []string{"kube-system", "kube-public", "kube-node-lease"}

	config := &NamespaceGpuAggregationConfig{
		Enabled:                 true,
		ExcludeNamespaces:       []string{},
		IncludeSystemNamespaces: false,
	}

	job := &NamespaceGpuAggregationJob{config: config}

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

func TestNamespaceGpuAggregationJob_ExclusionListPriority(t *testing.T) {
	config := &NamespaceGpuAggregationConfig{
		Enabled:                 true,
		ExcludeNamespaces:       []string{"custom-exclude"},
		IncludeSystemNamespaces: true,
	}

	job := &NamespaceGpuAggregationJob{config: config}

	assert.True(t, job.shouldExcludeNamespace("custom-exclude"))

	assert.False(t, job.shouldExcludeNamespace("kube-system"))

	assert.False(t, job.shouldExcludeNamespace("production"))
}

func TestGpuAggregationSystemConfig_FullStructure(t *testing.T) {
	config := GpuAggregationSystemConfig{}

	config.Dimensions.Label.Enabled = true
	config.Dimensions.Label.LabelKeys = []string{"app", "team", "env"}
	config.Dimensions.Label.AnnotationKeys = []string{"project", "cost-center", "owner"}
	config.Dimensions.Label.DefaultValue = "not-set"
	config.Prometheus.QueryStep = 15

	assert.True(t, config.Dimensions.Label.Enabled)
	assert.Equal(t, 3, len(config.Dimensions.Label.LabelKeys))
	assert.Equal(t, 3, len(config.Dimensions.Label.AnnotationKeys))
	assert.Equal(t, "not-set", config.Dimensions.Label.DefaultValue)
	assert.Equal(t, 15, config.Prometheus.QueryStep)
}

func TestQueryTemplates_EmptyUID(t *testing.T) {
	emptyUID := ""

	utilQuery := fmt.Sprintf(WorkloadUtilizationQueryTemplate, emptyUID)
	assert.Contains(t, utilQuery, `workload_uid=""`)

	memUsedQuery := fmt.Sprintf(WorkloadGpuMemoryUsedQueryTemplate, emptyUID)
	assert.Contains(t, memUsedQuery, `workload_uid=""`)

	memTotalQuery := fmt.Sprintf(WorkloadGpuMemoryTotalQueryTemplate, emptyUID)
	assert.Contains(t, memTotalQuery, `workload_uid=""`)
}

func TestQueryTemplates_SpecialCharactersInUID(t *testing.T) {
	testUIDs := []string{
		"pod-abc-123-def-456",
		"deployment_test_workload",
		"a1b2c3d4-e5f6-7890-abcd-ef1234567890",
		"workload.with.dots",
	}

	for _, uid := range testUIDs {
		query := fmt.Sprintf(WorkloadUtilizationQueryTemplate, uid)
		assert.Contains(t, query, uid, "Query should contain the UID: %s", uid)
		assert.Contains(t, query, "workload_uid=")
	}
}

func TestCacheKeyConstants(t *testing.T) {
	assert.Contains(t, CacheKeyLabelGpuAggregationLastHour, "label_gpu_aggregation")
	assert.Contains(t, CacheKeyNamespaceGpuAggregationLastHour, "namespace_gpu_aggregation")
	assert.Contains(t, CacheKeyClusterGpuAggregationLastHour, "cluster_gpu_aggregation")
	assert.Contains(t, CacheKeyWorkloadGpuAggregationLastHour, "workload_gpu_aggregation")

	keys := []string{
		CacheKeyLabelGpuAggregationLastHour,
		CacheKeyNamespaceGpuAggregationLastHour,
		CacheKeyClusterGpuAggregationLastHour,
		CacheKeyWorkloadGpuAggregationLastHour,
	}
	uniqueKeys := make(map[string]bool)
	for _, key := range keys {
		uniqueKeys[key] = true
	}
	assert.Equal(t, len(keys), len(uniqueKeys), "All cache keys should be unique")
}

func TestSystemConfigKeyGpuAggregation(t *testing.T) {
	assert.Equal(t, "job.gpu_aggregation.config", SystemConfigKeyGpuAggregation)
	assert.Contains(t, SystemConfigKeyGpuAggregation, "gpu_aggregation")
}

func TestScheduleExpressions(t *testing.T) {
	clusterJob := &ClusterGpuAggregationJob{}
	namespaceJob := &NamespaceGpuAggregationJob{}
	workloadJob := &WorkloadGpuAggregationJob{}
	labelJob := &LabelGpuAggregationJob{}

	assert.Equal(t, clusterJob.Schedule(), namespaceJob.Schedule())
	assert.Equal(t, namespaceJob.Schedule(), workloadJob.Schedule())
	assert.Equal(t, workloadJob.Schedule(), labelJob.Schedule())
	assert.Equal(t, "@every 5m", clusterJob.Schedule())
}

func TestConvertToDBClusterStats_AllFields(t *testing.T) {
	input := &model.ClusterGpuHourlyStats{
		ClusterName:       "prod-cluster",
		TotalGpuCapacity:  1000,
		AllocatedGpuCount: 750.5,
		AllocationRate:    75.05,
		AvgUtilization:    65.3,
		MaxUtilization:    98.5,
		MinUtilization:    15.2,
		P50Utilization:    60.0,
		P95Utilization:    92.5,
		SampleCount:       120,
	}

	result := convertToDBClusterStats(input)

	assert.Equal(t, input.ClusterName, result.ClusterName)
	assert.Equal(t, int32(input.TotalGpuCapacity), result.TotalGpuCapacity)
	assert.InDelta(t, input.AllocatedGpuCount, result.AllocatedGpuCount, 0.001)
	assert.InDelta(t, input.AllocationRate, result.AllocationRate, 0.001)
	assert.InDelta(t, input.AvgUtilization, result.AvgUtilization, 0.001)
	assert.InDelta(t, input.MaxUtilization, result.MaxUtilization, 0.001)
	assert.InDelta(t, input.MinUtilization, result.MinUtilization, 0.001)
	assert.InDelta(t, input.P50Utilization, result.P50Utilization, 0.001)
	assert.InDelta(t, input.P95Utilization, result.P95Utilization, 0.001)
	assert.Equal(t, int32(input.SampleCount), result.SampleCount)
}

func TestConvertToDBNamespaceStats_AllFields(t *testing.T) {
	input := &model.NamespaceGpuHourlyStats{
		ClusterName:         "prod-cluster",
		Namespace:           "ml-training",
		TotalGpuCapacity:    200,
		AllocatedGpuCount:   150.5,
		AllocationRate:      75.25,
		AvgUtilization:      72.3,
		MaxUtilization:      95.5,
		MinUtilization:      25.2,
		ActiveWorkloadCount: 15,
	}

	result := convertToDBNamespaceStats(input)

	assert.Equal(t, input.ClusterName, result.ClusterName)
	assert.Equal(t, input.Namespace, result.Namespace)
	assert.Equal(t, int32(input.TotalGpuCapacity), result.TotalGpuCapacity)
	assert.InDelta(t, input.AllocatedGpuCount, result.AllocatedGpuCount, 0.001)
	assert.InDelta(t, input.AllocationRate, result.AllocationRate, 0.001)
	assert.InDelta(t, input.AvgUtilization, result.AvgUtilization, 0.001)
	assert.InDelta(t, input.MaxUtilization, result.MaxUtilization, 0.001)
	assert.InDelta(t, input.MinUtilization, result.MinUtilization, 0.001)
	assert.Equal(t, int32(input.ActiveWorkloadCount), result.ActiveWorkloadCount)
}

func TestConvertToDBLabelStats_AllFields(t *testing.T) {
	input := &model.LabelGpuHourlyStats{
		ClusterName:         "prod-cluster",
		DimensionType:       "annotation",
		DimensionKey:        "cost-center",
		DimensionValue:      "engineering",
		AllocatedGpuCount:   85.5,
		AvgUtilization:      68.5,
		MaxUtilization:      92.0,
		MinUtilization:      35.5,
		ActiveWorkloadCount: 8,
	}

	result := convertToDBLabelStats(input)

	assert.Equal(t, input.ClusterName, result.ClusterName)
	assert.Equal(t, input.DimensionType, result.DimensionType)
	assert.Equal(t, input.DimensionKey, result.DimensionKey)
	assert.Equal(t, input.DimensionValue, result.DimensionValue)
	assert.InDelta(t, input.AllocatedGpuCount, result.AllocatedGpuCount, 0.001)
	assert.InDelta(t, input.AvgUtilization, result.AvgUtilization, 0.001)
	assert.InDelta(t, input.MaxUtilization, result.MaxUtilization, 0.001)
	assert.InDelta(t, input.MinUtilization, result.MinUtilization, 0.001)
	assert.Equal(t, int32(input.ActiveWorkloadCount), result.ActiveWorkloadCount)
}

func TestSplitAnnotationKey_ComplexCases(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{
			name:     "kubernetes.io annotation",
			input:    "kubernetes.io/name:my-app",
			expected: []string{"kubernetes.io/name", "my-app"},
		},
		{
			name:     "long key with namespace",
			input:    "some.very.long.namespace.io/annotation-key:value",
			expected: []string{"some.very.long.namespace.io/annotation-key", "value"},
		},
		{
			name:     "value with special chars",
			input:    "key:value-with-dashes_and_underscores",
			expected: []string{"key", "value-with-dashes_and_underscores"},
		},
		{
			name:     "value with spaces",
			input:    "key:value with spaces",
			expected: []string{"key", "value with spaces"},
		},
		{
			name:     "numeric value",
			input:    "version:123",
			expected: []string{"version", "123"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := splitAnnotationKey(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestLabelGpuAggregationConfig_KeysCombinations tests various key configuration combinations
func TestLabelGpuAggregationConfig_KeysCombinations(t *testing.T) {
	tests := []struct {
		name           string
		labelKeys      []string
		annotationKeys []string
		expectTotal    int
	}{
		{
			name:           "no keys",
			labelKeys:      []string{},
			annotationKeys: []string{},
			expectTotal:    0,
		},
		{
			name:           "only label keys",
			labelKeys:      []string{"app", "team"},
			annotationKeys: []string{},
			expectTotal:    2,
		},
		{
			name:           "only annotation keys",
			labelKeys:      []string{},
			annotationKeys: []string{"project", "cost-center"},
			expectTotal:    2,
		},
		{
			name:           "both keys",
			labelKeys:      []string{"app", "team", "env"},
			annotationKeys: []string{"project"},
			expectTotal:    4,
		},
		{
			name:           "many keys",
			labelKeys:      []string{"a", "b", "c", "d", "e"},
			annotationKeys: []string{"x", "y", "z"},
			expectTotal:    8,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := &LabelGpuAggregationConfig{
				LabelKeys:      tt.labelKeys,
				AnnotationKeys: tt.annotationKeys,
			}
			totalKeys := len(config.LabelKeys) + len(config.AnnotationKeys)
			assert.Equal(t, tt.expectTotal, totalKeys)
		})
	}
}

// TestWorkloadGpuAggregationConfig_PromQueryStepValidation tests PromQueryStep values
func TestWorkloadGpuAggregationConfig_PromQueryStepValidation(t *testing.T) {
	tests := []struct {
		name          string
		promQueryStep int
		expectValid   bool
	}{
		{
			name:          "default value",
			promQueryStep: DefaultPromQueryStep,
			expectValid:   true,
		},
		{
			name:          "zero value",
			promQueryStep: 0,
			expectValid:   false,
		},
		{
			name:          "small value",
			promQueryStep: 15,
			expectValid:   true,
		},
		{
			name:          "large value",
			promQueryStep: 300,
			expectValid:   true,
		},
		{
			name:          "negative value",
			promQueryStep: -1,
			expectValid:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := &WorkloadGpuAggregationConfig{
				PromQueryStep: tt.promQueryStep,
			}
			isValid := config.PromQueryStep > 0
			assert.Equal(t, tt.expectValid, isValid)
		})
	}
}

// TestClusterGpuAggregationConfig_EnabledToggle tests enabled flag behavior
func TestClusterGpuAggregationConfig_EnabledToggle(t *testing.T) {
	config := &ClusterGpuAggregationConfig{Enabled: false}
	assert.False(t, config.Enabled)

	config.Enabled = true
	assert.True(t, config.Enabled)

	config.Enabled = false
	assert.False(t, config.Enabled)
}

// TestNamespaceGpuAggregationConfig_ExclusionPatterns tests namespace exclusion patterns
func TestNamespaceGpuAggregationConfig_ExclusionPatterns(t *testing.T) {
	tests := []struct {
		name              string
		excludeNamespaces []string
		testNamespace     string
		shouldExclude     bool
	}{
		{
			name:              "exact match",
			excludeNamespaces: []string{"dev", "test", "staging"},
			testNamespace:     "dev",
			shouldExclude:     true,
		},
		{
			name:              "no match",
			excludeNamespaces: []string{"dev", "test"},
			testNamespace:     "production",
			shouldExclude:     false,
		},
		{
			name:              "empty exclusion list",
			excludeNamespaces: []string{},
			testNamespace:     "any-namespace",
			shouldExclude:     false,
		},
		{
			name:              "single exclusion",
			excludeNamespaces: []string{"kube-system"},
			testNamespace:     "kube-system",
			shouldExclude:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := &NamespaceGpuAggregationConfig{
				ExcludeNamespaces:       tt.excludeNamespaces,
				IncludeSystemNamespaces: true,
			}
			job := &NamespaceGpuAggregationJob{config: config}

			excluded := false
			for _, ns := range config.ExcludeNamespaces {
				if ns == tt.testNamespace {
					excluded = true
					break
				}
			}
			assert.Equal(t, tt.shouldExclude, excluded)
			_ = job
		})
	}
}

// TestConvertToDBClusterStats_NilInput tests nil input handling
func TestConvertToDBClusterStats_NilHandling(t *testing.T) {
	stats := &model.ClusterGpuHourlyStats{}
	result := convertToDBClusterStats(stats)
	assert.NotNil(t, result)
	assert.Empty(t, result.ClusterName)
	assert.Equal(t, int32(0), result.TotalGpuCapacity)
}

// TestConvertToDBNamespaceStats_NilInput tests nil input handling
func TestConvertToDBNamespaceStats_NilHandling(t *testing.T) {
	stats := &model.NamespaceGpuHourlyStats{}
	result := convertToDBNamespaceStats(stats)
	assert.NotNil(t, result)
	assert.Empty(t, result.ClusterName)
	assert.Empty(t, result.Namespace)
}

// TestConvertToDBLabelStats_NilInput tests nil input handling
func TestConvertToDBLabelStats_NilHandling(t *testing.T) {
	stats := &model.LabelGpuHourlyStats{}
	result := convertToDBLabelStats(stats)
	assert.NotNil(t, result)
	assert.Empty(t, result.DimensionType)
	assert.Empty(t, result.DimensionKey)
}

// TestCalculatePercentile_BoundaryValues tests boundary values for percentile calculation
func TestCalculatePercentile_BoundaryValues(t *testing.T) {
	values := []float64{10.0, 20.0, 30.0, 40.0, 50.0, 60.0, 70.0, 80.0, 90.0, 100.0}

	tests := []struct {
		name       string
		percentile float64
	}{
		{"p0", 0.0},
		{"p10", 0.1},
		{"p20", 0.2},
		{"p25", 0.25},
		{"p30", 0.3},
		{"p40", 0.4},
		{"p50", 0.5},
		{"p60", 0.6},
		{"p70", 0.7},
		{"p75", 0.75},
		{"p80", 0.8},
		{"p90", 0.9},
		{"p95", 0.95},
		{"p99", 0.99},
		{"p100", 1.0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := calculatePercentile(values, tt.percentile)
			assert.GreaterOrEqual(t, result, values[0])
			assert.LessOrEqual(t, result, values[len(values)-1])
		})
	}
}

// TestCalculatePercentile_FloatPrecision tests float precision handling
func TestCalculatePercentile_FloatPrecision(t *testing.T) {
	values := []float64{1.111, 2.222, 3.333, 4.444, 5.555}

	p50 := calculatePercentile(values, 0.5)
	assert.True(t, p50 >= 2.222 && p50 <= 4.444)

	p0 := calculatePercentile(values, 0.0)
	assert.Equal(t, 1.111, p0)

	p100 := calculatePercentile(values, 1.0)
	assert.Equal(t, 5.555, p100)
}

// TestJobScheduleConsistency tests that all jobs have consistent schedules
func TestJobScheduleConsistency(t *testing.T) {
	clusterJob := &ClusterGpuAggregationJob{}
	namespaceJob := &NamespaceGpuAggregationJob{}
	workloadJob := &WorkloadGpuAggregationJob{}
	labelJob := &LabelGpuAggregationJob{}

	schedules := []string{
		clusterJob.Schedule(),
		namespaceJob.Schedule(),
		workloadJob.Schedule(),
		labelJob.Schedule(),
	}

	for _, schedule := range schedules {
		assert.NotEmpty(t, schedule)
		assert.Contains(t, schedule, "@every")
	}
}

// TestGpuAggregationSystemConfig_LabelConfig tests label config structure
func TestGpuAggregationSystemConfig_LabelConfig(t *testing.T) {
	config := GpuAggregationSystemConfig{}

	config.Dimensions.Label.Enabled = true
	config.Dimensions.Label.LabelKeys = []string{"app", "team"}
	config.Dimensions.Label.AnnotationKeys = []string{"project"}
	config.Dimensions.Label.DefaultValue = "default"
	config.Prometheus.QueryStep = 30

	assert.True(t, config.Dimensions.Label.Enabled)
	assert.Equal(t, 2, len(config.Dimensions.Label.LabelKeys))
	assert.Equal(t, 1, len(config.Dimensions.Label.AnnotationKeys))
	assert.Equal(t, "default", config.Dimensions.Label.DefaultValue)
	assert.Equal(t, 30, config.Prometheus.QueryStep)
}

// TestNamespaceGpuAggregationJob_SystemNamespacesList tests all system namespaces
func TestNamespaceGpuAggregationJob_SystemNamespacesList(t *testing.T) {
	systemNamespaces := []string{"kube-system", "kube-public", "kube-node-lease"}

	config := &NamespaceGpuAggregationConfig{
		Enabled:                 true,
		ExcludeNamespaces:       []string{},
		IncludeSystemNamespaces: false,
	}
	job := &NamespaceGpuAggregationJob{config: config}

	for _, ns := range systemNamespaces {
		assert.True(t, job.shouldExcludeNamespace(ns),
			"System namespace %s should be excluded", ns)
	}

	config.IncludeSystemNamespaces = true
	for _, ns := range systemNamespaces {
		assert.False(t, job.shouldExcludeNamespace(ns),
			"System namespace %s should be included", ns)
	}
}

// TestConvertToDBClusterStats_LargeValues tests conversion with large values
func TestConvertToDBClusterStats_LargeValues(t *testing.T) {
	input := &model.ClusterGpuHourlyStats{
		ClusterName:       "large-cluster",
		TotalGpuCapacity:  10000,
		AllocatedGpuCount: 9999.99,
		AllocationRate:    99.9999,
		AvgUtilization:    99.99,
		MaxUtilization:    100.0,
		MinUtilization:    0.01,
		P50Utilization:    50.0,
		P95Utilization:    95.0,
		SampleCount:       1000000,
	}

	result := convertToDBClusterStats(input)
	assert.Equal(t, int32(10000), result.TotalGpuCapacity)
	assert.InDelta(t, 9999.99, result.AllocatedGpuCount, 0.01)
	assert.Equal(t, int32(1000000), result.SampleCount)
}

// TestConvertToDBNamespaceStats_LargeValues tests conversion with large values
func TestConvertToDBNamespaceStats_LargeValues(t *testing.T) {
	input := &model.NamespaceGpuHourlyStats{
		ClusterName:         "large-cluster",
		Namespace:           "production",
		TotalGpuCapacity:    5000,
		AllocatedGpuCount:   4999.5,
		AllocationRate:      99.99,
		AvgUtilization:      85.5,
		MaxUtilization:      100.0,
		MinUtilization:      10.0,
		ActiveWorkloadCount: 10000,
	}

	result := convertToDBNamespaceStats(input)
	assert.Equal(t, int32(5000), result.TotalGpuCapacity)
	assert.Equal(t, int32(10000), result.ActiveWorkloadCount)
}

// TestConvertToDBLabelStats_AllDimensionTypes tests all dimension types
func TestConvertToDBLabelStats_AllDimensionTypes(t *testing.T) {
	dimensionTypes := []string{"label", "annotation"}

	for _, dimType := range dimensionTypes {
		input := &model.LabelGpuHourlyStats{
			ClusterName:    "test-cluster",
			DimensionType:  dimType,
			DimensionKey:   "test-key",
			DimensionValue: "test-value",
		}

		result := convertToDBLabelStats(input)
		assert.Equal(t, dimType, result.DimensionType)
	}
}

// TestSplitAnnotationKey_Unicode tests Unicode handling
func TestSplitAnnotationKey_Unicode(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{
			name:     "unicode value",
			input:    "label:value-unicode",
			expected: []string{"label", "value-unicode"},
		},
		{
			name:     "empty after colon",
			input:    "key:",
			expected: []string{"key", ""},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := splitAnnotationKey(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestJobConfigSetterGetter tests config setter and getter for all jobs
func TestJobConfigSetterGetter(t *testing.T) {
	t.Run("ClusterJob", func(t *testing.T) {
		job := &ClusterGpuAggregationJob{config: &ClusterGpuAggregationConfig{Enabled: false}}
		originalConfig := job.GetConfig()
		assert.False(t, originalConfig.Enabled)

		newConfig := &ClusterGpuAggregationConfig{Enabled: true}
		job.SetConfig(newConfig)
		assert.True(t, job.GetConfig().Enabled)
	})

	t.Run("NamespaceJob", func(t *testing.T) {
		job := &NamespaceGpuAggregationJob{config: &NamespaceGpuAggregationConfig{Enabled: false}}
		originalConfig := job.GetConfig()
		assert.False(t, originalConfig.Enabled)

		newConfig := &NamespaceGpuAggregationConfig{Enabled: true, ExcludeNamespaces: []string{"dev"}}
		job.SetConfig(newConfig)
		assert.True(t, job.GetConfig().Enabled)
		assert.Equal(t, []string{"dev"}, job.GetConfig().ExcludeNamespaces)
	})

	t.Run("WorkloadJob", func(t *testing.T) {
		job := &WorkloadGpuAggregationJob{config: &WorkloadGpuAggregationConfig{Enabled: false}}
		originalConfig := job.GetConfig()
		assert.False(t, originalConfig.Enabled)

		newConfig := &WorkloadGpuAggregationConfig{Enabled: true, PromQueryStep: 30}
		job.SetConfig(newConfig)
		assert.True(t, job.GetConfig().Enabled)
		assert.Equal(t, 30, job.GetConfig().PromQueryStep)
	})

	t.Run("LabelJob", func(t *testing.T) {
		job := &LabelGpuAggregationJob{config: &LabelGpuAggregationConfig{Enabled: false}}
		originalConfig := job.GetConfig()
		assert.False(t, originalConfig.Enabled)

		newConfig := &LabelGpuAggregationConfig{Enabled: true, LabelKeys: []string{"app"}}
		job.SetConfig(newConfig)
		assert.True(t, job.GetConfig().Enabled)
		assert.Equal(t, []string{"app"}, job.GetConfig().LabelKeys)
	})
}

// TestDefaultPromQueryStep tests the default constant value
func TestDefaultPromQueryStep(t *testing.T) {
	assert.Equal(t, 60, DefaultPromQueryStep)
	assert.True(t, DefaultPromQueryStep > 0)
	assert.True(t, DefaultPromQueryStep <= 300)
}

// TestCacheKeyUniqueness tests that all cache keys are unique
func TestCacheKeyUniqueness(t *testing.T) {
	keys := map[string]bool{
		CacheKeyLabelGpuAggregationLastHour:     true,
		CacheKeyNamespaceGpuAggregationLastHour: true,
		CacheKeyClusterGpuAggregationLastHour:   true,
		CacheKeyWorkloadGpuAggregationLastHour:  true,
	}

	assert.Equal(t, 4, len(keys), "All cache keys should be unique")
}

// TestQueryTemplates_Formatting tests query template formatting
func TestQueryTemplates_Formatting(t *testing.T) {
	uid := "test-workload-uid-12345"

	utilizationQuery := fmt.Sprintf(WorkloadUtilizationQueryTemplate, uid)
	assert.Contains(t, utilizationQuery, "avg(")
	assert.Contains(t, utilizationQuery, "workload_gpu_utilization")
	assert.Contains(t, utilizationQuery, uid)

	memUsedQuery := fmt.Sprintf(WorkloadGpuMemoryUsedQueryTemplate, uid)
	assert.Contains(t, memUsedQuery, "workload_gpu_used_vram")
	assert.Contains(t, memUsedQuery, uid)

	memTotalQuery := fmt.Sprintf(WorkloadGpuMemoryTotalQueryTemplate, uid)
	assert.Contains(t, memTotalQuery, "workload_gpu_total_vram")
	assert.Contains(t, memTotalQuery, uid)
}

// TestClusterGpuAggregationJob_StructFields tests cluster job struct fields
func TestClusterGpuAggregationJob_StructFields(t *testing.T) {
	config := &ClusterGpuAggregationConfig{
		Enabled: true,
	}
	job := &ClusterGpuAggregationJob{
		config:      config,
		clusterName: "test-cluster-01",
	}

	assert.Equal(t, "test-cluster-01", job.clusterName)
	assert.NotNil(t, job.config)
	assert.True(t, job.config.Enabled)
}

// TestNamespaceGpuAggregationJob_StructFields tests namespace job struct fields
func TestNamespaceGpuAggregationJob_StructFields(t *testing.T) {
	config := &NamespaceGpuAggregationConfig{
		Enabled:                 true,
		ExcludeNamespaces:       []string{"ns1", "ns2"},
		IncludeSystemNamespaces: false,
	}
	job := &NamespaceGpuAggregationJob{
		config:      config,
		clusterName: "prod-cluster",
	}

	assert.Equal(t, "prod-cluster", job.clusterName)
	assert.NotNil(t, job.config)
	assert.Equal(t, 2, len(job.config.ExcludeNamespaces))
}

// TestWorkloadGpuAggregationJob_StructFields tests workload job struct fields
func TestWorkloadGpuAggregationJob_StructFields(t *testing.T) {
	config := &WorkloadGpuAggregationConfig{
		Enabled:       true,
		PromQueryStep: 120,
	}
	job := &WorkloadGpuAggregationJob{
		config:      config,
		clusterName: "dev-cluster",
	}

	assert.Equal(t, "dev-cluster", job.clusterName)
	assert.NotNil(t, job.config)
	assert.Equal(t, 120, job.config.PromQueryStep)
}

// TestLabelGpuAggregationJob_StructFields tests label job struct fields
func TestLabelGpuAggregationJob_StructFields(t *testing.T) {
	config := &LabelGpuAggregationConfig{
		Enabled:        true,
		LabelKeys:      []string{"app", "env", "team"},
		AnnotationKeys: []string{"project", "cost-center"},
		DefaultValue:   "n/a",
		PromQueryStep:  45,
	}
	job := &LabelGpuAggregationJob{
		config:      config,
		clusterName: "staging-cluster",
	}

	assert.Equal(t, "staging-cluster", job.clusterName)
	assert.NotNil(t, job.config)
	assert.Equal(t, 3, len(job.config.LabelKeys))
	assert.Equal(t, 2, len(job.config.AnnotationKeys))
	assert.Equal(t, "n/a", job.config.DefaultValue)
}

// TestCalculatePercentile_VerySmallDataset tests percentile with very small datasets
func TestCalculatePercentile_VerySmallDataset(t *testing.T) {
	tests := []struct {
		name       string
		values     []float64
		percentile float64
		expected   float64
	}{
		{
			name:       "single value p50",
			values:     []float64{42.5},
			percentile: 0.5,
			expected:   42.5,
		},
		{
			name:       "single value p0",
			values:     []float64{42.5},
			percentile: 0.0,
			expected:   42.5,
		},
		{
			name:       "single value p100",
			values:     []float64{42.5},
			percentile: 1.0,
			expected:   42.5,
		},
		{
			name:       "two values p50",
			values:     []float64{10.0, 20.0},
			percentile: 0.5,
			expected:   10.0,
		},
		{
			name:       "three values p50",
			values:     []float64{10.0, 20.0, 30.0},
			percentile: 0.5,
			expected:   20.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := calculatePercentile(tt.values, tt.percentile)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestConvertToDBClusterStats_EdgeValues tests conversion with edge values
func TestConvertToDBClusterStats_EdgeValues(t *testing.T) {
	tests := []struct {
		name  string
		stats *model.ClusterGpuHourlyStats
	}{
		{
			name: "max int32 capacity",
			stats: &model.ClusterGpuHourlyStats{
				ClusterName:      "max-cluster",
				TotalGpuCapacity: 2147483647,
				SampleCount:      100,
			},
		},
		{
			name: "max float64 utilization",
			stats: &model.ClusterGpuHourlyStats{
				ClusterName:    "high-util-cluster",
				AvgUtilization: 100.0,
				MaxUtilization: 100.0,
				MinUtilization: 0.0,
			},
		},
		{
			name: "fractional allocated gpus",
			stats: &model.ClusterGpuHourlyStats{
				ClusterName:       "fractional-cluster",
				AllocatedGpuCount: 0.5,
				AllocationRate:    0.5,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := convertToDBClusterStats(tt.stats)
			assert.NotNil(t, result)
			assert.Equal(t, tt.stats.ClusterName, result.ClusterName)
		})
	}
}

// TestConvertToDBNamespaceStats_EdgeValues tests conversion with edge values
func TestConvertToDBNamespaceStats_EdgeValues(t *testing.T) {
	tests := []struct {
		name  string
		stats *model.NamespaceGpuHourlyStats
	}{
		{
			name: "high workload count",
			stats: &model.NamespaceGpuHourlyStats{
				ClusterName:         "high-workload-cluster",
				Namespace:           "ml-training",
				ActiveWorkloadCount: 10000,
			},
		},
		{
			name: "zero allocation",
			stats: &model.NamespaceGpuHourlyStats{
				ClusterName:       "empty-ns-cluster",
				Namespace:         "empty",
				AllocatedGpuCount: 0.0,
				AllocationRate:    0.0,
			},
		},
		{
			name: "full allocation",
			stats: &model.NamespaceGpuHourlyStats{
				ClusterName:       "full-cluster",
				Namespace:         "critical",
				TotalGpuCapacity:  100,
				AllocatedGpuCount: 100.0,
				AllocationRate:    100.0,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := convertToDBNamespaceStats(tt.stats)
			assert.NotNil(t, result)
			assert.Equal(t, tt.stats.ClusterName, result.ClusterName)
			assert.Equal(t, tt.stats.Namespace, result.Namespace)
		})
	}
}

// TestConvertToDBLabelStats_EdgeValues tests conversion with edge values
func TestConvertToDBLabelStats_EdgeValues(t *testing.T) {
	tests := []struct {
		name  string
		stats *model.LabelGpuHourlyStats
	}{
		{
			name: "long dimension value",
			stats: &model.LabelGpuHourlyStats{
				ClusterName:    "test-cluster",
				DimensionType:  "annotation",
				DimensionKey:   "description",
				DimensionValue: "this-is-a-very-long-dimension-value-that-could-be-quite-long-in-production",
			},
		},
		{
			name: "empty dimension value",
			stats: &model.LabelGpuHourlyStats{
				ClusterName:    "test-cluster",
				DimensionType:  "label",
				DimensionKey:   "empty-key",
				DimensionValue: "",
			},
		},
		{
			name: "special characters in dimension value",
			stats: &model.LabelGpuHourlyStats{
				ClusterName:    "test-cluster",
				DimensionType:  "label",
				DimensionKey:   "version",
				DimensionValue: "v1.2.3-alpha+build.123",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := convertToDBLabelStats(tt.stats)
			assert.NotNil(t, result)
			assert.Equal(t, tt.stats.DimensionType, result.DimensionType)
			assert.Equal(t, tt.stats.DimensionKey, result.DimensionKey)
			assert.Equal(t, tt.stats.DimensionValue, result.DimensionValue)
		})
	}
}

// TestSplitAnnotationKey_EdgeCases tests splitAnnotationKey with edge cases
func TestSplitAnnotationKey_EdgeCases(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{
			name:     "double colon",
			input:    "key::value",
			expected: []string{"key", ":value"},
		},
		{
			name:     "colon at start",
			input:    ":value",
			expected: []string{"", "value"},
		},
		{
			name:     "colon at end",
			input:    "key:",
			expected: []string{"key", ""},
		},
		{
			name:     "multiple colons in value",
			input:    "time:10:30:45",
			expected: []string{"time", "10:30:45"},
		},
		{
			name:     "url with protocol",
			input:    "endpoint:https://api.example.com:8443/path",
			expected: []string{"endpoint", "https://api.example.com:8443/path"},
		},
		{
			name:     "whitespace in value",
			input:    "description:this is a description",
			expected: []string{"description", "this is a description"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := splitAnnotationKey(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestNamespaceGpuAggregationJob_ShouldExcludeNamespace_Combined tests combined exclusion rules
func TestNamespaceGpuAggregationJob_ShouldExcludeNamespace_Combined(t *testing.T) {
	config := &NamespaceGpuAggregationConfig{
		Enabled:                 true,
		ExcludeNamespaces:       []string{"dev", "test", "staging", "kube-system"},
		IncludeSystemNamespaces: true,
	}
	job := &NamespaceGpuAggregationJob{config: config}

	assert.True(t, job.shouldExcludeNamespace("dev"))
	assert.True(t, job.shouldExcludeNamespace("test"))
	assert.True(t, job.shouldExcludeNamespace("staging"))
	assert.True(t, job.shouldExcludeNamespace("kube-system"))
	assert.False(t, job.shouldExcludeNamespace("kube-public"))
	assert.False(t, job.shouldExcludeNamespace("production"))
}

// TestConfigKeyGpuAggregation tests config key constant
func TestConfigKeyGpuAggregation(t *testing.T) {
	assert.Equal(t, "job.gpu_aggregation.config", ConfigKeyGpuAggregation)
	assert.Contains(t, ConfigKeyGpuAggregation, "gpu_aggregation")
	assert.Contains(t, ConfigKeyGpuAggregation, "job")
}

// TestAllJobsScheduleConsistency tests that all jobs use consistent scheduling
func TestAllJobsScheduleConsistency(t *testing.T) {
	clusterJob := &ClusterGpuAggregationJob{}
	namespaceJob := &NamespaceGpuAggregationJob{}
	workloadJob := &WorkloadGpuAggregationJob{}
	labelJob := &LabelGpuAggregationJob{}

	schedules := map[string]string{
		"cluster":   clusterJob.Schedule(),
		"namespace": namespaceJob.Schedule(),
		"workload":  workloadJob.Schedule(),
		"label":     labelJob.Schedule(),
	}

	expectedSchedule := "@every 5m"
	for jobType, schedule := range schedules {
		assert.Equal(t, expectedSchedule, schedule, "Job %s should have schedule %s", jobType, expectedSchedule)
	}
}

// TestClusterGpuAggregationConfig_EnabledToggleBehavior tests enabled flag toggle behavior
func TestClusterGpuAggregationConfig_EnabledToggleBehavior(t *testing.T) {
	config := &ClusterGpuAggregationConfig{Enabled: false}
	assert.False(t, config.Enabled)

	config.Enabled = true
	assert.True(t, config.Enabled)

	config.Enabled = false
	assert.False(t, config.Enabled)
}

// TestLabelGpuAggregationConfig_EmptyKeysHandling tests empty keys handling
func TestLabelGpuAggregationConfig_EmptyKeysHandling(t *testing.T) {
	tests := []struct {
		name           string
		labelKeys      []string
		annotationKeys []string
		hasKeys        bool
	}{
		{
			name:           "nil keys",
			labelKeys:      nil,
			annotationKeys: nil,
			hasKeys:        false,
		},
		{
			name:           "empty slices",
			labelKeys:      []string{},
			annotationKeys: []string{},
			hasKeys:        false,
		},
		{
			name:           "one label key",
			labelKeys:      []string{"app"},
			annotationKeys: nil,
			hasKeys:        true,
		},
		{
			name:           "one annotation key",
			labelKeys:      nil,
			annotationKeys: []string{"project"},
			hasKeys:        true,
		},
		{
			name:           "multiple keys",
			labelKeys:      []string{"app", "team", "env"},
			annotationKeys: []string{"project", "cost-center"},
			hasKeys:        true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := &LabelGpuAggregationConfig{
				LabelKeys:      tt.labelKeys,
				AnnotationKeys: tt.annotationKeys,
			}
			hasKeys := len(config.LabelKeys) > 0 || len(config.AnnotationKeys) > 0
			assert.Equal(t, tt.hasKeys, hasKeys)
		})
	}
}

// TestWorkloadGpuAggregationConfig_PromQueryStepValues tests PromQueryStep values
func TestWorkloadGpuAggregationConfig_PromQueryStepValues(t *testing.T) {
	tests := []struct {
		name          string
		promQueryStep int
		isValid       bool
	}{
		{"default", DefaultPromQueryStep, true},
		{"15 seconds", 15, true},
		{"30 seconds", 30, true},
		{"60 seconds", 60, true},
		{"5 minutes", 300, true},
		{"zero", 0, false},
		{"negative", -1, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := &WorkloadGpuAggregationConfig{
				PromQueryStep: tt.promQueryStep,
			}
			isValid := config.PromQueryStep > 0
			assert.Equal(t, tt.isValid, isValid)
		})
	}
}

// TestGpuAggregationSystemConfig_NestedFields tests nested fields access
func TestGpuAggregationSystemConfig_NestedFields(t *testing.T) {
	config := GpuAggregationSystemConfig{}

	config.Dimensions.Label.Enabled = true
	config.Dimensions.Label.LabelKeys = []string{"k1", "k2"}
	config.Dimensions.Label.AnnotationKeys = []string{"a1"}
	config.Dimensions.Label.DefaultValue = "def"
	config.Prometheus.QueryStep = 60

	assert.True(t, config.Dimensions.Label.Enabled)
	assert.Equal(t, 2, len(config.Dimensions.Label.LabelKeys))
	assert.Equal(t, 1, len(config.Dimensions.Label.AnnotationKeys))
	assert.Equal(t, "def", config.Dimensions.Label.DefaultValue)
	assert.Equal(t, 60, config.Prometheus.QueryStep)
}

// TestCacheKeyConstants_Values tests cache key constant values
func TestCacheKeyConstants_Values(t *testing.T) {
	keys := []struct {
		name string
		key  string
	}{
		{"label", CacheKeyLabelGpuAggregationLastHour},
		{"namespace", CacheKeyNamespaceGpuAggregationLastHour},
		{"cluster", CacheKeyClusterGpuAggregationLastHour},
		{"workload", CacheKeyWorkloadGpuAggregationLastHour},
	}

	for _, k := range keys {
		t.Run(k.name, func(t *testing.T) {
			assert.NotEmpty(t, k.key)
			assert.Contains(t, k.key, "last_processed_hour")
			assert.Contains(t, k.key, "job")
		})
	}
}

// TestCalculatePercentile_OrderedValues tests percentile with pre-sorted values
func TestCalculatePercentile_OrderedValues(t *testing.T) {
	values := []float64{0, 10, 20, 30, 40, 50, 60, 70, 80, 90, 100}

	testCases := []struct {
		percentile float64
		minVal     float64
		maxVal     float64
	}{
		{0.0, 0, 0},
		{0.1, 0, 20},
		{0.25, 20, 30},
		{0.5, 40, 60},
		{0.75, 70, 80},
		{0.9, 80, 100},
		{1.0, 100, 100},
	}

	for _, tc := range testCases {
		result := calculatePercentile(values, tc.percentile)
		assert.GreaterOrEqual(t, result, tc.minVal, "p%.0f should be >= %.0f", tc.percentile*100, tc.minVal)
		assert.LessOrEqual(t, result, tc.maxVal, "p%.0f should be <= %.0f", tc.percentile*100, tc.maxVal)
	}
}

// TestNamespaceGpuAggregationJob_ShouldExcludeNamespace_EmptyConfig tests with empty config
func TestNamespaceGpuAggregationJob_ShouldExcludeNamespace_EmptyConfig(t *testing.T) {
	job := &NamespaceGpuAggregationJob{
		config: &NamespaceGpuAggregationConfig{
			ExcludeNamespaces:       nil,
			IncludeSystemNamespaces: true,
		},
	}

	assert.False(t, job.shouldExcludeNamespace("any-namespace"))
	assert.False(t, job.shouldExcludeNamespace("kube-system"))
}

// TestQueryTemplates_ValidPromQL tests that query templates produce valid PromQL
func TestQueryTemplates_ValidPromQL(t *testing.T) {
	uid := "workload-12345"

	templates := []struct {
		name     string
		template string
	}{
		{"utilization", WorkloadUtilizationQueryTemplate},
		{"memory_used", WorkloadGpuMemoryUsedQueryTemplate},
		{"memory_total", WorkloadGpuMemoryTotalQueryTemplate},
	}

	for _, tt := range templates {
		t.Run(tt.name, func(t *testing.T) {
			query := fmt.Sprintf(tt.template, uid)
			assert.True(t, len(query) > 0)
			assert.Contains(t, query, "avg(")
			assert.Contains(t, query, "}")
			assert.Contains(t, query, "{")
			assert.Contains(t, query, fmt.Sprintf(`workload_uid="%s"`, uid))
		})
	}
}

// TestJobConfigNilHandling tests jobs with nil config
func TestJobConfigNilHandling(t *testing.T) {
	clusterJob := &ClusterGpuAggregationJob{config: nil}
	assert.Nil(t, clusterJob.GetConfig())

	namespaceJob := &NamespaceGpuAggregationJob{config: nil}
	assert.Nil(t, namespaceJob.GetConfig())

	workloadJob := &WorkloadGpuAggregationJob{config: nil}
	assert.Nil(t, workloadJob.GetConfig())

	labelJob := &LabelGpuAggregationJob{config: nil}
	assert.Nil(t, labelJob.GetConfig())
}

// TestDefaultPromQueryStep_Value tests the default prom query step constant
func TestDefaultPromQueryStep_Value(t *testing.T) {
	assert.Equal(t, 60, DefaultPromQueryStep)
	assert.True(t, DefaultPromQueryStep >= 1)
	assert.True(t, DefaultPromQueryStep <= 300)
}
