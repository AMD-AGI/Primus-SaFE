/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package informer

import (
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrlfake "sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
)

func newInformer(t *testing.T) *WorkloadInformer {
	t.Helper()
	s := runtime.NewScheme()
	if err := v1.AddToScheme(s); err != nil {
		t.Fatal(err)
	}
	if err := corev1.AddToScheme(s); err != nil {
		t.Fatal(err)
	}
	cl := ctrlfake.NewClientBuilder().WithScheme(s).Build()
	return NewWorkloadInformer(cl)
}

func TestOnAdd(t *testing.T) {
	w := newInformer(t)
	// Wrong type -> no panic.
	w.OnAdd("not-a-workload", false)
	// In initial list -> skipped.
	w.OnAdd(&v1.Workload{Status: v1.WorkloadStatus{Phase: v1.WorkloadRunning}}, true)
	// Empty phase -> skipped.
	w.OnAdd(&v1.Workload{}, false)
	// Phase set, user not found -> handled gracefully.
	w.OnAdd(&v1.Workload{ObjectMeta: metav1.ObjectMeta{Name: "wl1"}, Status: v1.WorkloadStatus{Phase: v1.WorkloadRunning}}, false)
}

func TestOnUpdate(t *testing.T) {
	w := newInformer(t)
	// Wrong new type.
	w.OnUpdate(&v1.Workload{}, "bad")
	// Wrong old type.
	w.OnUpdate("bad", &v1.Workload{Status: v1.WorkloadStatus{Phase: v1.WorkloadRunning}})
	// Empty phase.
	w.OnUpdate(&v1.Workload{}, &v1.Workload{})
	// Same phase -> skipped.
	same := &v1.Workload{Status: v1.WorkloadStatus{Phase: v1.WorkloadRunning}}
	w.OnUpdate(same, same.DeepCopy())
	// Phase changed -> reaches submit (user not found -> graceful).
	oldW := &v1.Workload{ObjectMeta: metav1.ObjectMeta{Name: "wl1"}, Status: v1.WorkloadStatus{Phase: v1.WorkloadPending}}
	newW := &v1.Workload{ObjectMeta: metav1.ObjectMeta{Name: "wl1"}, Status: v1.WorkloadStatus{Phase: v1.WorkloadRunning}}
	w.OnUpdate(oldW, newW)
}

func TestOnDelete(t *testing.T) {
	w := newInformer(t)
	w.OnDelete(&v1.Workload{})
	assert.NotNil(t, w)
}
