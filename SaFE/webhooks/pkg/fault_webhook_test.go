/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package webhooks

import (
	"context"
	"testing"

	"gotest.tools/assert"
	admissionv1 "k8s.io/api/admission/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
)

// TestFaultMutateOnCreation verifies fault label/finalizer mutations on creation.
func TestFaultMutateOnCreation(t *testing.T) {
	scheme := newScheme(t)
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	m := &FaultMutator{Client: k8sClient}
	fault := &v1.Fault{
		ObjectMeta: metav1.ObjectMeta{Name: "MyFault"},
		Spec: v1.FaultSpec{
			MonitorId: "m1",
			Node:      &v1.FaultNode{ClusterName: "cluster1", AdminName: "node1"},
		},
	}
	m.mutateOnCreation(context.Background(), fault)
	assert.Equal(t, v1.GetClusterId(fault), "cluster1")
	assert.Equal(t, v1.GetNodeId(fault), "node1")
}

// TestFaultMutateOnCreationNilNode verifies mutation does not panic when Node is nil.
func TestFaultMutateOnCreationNilNode(t *testing.T) {
	scheme := newScheme(t)
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	m := &FaultMutator{Client: k8sClient}
	fault := &v1.Fault{
		ObjectMeta: metav1.ObjectMeta{Name: "MyFault"},
		Spec:       v1.FaultSpec{MonitorId: "m1"},
	}
	m.mutateOnCreation(context.Background(), fault)
	assert.Equal(t, v1.GetLabel(fault, v1.FaultMonitorId), "m1")
	assert.Equal(t, v1.GetClusterId(fault), "")
}

// TestFaultMutatorHandle verifies the fault mutator admission handler.
func TestFaultMutatorHandle(t *testing.T) {
	scheme := newScheme(t)
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	m := &FaultMutator{Client: k8sClient, decoder: newDecoder(t)}
	fault := &v1.Fault{
		ObjectMeta: metav1.ObjectMeta{Name: "fault1"},
		Spec: v1.FaultSpec{
			MonitorId: "m1",
			Node:      &v1.FaultNode{ClusterName: "cluster1", AdminName: "node1"},
		},
	}
	resp := m.Handle(context.Background(), newRequest(t, admissionv1.Create, fault, nil))
	assert.Assert(t, resp.Allowed)

	resp = m.Handle(context.Background(), newRequest(t, admissionv1.Update, fault, nil))
	assert.Assert(t, resp.Allowed)
}

// TestFaultValidateFaultSpec verifies the fault spec validation rules.
func TestFaultValidateFaultSpec(t *testing.T) {
	v := &FaultValidator{}
	assert.Assert(t, v.validateFaultSpec(&v1.Fault{}) != nil)

	noCluster := &v1.Fault{Spec: v1.FaultSpec{MonitorId: "m1", Node: &v1.FaultNode{}}}
	assert.Assert(t, v.validateFaultSpec(noCluster) != nil)

	noAdmin := &v1.Fault{
		ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{v1.ClusterIdLabel: "c1"}},
		Spec:       v1.FaultSpec{MonitorId: "m1", Node: &v1.FaultNode{ClusterName: "c1"}},
	}
	assert.Assert(t, v.validateFaultSpec(noAdmin) != nil)

	ok := &v1.Fault{
		ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{v1.ClusterIdLabel: "c1"}},
		Spec:       v1.FaultSpec{MonitorId: "m1", Node: &v1.FaultNode{ClusterName: "c1", AdminName: "n1"}},
	}
	assert.NilError(t, v.validateFaultSpec(ok))
}

// TestFaultValidateOnCreation verifies create-time validation.
func TestFaultValidateOnCreation(t *testing.T) {
	v := &FaultValidator{}
	fault := &v1.Fault{Spec: v1.FaultSpec{MonitorId: "m1"}}
	assert.NilError(t, v.validateOnCreation(fault))
	assert.Assert(t, v.validateOnCreation(&v1.Fault{}) != nil)
}

// TestFaultValidateOnUpdate verifies update-time validation.
func TestFaultValidateOnUpdate(t *testing.T) {
	v := &FaultValidator{}
	fault := &v1.Fault{Spec: v1.FaultSpec{MonitorId: "m1"}}
	assert.NilError(t, v.validateOnUpdate(fault, fault))
}

// TestFaultValidatorHandle verifies the fault validator admission handler.
func TestFaultValidatorHandle(t *testing.T) {
	v := &FaultValidator{decoder: newDecoder(t)}
	fault := &v1.Fault{
		ObjectMeta: metav1.ObjectMeta{Name: "fault1"},
		Spec:       v1.FaultSpec{MonitorId: "m1"},
	}
	resp := v.Handle(context.Background(), newRequest(t, admissionv1.Create, fault, nil))
	assert.Assert(t, resp.Allowed)

	resp = v.Handle(context.Background(), newRequest(t, admissionv1.Update, fault, fault))
	assert.Assert(t, resp.Allowed)

	bad := &v1.Fault{ObjectMeta: metav1.ObjectMeta{Name: "fault1"}}
	resp = v.Handle(context.Background(), newRequest(t, admissionv1.Create, bad, nil))
	assert.Assert(t, !resp.Allowed)
}
