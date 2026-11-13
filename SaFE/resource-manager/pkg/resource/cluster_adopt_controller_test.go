/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package resource

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/types"
	ctrlruntime "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/event"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
)

func TestAdoptedClusterPredicateIgnoresUnrelatedUpdates(t *testing.T) {
	p := adoptedClusterPredicate()
	old := testCluster("c1")
	v1.SetAnnotation(old, v1.ClusterAdoptAnnotation, v1.TrueStr)
	old.Generation = 1
	old.Status.ControlPlaneStatus.Phase = v1.ReadyPhase

	touched := old.DeepCopy()
	v1.SetAnnotation(touched, "example", "1")
	assert.False(t, p.Update(event.UpdateEvent{ObjectOld: old, ObjectNew: touched}))

	edited := old.DeepCopy()
	edited.Generation = 2
	assert.True(t, p.Update(event.UpdateEvent{ObjectOld: old, ObjectNew: edited}))

	pending := old.DeepCopy()
	pending.Status.ControlPlaneStatus.Phase = v1.PendingPhase
	assert.True(t, p.Update(event.UpdateEvent{ObjectOld: pending, ObjectNew: old}))
}

func TestClusterAdoptReconcileRequeuesPeriodically(t *testing.T) {
	cluster := testCluster("c1")
	v1.SetAnnotation(cluster, v1.ClusterAdoptAnnotation, v1.TrueStr)
	r := &ClusterAdoptReconciler{ClusterReconciler: newPlaneReconciler(t, cluster)}
	result, err := r.Reconcile(context.Background(), ctrlruntime.Request{NamespacedName: types.NamespacedName{Name: "c1"}})
	assert.NoError(t, err)
	assert.Positive(t, result.RequeueAfter)
}
