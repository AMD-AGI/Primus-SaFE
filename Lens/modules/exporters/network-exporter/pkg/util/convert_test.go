package util

import (
	"math"
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestConvertToFloat64(t *testing.T) {
	tests := []struct {
		name          string
		value         interface{}
		expectedFloat float64
		expectedOk    bool
	}{
		// 整数类型
		{
			name:          "int类型-正数",
			value:         int(42),
			expectedFloat: 42.0,
			expectedOk:    true,
		},
		{
			name:          "int类型-负数",
			value:         int(-100),
			expectedFloat: -100.0,
			expectedOk:    true,
		},
		{
			name:          "int类型-零",
			value:         int(0),
			expectedFloat: 0.0,
			expectedOk:    true,
		},
		{
			name:          "int8类型",
			value:         int8(127),
			expectedFloat: 127.0,
			expectedOk:    true,
		},
		{
			name:          "int8类型-最小值",
			value:         int8(-128),
			expectedFloat: -128.0,
			expectedOk:    true,
		},
		{
			name:          "int16类型",
			value:         int16(32767),
			expectedFloat: 32767.0,
			expectedOk:    true,
		},
		{
			name:          "int16类型-负数",
			value:         int16(-32768),
			expectedFloat: -32768.0,
			expectedOk:    true,
		},
		{
			name:          "int32类型",
			value:         int32(2147483647),
			expectedFloat: 2147483647.0,
			expectedOk:    true,
		},
		{
			name:          "int64类型-大数",
			value:         int64(9223372036854775807),
			expectedFloat: 9223372036854775807.0,
			expectedOk:    true,
		},
		
		// 无符号整数类型
		{
			name:          "uint类型",
			value:         uint(42),
			expectedFloat: 42.0,
			expectedOk:    true,
		},
		{
			name:          "uint8类型",
			value:         uint8(255),
			expectedFloat: 255.0,
			expectedOk:    true,
		},
		{
			name:          "uint16类型",
			value:         uint16(65535),
			expectedFloat: 65535.0,
			expectedOk:    true,
		},
		{
			name:          "uint32类型",
			value:         uint32(4294967295),
			expectedFloat: 4294967295.0,
			expectedOk:    true,
		},
		{
			name:          "uint64类型-大数",
			value:         uint64(18446744073709551615),
			expectedFloat: 18446744073709551615.0,
			expectedOk:    true,
		},
		
		// 浮点数类型
		{
			name:          "float32类型",
			value:         float32(3.14),
			expectedFloat: 3.140000104904175, // float32 精度损失
			expectedOk:    true,
		},
		{
			name:          "float32类型-负数",
			value:         float32(-2.718),
			expectedFloat: -2.7179999351501465,
			expectedOk:    true,
		},
		{
			name:          "float64类型",
			value:         float64(3.141592653589793),
			expectedFloat: 3.141592653589793,
			expectedOk:    true,
		},
		{
			name:          "float64类型-负数",
			value:         float64(-2.718281828459045),
			expectedFloat: -2.718281828459045,
			expectedOk:    true,
		},
		{
			name:          "float64类型-零",
			value:         float64(0.0),
			expectedFloat: 0.0,
			expectedOk:    true,
		},
		{
			name:          "float64类型-NaN",
			value:         math.NaN(),
			expectedFloat: math.NaN(),
			expectedOk:    true,
		},
		{
			name:          "float64类型-正无穷",
			value:         math.Inf(1),
			expectedFloat: math.Inf(1),
			expectedOk:    true,
		},
		{
			name:          "float64类型-负无穷",
			value:         math.Inf(-1),
			expectedFloat: math.Inf(-1),
			expectedOk:    true,
		},
		
		// 非数值类型 - 应该返回 false
		{
			name:          "string类型-不可转换",
			value:         "123",
			expectedFloat: 0,
			expectedOk:    false,
		},
		{
			name:          "bool类型-不可转换",
			value:         true,
			expectedFloat: 0,
			expectedOk:    false,
		},
		{
			name:          "slice类型-不可转换",
			value:         []int{1, 2, 3},
			expectedFloat: 0,
			expectedOk:    false,
		},
		{
			name:          "map类型-不可转换",
			value:         map[string]int{"a": 1},
			expectedFloat: 0,
			expectedOk:    false,
		},
		{
			name:          "struct类型-不可转换",
			value:         struct{ X int }{X: 42},
			expectedFloat: 0,
			expectedOk:    false,
		},
		{
			name:          "指针类型-不可转换",
			value:         new(int),
			expectedFloat: 0,
			expectedOk:    false,
		},
		{
			name:          "chan类型-不可转换",
			value:         make(chan int),
			expectedFloat: 0,
			expectedOk:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			value := reflect.ValueOf(tt.value)
			result, ok := ConvertToFloat64(value)
			
			assert.Equal(t, tt.expectedOk, ok, "转换结果的 ok 状态不匹配")
			
			if tt.expectedOk {
				if math.IsNaN(tt.expectedFloat) {
					assert.True(t, math.IsNaN(result), "应该是 NaN")
				} else if math.IsInf(tt.expectedFloat, 0) {
					assert.True(t, math.IsInf(result, int(math.Copysign(1, tt.expectedFloat))), "应该是无穷大")
				} else {
					// 对于 float32 的精度问题，使用 InDelta
					assert.InDelta(t, tt.expectedFloat, result, 0.0001, "转换结果的值不匹配")
				}
			} else {
				assert.Equal(t, 0.0, result, "非数值类型应该返回 0")
			}
		})
	}
}

func TestConvertToFloat64_EdgeCases(t *testing.T) {
	t.Run("int64最大值", func(t *testing.T) {
		maxInt64 := int64(9223372036854775807)
		value := reflect.ValueOf(maxInt64)
		result, ok := ConvertToFloat64(value)
		assert.True(t, ok)
		assert.Equal(t, float64(maxInt64), result)
	})

	t.Run("int64最小值", func(t *testing.T) {
		minInt64 := int64(-9223372036854775808)
		value := reflect.ValueOf(minInt64)
		result, ok := ConvertToFloat64(value)
		assert.True(t, ok)
		assert.Equal(t, float64(minInt64), result)
	})

	t.Run("uint64最大值", func(t *testing.T) {
		maxUint64 := uint64(18446744073709551615)
		value := reflect.ValueOf(maxUint64)
		result, ok := ConvertToFloat64(value)
		assert.True(t, ok)
		assert.Equal(t, float64(maxUint64), result)
	})
	
	t.Run("零值reflect.Value", func(t *testing.T) {
		var zeroValue reflect.Value
		result, ok := ConvertToFloat64(zeroValue)
		assert.False(t, ok)
		assert.Equal(t, 0.0, result)
	})
}

