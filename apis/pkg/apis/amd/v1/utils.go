/*
 * Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package v1

import (
	"strconv"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func GetLabel(obj metav1.Object, key string) string {
	if obj == nil || len(obj.GetLabels()) == 0 {
		return ""
	}
	val, ok := obj.GetLabels()[key]
	if !ok {
		return ""
	}
	return val
}

func GetAnnotation(obj metav1.Object, key string) string {
	if obj == nil || len(obj.GetAnnotations()) == 0 {
		return ""
	}
	val, ok := obj.GetAnnotations()[key]
	if !ok {
		return ""
	}
	return val
}

func RemoveLabel(obj metav1.Object, key string) bool {
	if obj == nil {
		return false
	}
	if _, ok := obj.GetLabels()[key]; !ok {
		return false
	}
	delete(obj.GetLabels(), key)
	return true
}

func RemoveEmptyLabel(obj metav1.Object, key string) bool {
	if obj == nil {
		return false
	}
	val, ok := obj.GetLabels()[key]
	if ok && val == "" {
		delete(obj.GetLabels(), key)
		return true
	}
	return false
}

func RemoveAnnotation(obj metav1.Object, key string) bool {
	if obj == nil {
		return false
	}
	if _, ok := obj.GetAnnotations()[key]; !ok {
		return false
	}
	delete(obj.GetAnnotations(), key)
	return true
}

func SetLabel(obj metav1.Object, key, val string) bool {
	if obj == nil {
		return false
	}
	if obj.GetLabels() == nil {
		obj.SetLabels(make(map[string]string))
	}
	if currentVal, _ := obj.GetLabels()[key]; currentVal == val {
		return false
	}
	obj.GetLabels()[key] = val
	return true
}

func SetAnnotation(obj metav1.Object, key, val string) bool {
	if obj == nil {
		return false
	}
	if obj.GetAnnotations() == nil {
		obj.SetAnnotations(make(map[string]string))
	}
	if currentVal, _ := obj.GetAnnotations()[key]; currentVal == val {
		return false
	}
	obj.GetAnnotations()[key] = val
	return true
}

func GetNodeGpuCount(obj metav1.Object) int {
	return atoi(GetLabel(obj, NodeGpuCountLabel))
}

func GetNodeStartupTime(obj metav1.Object) string {
	return GetLabel(obj, NodeStartupTimeLabel)
}

func GetClusterId(obj metav1.Object) string {
	return GetLabel(obj, ClusterIdLabel)
}

func GetWorkspaceId(obj metav1.Object) string {
	return GetLabel(obj, WorkspaceIdLabel)
}

func GetNodeFlavorId(obj metav1.Object) string {
	return GetLabel(obj, NodeFlavorIdLabel)
}

func GetDisplayName(obj metav1.Object) string {
	return GetLabel(obj, DisplayNameLabel)
}

func GetGpuProductName(obj metav1.Object) string {
	return GetAnnotation(obj, GpuProductNameAnnotation)
}

func GetGpuResourceName(obj metav1.Object) string {
	return GetAnnotation(obj, GpuResourceNameAnnotation)
}

func GetNodesLabelAction(obj metav1.Object) string {
	return GetAnnotation(obj, NodesLabelAction)
}

func GetNodesAnnotationAction(obj metav1.Object) string {
	return GetAnnotation(obj, NodesAnnotationAction)
}

func atoi(str string) int {
	if str == "" {
		return 0
	}
	n, err := strconv.Atoi(str)
	if err != nil {
		return 0
	}
	return n
}
