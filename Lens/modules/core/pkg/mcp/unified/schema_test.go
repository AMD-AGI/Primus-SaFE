// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package unified

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type SimpleRequest struct {
	Name   string `json:"name" mcp:"name,description=The name of the resource"`
	Count  int    `json:"count" mcp:"count,description=Number of items,required"`
	Active bool   `json:"active"`
}

func TestGenerateJSONSchema_SimpleStruct(t *testing.T) {
	schema := GenerateJSONSchema[SimpleRequest]()

	assert.Equal(t, "object", schema["type"])

	props, ok := schema["properties"].(map[string]any)
	require.True(t, ok)

	// Check name property
	nameProp, ok := props["name"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "string", nameProp["type"])
	assert.Equal(t, "The name of the resource", nameProp["description"])

	// Check count property
	countProp, ok := props["count"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "integer", countProp["type"])
	assert.Equal(t, "Number of items", countProp["description"])

	// Check required fields
	required, ok := schema["required"].([]string)
	require.True(t, ok)
	assert.Contains(t, required, "count")
}

type ComplexRequest struct {
	Cluster    string   `json:"cluster" mcp:"cluster,description=Cluster name"`
	Namespaces []string `json:"namespaces" mcp:"namespaces,description=List of namespaces"`
	Limit      int      `json:"limit" mcp:"limit,description=Max results"`
	Filters    struct {
		Status string `json:"status"`
		Type   string `json:"type"`
	} `json:"filters"`
}

func TestGenerateJSONSchema_ComplexStruct(t *testing.T) {
	schema := GenerateJSONSchema[ComplexRequest]()

	assert.Equal(t, "object", schema["type"])

	props, ok := schema["properties"].(map[string]any)
	require.True(t, ok)

	// Check array property
	nsProp, ok := props["namespaces"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "array", nsProp["type"])

	items, ok := nsProp["items"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "string", items["type"])

	// Check nested struct
	filtersProp, ok := props["filters"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "object", filtersProp["type"])

	filterProps, ok := filtersProp["properties"].(map[string]any)
	require.True(t, ok)
	assert.Contains(t, filterProps, "status")
	assert.Contains(t, filterProps, "type")
}

type EmbeddedPagination struct {
	Page     int `json:"page" mcp:"page,description=Page number"`
	PageSize int `json:"page_size" mcp:"page_size,description=Items per page"`
}

type RequestWithEmbedded struct {
	EmbeddedPagination
	Query string `json:"query" mcp:"query,description=Search query,required"`
}

func TestGenerateJSONSchema_EmbeddedStruct(t *testing.T) {
	schema := GenerateJSONSchema[RequestWithEmbedded]()

	props, ok := schema["properties"].(map[string]any)
	require.True(t, ok)

	// Embedded fields should be flattened
	assert.Contains(t, props, "page")
	assert.Contains(t, props, "page_size")
	assert.Contains(t, props, "query")
}

type AllTypesRequest struct {
	StringField   string    `json:"string_field"`
	IntField      int       `json:"int_field"`
	Int64Field    int64     `json:"int64_field"`
	UintField     uint      `json:"uint_field"`
	Float32Field  float32   `json:"float32_field"`
	Float64Field  float64   `json:"float64_field"`
	BoolField     bool      `json:"bool_field"`
	TimeField     time.Time `json:"time_field"`
	StringSlice   []string  `json:"string_slice"`
	IntSlice      []int     `json:"int_slice"`
	StringPointer *string   `json:"string_pointer"`
}

func TestGenerateJSONSchema_AllTypes(t *testing.T) {
	schema := GenerateJSONSchema[AllTypesRequest]()

	props, ok := schema["properties"].(map[string]any)
	require.True(t, ok)

	// String type
	strProp := props["string_field"].(map[string]any)
	assert.Equal(t, "string", strProp["type"])

	// Integer types
	intProp := props["int_field"].(map[string]any)
	assert.Equal(t, "integer", intProp["type"])

	int64Prop := props["int64_field"].(map[string]any)
	assert.Equal(t, "integer", int64Prop["type"])

	uintProp := props["uint_field"].(map[string]any)
	assert.Equal(t, "integer", uintProp["type"])

	// Float types
	float32Prop := props["float32_field"].(map[string]any)
	assert.Equal(t, "number", float32Prop["type"])

	float64Prop := props["float64_field"].(map[string]any)
	assert.Equal(t, "number", float64Prop["type"])

	// Bool type
	boolProp := props["bool_field"].(map[string]any)
	assert.Equal(t, "boolean", boolProp["type"])

	// Time type (should be string with ISO 8601)
	timeProp := props["time_field"].(map[string]any)
	assert.Equal(t, "string", timeProp["type"])

	// Slice types
	strSliceProp := props["string_slice"].(map[string]any)
	assert.Equal(t, "array", strSliceProp["type"])
	strSliceItems := strSliceProp["items"].(map[string]any)
	assert.Equal(t, "string", strSliceItems["type"])

	intSliceProp := props["int_slice"].(map[string]any)
	assert.Equal(t, "array", intSliceProp["type"])
	intSliceItems := intSliceProp["items"].(map[string]any)
	assert.Equal(t, "integer", intSliceItems["type"])

	// Pointer type (should unwrap to underlying type)
	ptrProp := props["string_pointer"].(map[string]any)
	assert.Equal(t, "string", ptrProp["type"])
}

// Test with different tag combinations
type RequestWithQueryTag struct {
	Cluster string `query:"cluster" mcp:"cluster,description=The cluster name"`
}

func TestGenerateJSONSchema_QueryTag(t *testing.T) {
	schema := GenerateJSONSchema[RequestWithQueryTag]()

	props, ok := schema["properties"].(map[string]any)
	require.True(t, ok)

	// Should use query tag name when json tag is not present
	assert.Contains(t, props, "cluster")
}

type RequestWithParamTag struct {
	NodeName string `param:"name" mcp:"name,description=Node name,required"`
}

func TestGenerateJSONSchema_ParamTag(t *testing.T) {
	schema := GenerateJSONSchema[RequestWithParamTag]()

	props, ok := schema["properties"].(map[string]any)
	require.True(t, ok)

	// Should use param tag name
	assert.Contains(t, props, "name")

	required, ok := schema["required"].([]string)
	require.True(t, ok)
	assert.Contains(t, required, "name")
}

func BenchmarkGenerateJSONSchema(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = GenerateJSONSchema[ComplexRequest]()
	}
}
