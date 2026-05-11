/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package utils

import (
	"testing"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

// TestRemoveOwnerReferences tests the removal of owner references by UID
func TestRemoveOwnerReferences(t *testing.T) {
	uid1 := types.UID("uid-1")
	uid2 := types.UID("uid-2")
	uid3 := types.UID("uid-3")

	tests := []struct {
		name        string
		references  []metav1.OwnerReference
		uidToRemove types.UID
		expected    []metav1.OwnerReference
	}{
		{
			name: "remove single reference",
			references: []metav1.OwnerReference{
				{UID: uid1, Name: "owner1"},
				{UID: uid2, Name: "owner2"},
			},
			uidToRemove: uid1,
			expected: []metav1.OwnerReference{
				{UID: uid2, Name: "owner2"},
			},
		},
		{
			name: "remove non-existent UID",
			references: []metav1.OwnerReference{
				{UID: uid1, Name: "owner1"},
				{UID: uid2, Name: "owner2"},
			},
			uidToRemove: uid3,
			expected: []metav1.OwnerReference{
				{UID: uid1, Name: "owner1"},
				{UID: uid2, Name: "owner2"},
			},
		},
		{
			name:        "remove from empty list",
			references:  []metav1.OwnerReference{},
			uidToRemove: uid1,
			expected:    []metav1.OwnerReference{},
		},
		{
			name: "remove all references with same UID",
			references: []metav1.OwnerReference{
				{UID: uid1, Name: "owner1"},
				{UID: uid1, Name: "owner1-duplicate"},
				{UID: uid2, Name: "owner2"},
			},
			uidToRemove: uid1,
			expected: []metav1.OwnerReference{
				{UID: uid2, Name: "owner2"},
			},
		},
		{
			name: "remove last reference",
			references: []metav1.OwnerReference{
				{UID: uid1, Name: "owner1"},
			},
			uidToRemove: uid1,
			expected:    []metav1.OwnerReference{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := RemoveOwnerReferences(tt.references, tt.uidToRemove)
			assert.Equal(t, len(tt.expected), len(result))
			for i, ref := range result {
				assert.Equal(t, tt.expected[i].UID, ref.UID)
				assert.Equal(t, tt.expected[i].Name, ref.Name)
			}
		})
	}
}
