package mapUtil

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConvertInterfaceToExt(t *testing.T) {
	t.Run("简单结构体转换", func(t *testing.T) {
		type Person struct {
			Name string
			Age  int
		}

		person := Person{Name: "John", Age: 30}
		result, err := ConvertInterfaceToExt(person)

		assert.NoError(t, err)
		assert.Equal(t, "John", result["Name"])
		assert.Equal(t, float64(30), result["Age"]) // JSON unmarshal 会将数字转为 float64
	})

	t.Run("嵌套结构体转换", func(t *testing.T) {
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

	t.Run("map直接转换", func(t *testing.T) {
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
	t.Run("基本类型转换", func(t *testing.T) {
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

	t.Run("空map", func(t *testing.T) {
		input := map[string]interface{}{}
		result := ConvertToStringMap(input)

		assert.NotNil(t, result)
		assert.Empty(t, result)
	})

	t.Run("nil值转换", func(t *testing.T) {
		input := map[string]interface{}{
			"nil": nil,
		}

		result := ConvertToStringMap(input)

		assert.Equal(t, "<nil>", result["nil"])
	})
}

func TestConvertToInterfaceMap(t *testing.T) {
	t.Run("字符串map转接口map", func(t *testing.T) {
		input := map[string]string{
			"key1": "value1",
			"key2": "value2",
		}

		result := ConvertToInterfaceMap(input)

		assert.Equal(t, "value1", result["key1"])
		assert.Equal(t, "value2", result["key2"])
	})

	t.Run("空map", func(t *testing.T) {
		input := map[string]string{}
		result := ConvertToInterfaceMap(input)

		assert.NotNil(t, result)
		assert.Empty(t, result)
	})
}

func TestDecodeKeyFromMap(t *testing.T) {
	t.Run("成功解码", func(t *testing.T) {
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

	t.Run("key不存在", func(t *testing.T) {
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
	t.Run("结构体编码为map", func(t *testing.T) {
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

	t.Run("带JSON标签的结构体", func(t *testing.T) {
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
	t.Run("map解码为结构体", func(t *testing.T) {
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

	t.Run("类型不匹配", func(t *testing.T) {
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
	t.Run("key存在时解码", func(t *testing.T) {
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

	t.Run("key不存在时不报错", func(t *testing.T) {
		type Person struct {
			Name string
		}

		input := map[string]interface{}{
			"other": "value",
		}

		var person Person
		err := DecodeKeyFromMapIfExists(input, "missing", &person)

		assert.NoError(t, err)
		assert.Equal(t, "", person.Name) // 保持默认值
	})
}

func TestParseJSONMap(t *testing.T) {
	t.Run("解析有效JSON", func(t *testing.T) {
		jsonStr := `{"key1":"value1","key2":"value2"}`
		result, err := ParseJSONMap(jsonStr)

		assert.NoError(t, err)
		assert.Equal(t, "value1", result["key1"])
		assert.Equal(t, "value2", result["key2"])
	})

	t.Run("解析空字符串", func(t *testing.T) {
		result, err := ParseJSONMap("")

		assert.NoError(t, err)
		assert.Nil(t, result)
	})

	t.Run("解析空JSON对象", func(t *testing.T) {
		jsonStr := `{}`
		result, err := ParseJSONMap(jsonStr)

		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.Empty(t, result)
	})

	t.Run("解析无效JSON", func(t *testing.T) {
		jsonStr := `{invalid json}`
		_, err := ParseJSONMap(jsonStr)

		assert.Error(t, err)
	})

	t.Run("解析JSON数组失败", func(t *testing.T) {
		jsonStr := `["value1","value2"]`
		_, err := ParseJSONMap(jsonStr)

		assert.Error(t, err)
	})

	t.Run("解析带特殊字符的JSON", func(t *testing.T) {
		jsonStr := `{"key":"value with spaces","special":"!@#$%"}`
		result, err := ParseJSONMap(jsonStr)

		assert.NoError(t, err)
		assert.Equal(t, "value with spaces", result["key"])
		assert.Equal(t, "!@#$%", result["special"])
	})
}

func TestRoundTripConversion(t *testing.T) {
	t.Run("结构体编码解码往返", func(t *testing.T) {
		type Person struct {
			Name string `json:"name"`
			Age  int    `json:"age"`
		}

		original := Person{Name: "John", Age: 30}

		// 编码
		encoded, err := EncodeMap(original)
		require.NoError(t, err)

		// 解码
		var decoded Person
		err = DecodeFromMap(encoded, &decoded)
		require.NoError(t, err)

		assert.Equal(t, original.Name, decoded.Name)
		assert.Equal(t, original.Age, decoded.Age)
	})

	t.Run("map转换往返", func(t *testing.T) {
		original := map[string]interface{}{
			"string": "hello",
			"number": 123,
		}

		// 转为字符串map
		stringMap := ConvertToStringMap(original)
		// 转回接口map
		interfaceMap := ConvertToInterfaceMap(stringMap)

		assert.Equal(t, "hello", interfaceMap["string"])
		assert.Equal(t, "123", interfaceMap["number"]) // 注意：数字变成了字符串
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

	// 编码为 map
	encoded, err := EncodeMap(original)
	require.NoError(t, err)

	// 验证编码结果
	assert.Equal(t, "John Doe", encoded["name"])
	assert.Equal(t, float64(30), encoded["age"])

	// 解码回结构体
	var decoded Person
	err = DecodeFromMap(encoded, &decoded)
	require.NoError(t, err)

	// 验证解码结果
	assert.Equal(t, original.Name, decoded.Name)
	assert.Equal(t, original.Age, decoded.Age)
	assert.Equal(t, original.Address.Street, decoded.Address.Street)
	assert.Equal(t, original.Address.City, decoded.Address.City)
	assert.Equal(t, original.Tags, decoded.Tags)
	assert.Equal(t, original.Metadata, decoded.Metadata)
}

func TestEdgeCases(t *testing.T) {
	t.Run("nil map转换", func(t *testing.T) {
		var input map[string]interface{}
		result := ConvertToStringMap(input)
		assert.NotNil(t, result)
		assert.Empty(t, result)
	})

	t.Run("带omitempty的结构体", func(t *testing.T) {
		type Config struct {
			Required string  `json:"required"`
			Optional *string `json:"optional,omitempty"`
		}

		config := Config{Required: "value"}
		encoded, err := EncodeMap(config)

		require.NoError(t, err)
		assert.Equal(t, "value", encoded["required"])
		_, exists := encoded["optional"]
		assert.False(t, exists) // omitempty 应该导致字段被省略
	})

	t.Run("JSON格式化后的map解析", func(t *testing.T) {
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

