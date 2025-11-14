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
		// float64 type
		{
			name:     "float64 type - positive number",
			input:    float64(123.45),
			expected: 123.45,
		},
		{
			name:     "float64 type - negative number",
			input:    float64(-123.45),
			expected: -123.45,
		},
		{
			name:     "float64 type - zero",
			input:    float64(0),
			expected: 0,
		},
		{
			name:     "float64 type - NaN",
			input:    math.NaN(),
			expected: math.NaN(),
		},
		{
			name:     "float64 type - positive infinity",
			input:    math.Inf(1),
			expected: math.Inf(1),
		},
		{
			name:     "float64 type - negative infinity",
			input:    math.Inf(-1),
			expected: math.Inf(-1),
		},

		// float32 type
		{
			name:     "float32 type - positive number",
			input:    float32(123.45),
			expected: float64(float32(123.45)),
		},
		{
			name:     "float32 type - negative number",
			input:    float32(-123.45),
			expected: float64(float32(-123.45)),
		},

		// int type
		{
			name:     "int type - positive number",
			input:    int(123),
			expected: 123.0,
		},
		{
			name:     "int type - negative number",
			input:    int(-123),
			expected: -123.0,
		},
		{
			name:     "int type - zero",
			input:    int(0),
			expected: 0.0,
		},
		{
			name:     "int type - max value",
			input:    int(math.MaxInt32),
			expected: float64(math.MaxInt32),
		},

		// int8 type
		{
			name:     "int8 type - positive number",
			input:    int8(100),
			expected: 100.0,
		},
		{
			name:     "int8 type - negative number",
			input:    int8(-100),
			expected: -100.0,
		},
		{
			name:     "int8 type - max value",
			input:    int8(127),
			expected: 127.0,
		},
		{
			name:     "int8 type - min value",
			input:    int8(-128),
			expected: -128.0,
		},

		// int16 type
		{
			name:     "int16 type - positive number",
			input:    int16(1000),
			expected: 1000.0,
		},
		{
			name:     "int16 type - negative number",
			input:    int16(-1000),
			expected: -1000.0,
		},

		// int32 type
		{
			name:     "int32 type - positive number",
			input:    int32(100000),
			expected: 100000.0,
		},
		{
			name:     "int32 type - negative number",
			input:    int32(-100000),
			expected: -100000.0,
		},

		// int64 type
		{
			name:     "int64 type - positive number",
			input:    int64(1000000000),
			expected: 1000000000.0,
		},
		{
			name:     "int64 type - negative number",
			input:    int64(-1000000000),
			expected: -1000000000.0,
		},
		{
			name:     "int64 type - max safe integer",
			input:    int64(9007199254740991), // 2^53 - 1
			expected: 9007199254740991.0,
		},

		// uint type
		{
			name:     "uint type - positive number",
			input:    uint(123),
			expected: 123.0,
		},
		{
			name:     "uint type - zero",
			input:    uint(0),
			expected: 0.0,
		},

		// uint8 type
		{
			name:     "uint8 type - positive number",
			input:    uint8(200),
			expected: 200.0,
		},
		{
			name:     "uint8 type - max value",
			input:    uint8(255),
			expected: 255.0,
		},

		// uint16 type
		{
			name:     "uint16 type - positive number",
			input:    uint16(50000),
			expected: 50000.0,
		},
		{
			name:     "uint16 type - max value",
			input:    uint16(65535),
			expected: 65535.0,
		},

		// uint32 type
		{
			name:     "uint32 type - positive number",
			input:    uint32(1000000),
			expected: 1000000.0,
		},

		// uint64 type
		{
			name:     "uint64 type - positive number",
			input:    uint64(1000000000000),
			expected: 1000000000000.0,
		},

		// json.Number type
		{
			name:     "json.Number type - integer",
			input:    json.Number("123"),
			expected: 123.0,
		},
		{
			name:     "json.Number type - float",
			input:    json.Number("123.45"),
			expected: 123.45,
		},
		{
			name:     "json.Number type - negative number",
			input:    json.Number("-123.45"),
			expected: -123.45,
		},
		{
			name:     "json.Number type - scientific notation",
			input:    json.Number("1.23e2"),
			expected: 123.0,
		},
		{
			name:     "json.Number type - invalid number",
			input:    json.Number("invalid"),
			expected: 0.0,
		},

		// unsupported types
		{
			name:     "string type",
			input:    "123",
			expected: 0.0,
		},
		{
			name:     "bool type - true",
			input:    true,
			expected: 0.0,
		},
		{
			name:     "bool type - false",
			input:    false,
			expected: 0.0,
		},
		{
			name:     "nil value",
			input:    nil,
			expected: 0.0,
		},
		{
			name:     "slice type",
			input:    []int{1, 2, 3},
			expected: 0.0,
		},
		{
			name:     "map type",
			input:    map[string]int{"a": 1},
			expected: 0.0,
		},
		{
			name:     "struct type",
			input:    struct{ Value int }{Value: 123},
			expected: 0.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := convertToFloat(tt.input)
			
			// Special handling for NaN case
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
	t.Run("very large integer", func(t *testing.T) {
		// Test large integer conversion
		bigInt := int64(9223372036854775807) // int64 max value
		result := convertToFloat(bigInt)
		assert.Equal(t, float64(bigInt), result)
	})

	t.Run("very large unsigned integer", func(t *testing.T) {
		bigUint := uint64(18446744073709551615) // uint64 max value
		result := convertToFloat(bigUint)
		assert.Equal(t, float64(bigUint), result)
	})

	t.Run("various zero representations", func(t *testing.T) {
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

	t.Run("json.Number empty string", func(t *testing.T) {
		result := convertToFloat(json.Number(""))
		assert.Equal(t, 0.0, result)
	})

	t.Run("json.Number special formats", func(t *testing.T) {
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
	t.Run("float32 precision loss", func(t *testing.T) {
		// float32 cannot precisely represent certain numbers
		f32 := float32(1.23456789)
		result := convertToFloat(f32)
		// Result should be the value after float32 converts to float64, with precision difference
		assert.InDelta(t, float64(f32), result, 1e-6)
	})

	t.Run("large integer to float precision", func(t *testing.T) {
		// Test precision when converting large integers
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
		"invalid", // unsupported type
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, v := range values {
			_ = convertToFloat(v)
		}
	}
}

