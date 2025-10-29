/*
 * Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package json

import (
	"bytes"
	"encoding/json"
	"strings"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/yaml"
)

// Unmarshal parses the JSON-encoded data and stores the result in the value pointed to by v.
// It uses a JSON decoder to read from the provided byte slice.
//
// Parameters:
//   - data: JSON-encoded byte slice to be unmarshaled
//   - v: Pointer to the value where the decoded result will be stored
//
// Returns:
//   - error: Any error that occurred during unmarshaling, or nil on success

func Unmarshal(data []byte, v interface{}) error {
	d := json.NewDecoder(bytes.NewReader(data))
	if err := d.Decode(v); err != nil {
		return err
	}
	return nil
}

// MarshalSilently converts the given value to its JSON representation.
// Unlike json.Marshal, this function doesn't return an error but returns nil if marshaling fails.
// This is useful when you want to avoid error handling and just get the JSON bytes or nil.
//
// Parameters:
//   - v: The value to be marshaled to JSON
//
// Returns:
//   - []byte: JSON-encoded byte slice of the value, or nil if marshaling fails or input is nil

func MarshalSilently(v interface{}) []byte {
	if v == nil {
		return nil
	}
	data, err := json.Marshal(v)
	if err != nil {
		return nil
	}
	return data
}

// ParseYamlToJson parses YAML data and converts it to an unstructured object.
// It takes a YAML string, converts it to JSON, and decodes it into an Unstructured object.
// This is useful for working with Kubernetes resources that are defined in YAML format.
//
// Parameters:
//   - data: YAML-formatted string to be parsed
//
// Returns:
//   - *unstructured.Unstructured: Parsed unstructured object
//   - error: Any error that occurred during parsing, or nil on success

func ParseYamlToJson(data string) (*unstructured.Unstructured, error) {
	decoder := yaml.NewYAMLToJSONDecoder(strings.NewReader(data))
	var obj unstructured.Unstructured
	err := decoder.Decode(&obj)
	return &obj, err
}
