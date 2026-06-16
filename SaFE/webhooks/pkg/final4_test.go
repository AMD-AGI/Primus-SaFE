/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package webhooks

import (
	"context"
	"testing"

	"gotest.tools/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
)

// TestNodeMutateOnUpdateSubnet covers the subnet annotation action branch.
func TestNodeMutateOnUpdateSubnet(t *testing.T) {
	scheme := newScheme(t)
	m := &NodeMutator{Client: fake.NewClientBuilder().WithScheme(scheme).Build()}
	oldNode := validNode()
	newNode := validNode()
	v1.SetAnnotation(newNode, v1.NodeSubnetAnnotation, "10.0.0.0/16")
	assert.Assert(t, m.mutateOnUpdate(context.Background(), newNode, oldNode))
	assert.Assert(t, v1.HasAnnotation(newNode, v1.NodeAnnotationAction))
}

// TestFaultValidateOnUpdateSpecError covers fault spec validation on update.
func TestFaultValidateOnUpdateSpecError(t *testing.T) {
	v := &FaultValidator{}
	newFault := &v1.Fault{ObjectMeta: metav1.ObjectMeta{Name: "f1"}}
	assert.Assert(t, v.validateOnUpdate(newFault, newFault) != nil)
}
