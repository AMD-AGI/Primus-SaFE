/*
 * Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package resource

import (
	"testing"

	"github.com/stretchr/testify/assert"
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
