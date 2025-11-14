package gpu_aggregation

import (
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

func TestShouldExcludeNamespace(t *testing.T) {
	tests := []struct {
		name      string
		config    *model.GpuAggregationConfig
		namespace string
		expected  bool
	}{
		{
			name: "namespace dimension disabled",
			config: &model.GpuAggregationConfig{
				Dimensions: struct {
					Cluster struct {
						Enabled bool `json:"enabled"`
					} `json:"cluster"`
					Namespace struct {
						Enabled                 bool     `json:"enabled"`
						IncludeSystemNamespaces bool     `json:"include_system_namespaces"`
						ExcludeNamespaces       []string `json:"exclude_namespaces"`
					} `json:"namespace"`
					Label struct {
						Enabled        bool     `json:"enabled"`
						LabelKeys      []string `json:"label_keys"`
						AnnotationKeys []string `json:"annotation_keys"`
						DefaultValue   string   `json:"default_value"`
					} `json:"label"`
					Workload struct {
						Enabled bool `json:"enabled"`
					} `json:"workload"`
				}{
					Namespace: struct {
						Enabled                 bool     `json:"enabled"`
						IncludeSystemNamespaces bool     `json:"include_system_namespaces"`
						ExcludeNamespaces       []string `json:"exclude_namespaces"`
					}{
						Enabled: false,
					},
				},
			},
			namespace: "default",
			expected:  true,
		},
		{
			name: "namespace in exclusion list",
			config: &model.GpuAggregationConfig{
				Dimensions: struct {
					Cluster struct {
						Enabled bool `json:"enabled"`
					} `json:"cluster"`
					Namespace struct {
						Enabled                 bool     `json:"enabled"`
						IncludeSystemNamespaces bool     `json:"include_system_namespaces"`
						ExcludeNamespaces       []string `json:"exclude_namespaces"`
					} `json:"namespace"`
					Label struct {
						Enabled        bool     `json:"enabled"`
						LabelKeys      []string `json:"label_keys"`
						AnnotationKeys []string `json:"annotation_keys"`
						DefaultValue   string   `json:"default_value"`
					} `json:"label"`
					Workload struct {
						Enabled bool `json:"enabled"`
					} `json:"workload"`
				}{
					Namespace: struct {
						Enabled                 bool     `json:"enabled"`
						IncludeSystemNamespaces bool     `json:"include_system_namespaces"`
						ExcludeNamespaces       []string `json:"exclude_namespaces"`
					}{
						Enabled:                 true,
						ExcludeNamespaces:       []string{"test", "dev", "staging"},
						IncludeSystemNamespaces: true,
					},
				},
			},
			namespace: "dev",
			expected:  true,
		},
		{
			name: "system namespace excluded when flag is false",
			config: &model.GpuAggregationConfig{
				Dimensions: struct {
					Cluster struct {
						Enabled bool `json:"enabled"`
					} `json:"cluster"`
					Namespace struct {
						Enabled                 bool     `json:"enabled"`
						IncludeSystemNamespaces bool     `json:"include_system_namespaces"`
						ExcludeNamespaces       []string `json:"exclude_namespaces"`
					} `json:"namespace"`
					Label struct {
						Enabled        bool     `json:"enabled"`
						LabelKeys      []string `json:"label_keys"`
						AnnotationKeys []string `json:"annotation_keys"`
						DefaultValue   string   `json:"default_value"`
					} `json:"label"`
					Workload struct {
						Enabled bool `json:"enabled"`
					} `json:"workload"`
				}{
					Namespace: struct {
						Enabled                 bool     `json:"enabled"`
						IncludeSystemNamespaces bool     `json:"include_system_namespaces"`
						ExcludeNamespaces       []string `json:"exclude_namespaces"`
					}{
						Enabled:                 true,
						ExcludeNamespaces:       []string{},
						IncludeSystemNamespaces: false,
					},
				},
			},
			namespace: "kube-system",
			expected:  true,
		},
		{
			name: "kube-public excluded when flag is false",
			config: &model.GpuAggregationConfig{
				Dimensions: struct {
					Cluster struct {
						Enabled bool `json:"enabled"`
					} `json:"cluster"`
					Namespace struct {
						Enabled                 bool     `json:"enabled"`
						IncludeSystemNamespaces bool     `json:"include_system_namespaces"`
						ExcludeNamespaces       []string `json:"exclude_namespaces"`
					} `json:"namespace"`
					Label struct {
						Enabled        bool     `json:"enabled"`
						LabelKeys      []string `json:"label_keys"`
						AnnotationKeys []string `json:"annotation_keys"`
						DefaultValue   string   `json:"default_value"`
					} `json:"label"`
					Workload struct {
						Enabled bool `json:"enabled"`
					} `json:"workload"`
				}{
					Namespace: struct {
						Enabled                 bool     `json:"enabled"`
						IncludeSystemNamespaces bool     `json:"include_system_namespaces"`
						ExcludeNamespaces       []string `json:"exclude_namespaces"`
					}{
						Enabled:                 true,
						ExcludeNamespaces:       []string{},
						IncludeSystemNamespaces: false,
					},
				},
			},
			namespace: "kube-public",
			expected:  true,
		},
		{
			name: "kube-node-lease excluded when flag is false",
			config: &model.GpuAggregationConfig{
				Dimensions: struct {
					Cluster struct {
						Enabled bool `json:"enabled"`
					} `json:"cluster"`
					Namespace struct {
						Enabled                 bool     `json:"enabled"`
						IncludeSystemNamespaces bool     `json:"include_system_namespaces"`
						ExcludeNamespaces       []string `json:"exclude_namespaces"`
					} `json:"namespace"`
					Label struct {
						Enabled        bool     `json:"enabled"`
						LabelKeys      []string `json:"label_keys"`
						AnnotationKeys []string `json:"annotation_keys"`
						DefaultValue   string   `json:"default_value"`
					} `json:"label"`
					Workload struct {
						Enabled bool `json:"enabled"`
					} `json:"workload"`
				}{
					Namespace: struct {
						Enabled                 bool     `json:"enabled"`
						IncludeSystemNamespaces bool     `json:"include_system_namespaces"`
						ExcludeNamespaces       []string `json:"exclude_namespaces"`
					}{
						Enabled:                 true,
						ExcludeNamespaces:       []string{},
						IncludeSystemNamespaces: false,
					},
				},
			},
			namespace: "kube-node-lease",
			expected:  true,
		},
		{
			name: "system namespace included when flag is true",
			config: &model.GpuAggregationConfig{
				Dimensions: struct {
					Cluster struct {
						Enabled bool `json:"enabled"`
					} `json:"cluster"`
					Namespace struct {
						Enabled                 bool     `json:"enabled"`
						IncludeSystemNamespaces bool     `json:"include_system_namespaces"`
						ExcludeNamespaces       []string `json:"exclude_namespaces"`
					} `json:"namespace"`
					Label struct {
						Enabled        bool     `json:"enabled"`
						LabelKeys      []string `json:"label_keys"`
						AnnotationKeys []string `json:"annotation_keys"`
						DefaultValue   string   `json:"default_value"`
					} `json:"label"`
					Workload struct {
						Enabled bool `json:"enabled"`
					} `json:"workload"`
				}{
					Namespace: struct {
						Enabled                 bool     `json:"enabled"`
						IncludeSystemNamespaces bool     `json:"include_system_namespaces"`
						ExcludeNamespaces       []string `json:"exclude_namespaces"`
					}{
						Enabled:                 true,
						ExcludeNamespaces:       []string{},
						IncludeSystemNamespaces: true,
					},
				},
			},
			namespace: "kube-system",
			expected:  false,
		},
		{
			name: "regular namespace not excluded",
			config: &model.GpuAggregationConfig{
				Dimensions: struct {
					Cluster struct {
						Enabled bool `json:"enabled"`
					} `json:"cluster"`
					Namespace struct {
						Enabled                 bool     `json:"enabled"`
						IncludeSystemNamespaces bool     `json:"include_system_namespaces"`
						ExcludeNamespaces       []string `json:"exclude_namespaces"`
					} `json:"namespace"`
					Label struct {
						Enabled        bool     `json:"enabled"`
						LabelKeys      []string `json:"label_keys"`
						AnnotationKeys []string `json:"annotation_keys"`
						DefaultValue   string   `json:"default_value"`
					} `json:"label"`
					Workload struct {
						Enabled bool `json:"enabled"`
					} `json:"workload"`
				}{
					Namespace: struct {
						Enabled                 bool     `json:"enabled"`
						IncludeSystemNamespaces bool     `json:"include_system_namespaces"`
						ExcludeNamespaces       []string `json:"exclude_namespaces"`
					}{
						Enabled:                 true,
						ExcludeNamespaces:       []string{"test"},
						IncludeSystemNamespaces: false,
					},
				},
			},
			namespace: "production",
			expected:  false,
		},
		{
			name: "empty exclusion list",
			config: &model.GpuAggregationConfig{
				Dimensions: struct {
					Cluster struct {
						Enabled bool `json:"enabled"`
					} `json:"cluster"`
					Namespace struct {
						Enabled                 bool     `json:"enabled"`
						IncludeSystemNamespaces bool     `json:"include_system_namespaces"`
						ExcludeNamespaces       []string `json:"exclude_namespaces"`
					} `json:"namespace"`
					Label struct {
						Enabled        bool     `json:"enabled"`
						LabelKeys      []string `json:"label_keys"`
						AnnotationKeys []string `json:"annotation_keys"`
						DefaultValue   string   `json:"default_value"`
					} `json:"label"`
					Workload struct {
						Enabled bool `json:"enabled"`
					} `json:"workload"`
				}{
					Namespace: struct {
						Enabled                 bool     `json:"enabled"`
						IncludeSystemNamespaces bool     `json:"include_system_namespaces"`
						ExcludeNamespaces       []string `json:"exclude_namespaces"`
					}{
						Enabled:                 true,
						ExcludeNamespaces:       []string{},
						IncludeSystemNamespaces: true,
					},
				},
			},
			namespace: "any-namespace",
			expected:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			job := &GpuAggregationJob{
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

func TestNewGpuAggregationJob(t *testing.T) {
	job := NewGpuAggregationJob()

	assert.NotNil(t, job, "Job should not be nil")
	assert.NotNil(t, job.snapshotCache, "Snapshot cache should be initialized")
	assert.Empty(t, job.snapshotCache, "Snapshot cache should be empty initially")
	assert.NotNil(t, job.configManager, "Config manager should be initialized")
	assert.NotEmpty(t, job.clusterName, "Cluster name should be set")
	assert.NotZero(t, job.currentHour, "Current hour should be set")
}

func TestNewGpuAggregationJobWithConfig(t *testing.T) {
	config := &model.GpuAggregationConfig{
		Enabled: true,
	}
	config.Sampling.Enabled = true
	config.Sampling.Interval = "5m"

	job := NewGpuAggregationJobWithConfig(config)

	assert.NotNil(t, job, "Job should not be nil")
	assert.NotNil(t, job.config, "Config should be set")
	assert.Equal(t, config, job.config, "Config should match input")
	assert.True(t, job.config.Enabled, "Config should be enabled")
	assert.NotNil(t, job.snapshotCache, "Snapshot cache should be initialized")
	assert.NotNil(t, job.configManager, "Config manager should be initialized")
}

func TestGetConfig(t *testing.T) {
	config := &model.GpuAggregationConfig{
		Enabled: true,
	}

	job := &GpuAggregationJob{
		config: config,
	}

	result := job.GetConfig()
	assert.Equal(t, config, result, "GetConfig should return the config")
}

func TestGetConfigNil(t *testing.T) {
	job := &GpuAggregationJob{
		config: nil,
	}

	result := job.GetConfig()
	assert.Nil(t, result, "GetConfig should return nil when config is nil")
}
