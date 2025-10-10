/*
 * Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package utils

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	utilrand "k8s.io/apimachinery/pkg/util/rand"
)

const (
	MaxNameLength          = 63
	randomLength           = 5
	MaxGeneratedNameLength = MaxNameLength - randomLength - 1
	// 12 is the fixed suffix length of pytorchjob.
	MaxDisplayNameLen = MaxGeneratedNameLength - 12
)

func GenerateName(base string) string {
	if base == "" {
		return ""
	}
	if len(base) > MaxGeneratedNameLength {
		base = base[0:MaxGeneratedNameLength]
	}
	return fmt.Sprintf("%s-%s", base, utilrand.String(randomLength))
}

func GetBaseFromName(name string) string {
	if len(name) <= randomLength+1 {
		return name
	}
	lastIndex := len(name) - randomLength - 1
	if name[lastIndex] != '-' {
		return name
	}
	return name[:lastIndex]
}

func GenObjectReference(typeMeta metav1.TypeMeta, objMeta metav1.ObjectMeta) *corev1.ObjectReference {
	return &corev1.ObjectReference{
		Namespace:       objMeta.GetNamespace(),
		Name:            objMeta.GetName(),
		UID:             objMeta.GetUID(),
		APIVersion:      typeMeta.APIVersion,
		Kind:            typeMeta.Kind,
		ResourceVersion: objMeta.GetResourceVersion(),
	}
}

func GenerateClusterPriorityClass(clusterId, priorityClass string) string {
	return clusterId + "-" + priorityClass
}

func GenerateClusterSecret(clusterId, secretName string) string {
	return clusterId + "-" + secretName
}
