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
			name: "Source为空-返回k8s",
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
			description: "当Source字段为空时，应该返回默认的k8s",
		},
		{
			name: "Source为k8s",
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
			description: "Source显式设置为k8s时，应该返回k8s",
		},
		{
			name: "Source为docker",
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
			description: "Source设置为docker时，应该返回docker",
		},
		{
			name: "Source为自定义值",
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
			description: "Source为自定义值时，应该返回该自定义值",
		},
		{
			name: "Source为空格",
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
			description: "Source为空格时，应该返回空格（不被视为空字符串）",
		},
		{
			name: "Deployment工作负载-Source为空",
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
			description: "Deployment类型的工作负载，Source为空时应返回k8s",
		},
		{
			name: "StatefulSet工作负载-Source为k8s",
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
			description: "StatefulSet类型的工作负载应返回k8s",
		},
		{
			name: "Job工作负载-Source为空",
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
			description: "Job类型的工作负载，Source为空时应返回k8s",
		},
		{
			name: "包含完整字段的工作负载-Source为空",
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
			description: "包含所有字段的完整工作负载对象，Source为空时应返回k8s",
		},
		{
			name: "包含完整字段的工作负载-Source为docker",
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
			description: "完整的Docker容器工作负载应返回docker",
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
	t.Run("nil工作负载-应该panic", func(t *testing.T) {
		// 当传入nil时，函数会panic，因为会尝试访问nil的字段
		assert.Panics(t, func() {
			getSource(nil)
		}, "传入nil应该会panic")
	})

	t.Run("最小化工作负载对象-只有Source字段", func(t *testing.T) {
		workload := &dbModel.GpuWorkload{
			Source: "test-source",
		}
		result := getSource(workload)
		assert.Equal(t, "test-source", result)
	})

	t.Run("最小化工作负载对象-Source为空", func(t *testing.T) {
		workload := &dbModel.GpuWorkload{
			Source: "",
		}
		result := getSource(workload)
		assert.Equal(t, constant.ContainerSourceK8S, result)
	})

	t.Run("Source为特殊字符", func(t *testing.T) {
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
			"运行时",           // 中文
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
				assert.Equal(t, source, result, "应该原样返回Source字段")
			})
		}
	})
}

func TestGetSource_BusinessScenarios(t *testing.T) {
	t.Run("K8s Pod场景", func(t *testing.T) {
		scenarios := []struct {
			name        string
			kind        string
			namespace   string
			source      string
			expected    string
		}{
			{"标准Pod", "Pod", "default", "", constant.ContainerSourceK8S},
			{"训练Pod", "Pod", "ml-training", "", constant.ContainerSourceK8S},
			{"系统Pod", "Pod", "kube-system", "", constant.ContainerSourceK8S},
			{"显式k8s Pod", "Pod", "default", constant.ContainerSourceK8S, constant.ContainerSourceK8S},
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

	t.Run("Docker容器场景", func(t *testing.T) {
		workload := &dbModel.GpuWorkload{
			Kind:      "Container",
			Namespace: "",
			Name:      "standalone-gpu-container",
			Source:    constant.ContainerSourceDocker,
		}
		result := getSource(workload)
		assert.Equal(t, constant.ContainerSourceDocker, result)
	})

	t.Run("混合环境场景", func(t *testing.T) {
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
			assert.Equal(t, expected[i], result, "工作负载 %s 的Source应该是 %s", workload.Name, expected[i])
		}
	})
}

func TestGetSource_Consistency(t *testing.T) {
	t.Run("多次调用返回一致结果", func(t *testing.T) {
		workload := &dbModel.GpuWorkload{
			ID:     1,
			Name:   "test-pod",
			Source: "",
		}

		// 多次调用应该返回相同的结果
		result1 := getSource(workload)
		result2 := getSource(workload)
		result3 := getSource(workload)

		assert.Equal(t, result1, result2)
		assert.Equal(t, result2, result3)
		assert.Equal(t, constant.ContainerSourceK8S, result1)
	})

	t.Run("修改Source后返回新值", func(t *testing.T) {
		workload := &dbModel.GpuWorkload{
			ID:     1,
			Name:   "test-pod",
			Source: "",
		}

		result1 := getSource(workload)
		assert.Equal(t, constant.ContainerSourceK8S, result1)

		// 修改Source
		workload.Source = constant.ContainerSourceDocker
		result2 := getSource(workload)
		assert.Equal(t, constant.ContainerSourceDocker, result2)

		// 再次修改
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

