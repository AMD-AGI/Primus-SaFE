// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package unified

import (
	"reflect"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func getProp(t *testing.T, m map[string]any, key string) map[string]any {
	t.Helper()
	props, ok := m["properties"].(map[string]any)
	require.True(t, ok)
	p, ok := props[key].(map[string]any)
	require.True(t, ok, "missing property %q", key)
	return p
}

func TestGenerateJSONSchema_Primitives(t *testing.T) {
	type T struct {
		S string  `json:"s"`
		I int     `json:"i"`
		B bool    `json:"b"`
		F float64 `json:"f"`
	}
	s := GenerateJSONSchema[T]()
	assert.Equal(t, "object", s["type"])
	assert.Equal(t, "string", getProp(t, s, "s")["type"])
	assert.Equal(t, "integer", getProp(t, s, "i")["type"])
	assert.Equal(t, "boolean", getProp(t, s, "b")["type"])
	assert.Equal(t, "number", getProp(t, s, "f")["type"])
}

func TestGenerateJSONSchema_Slice(t *testing.T) {
	type T struct {
		Strs []string `json:"strs"`
		Nums []int    `json:"nums"`
	}
	s := GenerateJSONSchema[T]()
	arrS := getProp(t, s, "strs")
	assert.Equal(t, "array", arrS["type"])
	itemsS, ok := arrS["items"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "string", itemsS["type"])

	arrN := getProp(t, s, "nums")
	assert.Equal(t, "array", arrN["type"])
	itemsN, ok := arrN["items"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "integer", itemsN["type"])
}

func TestGenerateJSONSchema_Pointer(t *testing.T) {
	type T struct {
		P *string `json:"p"`
	}
	s := GenerateJSONSchema[T]()
	assert.Equal(t, "string", getProp(t, s, "p")["type"])
}

func TestGenerateJSONSchema_Struct(t *testing.T) {
	type Inner struct {
		Z int `json:"z"`
	}
	type T struct {
		N Inner `json:"n"`
	}
	s := GenerateJSONSchema[T]()
	nested := getProp(t, s, "n")
	assert.Equal(t, "object", nested["type"])
	zProp, ok := nested["properties"].(map[string]any)["z"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "integer", zProp["type"])
}

func TestGenerateJSONSchema_TimeField(t *testing.T) {
	type T struct {
		When time.Time `json:"when"`
	}
	s := GenerateJSONSchema[T]()
	p := getProp(t, s, "when")
	assert.Equal(t, "string", p["type"])
	assert.Equal(t, "ISO 8601 datetime", p["description"])
}

func TestGenerateJSONSchema_Map(t *testing.T) {
	type T struct {
		M map[string]int `json:"m"`
	}
	s := GenerateJSONSchema[T]()
	assert.Equal(t, "object", getProp(t, s, "m")["type"])
}

func TestGenerateJSONSchema_FieldNameJSON(t *testing.T) {
	type T struct {
		X string `json:"my_name,omitempty"`
	}
	s := GenerateJSONSchema[T]()
	props, ok := s["properties"].(map[string]any)
	require.True(t, ok)
	_, ok = props["my_name"]
	assert.True(t, ok)
}

func TestGenerateJSONSchema_FieldNameQuery(t *testing.T) {
	type T struct {
		X string `query:"q_name"`
	}
	s := GenerateJSONSchema[T]()
	_, ok := s["properties"].(map[string]any)["q_name"]
	assert.True(t, ok)
}

func TestGenerateJSONSchema_FieldNameParam(t *testing.T) {
	type T struct {
		X string `param:"id"`
	}
	s := GenerateJSONSchema[T]()
	_, ok := s["properties"].(map[string]any)["id"]
	assert.True(t, ok)
}

func TestGenerateJSONSchema_FieldNameDash(t *testing.T) {
	type T struct {
		Secret string `json:"-"`
		Public string `json:"pub"`
	}
	s := GenerateJSONSchema[T]()
	props := s["properties"].(map[string]any)
	_, bad := props["-"]
	assert.False(t, bad)
	_, ok := props["pub"]
	assert.True(t, ok)
}

func TestGenerateJSONSchema_MCPTag_DescriptionAndRequired(t *testing.T) {
	type T struct {
		Foo string `mcp:"foo,description=hello,required"`
	}
	m := GenerateJSONSchema[T]()
	p := getProp(t, m, "foo")
	assert.Equal(t, "hello", p["description"])
	req, ok := m["required"].([]string)
	require.True(t, ok)
	assert.Contains(t, req, "foo", "required should name the property key used in schema (see generateSchemaFromType ordering)")
}

func TestGenerateJSONSchema_MCPTagRenames(t *testing.T) {
	type T struct {
		X string `mcp:"renamed"`
	}
	m := GenerateJSONSchema[T]()
	props := m["properties"].(map[string]any)
	_, hasRenamed := props["renamed"]
	assert.True(t, hasRenamed)
}

func TestGenerateJSONSchema_NonStruct(t *testing.T) {
	m := GenerateJSONSchema[int]()
	assert.Equal(t, "object", m["type"])
}

func TestGenerateJSONSchema_UnexportedSkipped(t *testing.T) {
	type T struct {
		Public  string `json:"public"`
		private string
	}
	m := GenerateJSONSchema[T]()
	props := m["properties"].(map[string]any)
	assert.Contains(t, props, "public")
	assert.NotContains(t, props, "private")
}

func TestGenerateJSONSchema_EmbeddedStruct(t *testing.T) {
	type Base struct {
		A int `json:"a"`
	}
	type T struct {
		Base
		B string `json:"b"`
	}
	m := GenerateJSONSchema[T]()
	props := m["properties"].(map[string]any)
	assert.Contains(t, props, "a")
	assert.Contains(t, props, "b")
}

func TestTypeToSchemaReflect(t *testing.T) {
	st := typeToSchema(reflect.TypeFor[string]())
	assert.Equal(t, "string", st.Type)
}

func TestGetFieldNameReflect(t *testing.T) {
	type S struct {
		Z int `json:"zeta"`
	}
	f, _ := reflect.TypeOf(S{}).FieldByName("Z")
	assert.Equal(t, "zeta", getFieldName(f))
}

func TestSchemaToMapEmpty(t *testing.T) {
	m := schemaToMap(JSONSchema{Type: "object"})
	assert.Equal(t, "object", m["type"])
	_, hasProps := m["properties"]
	assert.False(t, hasProps)
}
