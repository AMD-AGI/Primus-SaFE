/*
 * Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package unstructured

import (
	"reflect"
	"testing"

	"gotest.tools/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
