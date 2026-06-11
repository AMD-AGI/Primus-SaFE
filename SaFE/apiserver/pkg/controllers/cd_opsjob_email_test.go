/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package controllers

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
)

func TestGetUserEmail(t *testing.T) {
	user := &v1.User{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "u1",
			Annotations: map[string]string{v1.UserEmailAnnotation: "u1@example.com"},
		},
	}
	scheme := runtime.NewScheme()
	_ = v1.AddToScheme(scheme)
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(user).Build()
	r := &CDOpsJobReconciler{Client: fakeClient}

	// Empty userId -> empty string.
	assert.Equal(t, "", r.getUserEmail(context.Background(), ""))

	// Existing user -> email from annotation.
	assert.Equal(t, "u1@example.com", r.getUserEmail(context.Background(), "u1"))

	// Missing user -> empty string.
	assert.Equal(t, "", r.getUserEmail(context.Background(), "missing"))
}
