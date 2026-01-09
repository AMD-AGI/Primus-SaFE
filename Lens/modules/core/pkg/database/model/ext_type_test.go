// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package model

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestExtType_Value(t *testing.T) {
	tests := []struct {
		name    string
		input   ExtType
		wantErr bool
	}{
		{
			name:    "empty map",
			input:   ExtType{},
			wantErr: false,
		},
		{
			name: "simple string values",
			input: ExtType{
				"key1": "value1",
				"key2": "value2",
			},
			wantErr: false,
		},
		{
			name: "mixed types",
			input: ExtType{
				"string": "value",
				"number": 123,
				"bool":   true,
				"null":   nil,
			},
			wantErr: false,
		},
		{
			name: "nested object",
			input: ExtType{
				"nested": map[string]interface{}{
					"inner": "value",
				},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			val, err := tt.input.Value()
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, val)
				// Verify it's valid JSON
				var parsed map[string]interface{}
				err := json.Unmarshal([]byte(val.(string)), &parsed)
				assert.NoError(t, err)
			}
		})
	}
}

func TestExtType_Scan(t *testing.T) {
	tests := []struct {
		name     string
		input    interface{}
		expected ExtType
		wantErr  bool
	}{
		{
			name:     "nil value",
			input:    nil,
			expected: ExtType{},
			wantErr:  false,
		},
		{
			name:  "bytes input",
			input: []byte(`{"key": "value"}`),
			expected: ExtType{
				"key": "value",
			},
			wantErr: false,
		},
		{
			name:  "string input",
			input: `{"key": "value", "num": 42}`,
			expected: ExtType{
				"key": "value",
				"num": float64(42),
			},
			wantErr: false,
		},
		{
			name:     "empty json object",
			input:    `{}`,
			expected: ExtType{},
			wantErr:  false,
		},
		{
			name:    "invalid type",
			input:   12345,
			wantErr: true,
		},
		{
			name:    "invalid json",
			input:   `{invalid json}`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var e ExtType
			err := e.Scan(tt.input)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				for k, v := range tt.expected {
					assert.Equal(t, v, e[k])
				}
			}
		})
	}
}

func TestExtType_GetStringValue(t *testing.T) {
	tests := []struct {
		name     string
		input    ExtType
		key      string
		expected string
	}{
		{
			name:     "existing string key",
			input:    ExtType{"key": "value"},
			key:      "key",
			expected: "value",
		},
		{
			name:     "missing key",
			input:    ExtType{"key": "value"},
			key:      "nonexistent",
			expected: "",
		},
		{
			name:     "non-string value",
			input:    ExtType{"key": 123},
			key:      "key",
			expected: "",
		},
		{
			name:     "empty map",
			input:    ExtType{},
			key:      "key",
			expected: "",
		},
		{
			name:     "nil value",
			input:    ExtType{"key": nil},
			key:      "key",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.input.GetStringValue(tt.key)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestExtJSON_Value(t *testing.T) {
	tests := []struct {
		name     string
		input    ExtJSON
		expected string
		wantErr  bool
	}{
		{
			name:     "empty",
			input:    ExtJSON{},
			expected: "null",
			wantErr:  false,
		},
		{
			name:     "json object",
			input:    ExtJSON(`{"key": "value"}`),
			expected: `{"key": "value"}`,
			wantErr:  false,
		},
		{
			name:     "json array",
			input:    ExtJSON(`[1, 2, 3]`),
			expected: `[1, 2, 3]`,
			wantErr:  false,
		},
		{
			name:     "json string",
			input:    ExtJSON(`"hello"`),
			expected: `"hello"`,
			wantErr:  false,
		},
		{
			name:     "json number",
			input:    ExtJSON(`42`),
			expected: `42`,
			wantErr:  false,
		},
		{
			name:     "json null",
			input:    ExtJSON(`null`),
			expected: `null`,
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			val, err := tt.input.Value()
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expected, val)
			}
		})
	}
}

func TestExtJSON_Scan(t *testing.T) {
	tests := []struct {
		name     string
		input    interface{}
		expected ExtJSON
		wantErr  bool
	}{
		{
			name:     "nil value",
			input:    nil,
			expected: ExtJSON("null"),
			wantErr:  false,
		},
		{
			name:     "bytes input",
			input:    []byte(`{"key": "value"}`),
			expected: ExtJSON(`{"key": "value"}`),
			wantErr:  false,
		},
		{
			name:     "string input",
			input:    `[1, 2, 3]`,
			expected: ExtJSON(`[1, 2, 3]`),
			wantErr:  false,
		},
		{
			name:    "invalid type",
			input:   12345,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var e ExtJSON
			err := e.Scan(tt.input)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expected, e)
			}
		})
	}
}

func TestExtJSON_UnmarshalTo(t *testing.T) {
	tests := []struct {
		name    string
		input   ExtJSON
		dest    interface{}
		wantErr bool
	}{
		{
			name:    "empty",
			input:   ExtJSON{},
			dest:    &map[string]interface{}{},
			wantErr: false,
		},
		{
			name:    "to map",
			input:   ExtJSON(`{"key": "value"}`),
			dest:    &map[string]string{},
			wantErr: false,
		},
		{
			name:    "to slice",
			input:   ExtJSON(`[1, 2, 3]`),
			dest:    &[]int{},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.input.UnmarshalTo(tt.dest)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestExtJSON_MarshalFrom(t *testing.T) {
	tests := []struct {
		name     string
		input    interface{}
		expected string
		wantErr  bool
	}{
		{
			name:     "map",
			input:    map[string]string{"key": "value"},
			expected: `{"key":"value"}`,
			wantErr:  false,
		},
		{
			name:     "slice",
			input:    []int{1, 2, 3},
			expected: `[1,2,3]`,
			wantErr:  false,
		},
		{
			name:     "string",
			input:    "hello",
			expected: `"hello"`,
			wantErr:  false,
		},
		{
			name:     "number",
			input:    42,
			expected: `42`,
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var e ExtJSON
			err := e.MarshalFrom(tt.input)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expected, string(e))
			}
		})
	}
}

func TestExtJSON_IsArray(t *testing.T) {
	tests := []struct {
		name     string
		input    ExtJSON
		expected bool
	}{
		{
			name:     "array",
			input:    ExtJSON(`[1, 2, 3]`),
			expected: true,
		},
		{
			name:     "array with whitespace",
			input:    ExtJSON(`  [1, 2, 3]`),
			expected: true,
		},
		{
			name:     "object",
			input:    ExtJSON(`{"key": "value"}`),
			expected: false,
		},
		{
			name:     "empty",
			input:    ExtJSON{},
			expected: false,
		},
		{
			name:     "string",
			input:    ExtJSON(`"hello"`),
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.input.IsArray()
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestExtJSON_IsObject(t *testing.T) {
	tests := []struct {
		name     string
		input    ExtJSON
		expected bool
	}{
		{
			name:     "object",
			input:    ExtJSON(`{"key": "value"}`),
			expected: true,
		},
		{
			name:     "object with whitespace",
			input:    ExtJSON(`  {"key": "value"}`),
			expected: true,
		},
		{
			name:     "array",
			input:    ExtJSON(`[1, 2, 3]`),
			expected: false,
		},
		{
			name:     "empty",
			input:    ExtJSON{},
			expected: false,
		},
		{
			name:     "string",
			input:    ExtJSON(`"hello"`),
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.input.IsObject()
			assert.Equal(t, tt.expected, result)
		})
	}
}

