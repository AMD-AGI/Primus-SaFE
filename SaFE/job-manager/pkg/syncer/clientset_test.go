/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package syncer

import (
	"context"
	"testing"
	"time"

	"gotest.tools/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	k8sfake "k8s.io/client-go/kubernetes/fake"

	commonclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/k8sclient"
)

func clientSetsWith() *ClusterClientSets {
	cs := k8sfake.NewSimpleClientset()
	return &ClusterClientSets{
		dataClientFactory: commonclient.NewClientFactoryWithOnlyClient(context.Background(), "c", cs),
	}
}

func TestGetK8sNodeFound(t *testing.T) {
	node := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node-1"}}
	cs := k8sfake.NewSimpleClientset(node)
	clientSets := &ClusterClientSets{
		dataClientFactory: commonclient.NewClientFactoryWithOnlyClient(context.Background(), "c", cs),
	}
	r := &SyncerReconciler{}
	got, err := r.getK8sNode(context.Background(), clientSets, "node-1")
	assert.NilError(t, err)
	assert.Equal(t, got.Name, "node-1")
}

func TestGetK8sNodeNotFound(t *testing.T) {
	clientSets := clientSetsWith()
	r := &SyncerReconciler{}
	_, err := r.getK8sNode(context.Background(), clientSets, "missing")
	assert.Assert(t, err != nil)
}

func TestDeletePodForceDelete(t *testing.T) {
	cs := k8sfake.NewSimpleClientset()
	clientSets := &ClusterClientSets{
		dataClientFactory: commonclient.NewClientFactoryWithOnlyClient(context.Background(), "c", cs),
	}
	r := &SyncerReconciler{}

	obj := &unstructured.Unstructured{Object: map[string]interface{}{}}
	obj.SetName("p1")
	obj.SetNamespace("ns")
	old := metav1.NewTime(time.Now().Add(-time.Duration(ForceDeleteDelaySeconds+60) * time.Second))
	obj.SetDeletionTimestamp(&old)

	// Old deletion timestamp -> force delete path (pod absent -> NotFound ignored).
	res, err := r.deletePod(context.Background(), obj, clientSets)
	assert.NilError(t, err)
	assert.Equal(t, res.RequeueAfter.Nanoseconds(), int64(0))
}
