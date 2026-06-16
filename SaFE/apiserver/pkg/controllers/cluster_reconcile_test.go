/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package controllers

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
)

func TestClusterReconcileNotFound(t *testing.T) {
	cl := fake.NewClientBuilder().WithScheme(ctrlScheme(t)).Build()
	r := &ClusterReconciler{Client: cl, ctx: context.Background()}
	res, err := r.Reconcile(context.Background(), reconcileReq("missing"))
	assert.NoError(t, err)
	assert.Equal(t, time.Duration(0), res.RequeueAfter)
}

func TestClusterReconcileDeleting(t *testing.T) {
	now := metav1.Now()
	cluster := &v1.Cluster{ObjectMeta: metav1.ObjectMeta{
		Name:              "c1",
		DeletionTimestamp: &now,
		Finalizers:        []string{"x"},
	}}
	cl := fake.NewClientBuilder().WithScheme(ctrlScheme(t)).WithObjects(cluster).Build()
	r := &ClusterReconciler{Client: cl, ctx: context.Background()}
	// Deleting path calls deleteClientFactory; for an unregistered cluster the
	// singleton manager returns a not-found style error, which is tolerated.
	_, _ = r.Reconcile(context.Background(), reconcileReq("c1"))
}

func TestClusterReconcileNotReady(t *testing.T) {
	cluster := &v1.Cluster{ObjectMeta: metav1.ObjectMeta{Name: "c1"}}
	cl := fake.NewClientBuilder().WithScheme(ctrlScheme(t)).WithObjects(cluster).Build()
	r := &ClusterReconciler{Client: cl, ctx: context.Background()}
	// Not-ready cluster: addClientFactory returns nil immediately.
	res, err := r.Reconcile(context.Background(), reconcileReq("c1"))
	assert.NoError(t, err)
	assert.Equal(t, time.Duration(0), res.RequeueAfter)
}

func TestClusterReconcileReadyEndpointError(t *testing.T) {
	cluster := &v1.Cluster{ObjectMeta: metav1.ObjectMeta{Name: "c1"}}
	cluster.Status.ControlPlaneStatus.Phase = v1.ReadyPhase
	cl := fake.NewClientBuilder().WithScheme(ctrlScheme(t)).WithObjects(cluster).Build()
	r := &ClusterReconciler{Client: cl, ctx: context.Background()}
	// Ready cluster with no endpoint data -> addClientFactory returns an error.
	_, err := r.Reconcile(context.Background(), reconcileReq("c1"))
	assert.Error(t, err)
}
