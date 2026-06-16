/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package syncer

import (
	"context"
	"testing"

	"gotest.tools/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
)

func TestGetK8sNodeEmptyName(t *testing.T) {
	r := &SyncerReconciler{}
	// Empty node name -> returns an empty node without touching the client.
	node, err := r.getK8sNode(context.Background(), nil, "")
	assert.NilError(t, err)
	assert.Assert(t, node != nil)
}

func TestUpdateWorkloadNodes(t *testing.T) {
	r := &SyncerReconciler{}
	w := &v1.Workload{ObjectMeta: metav1.ObjectMeta{
		Name:   "w",
		Labels: map[string]string{v1.WorkloadDispatchCntLabel: "1"},
	}}
	w.Status.Pods = []v1.WorkloadPod{
		{PodId: "p1", AdminNodeName: "n1", Rank: "0"},
		{PodId: "p2", AdminNodeName: "n2", Rank: "1"},
	}
	r.updateWorkloadNodes(w)
	assert.Equal(t, len(w.Status.Nodes), 1)
	assert.Equal(t, len(w.Status.Nodes[0]), 2)
}
