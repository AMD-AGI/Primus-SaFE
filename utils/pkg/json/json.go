/*
 * Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
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

func UnmarshalWithCheck(data []byte, v interface{}) error {
	d := json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	if err := d.Decode(v); err != nil {
		return err
	}
	return nil
}

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

func ParseYamlToJson(data string) (*unstructured.Unstructured, error) {
	decoder := yaml.NewYAMLToJSONDecoder(strings.NewReader(data))
	var obj unstructured.Unstructured
	err := decoder.Decode(&obj)
	return &obj, err
}

func DecodeFromMapWithJson(data interface{}, targetObject interface{}) error {
	jsonByte, err := json.Marshal(data)
	if err != nil {
		return err
	}
	return json.Unmarshal(jsonByte, targetObject)
}
