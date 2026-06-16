/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package json

import (
	"testing"

	"gotest.tools/assert"
)

// TestUnmarshal verifies decoding of valid and invalid JSON data.
func TestUnmarshal(t *testing.T) {
	var v map[string]interface{}
	err := Unmarshal([]byte(`{"a":1}`), &v)
	assert.NilError(t, err)
	assert.Equal(t, v["a"], float64(1))

	err = Unmarshal([]byte(`{invalid`), &v)
	assert.Assert(t, err != nil)
}

// TestMarshalSilently verifies silent marshaling and nil fallbacks.
func TestMarshalSilently(t *testing.T) {
	assert.Assert(t, MarshalSilently(nil) == nil)
	assert.Equal(t, string(MarshalSilently(map[string]string{"a": "b"})), `{"a":"b"}`)
	assert.Assert(t, MarshalSilently(make(chan int)) == nil)
}

// TestParseYamlToJson verifies YAML is parsed into an unstructured object.
func TestParseYamlToJson(t *testing.T) {
	yamlStr := "apiVersion: v1\nkind: Pod\nmetadata:\n  name: test\n"
	obj, err := ParseYamlToJson(yamlStr)
	assert.NilError(t, err)
	assert.Equal(t, obj.GetKind(), "Pod")
	assert.Equal(t, obj.GetName(), "test")
}
