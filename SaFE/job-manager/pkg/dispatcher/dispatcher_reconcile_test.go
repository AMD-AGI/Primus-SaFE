/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package dispatcher

import (
	"context"
	"testing"

	"gotest.tools/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrlruntime "sigs.k8s.io/controller-runtime"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/event"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	commonutils "github.com/AMD-AIG-AIMA/SAFE/common/pkg/utils"
)

func TestDispatcherReconcileNotFound(t *testing.T) {
	scheme, err := genMockScheme()
	assert.NilError(t, err)
	cl := fake.NewClientBuilder().WithScheme(scheme).Build()
	r := &DispatcherReconciler{Client: cl}
	_, rerr := r.Reconcile(context.Background(), ctrlruntime.Request{
		NamespacedName: ctrlclient.ObjectKey{Name: "missing"},
	})
	assert.NilError(t, rerr)
}

func TestDispatcherRelevantChangePredicateCreate(t *testing.T) {
	p := relevantChangePredicate{}

	// Non-workload object -> false.
	assert.Equal(t, p.Create(event.CreateEvent{Object: &corev1.Pod{}}), false)

	// Scheduled but not dispatched -> dispatchable -> true.
	w := &v1.Workload{ObjectMeta: metav1.ObjectMeta{
		Annotations: map[string]string{v1.WorkloadScheduledAnnotation: "true"},
	}}
	assert.Equal(t, p.Create(event.CreateEvent{Object: w}), true)

	// Neither scheduled nor dispatched -> false.
	assert.Equal(t, p.Create(event.CreateEvent{Object: &v1.Workload{}}), false)
}

func TestDispatcherRelevantChangePredicateUpdate(t *testing.T) {
	p := relevantChangePredicate{}

	// Wrong types -> false.
	assert.Equal(t, p.Update(event.UpdateEvent{ObjectOld: &corev1.Pod{}, ObjectNew: &corev1.Pod{}}), false)

	// Transition into dispatchable -> true.
	oldW := &v1.Workload{}
	newW := &v1.Workload{ObjectMeta: metav1.ObjectMeta{
		Annotations: map[string]string{v1.WorkloadScheduledAnnotation: "true"},
	}}
	assert.Equal(t, p.Update(event.UpdateEvent{ObjectOld: oldW, ObjectNew: newW}), true)
}

func TestProcessTorchFTWorkloadNoLighthouse(t *testing.T) {
	// With no TorchFT lighthouse configured, processing fails fast.
	r := &DispatcherReconciler{}
	w := &v1.Workload{ObjectMeta: metav1.ObjectMeta{Name: "w"}}
	_, err := r.processTorchFTWorkload(context.Background(), w)
	assert.Assert(t, err != nil)
}

func TestProcessWorkloadNoClusterClientSets(t *testing.T) {
	r := &DispatcherReconciler{clusterClientSets: commonutils.NewObjectManager()}
	w := &v1.Workload{ObjectMeta: metav1.ObjectMeta{Name: "w"}}
	res, err := r.processWorkload(context.Background(), w)
	assert.NilError(t, err)
	// No cluster client sets -> requeue.
	assert.Assert(t, res.RequeueAfter > 0)
}

func TestGenerateJobPortAlreadyDispatched(t *testing.T) {
	r := &DispatcherReconciler{}
	w := &v1.Workload{ObjectMeta: metav1.ObjectMeta{
		Annotations: map[string]string{v1.WorkloadDispatchedAnnotation: "true"},
	}}
	// Already dispatched -> no-op, returns nil.
	assert.NilError(t, r.generateJobPort(context.Background(), w))
}
