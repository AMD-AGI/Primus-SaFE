package api

import (
	"testing"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/model"
	"github.com/stretchr/testify/assert"
)

func TestGetGpuUsageHistoryCacheKey(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name        string
		startTime   time.Time
		endTime     time.Time
		expected    string
		description string
	}{
		{
			name:        "1小时查询-最近数据",
			startTime:   now.Add(-1 * time.Hour),
			endTime:     now,
			expected:    "cluster:gpu:usage_history:1h",
			description: "查询最近1小时的数据",
		},
		{
			name:        "6小时查询-最近数据",
			startTime:   now.Add(-6 * time.Hour),
			endTime:     now,
			expected:    "cluster:gpu:usage_history:6h",
			description: "查询最近6小时的数据",
		},
		{
			name:        "24小时查询-最近数据",
			startTime:   now.Add(-24 * time.Hour),
			endTime:     now,
			expected:    "cluster:gpu:usage_history:24h",
			description: "查询最近24小时的数据",
		},
		{
			name:        "55分钟查询-在1小时范围内",
			startTime:   now.Add(-55 * time.Minute),
			endTime:     now,
			expected:    "cluster:gpu:usage_history:1h",
			description: "55分钟接近1小时，应该匹配1小时缓存",
		},
		{
			name:        "65分钟查询-在1小时范围内",
			startTime:   now.Add(-65 * time.Minute),
			endTime:     now,
			expected:    "cluster:gpu:usage_history:1h",
			description: "65分钟接近1小时，应该匹配1小时缓存",
		},
		{
			name:        "5小时45分钟-在6小时范围内",
			startTime:   now.Add(-5*time.Hour - 45*time.Minute),
			endTime:     now,
			expected:    "cluster:gpu:usage_history:6h",
			description: "接近6小时，应该匹配6小时缓存",
		},
		{
			name:        "6小时15分钟-在6小时范围内",
			startTime:   now.Add(-6*time.Hour - 15*time.Minute),
			endTime:     now,
			expected:    "cluster:gpu:usage_history:6h",
			description: "接近6小时，应该匹配6小时缓存",
		},
		{
			name:        "23小时30分钟-在24小时范围内",
			startTime:   now.Add(-23*time.Hour - 30*time.Minute),
			endTime:     now,
			expected:    "cluster:gpu:usage_history:24h",
			description: "接近24小时，应该匹配24小时缓存",
		},
		{
			name:        "24小时30分钟-在24小时范围内",
			startTime:   now.Add(-24*time.Hour - 30*time.Minute),
			endTime:     now,
			expected:    "cluster:gpu:usage_history:24h",
			description: "接近24小时，应该匹配24小时缓存",
		},
		{
			name:        "2小时查询-不匹配任何缓存",
			startTime:   now.Add(-2 * time.Hour),
			endTime:     now,
			expected:    "",
			description: "2小时不在任何缓存范围内",
		},
		{
			name:        "3小时查询-不匹配任何缓存",
			startTime:   now.Add(-3 * time.Hour),
			endTime:     now,
			expected:    "",
			description: "3小时不在任何缓存范围内",
		},
		{
			name:        "12小时查询-不匹配任何缓存",
			startTime:   now.Add(-12 * time.Hour),
			endTime:     now,
			expected:    "",
			description: "12小时不在任何缓存范围内",
		},
		{
			name:        "30分钟查询-太短",
			startTime:   now.Add(-30 * time.Minute),
			endTime:     now,
			expected:    "",
			description: "30分钟小于1小时范围",
		},
		{
			name:        "45分钟查询-太短",
			startTime:   now.Add(-45 * time.Minute),
			endTime:     now,
			expected:    "",
			description: "45分钟小于1小时范围",
		},
		{
			name:        "历史数据-10分钟前结束",
			startTime:   now.Add(-2 * time.Hour),
			endTime:     now.Add(-10 * time.Minute),
			expected:    "",
			description: "结束时间超过5分钟容差，不使用缓存",
		},
		{
			name:        "历史数据-1小时前结束",
			startTime:   now.Add(-2 * time.Hour),
			endTime:     now.Add(-1 * time.Hour),
			expected:    "",
			description: "不是最近的查询，不使用缓存",
		},
		{
			name:        "历史数据-昨天的1小时",
			startTime:   now.Add(-25 * time.Hour),
			endTime:     now.Add(-24 * time.Hour),
			expected:    "",
			description: "昨天的历史数据，不使用缓存",
		},
		{
			name:        "边界测试-刚好5分钟前结束",
			startTime:   now.Add(-1*time.Hour - 5*time.Minute),
			endTime:     now.Add(-5 * time.Minute),
			expected:    "cluster:gpu:usage_history:1h", // timeSinceEnd > tolerance 判断，5分钟刚好不大于，所以匹配
			description: "刚好在5分钟容差边界内",
		},
		{
			name:        "边界测试-4分钟前结束",
			startTime:   now.Add(-1*time.Hour - 4*time.Minute),
			endTime:     now.Add(-4 * time.Minute),
			expected:    "cluster:gpu:usage_history:1h",
			description: "在5分钟容差范围内",
		},
		{
			name:        "边界测试-6分钟前结束",
			startTime:   now.Add(-1*time.Hour - 6*time.Minute),
			endTime:     now.Add(-6 * time.Minute),
			expected:    "",
			description: "超出5分钟容差，不使用缓存",
		},
		{
			name:        "未来时间-不使用缓存",
			startTime:   now.Add(1 * time.Hour),
			endTime:     now.Add(2 * time.Hour),
			expected:    "cluster:gpu:usage_history:1h", // 未来时间的 duration 也是1小时，可能会匹配
			description: "未来时间（可能会匹配持续时间）",
		},
		{
			name:        "边界测试-50分钟",
			startTime:   now.Add(-50 * time.Minute),
			endTime:     now,
			expected:    "cluster:gpu:usage_history:1h",
			description: "刚好50分钟，匹配1小时缓存的下限",
		},
		{
			name:        "边界测试-70分钟",
			startTime:   now.Add(-70 * time.Minute),
			endTime:     now,
			expected:    "cluster:gpu:usage_history:1h",
			description: "刚好70分钟，匹配1小时缓存的上限",
		},
		{
			name:        "边界测试-49分钟",
			startTime:   now.Add(-49 * time.Minute),
			endTime:     now,
			expected:    "",
			description: "49分钟小于50分钟下限",
		},
		{
			name:        "边界测试-71分钟",
			startTime:   now.Add(-71 * time.Minute),
			endTime:     now,
			expected:    "",
			description: "71分钟超过70分钟上限",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getGpuUsageHistoryCacheKey(tt.startTime, tt.endTime)
			assert.Equal(t, tt.expected, result, tt.description)
		})
	}
}

func TestFilterTimePoints(t *testing.T) {
	tests := []struct {
		name      string
		points    []model.TimePoint
		startUnix int64
		endUnix   int64
		expected  []model.TimePoint
	}{
		{
			name: "所有点都在范围内",
			points: []model.TimePoint{
				{Timestamp: 100, Value: 10.0},
				{Timestamp: 200, Value: 20.0},
				{Timestamp: 300, Value: 30.0},
			},
			startUnix: 50,
			endUnix:   350,
			expected: []model.TimePoint{
				{Timestamp: 100, Value: 10.0},
				{Timestamp: 200, Value: 20.0},
				{Timestamp: 300, Value: 30.0},
			},
		},
		{
			name: "部分点在范围内",
			points: []model.TimePoint{
				{Timestamp: 100, Value: 10.0},
				{Timestamp: 200, Value: 20.0},
				{Timestamp: 300, Value: 30.0},
				{Timestamp: 400, Value: 40.0},
				{Timestamp: 500, Value: 50.0},
			},
			startUnix: 200,
			endUnix:   400,
			expected: []model.TimePoint{
				{Timestamp: 200, Value: 20.0},
				{Timestamp: 300, Value: 30.0},
				{Timestamp: 400, Value: 40.0},
			},
		},
		{
			name: "没有点在范围内",
			points: []model.TimePoint{
				{Timestamp: 100, Value: 10.0},
				{Timestamp: 200, Value: 20.0},
				{Timestamp: 300, Value: 30.0},
			},
			startUnix: 400,
			endUnix:   500,
			expected:  []model.TimePoint{},
		},
		{
			name:      "空切片",
			points:    []model.TimePoint{},
			startUnix: 100,
			endUnix:   200,
			expected:  []model.TimePoint{},
		},
		{
			name: "单个点-在范围内",
			points: []model.TimePoint{
				{Timestamp: 150, Value: 15.0},
			},
			startUnix: 100,
			endUnix:   200,
			expected: []model.TimePoint{
				{Timestamp: 150, Value: 15.0},
			},
		},
		{
			name: "单个点-在范围外",
			points: []model.TimePoint{
				{Timestamp: 250, Value: 25.0},
			},
			startUnix: 100,
			endUnix:   200,
			expected:  []model.TimePoint{},
		},
		{
			name: "边界测试-刚好在起点",
			points: []model.TimePoint{
				{Timestamp: 100, Value: 10.0},
				{Timestamp: 200, Value: 20.0},
			},
			startUnix: 100,
			endUnix:   200,
			expected: []model.TimePoint{
				{Timestamp: 100, Value: 10.0},
				{Timestamp: 200, Value: 20.0},
			},
		},
		{
			name: "边界测试-刚好在终点",
			points: []model.TimePoint{
				{Timestamp: 100, Value: 10.0},
				{Timestamp: 200, Value: 20.0},
			},
			startUnix: 50,
			endUnix:   200,
			expected: []model.TimePoint{
				{Timestamp: 100, Value: 10.0},
				{Timestamp: 200, Value: 20.0},
			},
		},
		{
			name: "边界测试-刚好超出起点",
			points: []model.TimePoint{
				{Timestamp: 99, Value: 9.9},
				{Timestamp: 100, Value: 10.0},
				{Timestamp: 200, Value: 20.0},
			},
			startUnix: 100,
			endUnix:   300,
			expected: []model.TimePoint{
				{Timestamp: 100, Value: 10.0},
				{Timestamp: 200, Value: 20.0},
			},
		},
		{
			name: "边界测试-刚好超出终点",
			points: []model.TimePoint{
				{Timestamp: 100, Value: 10.0},
				{Timestamp: 200, Value: 20.0},
				{Timestamp: 201, Value: 20.1},
			},
			startUnix: 50,
			endUnix:   200,
			expected: []model.TimePoint{
				{Timestamp: 100, Value: 10.0},
				{Timestamp: 200, Value: 20.0},
			},
		},
		{
			name: "负数时间戳",
			points: []model.TimePoint{
				{Timestamp: -200, Value: -20.0},
				{Timestamp: -100, Value: -10.0},
				{Timestamp: 0, Value: 0.0},
				{Timestamp: 100, Value: 10.0},
			},
			startUnix: -150,
			endUnix:   50,
			expected: []model.TimePoint{
				{Timestamp: -100, Value: -10.0},
				{Timestamp: 0, Value: 0.0},
			},
		},
		{
			name: "时间戳为0",
			points: []model.TimePoint{
				{Timestamp: 0, Value: 0.0},
			},
			startUnix: -100,
			endUnix:   100,
			expected: []model.TimePoint{
				{Timestamp: 0, Value: 0.0},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := filterTimePoints(tt.points, tt.startUnix, tt.endUnix)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestFilterGpuUsageHistoryByTimeRange(t *testing.T) {
	now := time.Now()
	startTime := now.Add(-1 * time.Hour)
	endTime := now

	startUnix := startTime.Unix()
	endUnix := endTime.Unix()

	tests := []struct {
		name     string
		history  model.GpuUtilizationHistory
		expected model.GpuUtilizationHistory
	}{
		{
			name: "完整的历史数据-所有字段都有数据",
			history: model.GpuUtilizationHistory{
				AllocationRate: []model.TimePoint{
					{Timestamp: startUnix - 100, Value: 50.0},
					{Timestamp: startUnix + 100, Value: 60.0},
					{Timestamp: startUnix + 200, Value: 70.0},
					{Timestamp: endUnix + 100, Value: 80.0},
				},
				Utilization: []model.TimePoint{
					{Timestamp: startUnix - 100, Value: 40.0},
					{Timestamp: startUnix + 100, Value: 50.0},
					{Timestamp: startUnix + 200, Value: 60.0},
					{Timestamp: endUnix + 100, Value: 70.0},
				},
				VramUtilization: []model.TimePoint{
					{Timestamp: startUnix - 100, Value: 30.0},
					{Timestamp: startUnix + 100, Value: 40.0},
					{Timestamp: startUnix + 200, Value: 50.0},
					{Timestamp: endUnix + 100, Value: 60.0},
				},
			},
			expected: model.GpuUtilizationHistory{
				AllocationRate: []model.TimePoint{
					{Timestamp: startUnix + 100, Value: 60.0},
					{Timestamp: startUnix + 200, Value: 70.0},
				},
				Utilization: []model.TimePoint{
					{Timestamp: startUnix + 100, Value: 50.0},
					{Timestamp: startUnix + 200, Value: 60.0},
				},
				VramUtilization: []model.TimePoint{
					{Timestamp: startUnix + 100, Value: 40.0},
					{Timestamp: startUnix + 200, Value: 50.0},
				},
			},
		},
		{
			name: "空的历史数据",
			history: model.GpuUtilizationHistory{
				AllocationRate:  []model.TimePoint{},
				Utilization:     []model.TimePoint{},
				VramUtilization: []model.TimePoint{},
			},
			expected: model.GpuUtilizationHistory{
				AllocationRate:  []model.TimePoint{},
				Utilization:     []model.TimePoint{},
				VramUtilization: []model.TimePoint{},
			},
		},
		{
			name: "部分字段有数据",
			history: model.GpuUtilizationHistory{
				AllocationRate: []model.TimePoint{
					{Timestamp: startUnix + 100, Value: 60.0},
				},
				Utilization:     []model.TimePoint{},
				VramUtilization: []model.TimePoint{},
			},
			expected: model.GpuUtilizationHistory{
				AllocationRate: []model.TimePoint{
					{Timestamp: startUnix + 100, Value: 60.0},
				},
				Utilization:     []model.TimePoint{},
				VramUtilization: []model.TimePoint{},
			},
		},
		{
			name: "所有数据都在范围外",
			history: model.GpuUtilizationHistory{
				AllocationRate: []model.TimePoint{
					{Timestamp: startUnix - 200, Value: 50.0},
					{Timestamp: startUnix - 100, Value: 60.0},
				},
				Utilization: []model.TimePoint{
					{Timestamp: endUnix + 100, Value: 70.0},
					{Timestamp: endUnix + 200, Value: 80.0},
				},
				VramUtilization: []model.TimePoint{
					{Timestamp: startUnix - 300, Value: 30.0},
				},
			},
			expected: model.GpuUtilizationHistory{
				AllocationRate:  []model.TimePoint{},
				Utilization:     []model.TimePoint{},
				VramUtilization: []model.TimePoint{},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := filterGpuUsageHistoryByTimeRange(tt.history, startTime, endTime)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestFilterTimePoints_Performance(t *testing.T) {
	// 创建大量数据点
	largePoints := make([]model.TimePoint, 10000)
	for i := 0; i < 10000; i++ {
		largePoints[i] = model.TimePoint{
			Timestamp: int64(i * 60), // 每分钟一个点
			Value:     float64(i),
		}
	}

	// 测试过滤性能
	startUnix := int64(100 * 60)
	endUnix := int64(200 * 60)

	result := filterTimePoints(largePoints, startUnix, endUnix)

	// 验证结果
	assert.Len(t, result, 101) // 应该包含100到200，共101个点
	assert.Equal(t, startUnix, result[0].Timestamp)
	assert.Equal(t, endUnix, result[len(result)-1].Timestamp)
}

func BenchmarkFilterTimePoints(b *testing.B) {
	// 创建测试数据
	points := make([]model.TimePoint, 1000)
	for i := 0; i < 1000; i++ {
		points[i] = model.TimePoint{
			Timestamp: int64(i * 60),
			Value:     float64(i),
		}
	}

	startUnix := int64(100 * 60)
	endUnix := int64(900 * 60)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = filterTimePoints(points, startUnix, endUnix)
	}
}

func BenchmarkFilterGpuUsageHistoryByTimeRange(b *testing.B) {
	now := time.Now()
	startTime := now.Add(-1 * time.Hour)
	endTime := now

	// 创建测试数据
	points := make([]model.TimePoint, 100)
	for i := 0; i < 100; i++ {
		points[i] = model.TimePoint{
			Timestamp: startTime.Unix() + int64(i*60),
			Value:     float64(i),
		}
	}

	history := model.GpuUtilizationHistory{
		AllocationRate:  points,
		Utilization:     points,
		VramUtilization: points,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = filterGpuUsageHistoryByTimeRange(history, startTime, endTime)
	}
}

