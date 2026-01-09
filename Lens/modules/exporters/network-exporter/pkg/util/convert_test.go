// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

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
		// integer types
		{
			name:          "int type - positive number",
			value:         int(42),
			expectedFloat: 42.0,
			expectedOk:    true,
		},
		{
			name:          "int type - negative number",
			value:         int(-100),
			expectedFloat: -100.0,
			expectedOk:    true,
		},
		{
			name:          "int type - zero",
			value:         int(0),
			expectedFloat: 0.0,
			expectedOk:    true,
		},
		{
			name:          "int8 type",
			value:         int8(127),
			expectedFloat: 127.0,
			expectedOk:    true,
		},
		{
			name:          "int8 type - minimum value",
			value:         int8(-128),
			expectedFloat: -128.0,
			expectedOk:    true,
		},
		{
			name:          "int16 type",
			value:         int16(32767),
			expectedFloat: 32767.0,
			expectedOk:    true,
		},
		{
			name:          "int16 type - negative number",
			value:         int16(-32768),
			expectedFloat: -32768.0,
			expectedOk:    true,
		},
		{
			name:          "int32 type",
			value:         int32(2147483647),
			expectedFloat: 2147483647.0,
			expectedOk:    true,
		},
		{
			name:          "int64 type - large number",
			value:         int64(9223372036854775807),
			expectedFloat: 9223372036854775807.0,
			expectedOk:    true,
		},
		
		// unsigned integer types
		{
			name:          "uint type",
			value:         uint(42),
			expectedFloat: 42.0,
			expectedOk:    true,
		},
		{
			name:          "uint8 type",
			value:         uint8(255),
			expectedFloat: 255.0,
			expectedOk:    true,
		},
		{
			name:          "uint16 type",
			value:         uint16(65535),
			expectedFloat: 65535.0,
			expectedOk:    true,
		},
		{
			name:          "uint32 type",
			value:         uint32(4294967295),
			expectedFloat: 4294967295.0,
			expectedOk:    true,
		},
		{
			name:          "uint64 type - large number",
			value:         uint64(18446744073709551615),
			expectedFloat: 18446744073709551615.0,
			expectedOk:    true,
		},
		
		// floating point types
		{
			name:          "float32 type",
			value:         float32(3.14),
			expectedFloat: 3.140000104904175, // float32 precision loss
			expectedOk:    true,
		},
		{
			name:          "float32 type - negative number",
			value:         float32(-2.718),
			expectedFloat: -2.7179999351501465,
			expectedOk:    true,
		},
		{
			name:          "float64 type",
			value:         float64(3.141592653589793),
			expectedFloat: 3.141592653589793,
			expectedOk:    true,
		},
		{
			name:          "float64 type - negative number",
			value:         float64(-2.718281828459045),
			expectedFloat: -2.718281828459045,
			expectedOk:    true,
		},
		{
			name:          "float64 type - zero",
			value:         float64(0.0),
			expectedFloat: 0.0,
			expectedOk:    true,
		},
		{
			name:          "float64 type - NaN",
			value:         math.NaN(),
			expectedFloat: math.NaN(),
			expectedOk:    true,
		},
		{
			name:          "float64 type - positive infinity",
			value:         math.Inf(1),
			expectedFloat: math.Inf(1),
			expectedOk:    true,
		},
		{
			name:          "float64 type - negative infinity",
			value:         math.Inf(-1),
			expectedFloat: math.Inf(-1),
			expectedOk:    true,
		},
		
		// non-numeric types - should return false
		{
			name:          "string type - not convertible",
			value:         "123",
			expectedFloat: 0,
			expectedOk:    false,
		},
		{
			name:          "bool type - not convertible",
			value:         true,
			expectedFloat: 0,
			expectedOk:    false,
		},
		{
			name:          "slice type - not convertible",
			value:         []int{1, 2, 3},
			expectedFloat: 0,
			expectedOk:    false,
		},
		{
			name:          "map type - not convertible",
			value:         map[string]int{"a": 1},
			expectedFloat: 0,
			expectedOk:    false,
		},
		{
			name:          "struct type - not convertible",
			value:         struct{ X int }{X: 42},
			expectedFloat: 0,
			expectedOk:    false,
		},
		{
			name:          "pointer type - not convertible",
			value:         new(int),
			expectedFloat: 0,
			expectedOk:    false,
		},
		{
			name:          "chan type - not convertible",
			value:         make(chan int),
			expectedFloat: 0,
			expectedOk:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			value := reflect.ValueOf(tt.value)
			result, ok := ConvertToFloat64(value)
			
			assert.Equal(t, tt.expectedOk, ok, "conversion result ok status mismatch")
			
			if tt.expectedOk {
				if math.IsNaN(tt.expectedFloat) {
					assert.True(t, math.IsNaN(result), "should be NaN")
				} else if math.IsInf(tt.expectedFloat, 0) {
					assert.True(t, math.IsInf(result, int(math.Copysign(1, tt.expectedFloat))), "should be infinity")
				} else {
					// for float32 precision issues, use InDelta
					assert.InDelta(t, tt.expectedFloat, result, 0.0001, "conversion result value mismatch")
				}
			} else {
				assert.Equal(t, 0.0, result, "non-numeric types should return 0")
			}
		})
	}
}

func TestConvertToFloat64_EdgeCases(t *testing.T) {
	t.Run("int64 maximum value", func(t *testing.T) {
		maxInt64 := int64(9223372036854775807)
		value := reflect.ValueOf(maxInt64)
		result, ok := ConvertToFloat64(value)
		assert.True(t, ok)
		assert.Equal(t, float64(maxInt64), result)
	})

	t.Run("int64 minimum value", func(t *testing.T) {
		minInt64 := int64(-9223372036854775808)
		value := reflect.ValueOf(minInt64)
		result, ok := ConvertToFloat64(value)
		assert.True(t, ok)
		assert.Equal(t, float64(minInt64), result)
	})

	t.Run("uint64 maximum value", func(t *testing.T) {
		maxUint64 := uint64(18446744073709551615)
		value := reflect.ValueOf(maxUint64)
		result, ok := ConvertToFloat64(value)
		assert.True(t, ok)
		assert.Equal(t, float64(maxUint64), result)
	})
	
	t.Run("zero value reflect.Value", func(t *testing.T) {
		var zeroValue reflect.Value
		result, ok := ConvertToFloat64(zeroValue)
		assert.False(t, ok)
		assert.Equal(t, 0.0, result)
	})
}

