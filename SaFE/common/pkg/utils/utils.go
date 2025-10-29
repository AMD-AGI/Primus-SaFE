/*
 * Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package utils

import (
	"encoding/json"
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

// GenerateName generates a unique name by appending a random string to the base name.
func GenerateName(base string) string {
	if base == "" {
		return ""
	}
	if len(base) > MaxGeneratedNameLength {
		base = base[0:MaxGeneratedNameLength]
	}
	return fmt.Sprintf("%s-%s", base, utilrand.String(randomLength))
}

// GetBaseFromName extracts the base name from a generated name by removing the random suffix.
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

// GenObjectReference creates an ObjectReference from TypeMeta and ObjectMeta.
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

// GenerateClusterPriorityClass creates a cluster-specific priority class name.
func GenerateClusterPriorityClass(clusterId, priorityClass string) string {
	return clusterId + "-" + priorityClass
}

// GenerateClusterSecret creates a cluster-specific secret name.
func GenerateClusterSecret(clusterId, secretName string) string {
	return clusterId + "-" + secretName
}

// TransMapToStruct converts a map to a struct using JSON serialization.
func TransMapToStruct(m map[string]interface{}, out interface{}) error {
	jsonBytes, err := json.Marshal(m)
	if err != nil {
		return err
	}
	err = json.Unmarshal(jsonBytes, out)
	if err != nil {
		return err
	}
	return nil
}

// StringsIn checks if a string is present in a slice of strings.
func StringsIn(str string, strs []string) bool {
	for _, s := range strs {
		if s == str {
			return true
		}
	}
	return false
}
