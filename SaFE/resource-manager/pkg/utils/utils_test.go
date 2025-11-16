/*
 * Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package utils

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"

	commonerrors "github.com/AMD-AIG-AIMA/SAFE/common/pkg/errors"
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

// TestIsNonRetryableError tests the identification of non-retryable errors
func TestIsNonRetryableError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "nil error",
			err:      nil,
			expected: false,
		},
		{
			name:     "bad request error",
			err:      commonerrors.NewBadRequest("invalid input"),
			expected: true,
		},
		{
			name:     "internal error",
			err:      commonerrors.NewInternalError("internal server error"),
			expected: true,
		},
		{
			name:     "not found error",
			err:      commonerrors.NewNotFound("resource", "test-resource"),
			expected: true,
		},
		{
			name:     "k8s forbidden error",
			err:      apierrors.NewForbidden(schema.GroupResource{Group: "apps", Resource: "deployments"}, "test", fmt.Errorf("forbidden")),
			expected: true,
		},
		{
			name:     "k8s not found error",
			err:      apierrors.NewNotFound(schema.GroupResource{Group: "apps", Resource: "deployments"}, "test"),
			expected: true,
		},
		{
			name:     "retryable error - timeout",
			err:      apierrors.NewTimeoutError("timeout", 30),
			expected: false,
		},
		{
			name:     "retryable error - service unavailable",
			err:      apierrors.NewServiceUnavailable("service unavailable"),
			expected: false,
		},
		{
			name:     "retryable error - too many requests",
			err:      apierrors.NewTooManyRequests("too many requests", 10),
			expected: false,
		},
		{
			name:     "generic error",
			err:      fmt.Errorf("some generic error"),
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsNonRetryableError(tt.err)
			assert.Equal(t, tt.expected, result)
		})
	}
}
