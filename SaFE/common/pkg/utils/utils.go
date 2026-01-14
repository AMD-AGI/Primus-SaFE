/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package utils

import (
	"context"
	"encoding/json"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	utilrand "k8s.io/apimachinery/pkg/util/rand"
	"sigs.k8s.io/controller-runtime/pkg/client"

	commonconfig "github.com/AMD-AIG-AIMA/SAFE/common/pkg/config"
)

const (
	MaxNameLength          = 63
	randomLength           = 5
	MaxGeneratedNameLength = MaxNameLength - randomLength - 1
	// 13 is the fixed suffix length of pytorchjob pod.
	MaxPytorchJobNameLen = MaxGeneratedNameLength - 13
	// 17 is the fixed suffix length of deployment pod.
	MaxDeploymentNameLen   = MaxGeneratedNameLength - 17
	MaxCICDScaleSetNameLen = 39
	MaxTorchFTNameLen      = MaxDeploymentNameLen - 4
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

// GenerateCICDNoPermissionName creates a cicd service account name.
func GenerateCICDNoPermissionName() string {
	return commonconfig.GetCICDRoleName() + "-no-permission"
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

// PatchObjectFinalizer updates the finalizers of a structured Kubernetes object using a merge patch.
// This function is used to add or remove finalizers from Kubernetes resources.
func PatchObjectFinalizer(ctx context.Context, cli client.Client, object client.Object) error {
	finalizers := object.GetFinalizers()
	if finalizers == nil {
		finalizers = []string{}
	}

	patchObj := map[string]any{
		"metadata": map[string]any{
			"resourceVersion": object.GetResourceVersion(),
			"finalizers":      finalizers,
		},
	}
	p, err := json.Marshal(patchObj)
	if err != nil {
		return err
	}
	if err = cli.Patch(ctx, object, client.RawPatch(types.MergePatchType, p)); err != nil {
		return err
	}
	return nil
}

func HasOwnerReferences(obj metav1.Object, name string) bool {
	for _, r := range obj.GetOwnerReferences() {
		if r.Name == name {
			return true
		}
	}
	return false
}
