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

func GetNodeGpuCount(obj metav1.Object) int {
	return atoi(GetLabel(obj, NodeGpuCountLabel))
}

func GetNodeStartupTime(obj metav1.Object) string {
	return GetLabel(obj, NodeStartupTimeLabel)
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
