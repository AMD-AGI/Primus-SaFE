package api

import (
	"testing"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/constant"
	dbModel "github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
	"github.com/stretchr/testify/assert"
	"gorm.io/gorm"
)

func TestGetSource(t *testing.T) {
	tests := []struct {
		name       string
		workload   *dbModel.GpuWorkload
		expected   string
		description string
	}{
		{
			name: "Source is empty - returns k8s",
			workload: &dbModel.GpuWorkload{
				ID:           1,
				GroupVersion: "v1",
				Kind:         "Pod",
				Namespace:    "default",
				Name:         "test-pod",
				UID:          "test-uid-1",
				Source:       "",
			},
			expected:    constant.ContainerSourceK8S,
			description: "When Source field is empty, should return default k8s",
		},
		{
			name: "Source is k8s",
			workload: &dbModel.GpuWorkload{
				ID:           2,
				GroupVersion: "v1",
				Kind:         "Pod",
				Namespace:    "default",
				Name:         "test-pod-2",
				UID:          "test-uid-2",
				Source:       constant.ContainerSourceK8S,
			},
			expected:    constant.ContainerSourceK8S,
			description: "When Source explicitly set to k8s, should return k8s",
		},
		{
			name: "Source is docker",
			workload: &dbModel.GpuWorkload{
				ID:           3,
				GroupVersion: "",
				Kind:         "Container",
				Namespace:    "",
				Name:         "docker-container",
				UID:          "test-uid-3",
				Source:       constant.ContainerSourceDocker,
			},
			expected:    constant.ContainerSourceDocker,
			description: "When Source set to docker, should return docker",
		},
		{
			name: "Source is custom value",
			workload: &dbModel.GpuWorkload{
				ID:           4,
				GroupVersion: "custom.io/v1",
				Kind:         "CustomWorkload",
				Namespace:    "custom-ns",
				Name:         "custom-workload",
				UID:          "test-uid-4",
				Source:       "custom-runtime",
			},
			expected:    "custom-runtime",
			description: "When Source is custom value, should return that custom value",
		},
		{
			name: "Source is whitespace",
			workload: &dbModel.GpuWorkload{
				ID:           5,
				GroupVersion: "v1",
				Kind:         "Pod",
				Namespace:    "default",
				Name:         "test-pod-3",
				UID:          "test-uid-5",
				Source:       "   ",
			},
			expected:    "   ",
			description: "When Source is whitespace, should return whitespace (not treated as empty string)",
		},
		{
			name: "Deployment workload - Source is empty",
			workload: &dbModel.GpuWorkload{
				ID:           6,
				GroupVersion: "apps/v1",
				Kind:         "Deployment",
				Namespace:    "production",
				Name:         "nginx-deployment",
				UID:          "test-uid-6",
				ParentUID:    "",
				GpuRequest:   4,
				Source:       "",
			},
			expected:    constant.ContainerSourceK8S,
			description: "Deployment type workload, when Source is empty should return k8s",
		},
		{
			name: "StatefulSet workload - Source is k8s",
			workload: &dbModel.GpuWorkload{
				ID:           7,
				GroupVersion: "apps/v1",
				Kind:         "StatefulSet",
				Namespace:    "database",
				Name:         "mysql-statefulset",
				UID:          "test-uid-7",
				ParentUID:    "",
				GpuRequest:   2,
				Source:       constant.ContainerSourceK8S,
			},
			expected:    constant.ContainerSourceK8S,
			description: "StatefulSet type workload should return k8s",
		},
		{
			name: "Job workload - Source is empty",
			workload: &dbModel.GpuWorkload{
				ID:           8,
				GroupVersion: "batch/v1",
				Kind:         "Job",
				Namespace:    "batch",
				Name:         "data-processing-job",
				UID:          "test-uid-8",
				ParentUID:    "",
				GpuRequest:   8,
				Source:       "",
			},
			expected:    constant.ContainerSourceK8S,
			description: "Job type workload, when Source is empty should return k8s",
		},
		{
			name: "Workload with complete fields - Source is empty",
			workload: &dbModel.GpuWorkload{
				ID:           9,
				GroupVersion: "v1",
				Kind:         "Pod",
				Namespace:    "ml-training",
				Name:         "pytorch-pod",
				UID:          "test-uid-9",
				ParentUID:    "parent-uid-1",
				GpuRequest:   4,
				CreatedAt:    time.Now(),
				UpdatedAt:    time.Now(),
				DeletedAt:    gorm.DeletedAt{},
				EndAt:        time.Time{},
				Status:       "Running",
				Source:       "",
				Labels:       dbModel.ExtType{},
				Annotations:  dbModel.ExtType{},
			},
			expected:    constant.ContainerSourceK8S,
			description: "Complete workload object with all fields, when Source is empty should return k8s",
		},
		{
			name: "Workload with complete fields - Source is docker",
			workload: &dbModel.GpuWorkload{
				ID:           10,
				GroupVersion: "",
				Kind:         "Container",
				Namespace:    "",
				Name:         "standalone-container",
				UID:          "test-uid-10",
				ParentUID:    "",
				GpuRequest:   2,
				CreatedAt:    time.Now(),
				UpdatedAt:    time.Now(),
				DeletedAt:    gorm.DeletedAt{},
				EndAt:        time.Time{},
				Status:       "Running",
				Source:       constant.ContainerSourceDocker,
				Labels:       dbModel.ExtType{},
				Annotations:  dbModel.ExtType{},
			},
			expected:    constant.ContainerSourceDocker,
			description: "Complete Docker container workload should return docker",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getSource(tt.workload)
			assert.Equal(t, tt.expected, result, tt.description)
		})
	}
}

func TestGetSource_EdgeCases(t *testing.T) {
	t.Run("nil workload - should panic", func(t *testing.T) {
		// When passing nil, the function will panic because it tries to access nil's fields
		assert.Panics(t, func() {
			getSource(nil)
		}, "Passing nil should panic")
	})

	t.Run("minimal workload object - only Source field", func(t *testing.T) {
		workload := &dbModel.GpuWorkload{
			Source: "test-source",
		}
		result := getSource(workload)
		assert.Equal(t, "test-source", result)
	})

	t.Run("minimal workload object - Source is empty", func(t *testing.T) {
		workload := &dbModel.GpuWorkload{
			Source: "",
		}
		result := getSource(workload)
		assert.Equal(t, constant.ContainerSourceK8S, result)
	})

	t.Run("Source with special characters", func(t *testing.T) {
		specialSources := []string{
			"k8s-v2",
			"docker-compose",
			"containerd",
			"cri-o",
			"podman",
			"k8s.io",
			"docker.io",
			"custom/runtime",
			"runtime@v1",
			"runtime:latest",
			"运行时",           // Chinese characters
			"🐳",             // emoji
			"source\nwith\nnewline",
			"source\twith\ttab",
		}

		for _, source := range specialSources {
			t.Run("Source="+source, func(t *testing.T) {
				workload := &dbModel.GpuWorkload{
					Source: source,
				}
				result := getSource(workload)
				assert.Equal(t, source, result, "Should return Source field as-is")
			})
		}
	})
}

func TestGetSource_BusinessScenarios(t *testing.T) {
	t.Run("K8s Pod scenarios", func(t *testing.T) {
		scenarios := []struct {
			name        string
			kind        string
			namespace   string
			source      string
			expected    string
		}{
			{"standard Pod", "Pod", "default", "", constant.ContainerSourceK8S},
			{"training Pod", "Pod", "ml-training", "", constant.ContainerSourceK8S},
			{"system Pod", "Pod", "kube-system", "", constant.ContainerSourceK8S},
			{"explicit k8s Pod", "Pod", "default", constant.ContainerSourceK8S, constant.ContainerSourceK8S},
		}

		for _, scenario := range scenarios {
			t.Run(scenario.name, func(t *testing.T) {
				workload := &dbModel.GpuWorkload{
					Kind:      scenario.kind,
					Namespace: scenario.namespace,
					Source:    scenario.source,
				}
				result := getSource(workload)
				assert.Equal(t, scenario.expected, result)
			})
		}
	})

	t.Run("Docker container scenarios", func(t *testing.T) {
		workload := &dbModel.GpuWorkload{
			Kind:      "Container",
			Namespace: "",
			Name:      "standalone-gpu-container",
			Source:    constant.ContainerSourceDocker,
		}
		result := getSource(workload)
		assert.Equal(t, constant.ContainerSourceDocker, result)
	})

	t.Run("mixed environment scenarios", func(t *testing.T) {
		workloads := []*dbModel.GpuWorkload{
			{Name: "k8s-pod-1", Source: ""},
			{Name: "k8s-pod-2", Source: constant.ContainerSourceK8S},
			{Name: "docker-container-1", Source: constant.ContainerSourceDocker},
			{Name: "custom-runtime-1", Source: "custom"},
		}

		expected := []string{
			constant.ContainerSourceK8S,
			constant.ContainerSourceK8S,
			constant.ContainerSourceDocker,
			"custom",
		}

		for i, workload := range workloads {
			result := getSource(workload)
			assert.Equal(t, expected[i], result, "Workload %s Source should be %s", workload.Name, expected[i])
		}
	})
}

func TestGetSource_Consistency(t *testing.T) {
	t.Run("multiple calls return consistent results", func(t *testing.T) {
		workload := &dbModel.GpuWorkload{
			ID:     1,
			Name:   "test-pod",
			Source: "",
		}

		// Multiple calls should return the same result
		result1 := getSource(workload)
		result2 := getSource(workload)
		result3 := getSource(workload)

		assert.Equal(t, result1, result2)
		assert.Equal(t, result2, result3)
		assert.Equal(t, constant.ContainerSourceK8S, result1)
	})

	t.Run("returns new value after modifying Source", func(t *testing.T) {
		workload := &dbModel.GpuWorkload{
			ID:     1,
			Name:   "test-pod",
			Source: "",
		}

		result1 := getSource(workload)
		assert.Equal(t, constant.ContainerSourceK8S, result1)

		// Modify Source
		workload.Source = constant.ContainerSourceDocker
		result2 := getSource(workload)
		assert.Equal(t, constant.ContainerSourceDocker, result2)

		// Modify again
		workload.Source = "custom"
		result3 := getSource(workload)
		assert.Equal(t, "custom", result3)
	})
}

func BenchmarkGetSource(b *testing.B) {
	workload := &dbModel.GpuWorkload{
		ID:           1,
		GroupVersion: "v1",
		Kind:         "Pod",
		Namespace:    "default",
		Name:         "test-pod",
		UID:          "test-uid",
		Source:       "",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = getSource(workload)
	}
}

func BenchmarkGetSource_WithSource(b *testing.B) {
	workload := &dbModel.GpuWorkload{
		ID:           1,
		GroupVersion: "v1",
		Kind:         "Pod",
		Namespace:    "default",
		Name:         "test-pod",
		UID:          "test-uid",
		Source:       constant.ContainerSourceK8S,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = getSource(workload)
	}
}

func BenchmarkGetSource_Parallel(b *testing.B) {
	workload := &dbModel.GpuWorkload{
		ID:           1,
		GroupVersion: "v1",
		Kind:         "Pod",
		Namespace:    "default",
		Name:         "test-pod",
		UID:          "test-uid",
		Source:       "",
	}

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_ = getSource(workload)
		}
	})
}

