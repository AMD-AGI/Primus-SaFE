/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package unstructured

import (
	"strings"
	"testing"

	"gotest.tools/assert"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// TestToString verifies an unstructured object is rendered to YAML.
func TestToString(t *testing.T) {
	obj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"kind": "Pod",
		},
	}
	result := ToString(obj)
	assert.Assert(t, strings.Contains(result, "kind: Pod"))
}

// TestToStringMarshalError verifies an empty string is returned when marshaling fails.
func TestToStringMarshalError(t *testing.T) {
	bad := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"ch": make(chan int),
		},
	}
	assert.Equal(t, ToString(bad), "")
}

// TestConvertUnstructuredToObjectErrors verifies the nil and wrong-type branches.
func TestConvertUnstructuredToObjectErrors(t *testing.T) {
	// nil input is treated as a no-op
	err := ConvertUnstructuredToObject(nil, &corev1.Node{})
	assert.NilError(t, err)

	// a non-unstructured input returns an error
	err = ConvertUnstructuredToObject("not-unstructured", &corev1.Node{})
	assert.Assert(t, err != nil)

	// a type mismatch makes the underlying conversion fail
	mismatch := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"metadata": map[string]interface{}{
				"name": 123,
			},
		},
	}
	err = ConvertUnstructuredToObject(mismatch, &corev1.Node{})
	assert.Assert(t, err != nil)
}
