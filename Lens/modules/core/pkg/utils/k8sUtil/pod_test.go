// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

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
	t.Run("pod completed successfully", func(t *testing.T) {
		pod := &corev1.Pod{
			Status: corev1.PodStatus{
				Phase: corev1.PodSucceeded,
			},
		}

		result := IsPodDone(pod)
		assert.True(t, result)
	})

	t.Run("pod running", func(t *testing.T) {
		pod := &corev1.Pod{
			Status: corev1.PodStatus{
				Phase: corev1.PodRunning,
			},
		}

		result := IsPodDone(pod)
		assert.False(t, result)
	})

	t.Run("pod failed", func(t *testing.T) {
		pod := &corev1.Pod{
			Status: corev1.PodStatus{
				Phase: corev1.PodFailed,
			},
		}

		result := IsPodDone(pod)
		assert.False(t, result)
	})

	t.Run("pod pending", func(t *testing.T) {
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
	t.Run("pod running and ready", func(t *testing.T) {
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

	t.Run("pod running but not ready", func(t *testing.T) {
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

	t.Run("pod not running", func(t *testing.T) {
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

	t.Run("pod running but no ready condition", func(t *testing.T) {
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

	t.Run("pod requests GPU resources", func(t *testing.T) {
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

	t.Run("pod limits GPU resources", func(t *testing.T) {
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

	t.Run("pod has no GPU resources", func(t *testing.T) {
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

	t.Run("GPU resource is zero", func(t *testing.T) {
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

	t.Run("multiple containers with GPU", func(t *testing.T) {
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

	t.Run("single container single GPU", func(t *testing.T) {
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

	t.Run("single container multiple GPUs", func(t *testing.T) {
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

	t.Run("multiple containers accumulated GPUs", func(t *testing.T) {
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

	t.Run("no GPU", func(t *testing.T) {
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

	t.Run("partial containers with GPU", func(t *testing.T) {
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

	t.Run("pod completed", func(t *testing.T) {
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

	t.Run("pod not completed", func(t *testing.T) {
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

	t.Run("pod failed but not completed", func(t *testing.T) {
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

	t.Run("no conditions", func(t *testing.T) {
		pod := &corev1.Pod{
			Status: corev1.PodStatus{
				Conditions: []corev1.PodCondition{},
			},
		}

		result := GetCompeletedAt(pod)
		assert.True(t, result.IsZero())
	})

	t.Run("multiple conditions but only one is completed", func(t *testing.T) {
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
	t.Run("complete pod lifecycle", func(t *testing.T) {
		gpuResource := "nvidia.com/gpu"

		// create a pod with GPU
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

		// verify GPU
		assert.True(t, HasGPU(pod, gpuResource))
		assert.Equal(t, 2, GetGpuAllocated(pod, gpuResource))

		// verify running status
		assert.True(t, IsPodRunning(pod))
		assert.False(t, IsPodDone(pod))

		// simulate pod completion
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

