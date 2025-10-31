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
func Unmarshal(data []byte, v interface{}) error {
	d := json.NewDecoder(bytes.NewReader(data))
	if err := d.Decode(v); err != nil {
		return err
	}
	return nil
}

// MarshalSilently converts the given value to its JSON representation.
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

// ParseYamlToJson parses the input data.
func ParseYamlToJson(data string) (*unstructured.Unstructured, error) {
	decoder := yaml.NewYAMLToJSONDecoder(strings.NewReader(data))
	var obj unstructured.Unstructured
	err := decoder.Decode(&obj)
	return &obj, err
}
