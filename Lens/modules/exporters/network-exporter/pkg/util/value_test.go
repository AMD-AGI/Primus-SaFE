package util

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCalculateMaxValue(t *testing.T) {
	tests := []struct {
		name     string
		bits     int
		expected uint64
	}{
		{
			name:     "1位-最大值为0",
			bits:     1,
			expected: 0, // 2^0 - 1 = 0
		},
		{
			name:     "2位-最大值为1",
			bits:     2,
			expected: 1, // 2^1 - 1 = 1
		},
		{
			name:     "8位-最大值为127",
			bits:     8,
			expected: 127, // 2^7 - 1 = 127
		},
		{
			name:     "16位-最大值为32767",
			bits:     16,
			expected: 32767, // 2^15 - 1 = 32767
		},
		{
			name:     "32位-最大值",
			bits:     32,
			expected: 2147483647, // 2^31 - 1
		},
		{
			name:     "64位-最大值",
			bits:     64,
			expected: 9223372036854775807, // 2^63 - 1
		},
		{
			name:     "4位-最大值为7",
			bits:     4,
			expected: 7, // 2^3 - 1 = 7
		},
		{
			name:     "10位-最大值为511",
			bits:     10,
			expected: 511, // 2^9 - 1 = 511
		},
		{
			name:     "20位-最大值",
			bits:     20,
			expected: 524287, // 2^19 - 1
		},
		
		// 边界值测试
		{
			name:     "边界值-最小有效位数1",
			bits:     1,
			expected: 0,
		},
		{
			name:     "边界值-最大有效位数64",
			bits:     64,
			expected: 9223372036854775807,
		},
		
		// 无效输入 - 应返回 0
		{
			name:     "无效输入-0位",
			bits:     0,
			expected: 0,
		},
		{
			name:     "无效输入-负数",
			bits:     -1,
			expected: 0,
		},
		{
			name:     "无效输入-负数大值",
			bits:     -100,
			expected: 0,
		},
		{
			name:     "无效输入-超过64位",
			bits:     65,
			expected: 0,
		},
		{
			name:     "无效输入-远超64位",
			bits:     100,
			expected: 0,
		},
		{
			name:     "无效输入-远超64位的大值",
			bits:     1000,
			expected: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := CalculateMaxValue(tt.bits)
			assert.Equal(t, tt.expected, result, "计算的最大值不匹配")
		})
	}
}

func TestCalculateMaxValue_CommonBitSizes(t *testing.T) {
	// 测试常用的位数大小
	commonTests := []struct {
		bits     int
		expected uint64
	}{
		{1, 0},
		{2, 1},
		{3, 3},
		{4, 7},
		{5, 15},
		{6, 31},
		{7, 63},
		{8, 127},
		{9, 255},
		{10, 511},
		{11, 1023},
		{12, 2047},
		{16, 32767},
		{24, 8388607},
		{32, 2147483647},
		{48, 140737488355327},
		{64, 9223372036854775807},
	}

	for _, tt := range commonTests {
		t.Run(fmt.Sprintf("%d位", tt.bits), func(t *testing.T) {
			result := CalculateMaxValue(tt.bits)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestCalculateMaxValue_PowerOfTwo(t *testing.T) {
	// 验证计算公式：max = 2^(bits-1) - 1
	tests := []struct {
		bits     int
		expected uint64
	}{
		{2, 1},      // 2^1 - 1 = 1
		{3, 3},      // 2^2 - 1 = 3
		{4, 7},      // 2^3 - 1 = 7
		{5, 15},     // 2^4 - 1 = 15
		{8, 127},    // 2^7 - 1 = 127
		{16, 32767}, // 2^15 - 1 = 32767
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("验证公式-2^%d-1", tt.bits-1), func(t *testing.T) {
			result := CalculateMaxValue(tt.bits)
			// 手动计算预期值并验证
			var manualCalc uint64
			if tt.bits >= 1 && tt.bits <= 64 {
				manualCalc = (uint64(1) << uint(tt.bits-1)) - 1
			}
			assert.Equal(t, manualCalc, result)
			assert.Equal(t, tt.expected, result)
		})
	}
}

