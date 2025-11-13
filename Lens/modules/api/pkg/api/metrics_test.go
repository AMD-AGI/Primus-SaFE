package api

import (
	"encoding/json"
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestConvertToFloat(t *testing.T) {
	tests := []struct {
		name     string
		input    any
		expected float64
	}{
		// float64 类型
		{
			name:     "float64类型-正数",
			input:    float64(123.45),
			expected: 123.45,
		},
		{
			name:     "float64类型-负数",
			input:    float64(-123.45),
			expected: -123.45,
		},
		{
			name:     "float64类型-零",
			input:    float64(0),
			expected: 0,
		},
		{
			name:     "float64类型-NaN",
			input:    math.NaN(),
			expected: math.NaN(),
		},
		{
			name:     "float64类型-正无穷",
			input:    math.Inf(1),
			expected: math.Inf(1),
		},
		{
			name:     "float64类型-负无穷",
			input:    math.Inf(-1),
			expected: math.Inf(-1),
		},

		// float32 类型
		{
			name:     "float32类型-正数",
			input:    float32(123.45),
			expected: float64(float32(123.45)),
		},
		{
			name:     "float32类型-负数",
			input:    float32(-123.45),
			expected: float64(float32(-123.45)),
		},

		// int 类型
		{
			name:     "int类型-正数",
			input:    int(123),
			expected: 123.0,
		},
		{
			name:     "int类型-负数",
			input:    int(-123),
			expected: -123.0,
		},
		{
			name:     "int类型-零",
			input:    int(0),
			expected: 0.0,
		},
		{
			name:     "int类型-最大值",
			input:    int(math.MaxInt32),
			expected: float64(math.MaxInt32),
		},

		// int8 类型
		{
			name:     "int8类型-正数",
			input:    int8(100),
			expected: 100.0,
		},
		{
			name:     "int8类型-负数",
			input:    int8(-100),
			expected: -100.0,
		},
		{
			name:     "int8类型-最大值",
			input:    int8(127),
			expected: 127.0,
		},
		{
			name:     "int8类型-最小值",
			input:    int8(-128),
			expected: -128.0,
		},

		// int16 类型
		{
			name:     "int16类型-正数",
			input:    int16(1000),
			expected: 1000.0,
		},
		{
			name:     "int16类型-负数",
			input:    int16(-1000),
			expected: -1000.0,
		},

		// int32 类型
		{
			name:     "int32类型-正数",
			input:    int32(100000),
			expected: 100000.0,
		},
		{
			name:     "int32类型-负数",
			input:    int32(-100000),
			expected: -100000.0,
		},

		// int64 类型
		{
			name:     "int64类型-正数",
			input:    int64(1000000000),
			expected: 1000000000.0,
		},
		{
			name:     "int64类型-负数",
			input:    int64(-1000000000),
			expected: -1000000000.0,
		},
		{
			name:     "int64类型-最大安全整数",
			input:    int64(9007199254740991), // 2^53 - 1
			expected: 9007199254740991.0,
		},

		// uint 类型
		{
			name:     "uint类型-正数",
			input:    uint(123),
			expected: 123.0,
		},
		{
			name:     "uint类型-零",
			input:    uint(0),
			expected: 0.0,
		},

		// uint8 类型
		{
			name:     "uint8类型-正数",
			input:    uint8(200),
			expected: 200.0,
		},
		{
			name:     "uint8类型-最大值",
			input:    uint8(255),
			expected: 255.0,
		},

		// uint16 类型
		{
			name:     "uint16类型-正数",
			input:    uint16(50000),
			expected: 50000.0,
		},
		{
			name:     "uint16类型-最大值",
			input:    uint16(65535),
			expected: 65535.0,
		},

		// uint32 类型
		{
			name:     "uint32类型-正数",
			input:    uint32(1000000),
			expected: 1000000.0,
		},

		// uint64 类型
		{
			name:     "uint64类型-正数",
			input:    uint64(1000000000000),
			expected: 1000000000000.0,
		},

		// json.Number 类型
		{
			name:     "json.Number类型-整数",
			input:    json.Number("123"),
			expected: 123.0,
		},
		{
			name:     "json.Number类型-浮点数",
			input:    json.Number("123.45"),
			expected: 123.45,
		},
		{
			name:     "json.Number类型-负数",
			input:    json.Number("-123.45"),
			expected: -123.45,
		},
		{
			name:     "json.Number类型-科学计数法",
			input:    json.Number("1.23e2"),
			expected: 123.0,
		},
		{
			name:     "json.Number类型-无效数字",
			input:    json.Number("invalid"),
			expected: 0.0,
		},

		// 不支持的类型
		{
			name:     "字符串类型",
			input:    "123",
			expected: 0.0,
		},
		{
			name:     "布尔类型-true",
			input:    true,
			expected: 0.0,
		},
		{
			name:     "布尔类型-false",
			input:    false,
			expected: 0.0,
		},
		{
			name:     "nil值",
			input:    nil,
			expected: 0.0,
		},
		{
			name:     "切片类型",
			input:    []int{1, 2, 3},
			expected: 0.0,
		},
		{
			name:     "map类型",
			input:    map[string]int{"a": 1},
			expected: 0.0,
		},
		{
			name:     "结构体类型",
			input:    struct{ Value int }{Value: 123},
			expected: 0.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := convertToFloat(tt.input)
			
			// 特殊处理 NaN 的情况
			if math.IsNaN(tt.expected) {
				assert.True(t, math.IsNaN(result), "Expected NaN but got %v", result)
			} else if math.IsInf(tt.expected, 1) {
				assert.True(t, math.IsInf(result, 1), "Expected +Inf but got %v", result)
			} else if math.IsInf(tt.expected, -1) {
				assert.True(t, math.IsInf(result, -1), "Expected -Inf but got %v", result)
			} else {
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}

func TestConvertToFloat_EdgeCases(t *testing.T) {
	t.Run("非常大的整数", func(t *testing.T) {
		// 测试大整数转换
		bigInt := int64(9223372036854775807) // int64 最大值
		result := convertToFloat(bigInt)
		assert.Equal(t, float64(bigInt), result)
	})

	t.Run("非常大的无符号整数", func(t *testing.T) {
		bigUint := uint64(18446744073709551615) // uint64 最大值
		result := convertToFloat(bigUint)
		assert.Equal(t, float64(bigUint), result)
	})

	t.Run("零值的各种表示", func(t *testing.T) {
		zeros := []any{
			int(0),
			int8(0),
			int16(0),
			int32(0),
			int64(0),
			uint(0),
			uint8(0),
			uint16(0),
			uint32(0),
			uint64(0),
			float32(0),
			float64(0),
			json.Number("0"),
		}

		for _, zero := range zeros {
			result := convertToFloat(zero)
			assert.Equal(t, 0.0, result, "Zero value of type %T should convert to 0.0", zero)
		}
	})

	t.Run("json.Number空字符串", func(t *testing.T) {
		result := convertToFloat(json.Number(""))
		assert.Equal(t, 0.0, result)
	})

	t.Run("json.Number特殊格式", func(t *testing.T) {
		tests := []struct {
			input    json.Number
			expected float64
		}{
			{json.Number("0.0"), 0.0},
			{json.Number("-0"), 0.0},
			{json.Number("1e10"), 1e10},
			{json.Number("1E10"), 1e10},
			{json.Number("1.23e-5"), 1.23e-5},
		}

		for _, tt := range tests {
			result := convertToFloat(tt.input)
			assert.Equal(t, tt.expected, result)
		}
	})
}

func TestConvertToFloat_TypePrecision(t *testing.T) {
	t.Run("float32精度损失", func(t *testing.T) {
		// float32 无法精确表示某些数字
		f32 := float32(1.23456789)
		result := convertToFloat(f32)
		// 结果应该是 float32 转 float64 后的值，会有精度差异
		assert.InDelta(t, float64(f32), result, 1e-6)
	})

	t.Run("大整数转浮点数精度", func(t *testing.T) {
		// 测试大整数转换时的精度
		largeInt := int64(1234567890123456)
		result := convertToFloat(largeInt)
		assert.Equal(t, float64(largeInt), result)
	})
}

func BenchmarkConvertToFloat(b *testing.B) {
	values := []any{
		int(123),
		int64(123456789),
		float64(123.45),
		json.Number("123.45"),
		"invalid", // 不支持的类型
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, v := range values {
			_ = convertToFloat(v)
		}
	}
}

