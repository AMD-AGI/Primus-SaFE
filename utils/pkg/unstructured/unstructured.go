/*
 * Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package unstructured

import (
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/yaml"
)

func ConvertObjectToUnstructured(obj metav1.Object) (*unstructured.Unstructured, error) {
	converter := runtime.DefaultUnstructuredConverter
	unstructuredObj, err := converter.ToUnstructured(obj)
	if err != nil {
		return nil, err
	}
	return &unstructured.Unstructured{
		Object: unstructuredObj,
	}, nil
}

func ConvertUnstructuredToObject(obj interface{}, result metav1.Object) error {
	if obj == nil {
		return nil
	}
	unstructuredObj, ok := obj.(*unstructured.Unstructured)
	if !ok {
		return fmt.Errorf("the object is not Unstructured")
	}
	err := runtime.DefaultUnstructuredConverter.FromUnstructured(unstructuredObj.Object, result)
	if err != nil {
		return err
	}
	return nil
}

func ToString(obj *unstructured.Unstructured) string {
	yamlBytes, err := yaml.Marshal(obj.Object)
	if err != nil {
		return ""
	}
	return string(yamlBytes)
}
