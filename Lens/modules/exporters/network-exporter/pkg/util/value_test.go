// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

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
			name:     "1 bit - max value is 0",
			bits:     1,
			expected: 0, // 2^0 - 1 = 0
		},
		{
			name:     "2 bits - max value is 1",
			bits:     2,
			expected: 1, // 2^1 - 1 = 1
		},
		{
			name:     "8 bits - max value is 127",
			bits:     8,
			expected: 127, // 2^7 - 1 = 127
		},
		{
			name:     "16 bits - max value is 32767",
			bits:     16,
			expected: 32767, // 2^15 - 1 = 32767
		},
		{
			name:     "32 bits - max value",
			bits:     32,
			expected: 2147483647, // 2^31 - 1
		},
		{
			name:     "64 bits - max value",
			bits:     64,
			expected: 9223372036854775807, // 2^63 - 1
		},
		{
			name:     "4 bits - max value is 7",
			bits:     4,
			expected: 7, // 2^3 - 1 = 7
		},
		{
			name:     "10 bits - max value is 511",
			bits:     10,
			expected: 511, // 2^9 - 1 = 511
		},
		{
			name:     "20 bits - max value",
			bits:     20,
			expected: 524287, // 2^19 - 1
		},
		
		// boundary value tests
		{
			name:     "boundary value - minimum valid bits 1",
			bits:     1,
			expected: 0,
		},
		{
			name:     "boundary value - maximum valid bits 64",
			bits:     64,
			expected: 9223372036854775807,
		},
		
		// invalid input - should return 0
		{
			name:     "invalid input - 0 bits",
			bits:     0,
			expected: 0,
		},
		{
			name:     "invalid input - negative number",
			bits:     -1,
			expected: 0,
		},
		{
			name:     "invalid input - large negative number",
			bits:     -100,
			expected: 0,
		},
		{
			name:     "invalid input - exceeds 64 bits",
			bits:     65,
			expected: 0,
		},
		{
			name:     "invalid input - far exceeds 64 bits",
			bits:     100,
			expected: 0,
		},
		{
			name:     "invalid input - very large value far exceeding 64 bits",
			bits:     1000,
			expected: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := CalculateMaxValue(tt.bits)
			assert.Equal(t, tt.expected, result, "calculated max value mismatch")
		})
	}
}

func TestCalculateMaxValue_CommonBitSizes(t *testing.T) {
	// test common bit sizes
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
		t.Run(fmt.Sprintf("%d bits", tt.bits), func(t *testing.T) {
			result := CalculateMaxValue(tt.bits)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestCalculateMaxValue_PowerOfTwo(t *testing.T) {
	// verify calculation formula: max = 2^(bits-1) - 1
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
		t.Run(fmt.Sprintf("verify formula - 2^%d-1", tt.bits-1), func(t *testing.T) {
			result := CalculateMaxValue(tt.bits)
			// manually calculate expected value and verify
			var manualCalc uint64
			if tt.bits >= 1 && tt.bits <= 64 {
				manualCalc = (uint64(1) << uint(tt.bits-1)) - 1
			}
			assert.Equal(t, manualCalc, result)
			assert.Equal(t, tt.expected, result)
		})
	}
}

