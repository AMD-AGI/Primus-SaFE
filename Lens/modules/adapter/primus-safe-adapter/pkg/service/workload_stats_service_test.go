package service

import (
	"testing"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	lensmodel "github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
	primusSafeV1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
)

func TestIsWorkloadRunning(t *testing.T) {
	s := &WorkloadStatsService{}

	tests := []struct {
		name     string
		workload *primusSafeV1.Workload
		expected bool
	}{
		{
			name: "Running状态",
			workload: &primusSafeV1.Workload{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-workload",
					Namespace: "default",
				},
				Status: primusSafeV1.WorkloadStatus{
					Phase: primusSafeV1.WorkloadRunning,
				},
			},
			expected: true,
		},
		{
			name: "Pending状态",
			workload: &primusSafeV1.Workload{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-workload",
					Namespace: "default",
				},
				Status: primusSafeV1.WorkloadStatus{
					Phase: primusSafeV1.WorkloadPending,
				},
			},
			expected: true,
		},
		{
			name: "Succeeded状态",
			workload: &primusSafeV1.Workload{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-workload",
					Namespace: "default",
				},
				Status: primusSafeV1.WorkloadStatus{
					Phase: primusSafeV1.WorkloadSucceeded,
				},
			},
			expected: false,
		},
		{
			name: "Failed状态",
			workload: &primusSafeV1.Workload{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-workload",
					Namespace: "default",
				},
				Status: primusSafeV1.WorkloadStatus{
					Phase: primusSafeV1.WorkloadFailed,
				},
			},
			expected: false,
		},
		{
			name: "空状态",
			workload: &primusSafeV1.Workload{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-workload",
					Namespace: "default",
				},
				Status: primusSafeV1.WorkloadStatus{
					Phase: "",
				},
			},
			expected: false,
		},
		{
			name: "未初始化的状态",
			workload: &primusSafeV1.Workload{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-workload",
					Namespace: "default",
				},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := s.isWorkloadRunning(tt.workload)
			assert.Equal(t, tt.expected, result, "Workload running status mismatch")
		})
	}
}

func TestCalculateUtilization(t *testing.T) {
	s := &WorkloadStatsService{}

	tests := []struct {
		name        string
		stats       []*lensmodel.WorkloadGpuHourlyStats
		expectedAvg float64
		expectedMax float64
	}{
		{
			name:        "空数据",
			stats:       []*lensmodel.WorkloadGpuHourlyStats{},
			expectedAvg: 0.0,
			expectedMax: 0.0,
		},
		{
			name: "单条数据",
			stats: []*lensmodel.WorkloadGpuHourlyStats{
				{
					AvgUtilization: 75.5,
					MaxUtilization: 90.0,
				},
			},
			expectedAvg: 75.5,
			expectedMax: 90.0,
		},
		{
			name: "多条数据-正常场景",
			stats: []*lensmodel.WorkloadGpuHourlyStats{
				{
					AvgUtilization: 50.0,
					MaxUtilization: 80.0,
				},
				{
					AvgUtilization: 60.0,
					MaxUtilization: 85.0,
				},
				{
					AvgUtilization: 70.0,
					MaxUtilization: 90.0,
				},
			},
			expectedAvg: 60.0, // (50 + 60 + 70) / 3
			expectedMax: 90.0,
		},
		{
			name: "所有数据相同",
			stats: []*lensmodel.WorkloadGpuHourlyStats{
				{
					AvgUtilization: 50.0,
					MaxUtilization: 50.0,
				},
				{
					AvgUtilization: 50.0,
					MaxUtilization: 50.0,
				},
				{
					AvgUtilization: 50.0,
					MaxUtilization: 50.0,
				},
			},
			expectedAvg: 50.0,
			expectedMax: 50.0,
		},
		{
			name: "包含零值",
			stats: []*lensmodel.WorkloadGpuHourlyStats{
				{
					AvgUtilization: 0.0,
					MaxUtilization: 0.0,
				},
				{
					AvgUtilization: 100.0,
					MaxUtilization: 100.0,
				},
			},
			expectedAvg: 50.0, // (0 + 100) / 2
			expectedMax: 100.0,
		},
		{
			name: "小数精度测试",
			stats: []*lensmodel.WorkloadGpuHourlyStats{
				{
					AvgUtilization: 33.33,
					MaxUtilization: 66.66,
				},
				{
					AvgUtilization: 66.67,
					MaxUtilization: 99.99,
				},
			},
			expectedAvg: 50.0, // (33.33 + 66.67) / 2
			expectedMax: 99.99,
		},
		{
			name: "大量数据",
			stats: func() []*lensmodel.WorkloadGpuHourlyStats {
				stats := make([]*lensmodel.WorkloadGpuHourlyStats, 100)
				for i := 0; i < 100; i++ {
					stats[i] = &lensmodel.WorkloadGpuHourlyStats{
						AvgUtilization: float64(i),
						MaxUtilization: float64(i * 2),
					}
				}
				return stats
			}(),
			expectedAvg: 49.5,  // (0 + 1 + 2 + ... + 99) / 100
			expectedMax: 198.0, // 99 * 2
		},
		{
			name: "最大值在中间",
			stats: []*lensmodel.WorkloadGpuHourlyStats{
				{
					AvgUtilization: 50.0,
					MaxUtilization: 60.0,
				},
				{
					AvgUtilization: 60.0,
					MaxUtilization: 95.0, // 最大值
				},
				{
					AvgUtilization: 40.0,
					MaxUtilization: 70.0,
				},
			},
			expectedAvg: 50.0, // (50 + 60 + 40) / 3
			expectedMax: 95.0,
		},
		{
			name: "边界值测试-100%利用率",
			stats: []*lensmodel.WorkloadGpuHourlyStats{
				{
					AvgUtilization: 100.0,
					MaxUtilization: 100.0,
				},
				{
					AvgUtilization: 100.0,
					MaxUtilization: 100.0,
				},
			},
			expectedAvg: 100.0,
			expectedMax: 100.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			avg, max := s.calculateUtilization(tt.stats)
			assert.InDelta(t, tt.expectedAvg, avg, 0.01, "Average utilization mismatch")
			assert.InDelta(t, tt.expectedMax, max, 0.01, "Max utilization mismatch")
		})
	}
}

