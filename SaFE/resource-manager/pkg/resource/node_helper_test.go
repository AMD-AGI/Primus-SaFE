/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package resource

import (
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/pointer"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
)

// TestGenNodeOwnerReference tests the generation of owner references for nodes
func TestGenNodeOwnerReference(t *testing.T) {
	tests := []struct {
		name     string
		node     *v1.Node
		validate func(*testing.T, metav1.OwnerReference)
	}{
		{
			name: "standard node",
			node: &v1.Node{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "amd.io/v1",
					Kind:       "Node",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-node-1",
					UID:  types.UID("node-uid-123"),
				},
			},
			validate: func(t *testing.T, ref metav1.OwnerReference) {
				assert.Equal(t, "amd.io/v1", ref.APIVersion)
				assert.Equal(t, "Node", ref.Kind)
				assert.Equal(t, "test-node-1", ref.Name)
				assert.Equal(t, types.UID("node-uid-123"), ref.UID)
				assert.NotNil(t, ref.Controller)
				assert.True(t, *ref.Controller)
				assert.NotNil(t, ref.BlockOwnerDeletion)
				assert.True(t, *ref.BlockOwnerDeletion)
			},
		},
		{
			name: "node with different API version",
			node: &v1.Node{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "amd.io/v2",
					Kind:       "Node",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-node-2",
					UID:  types.UID("node-uid-456"),
				},
			},
			validate: func(t *testing.T, ref metav1.OwnerReference) {
				assert.Equal(t, "amd.io/v2", ref.APIVersion)
				assert.Equal(t, "test-node-2", ref.Name)
				assert.Equal(t, types.UID("node-uid-456"), ref.UID)
			},
		},
		{
			name: "node with long name",
			node: &v1.Node{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "amd.io/v1",
					Kind:       "Node",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name: "very-long-node-name-with-many-characters-12345",
					UID:  types.UID("node-uid-789"),
				},
			},
			validate: func(t *testing.T, ref metav1.OwnerReference) {
				assert.Equal(t, "very-long-node-name-with-many-characters-12345", ref.Name)
				assert.Equal(t, types.UID("node-uid-789"), ref.UID)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := genNodeOwnerReference(tt.node)
			tt.validate(t, result)
		})
	}
}

// TestGenNodeOwnerReferenceFields tests that all fields are properly set
func TestGenNodeOwnerReferenceFields(t *testing.T) {
	node := &v1.Node{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "amd.io/v1",
			Kind:       "Node",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-node",
			UID:  types.UID("test-uid"),
		},
	}

	ref := genNodeOwnerReference(node)

	// Verify all required fields are set
	assert.NotEmpty(t, ref.APIVersion, "APIVersion should not be empty")
	assert.NotEmpty(t, ref.Kind, "Kind should not be empty")
	assert.NotEmpty(t, ref.Name, "Name should not be empty")
	assert.NotEmpty(t, ref.UID, "UID should not be empty")

	// Verify pointer fields are not nil and have correct values
	assert.NotNil(t, ref.Controller, "Controller should not be nil")
	assert.Equal(t, pointer.Bool(true), ref.Controller, "Controller should be true")

	assert.NotNil(t, ref.BlockOwnerDeletion, "BlockOwnerDeletion should not be nil")
	assert.Equal(t, pointer.Bool(true), ref.BlockOwnerDeletion, "BlockOwnerDeletion should be true")
}

func TestGetKubeSprayScaleCMDs(t *testing.T) {
	up := getKubeSprayScaleUpCMD("u", "n1", "env")
	assert.Contains(t, up, "scale.yml")
	assert.Contains(t, up, "n1")
	down := getKubeSprayScaleDownCMD("u", "n1", "env")
	assert.Contains(t, down, "remove-node.yml")
	assert.Contains(t, down, "n1")
}

func TestIsCommandSuccessful(t *testing.T) {
	status := []v1.CommandStatus{{Name: "c1", Phase: v1.CommandSucceeded}}
	assert.True(t, isCommandSuccessful(status, "c1"))
	assert.False(t, isCommandSuccessful(status, "c2"))
}

func TestSetCommandStatus(t *testing.T) {
	var status []v1.CommandStatus
	status = setCommandStatus(status, "c1", v1.CommandSucceeded)
	assert.Len(t, status, 1)
	// Update existing.
	status = setCommandStatus(status, "c1", v1.CommandFailed)
	assert.Len(t, status, 1)
	assert.Equal(t, v1.CommandFailed, status[0].Phase)
}

func TestIsK8sNodeReady(t *testing.T) {
	ready := &corev1.Node{Status: corev1.NodeStatus{Conditions: []corev1.NodeCondition{
		{Type: corev1.NodeReady, Status: corev1.ConditionTrue},
	}}}
	assert.True(t, isK8sNodeReady(ready))
	notReady := &corev1.Node{Status: corev1.NodeStatus{Conditions: []corev1.NodeCondition{
		{Type: corev1.NodeReady, Status: corev1.ConditionFalse},
	}}}
	assert.False(t, isK8sNodeReady(notReady))
}

func TestIsConditionsChanged(t *testing.T) {
	old := []corev1.NodeCondition{{Type: corev1.NodeReady, Status: corev1.ConditionTrue}}
	same := []corev1.NodeCondition{{Type: corev1.NodeReady, Status: corev1.ConditionTrue}}
	assert.False(t, isConditionsChanged(old, same))

	diffLen := []corev1.NodeCondition{}
	assert.True(t, isConditionsChanged(old, diffLen))

	diffStatus := []corev1.NodeCondition{{Type: corev1.NodeReady, Status: corev1.ConditionFalse}}
	assert.True(t, isConditionsChanged(old, diffStatus))
}
