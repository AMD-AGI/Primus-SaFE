/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package syncer

import (
	"context"
	"testing"

	"gotest.tools/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sfake "k8s.io/client-go/kubernetes/fake"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	commonclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/k8sclient"
)

func TestWaitAllPodsDeletedEmpty(t *testing.T) {
	clientSets := clientSetsWith()
	r := &SyncerReconciler{}
	ok, err := r.waitAllPodsDeleted(context.Background(),
		&resourceMessage{name: "obj", namespace: "ns"}, clientSets)
	assert.NilError(t, err)
	// No pods -> considered fully deleted.
	assert.Equal(t, ok, true)
}

func TestWaitAllPodsDeletedRemaining(t *testing.T) {
	pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{
		Name:      "p1",
		Namespace: "ns",
		Labels:    map[string]string{v1.K8sObjectIdLabel: "obj"},
	}}
	cs := k8sfake.NewSimpleClientset(pod)
	clientSets := &ClusterClientSets{
		dataClientFactory: commonclient.NewClientFactoryWithOnlyClient(context.Background(), "c", cs),
	}
	r := &SyncerReconciler{}
	ok, err := r.waitAllPodsDeleted(context.Background(),
		&resourceMessage{name: "obj", namespace: "ns"}, clientSets)
	assert.NilError(t, err)
	// A matching pod still exists -> not fully deleted.
	assert.Equal(t, ok, false)
}

func TestShouldReScheduleEnded(t *testing.T) {
	r := &SyncerReconciler{}
	w := &v1.Workload{}
	w.Status.Phase = v1.WorkloadFailed
	ok, err := r.shouldReSchedule(context.Background(), w, &resourceMessage{}, nil)
	assert.NilError(t, err)
	// Ended workloads are never rescheduled.
	assert.Equal(t, ok, false)
}

func TestShouldReScheduleResourceDeleted(t *testing.T) {
	r := &SyncerReconciler{}
	w := &v1.Workload{}
	ok, err := r.shouldReSchedule(context.Background(), w,
		&resourceMessage{action: ResourceDel}, nil)
	assert.NilError(t, err)
	// A delete event on a live workload triggers reschedule.
	assert.Equal(t, ok, true)
}
