// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package mapUtil

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConvertInterfaceToExt(t *testing.T) {
	t.Run("simple struct conversion", func(t *testing.T) {
		type Person struct {
			Name string
			Age  int
		}

		person := Person{Name: "John", Age: 30}
		result, err := ConvertInterfaceToExt(person)

		assert.NoError(t, err)
		assert.Equal(t, "John", result["Name"])
		assert.Equal(t, float64(30), result["Age"]) // JSON unmarshal converts numbers to float64
	})

	t.Run("nested struct conversion", func(t *testing.T) {
		type Address struct {
			City    string
			Country string
		}
		type Person struct {
			Name    string
			Address Address
		}

		person := Person{
			Name: "John",
			Address: Address{
				City:    "New York",
				Country: "USA",
			},
		}
		result, err := ConvertInterfaceToExt(person)

		assert.NoError(t, err)
		assert.Equal(t, "John", result["Name"])
		address, ok := result["Address"].(map[string]interface{})
		require.True(t, ok)
		assert.Equal(t, "New York", address["City"])
		assert.Equal(t, "USA", address["Country"])
	})

	t.Run("direct map conversion", func(t *testing.T) {
		input := map[string]interface{}{
			"key1": "value1",
			"key2": 123,
		}

		result, err := ConvertInterfaceToExt(input)

		assert.NoError(t, err)
		assert.Equal(t, "value1", result["key1"])
		assert.Equal(t, float64(123), result["key2"])
	})
}

func TestConvertToStringMap(t *testing.T) {
	t.Run("basic type conversion", func(t *testing.T) {
		input := map[string]interface{}{
			"string": "hello",
			"int":    123,
			"float":  45.67,
			"bool":   true,
		}

		result := ConvertToStringMap(input)

		assert.Equal(t, "hello", result["string"])
		assert.Equal(t, "123", result["int"])
		assert.Equal(t, "45.67", result["float"])
		assert.Equal(t, "true", result["bool"])
	})

	t.Run("empty map", func(t *testing.T) {
		input := map[string]interface{}{}
		result := ConvertToStringMap(input)

		assert.NotNil(t, result)
		assert.Empty(t, result)
	})

	t.Run("nil value conversion", func(t *testing.T) {
		input := map[string]interface{}{
			"nil": nil,
		}

		result := ConvertToStringMap(input)

		assert.Equal(t, "<nil>", result["nil"])
	})
}

func TestConvertToInterfaceMap(t *testing.T) {
	t.Run("string map to interface map", func(t *testing.T) {
		input := map[string]string{
			"key1": "value1",
			"key2": "value2",
		}

		result := ConvertToInterfaceMap(input)

		assert.Equal(t, "value1", result["key1"])
		assert.Equal(t, "value2", result["key2"])
	})

	t.Run("empty map", func(t *testing.T) {
		input := map[string]string{}
		result := ConvertToInterfaceMap(input)

		assert.NotNil(t, result)
		assert.Empty(t, result)
	})
}

func TestDecodeKeyFromMap(t *testing.T) {
	t.Run("successful decoding", func(t *testing.T) {
		type Person struct {
			Name string
			Age  int
		}

		input := map[string]interface{}{
			"person": map[string]interface{}{
				"Name": "John",
				"Age":  30,
			},
		}

		var person Person
		err := DecodeKeyFromMap(input, "person", &person)

		assert.NoError(t, err)
		assert.Equal(t, "John", person.Name)
		assert.Equal(t, 30, person.Age)
	})

	t.Run("key does not exist", func(t *testing.T) {
		input := map[string]interface{}{
			"other": "value",
		}

		var result string
		err := DecodeKeyFromMap(input, "missing", &result)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "key not exist")
	})
}

func TestEncodeMap(t *testing.T) {
	t.Run("struct encoded to map", func(t *testing.T) {
		type Person struct {
			Name string
			Age  int
		}

		person := Person{Name: "John", Age: 30}
		result, err := EncodeMap(person)

		assert.NoError(t, err)
		assert.Equal(t, "John", result["Name"])
		assert.Equal(t, float64(30), result["Age"])
	})

	t.Run("struct with JSON tags", func(t *testing.T) {
		type Person struct {
			Name string `json:"name"`
			Age  int    `json:"age"`
		}

		person := Person{Name: "John", Age: 30}
		result, err := EncodeMap(person)

		assert.NoError(t, err)
		assert.Equal(t, "John", result["name"])
		assert.Equal(t, float64(30), result["age"])
	})
}

func TestDecodeFromMap(t *testing.T) {
	t.Run("map decoded to struct", func(t *testing.T) {
		type Person struct {
			Name string
			Age  int
		}

		input := map[string]interface{}{
			"Name": "John",
			"Age":  30,
		}

		var person Person
		err := DecodeFromMap(input, &person)

		assert.NoError(t, err)
		assert.Equal(t, "John", person.Name)
		assert.Equal(t, 30, person.Age)
	})

	t.Run("type mismatch", func(t *testing.T) {
		type Person struct {
			Name string
			Age  int
		}

		input := map[string]interface{}{
			"Name": "John",
			"Age":  "not a number",
		}

		var person Person
		err := DecodeFromMap(input, &person)

		assert.Error(t, err)
	})
}

func TestDecodeKeyFromMapIfExists(t *testing.T) {
	t.Run("decode when key exists", func(t *testing.T) {
		type Person struct {
			Name string
		}

		input := map[string]interface{}{
			"person": map[string]interface{}{
				"Name": "John",
			},
		}

		var person Person
		err := DecodeKeyFromMapIfExists(input, "person", &person)

		assert.NoError(t, err)
		assert.Equal(t, "John", person.Name)
	})

	t.Run("no error when key does not exist", func(t *testing.T) {
		type Person struct {
			Name string
		}

		input := map[string]interface{}{
			"other": "value",
		}

		var person Person
		err := DecodeKeyFromMapIfExists(input, "missing", &person)

		assert.NoError(t, err)
		assert.Equal(t, "", person.Name) // keeps default value
	})
}

func TestParseJSONMap(t *testing.T) {
	t.Run("parse valid JSON", func(t *testing.T) {
		jsonStr := `{"key1":"value1","key2":"value2"}`
		result, err := ParseJSONMap(jsonStr)

		assert.NoError(t, err)
		assert.Equal(t, "value1", result["key1"])
		assert.Equal(t, "value2", result["key2"])
	})

	t.Run("parse empty string", func(t *testing.T) {
		result, err := ParseJSONMap("")

		assert.NoError(t, err)
		assert.Nil(t, result)
	})

	t.Run("parse empty JSON object", func(t *testing.T) {
		jsonStr := `{}`
		result, err := ParseJSONMap(jsonStr)

		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.Empty(t, result)
	})

	t.Run("parse invalid JSON", func(t *testing.T) {
		jsonStr := `{invalid json}`
		_, err := ParseJSONMap(jsonStr)

		assert.Error(t, err)
	})

	t.Run("parsing JSON array fails", func(t *testing.T) {
		jsonStr := `["value1","value2"]`
		_, err := ParseJSONMap(jsonStr)

		assert.Error(t, err)
	})

	t.Run("parse JSON with special characters", func(t *testing.T) {
		jsonStr := `{"key":"value with spaces","special":"!@#$%"}`
		result, err := ParseJSONMap(jsonStr)

		assert.NoError(t, err)
		assert.Equal(t, "value with spaces", result["key"])
		assert.Equal(t, "!@#$%", result["special"])
	})
}

func TestRoundTripConversion(t *testing.T) {
	t.Run("struct encode-decode round trip", func(t *testing.T) {
		type Person struct {
			Name string `json:"name"`
			Age  int    `json:"age"`
		}

		original := Person{Name: "John", Age: 30}

		// encode
		encoded, err := EncodeMap(original)
		require.NoError(t, err)

		// decode
		var decoded Person
		err = DecodeFromMap(encoded, &decoded)
		require.NoError(t, err)

		assert.Equal(t, original.Name, decoded.Name)
		assert.Equal(t, original.Age, decoded.Age)
	})

	t.Run("map conversion round trip", func(t *testing.T) {
		original := map[string]interface{}{
			"string": "hello",
			"number": 123,
		}

		// convert to string map
		stringMap := ConvertToStringMap(original)
		// convert back to interface map
		interfaceMap := ConvertToInterfaceMap(stringMap)

		assert.Equal(t, "hello", interfaceMap["string"])
		assert.Equal(t, "123", interfaceMap["number"]) // note: number becomes string
	})
}

func TestComplexNestedStructure(t *testing.T) {
	type Address struct {
		Street  string `json:"street"`
		City    string `json:"city"`
		ZipCode string `json:"zip_code"`
	}

	type Person struct {
		Name     string            `json:"name"`
		Age      int               `json:"age"`
		Address  Address           `json:"address"`
		Tags     []string          `json:"tags"`
		Metadata map[string]string `json:"metadata"`
	}

	original := Person{
		Name: "John Doe",
		Age:  30,
		Address: Address{
			Street:  "123 Main St",
			City:    "New York",
			ZipCode: "10001",
		},
		Tags: []string{"developer", "golang"},
		Metadata: map[string]string{
			"department": "engineering",
			"level":      "senior",
		},
	}

	// encode to map
	encoded, err := EncodeMap(original)
	require.NoError(t, err)

	// verify encoding result
	assert.Equal(t, "John Doe", encoded["name"])
	assert.Equal(t, float64(30), encoded["age"])

	// decode back to struct
	var decoded Person
	err = DecodeFromMap(encoded, &decoded)
	require.NoError(t, err)

	// verify decoding result
	assert.Equal(t, original.Name, decoded.Name)
	assert.Equal(t, original.Age, decoded.Age)
	assert.Equal(t, original.Address.Street, decoded.Address.Street)
	assert.Equal(t, original.Address.City, decoded.Address.City)
	assert.Equal(t, original.Tags, decoded.Tags)
	assert.Equal(t, original.Metadata, decoded.Metadata)
}

func TestEdgeCases(t *testing.T) {
	t.Run("nil map conversion", func(t *testing.T) {
		var input map[string]interface{}
		result := ConvertToStringMap(input)
		assert.NotNil(t, result)
		assert.Empty(t, result)
	})

	t.Run("struct with omitempty", func(t *testing.T) {
		type Config struct {
			Required string  `json:"required"`
			Optional *string `json:"optional,omitempty"`
		}

		config := Config{Required: "value"}
		encoded, err := EncodeMap(config)

		require.NoError(t, err)
		assert.Equal(t, "value", encoded["required"])
		_, exists := encoded["optional"]
		assert.False(t, exists) // omitempty should cause field to be omitted
	})

	t.Run("parse formatted JSON map", func(t *testing.T) {
		data := map[string]string{
			"key1": "value1",
			"key2": "value2",
		}

		jsonBytes, err := json.Marshal(data)
		require.NoError(t, err)

		result, err := ParseJSONMap(string(jsonBytes))
		require.NoError(t, err)

		assert.Equal(t, data, result)
	})
}

