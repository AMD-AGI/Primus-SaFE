/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package unstructured

import (
	"reflect"
	"strings"
	"testing"

	"gotest.tools/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestConvert(t *testing.T) {
	n := corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test",
			Labels: map[string]string{
				"kubernetes.io/hostname": "localhost",
			},
		},
		Spec: corev1.NodeSpec{
			ProviderID: "test",
		},
	}

	unstructuredObj, err := ConvertObjectToUnstructured(&n)
	assert.NilError(t, err)
	assert.Equal(t, unstructuredObj.GetLabels()["kubernetes.io/hostname"], "localhost")

	n2 := &corev1.Node{}
	err = ConvertUnstructuredToObject(unstructuredObj, n2)
	assert.NilError(t, err)
	assert.Equal(t, n.Name, n2.Name)
	assert.Equal(t, reflect.DeepEqual(n.GetLabels(), n2.GetLabels()), true)
	assert.Equal(t, n.Spec.ProviderID, n2.Spec.ProviderID)
}

// --- merged from to_string_test.go ---

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
