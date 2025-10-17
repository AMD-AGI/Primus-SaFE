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

// GenerateName generates a unique name by appending a random string to the base name
// Parameters:
//
//	base: Base name string to which random suffix will be appended
//
// Returns:
//
//	Generated name with random suffix, truncated if necessary to meet length limits
//
// Logic:
//  1. If base is empty, return empty string
//  2. If base exceeds MaxGeneratedNameLength, truncate it
//  3. Append random string of length randomLength separated by hyphen
func GenerateName(base string) string {
	if base == "" {
		return ""
	}
	if len(base) > MaxGeneratedNameLength {
		base = base[0:MaxGeneratedNameLength]
	}
	return fmt.Sprintf("%s-%s", base, utilrand.String(randomLength))
}

// GetBaseFromName extracts the base name from a generated name by removing the random suffix
// Parameters:
//   name: Generated name containing base and random suffix
// Returns:
//   Base name without random suffix, or original name if format doesn't match
// Logic:
//   1. Check if name length is sufficient to contain random suffix
//   2. Verify the expected hyphen separator exists at the correct position
//   3. Return the base portion before the separator
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

// GenObjectReference creates an ObjectReference from TypeMeta and ObjectMeta
// Parameters:
//   typeMeta: Type metadata containing APIVersion and Kind
//   objMeta: Object metadata containing namespace, name, UID, and resource version
// Returns:
//   Pointer to corev1.ObjectReference populated with the provided metadat
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

// GenerateClusterPriorityClass creates a cluster-specific priority class name
// Parameters:
//   clusterId: Cluster identifier
//   priorityClass: Base priority class name
// Returns:
//   Combined string in format "clusterId-priorityClass"
func GenerateClusterPriorityClass(clusterId, priorityClass string) string {
	return clusterId + "-" + priorityClass
}

// GenerateClusterSecret creates a cluster-specific secret name
// Parameters:
//   clusterId: Cluster identifier
//   secretName: Base secret name
// Returns:
//   Combined string in format "clusterId-secretName
func GenerateClusterSecret(clusterId, secretName string) string {
	return clusterId + "-" + secretName
}
