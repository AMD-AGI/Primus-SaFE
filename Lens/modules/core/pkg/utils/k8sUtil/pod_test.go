package k8sUtil

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestIsPodDone(t *testing.T) {
	t.Run("Pod成功完成", func(t *testing.T) {
		pod := &corev1.Pod{
			Status: corev1.PodStatus{
				Phase: corev1.PodSucceeded,
			},
		}

		result := IsPodDone(pod)
		assert.True(t, result)
	})

	t.Run("Pod运行中", func(t *testing.T) {
		pod := &corev1.Pod{
			Status: corev1.PodStatus{
				Phase: corev1.PodRunning,
			},
		}

		result := IsPodDone(pod)
		assert.False(t, result)
	})

	t.Run("Pod失败", func(t *testing.T) {
		pod := &corev1.Pod{
			Status: corev1.PodStatus{
				Phase: corev1.PodFailed,
			},
		}

		result := IsPodDone(pod)
		assert.False(t, result)
	})

	t.Run("Pod挂起", func(t *testing.T) {
		pod := &corev1.Pod{
			Status: corev1.PodStatus{
				Phase: corev1.PodPending,
			},
		}

		result := IsPodDone(pod)
		assert.False(t, result)
	})

	t.Run("nil Pod", func(t *testing.T) {
		result := IsPodDone(nil)
		assert.False(t, result)
	})
}

func TestIsPodRunning(t *testing.T) {
	t.Run("Pod运行中且就绪", func(t *testing.T) {
		pod := &corev1.Pod{
			Status: corev1.PodStatus{
				Phase: corev1.PodRunning,
				Conditions: []corev1.PodCondition{
					{
						Type:   corev1.PodReady,
						Status: corev1.ConditionTrue,
					},
				},
			},
		}

		result := IsPodRunning(pod)
		assert.True(t, result)
	})

	t.Run("Pod运行中但未就绪", func(t *testing.T) {
		pod := &corev1.Pod{
			Status: corev1.PodStatus{
				Phase: corev1.PodRunning,
				Conditions: []corev1.PodCondition{
					{
						Type:   corev1.PodReady,
						Status: corev1.ConditionFalse,
					},
				},
			},
		}

		result := IsPodRunning(pod)
		assert.False(t, result)
	})

	t.Run("Pod不在运行状态", func(t *testing.T) {
		pod := &corev1.Pod{
			Status: corev1.PodStatus{
				Phase: corev1.PodPending,
			},
		}

		result := IsPodRunning(pod)
		assert.False(t, result)
	})

	t.Run("nil Pod", func(t *testing.T) {
		result := IsPodRunning(nil)
		assert.False(t, result)
	})

	t.Run("Pod运行中但无就绪条件", func(t *testing.T) {
		pod := &corev1.Pod{
			Status: corev1.PodStatus{
				Phase:      corev1.PodRunning,
				Conditions: []corev1.PodCondition{},
			},
		}

		result := IsPodRunning(pod)
		assert.False(t, result)
	})
}

func TestHasGPU(t *testing.T) {
	gpuResource := "nvidia.com/gpu"

	t.Run("Pod请求GPU资源", func(t *testing.T) {
		pod := &corev1.Pod{
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{
					{
						Resources: corev1.ResourceRequirements{
							Requests: corev1.ResourceList{
								corev1.ResourceName(gpuResource): resource.MustParse("1"),
							},
						},
					},
				},
			},
		}

		result := HasGPU(pod, gpuResource)
		assert.True(t, result)
	})

	t.Run("Pod限制GPU资源", func(t *testing.T) {
		pod := &corev1.Pod{
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{
					{
						Resources: corev1.ResourceRequirements{
							Limits: corev1.ResourceList{
								corev1.ResourceName(gpuResource): resource.MustParse("2"),
							},
						},
					},
				},
			},
		}

		result := HasGPU(pod, gpuResource)
		assert.True(t, result)
	})

	t.Run("Pod无GPU资源", func(t *testing.T) {
		pod := &corev1.Pod{
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{
					{
						Resources: corev1.ResourceRequirements{
							Requests: corev1.ResourceList{
								corev1.ResourceCPU:    resource.MustParse("1"),
								corev1.ResourceMemory: resource.MustParse("1Gi"),
							},
						},
					},
				},
			},
		}

		result := HasGPU(pod, gpuResource)
		assert.False(t, result)
	})

	t.Run("GPU资源为零", func(t *testing.T) {
		pod := &corev1.Pod{
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{
					{
						Resources: corev1.ResourceRequirements{
							Requests: corev1.ResourceList{
								corev1.ResourceName(gpuResource): resource.MustParse("0"),
							},
						},
					},
				},
			},
		}

		result := HasGPU(pod, gpuResource)
		assert.False(t, result)
	})

	t.Run("多容器有GPU", func(t *testing.T) {
		pod := &corev1.Pod{
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{
					{
						Resources: corev1.ResourceRequirements{},
					},
					{
						Resources: corev1.ResourceRequirements{
							Requests: corev1.ResourceList{
								corev1.ResourceName(gpuResource): resource.MustParse("1"),
							},
						},
					},
				},
			},
		}

		result := HasGPU(pod, gpuResource)
		assert.True(t, result)
	})
}

func TestGetGpuAllocated(t *testing.T) {
	gpuResource := "nvidia.com/gpu"

	t.Run("单容器单GPU", func(t *testing.T) {
		pod := &corev1.Pod{
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{
					{
						Resources: corev1.ResourceRequirements{
							Requests: corev1.ResourceList{
								corev1.ResourceName(gpuResource): resource.MustParse("1"),
							},
						},
					},
				},
			},
		}

		result := GetGpuAllocated(pod, gpuResource)
		assert.Equal(t, 1, result)
	})

	t.Run("单容器多GPU", func(t *testing.T) {
		pod := &corev1.Pod{
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{
					{
						Resources: corev1.ResourceRequirements{
							Requests: corev1.ResourceList{
								corev1.ResourceName(gpuResource): resource.MustParse("4"),
							},
						},
					},
				},
			},
		}

		result := GetGpuAllocated(pod, gpuResource)
		assert.Equal(t, 4, result)
	})

	t.Run("多容器累加GPU", func(t *testing.T) {
		pod := &corev1.Pod{
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{
					{
						Resources: corev1.ResourceRequirements{
							Requests: corev1.ResourceList{
								corev1.ResourceName(gpuResource): resource.MustParse("2"),
							},
						},
					},
					{
						Resources: corev1.ResourceRequirements{
							Requests: corev1.ResourceList{
								corev1.ResourceName(gpuResource): resource.MustParse("3"),
							},
						},
					},
				},
			},
		}

		result := GetGpuAllocated(pod, gpuResource)
		assert.Equal(t, 5, result)
	})

	t.Run("无GPU", func(t *testing.T) {
		pod := &corev1.Pod{
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{
					{
						Resources: corev1.ResourceRequirements{},
					},
				},
			},
		}

		result := GetGpuAllocated(pod, gpuResource)
		assert.Equal(t, 0, result)
	})

	t.Run("部分容器有GPU", func(t *testing.T) {
		pod := &corev1.Pod{
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{
					{
						Resources: corev1.ResourceRequirements{},
					},
					{
						Resources: corev1.ResourceRequirements{
							Requests: corev1.ResourceList{
								corev1.ResourceName(gpuResource): resource.MustParse("2"),
							},
						},
					},
					{
						Resources: corev1.ResourceRequirements{},
					},
				},
			},
		}

		result := GetGpuAllocated(pod, gpuResource)
		assert.Equal(t, 2, result)
	})
}

func TestGetCompeletedAt(t *testing.T) {
	now := metav1.Now()

	t.Run("Pod已完成", func(t *testing.T) {
		pod := &corev1.Pod{
			Status: corev1.PodStatus{
				Conditions: []corev1.PodCondition{
					{
						Type:               corev1.PodReady,
						Status:             corev1.ConditionFalse,
						Reason:             "PodCompleted",
						LastTransitionTime: now,
					},
				},
			},
		}

		result := GetCompeletedAt(pod)
		assert.Equal(t, now.Time, result)
	})

	t.Run("Pod未完成", func(t *testing.T) {
		pod := &corev1.Pod{
			Status: corev1.PodStatus{
				Conditions: []corev1.PodCondition{
					{
						Type:   corev1.PodReady,
						Status: corev1.ConditionTrue,
					},
				},
			},
		}

		result := GetCompeletedAt(pod)
		assert.True(t, result.IsZero())
	})

	t.Run("Pod失败但不是完成状态", func(t *testing.T) {
		pod := &corev1.Pod{
			Status: corev1.PodStatus{
				Conditions: []corev1.PodCondition{
					{
						Type:   corev1.PodReady,
						Status: corev1.ConditionFalse,
						Reason: "ContainersNotReady",
					},
				},
			},
		}

		result := GetCompeletedAt(pod)
		assert.True(t, result.IsZero())
	})

	t.Run("无条件", func(t *testing.T) {
		pod := &corev1.Pod{
			Status: corev1.PodStatus{
				Conditions: []corev1.PodCondition{},
			},
		}

		result := GetCompeletedAt(pod)
		assert.True(t, result.IsZero())
	})

	t.Run("多个条件但只有一个是完成", func(t *testing.T) {
		pod := &corev1.Pod{
			Status: corev1.PodStatus{
				Conditions: []corev1.PodCondition{
					{
						Type:   corev1.PodScheduled,
						Status: corev1.ConditionTrue,
					},
					{
						Type:               corev1.PodReady,
						Status:             corev1.ConditionFalse,
						Reason:             "PodCompleted",
						LastTransitionTime: now,
					},
				},
			},
		}

		result := GetCompeletedAt(pod)
		assert.Equal(t, now.Time, result)
	})
}

func TestPodIntegration(t *testing.T) {
	t.Run("完整的Pod生命周期", func(t *testing.T) {
		gpuResource := "nvidia.com/gpu"

		// 创建一个带GPU的Pod
		pod := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-gpu-pod",
				Namespace: "default",
			},
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{
					{
						Name:  "main",
						Image: "nvidia/cuda:latest",
						Resources: corev1.ResourceRequirements{
							Requests: corev1.ResourceList{
								corev1.ResourceCPU:               resource.MustParse("2"),
								corev1.ResourceMemory:            resource.MustParse("4Gi"),
								corev1.ResourceName(gpuResource): resource.MustParse("2"),
							},
							Limits: corev1.ResourceList{
								corev1.ResourceCPU:               resource.MustParse("4"),
								corev1.ResourceMemory:            resource.MustParse("8Gi"),
								corev1.ResourceName(gpuResource): resource.MustParse("2"),
							},
						},
					},
				},
			},
			Status: corev1.PodStatus{
				Phase: corev1.PodRunning,
				Conditions: []corev1.PodCondition{
					{
						Type:   corev1.PodReady,
						Status: corev1.ConditionTrue,
					},
				},
			},
		}

		// 验证GPU
		assert.True(t, HasGPU(pod, gpuResource))
		assert.Equal(t, 2, GetGpuAllocated(pod, gpuResource))

		// 验证运行状态
		assert.True(t, IsPodRunning(pod))
		assert.False(t, IsPodDone(pod))

		// 模拟Pod完成
		pod.Status.Phase = corev1.PodSucceeded
		pod.Status.Conditions = []corev1.PodCondition{
			{
				Type:               corev1.PodReady,
				Status:             corev1.ConditionFalse,
				Reason:             "PodCompleted",
				LastTransitionTime: metav1.NewTime(time.Now()),
			},
		}

		assert.False(t, IsPodRunning(pod))
		assert.True(t, IsPodDone(pod))
		assert.False(t, GetCompeletedAt(pod).IsZero())
	})
}

