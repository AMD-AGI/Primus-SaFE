package workload_statistic

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewHistogram(t *testing.T) {
	hist := NewHistogram()
	
	assert.NotNil(t, hist)
	assert.Equal(t, 10, len(hist.Buckets), "Should have 10 buckets (0-10, 10-20, ..., 90-100)")
	
	// 验证桶范围
	for i := 0; i < 10; i++ {
		assert.Equal(t, float64(i*10), hist.Buckets[i].Lower)
		assert.Equal(t, float64((i+1)*10), hist.Buckets[i].Upper)
		assert.Equal(t, 0, hist.Buckets[i].Count, "Initial count should be 0")
	}
}

func TestHistogramAddValues(t *testing.T) {
	hist := NewHistogram()
	
	// 添加一些测试值
	values := []float64{5, 15, 25, 35, 45, 55, 65, 75, 85, 95}
	hist.AddValues(values)
	
	// 每个桶应该有1个值
	for i := 0; i < 10; i++ {
		assert.Equal(t, 1, hist.Buckets[i].Count, "Each bucket should have 1 value")
	}
	
	// 测试边界值
	hist2 := NewHistogram()
	hist2.AddValues([]float64{0, 10, 20, 30, 40, 50, 60, 70, 80, 90, 100})
	
	// 0 应该在第一个桶，100 应该在最后一个桶
	assert.Greater(t, hist2.Buckets[0].Count, 0, "First bucket should have values")
	assert.Greater(t, hist2.Buckets[9].Count, 0, "Last bucket should have values")
}

func TestHistogramCalculatePercentile(t *testing.T) {
	hist := NewHistogram()
	
	// 创建一个均匀分布
	values := make([]float64, 100)
	for i := 0; i < 100; i++ {
		values[i] = float64(i)
	}
	hist.AddValues(values)
	
	tests := []struct {
		name       string
		percentile float64
		expected   float64
		delta      float64
	}{
		{"P50", 50, 50, 5},
		{"P90", 90, 90, 5},
		{"P95", 95, 95, 5},
		{"P99", 99, 99, 5},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := hist.CalculatePercentile(tt.percentile)
			assert.InDelta(t, tt.expected, result, tt.delta, 
				"Percentile %.0f should be around %.1f", tt.percentile, tt.expected)
		})
	}
}

func TestHistogramJSON(t *testing.T) {
	hist := NewHistogram()
	hist.AddValues([]float64{25, 50, 75})
	
	// 转换为 JSON
	jsonData, err := hist.ToJSON()
	assert.NoError(t, err)
	assert.NotEmpty(t, jsonData)
	
	// 从 JSON 恢复
	hist2, err := FromJSON(jsonData)
	assert.NoError(t, err)
	assert.NotNil(t, hist2)
	assert.Equal(t, len(hist.Buckets), len(hist2.Buckets))
	
	// 验证数据一致
	for i := range hist.Buckets {
		assert.Equal(t, hist.Buckets[i].Count, hist2.Buckets[i].Count)
	}
}

func TestHistogramEmptyJSON(t *testing.T) {
	// 测试空 JSON
	hist, err := FromJSON([]byte{})
	assert.NoError(t, err)
	assert.NotNil(t, hist)
	assert.Equal(t, 10, len(hist.Buckets))
}

func TestHistogramGetTotalCount(t *testing.T) {
	hist := NewHistogram()
	assert.Equal(t, 0, hist.GetTotalCount())
	
	hist.AddValues([]float64{10, 20, 30})
	assert.Equal(t, 3, hist.GetTotalCount())
	
	hist.AddValues([]float64{40, 50})
	assert.Equal(t, 5, hist.GetTotalCount())
}

func TestCalculatePercentilesFromHistogram(t *testing.T) {
	hist := NewHistogram()
	
	// 创建测试数据
	values := []float64{10, 20, 30, 40, 50, 60, 70, 80, 90, 95}
	hist.AddValues(values)
	
	p50, p90, p95 := calculatePercentilesFromHistogram(hist)
	
	// 验证结果在合理范围内
	assert.InDelta(t, 50, p50, 15, "P50 should be around 50")
	assert.InDelta(t, 90, p90, 10, "P90 should be around 90")
	assert.InDelta(t, 95, p95, 10, "P95 should be around 95")
}

func TestCalculatePercentilesFromValues(t *testing.T) {
	values := []float64{10, 20, 30, 40, 50, 60, 70, 80, 90, 100}
	
	p50, p90, p95 := calculatePercentilesFromValues(values)
	
	assert.InDelta(t, 55, p50, 1, "P50 should be around 55")
	assert.InDelta(t, 91, p90, 1, "P90 should be around 91")
	assert.InDelta(t, 95.5, p95, 1, "P95 should be around 95.5")
}

func TestHistogramEdgeCases(t *testing.T) {
	t.Run("Empty histogram percentile", func(t *testing.T) {
		hist := NewHistogram()
		p50 := hist.CalculatePercentile(50)
		assert.Equal(t, 0.0, p50)
	})
	
	t.Run("Single value", func(t *testing.T) {
		hist := NewHistogram()
		hist.AddValues([]float64{42})
		
		p50 := hist.CalculatePercentile(50)
		assert.InDelta(t, 42, p50, 5)
	})
	
	t.Run("Values outside range", func(t *testing.T) {
		hist := NewHistogram()
		hist.AddValues([]float64{-10, 150}) // 应该被裁剪到 0-100
		
		// 验证没有崩溃，并且值被正确处理
		total := hist.GetTotalCount()
		assert.Equal(t, 2, total)
	})
}

func TestHistogramIncrementalUpdate(t *testing.T) {
	hist := NewHistogram()
	
	// 第一次添加数据
	hist.AddValues([]float64{10, 20, 30, 40, 50})
	count1 := hist.GetTotalCount()
	p50_1 := hist.CalculatePercentile(50)
	
	// 第二次添加数据（增量更新）
	hist.AddValues([]float64{60, 70, 80, 90})
	count2 := hist.GetTotalCount()
	p50_2 := hist.CalculatePercentile(50)
	
	assert.Equal(t, 5, count1)
	assert.Equal(t, 9, count2)
	assert.NotEqual(t, p50_1, p50_2, "P50 should change after adding more data")
}

