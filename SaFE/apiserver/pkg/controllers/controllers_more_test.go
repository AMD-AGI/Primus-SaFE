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
	"sigs.k8s.io/controller-runtime/pkg/event"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
)

func TestGetParameterValue(t *testing.T) {
	job := &v1.OpsJob{
		Spec: v1.OpsJobSpec{Inputs: []v1.Parameter{{Name: "workload", Value: "wl-1"}}},
	}
	assert.Equal(t, "wl-1", getParameterValue(job, "workload"))
	assert.Equal(t, "", getParameterValue(job, "missing"))
}

func TestGetJobFailureReasonImpl(t *testing.T) {
	// From outputs.
	job := &v1.OpsJob{}
	job.Status.Outputs = []v1.Parameter{{Name: "result", Value: "out-of-memory"}}
	assert.Equal(t, "out-of-memory", getJobFailureReason(job))

	// From conditions.
	job2 := &v1.OpsJob{}
	job2.Status.Conditions = []metav1.Condition{{Status: "False", Message: "bad node"}}
	assert.Equal(t, "bad node", getJobFailureReason(job2))

	// Default.
	assert.Equal(t, "CD deployment failed", getJobFailureReason(&v1.OpsJob{}))
}

func TestRelevantChangePredicate(t *testing.T) {
	r := &ClusterReconciler{}
	p := r.relevantChangePredicate()

	ready := &v1.Cluster{ObjectMeta: metav1.ObjectMeta{Name: "c1"}}
	ready.Status.ControlPlaneStatus.Phase = v1.ReadyPhase
	notReady := &v1.Cluster{ObjectMeta: metav1.ObjectMeta{Name: "c2"}}

	// Create: only ready clusters pass.
	assert.True(t, p.Create(event.CreateEvent{Object: ready}))
	assert.False(t, p.Create(event.CreateEvent{Object: notReady}))

	// Update: transition not-ready -> ready passes.
	assert.True(t, p.Update(event.UpdateEvent{ObjectOld: notReady, ObjectNew: ready}))
	// ready -> ready (no transition) does not pass.
	assert.False(t, p.Update(event.UpdateEvent{ObjectOld: ready, ObjectNew: ready}))

	// Delete: any cluster passes.
	assert.True(t, p.Delete(event.DeleteEvent{Object: ready}))
}

func TestAddClientFactoryNotReady(t *testing.T) {
	r := &ClusterReconciler{}
	// Non-ready cluster -> returns nil immediately (no endpoint lookup).
	err := r.addClientFactory(context.Background(), &v1.Cluster{ObjectMeta: metav1.ObjectMeta{Name: "c1"}})
	assert.NoError(t, err)
}

func TestDeleteClientFactory(t *testing.T) {
	r := &ClusterReconciler{}
	// Deleting an unregistered cluster is a no-op error path tolerated by the
	// singleton manager; either nil or a not-found style error is acceptable.
	_ = r.deleteClientFactory(&v1.Cluster{ObjectMeta: metav1.ObjectMeta{Name: "nonexistent-cluster"}})
}
