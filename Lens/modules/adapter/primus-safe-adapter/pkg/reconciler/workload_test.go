package reconciler

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	primusSafeV1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
)

func TestCalculateGpuRequest(t *testing.T) {
	r := &WorkloadReconciler{}
	ctx := context.Background()

	tests := []struct {
		name     string
		workload *primusSafeV1.Workload
		expected int32
	}{
		{
			name: "正常计算-单副本单GPU",
			workload: &primusSafeV1.Workload{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-workload",
					Namespace: "default",
				},
				Spec: primusSafeV1.WorkloadSpec{
					Resource: primusSafeV1.WorkloadResource{
						GPU:     "1",
						Replica: 1,
					},
				},
			},
			expected: 1,
		},
		{
			name: "正常计算-多副本多GPU",
			workload: &primusSafeV1.Workload{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-workload",
					Namespace: "default",
				},
				Spec: primusSafeV1.WorkloadSpec{
					Resource: primusSafeV1.WorkloadResource{
						GPU:     "4",
						Replica: 8,
					},
				},
			},
			expected: 32,
		},
		{
			name: "零副本",
			workload: &primusSafeV1.Workload{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-workload",
					Namespace: "default",
				},
				Spec: primusSafeV1.WorkloadSpec{
					Resource: primusSafeV1.WorkloadResource{
						GPU:     "2",
						Replica: 0,
					},
				},
			},
			expected: 0,
		},
		{
			name: "GPU数量为0",
			workload: &primusSafeV1.Workload{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-workload",
					Namespace: "default",
				},
				Spec: primusSafeV1.WorkloadSpec{
					Resource: primusSafeV1.WorkloadResource{
						GPU:     "0",
						Replica: 5,
					},
				},
			},
			expected: 0,
		},
		{
			name: "无效的GPU字符串-空字符串",
			workload: &primusSafeV1.Workload{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-workload",
					Namespace: "default",
				},
				Spec: primusSafeV1.WorkloadSpec{
					Resource: primusSafeV1.WorkloadResource{
						GPU:     "",
						Replica: 3,
					},
				},
			},
			expected: 0,
		},
		{
			name: "无效的GPU字符串-非数字",
			workload: &primusSafeV1.Workload{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-workload",
					Namespace: "default",
				},
				Spec: primusSafeV1.WorkloadSpec{
					Resource: primusSafeV1.WorkloadResource{
						GPU:     "invalid",
						Replica: 3,
					},
				},
			},
			expected: 0,
		},
		{
			name: "大数值计算",
			workload: &primusSafeV1.Workload{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-workload",
					Namespace: "default",
				},
				Spec: primusSafeV1.WorkloadSpec{
					Resource: primusSafeV1.WorkloadResource{
						GPU:     "8",
						Replica: 100,
					},
				},
			},
			expected: 800,
		},
		{
			name: "负数GPU字符串",
			workload: &primusSafeV1.Workload{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-workload",
					Namespace: "default",
				},
				Spec: primusSafeV1.WorkloadSpec{
					Resource: primusSafeV1.WorkloadResource{
						GPU:     "-2",
						Replica: 4,
					},
				},
			},
			expected: -8,
		},
		{
			name: "小数GPU字符串-应该失败",
			workload: &primusSafeV1.Workload{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-workload",
					Namespace: "default",
				},
				Spec: primusSafeV1.WorkloadSpec{
					Resource: primusSafeV1.WorkloadResource{
						GPU:     "2.5",
						Replica: 4,
					},
				},
			},
			expected: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := r.calculateGpuRequest(ctx, tt.workload)
			assert.Equal(t, tt.expected, result, "GPU request calculation mismatch")
		})
	}
}

