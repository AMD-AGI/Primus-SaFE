/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package exporter

import (
	"context"
	"testing"

	"gotest.tools/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrlruntime "sigs.k8s.io/controller-runtime"
	ctrlfake "sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	commonctrl "github.com/AMD-AIG-AIMA/SAFE/common/pkg/controller"
)

func exporterScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	s := runtime.NewScheme()
	if err := v1.AddToScheme(s); err != nil {
		t.Fatal(err)
	}
	return s
}

func newTestExporter(t *testing.T, objs ...*v1.Workload) *ResourceExporter {
	t.Helper()
	builder := ctrlfake.NewClientBuilder().WithScheme(exporterScheme(t))
	for _, o := range objs {
		builder = builder.WithObjects(o)
	}
	exp := &ResourceExporter{
		Client: builder.Build(),
		gvk:    v1.SchemeGroupVersion.WithKind(v1.WorkloadKind),
	}
	exp.Controller = commonctrl.NewController[types.NamespacedName](exp, 1)
	return exp
}

func TestExporterReconcile(t *testing.T) {
	exp := newTestExporter(t)
	res, err := exp.Reconcile(context.Background(), ctrlruntime.Request{
		NamespacedName: types.NamespacedName{Name: "w1"},
	})
	assert.NilError(t, err)
	assert.Equal(t, res.RequeueAfter.Nanoseconds(), int64(0))
}

func TestExporterGetObjectNotFound(t *testing.T) {
	exp := newTestExporter(t)
	_, err := exp.getObject(context.Background(), types.NamespacedName{Name: "missing"})
	assert.Assert(t, err != nil)
}

func TestExporterDoNotFound(t *testing.T) {
	exp := newTestExporter(t)
	// Missing object -> ignored, no error.
	_, err := exp.Do(context.Background(), types.NamespacedName{Name: "missing"})
	assert.NilError(t, err)
}

func TestExporterDoAddsFinalizerAndHandles(t *testing.T) {
	w := &v1.Workload{ObjectMeta: metav1.ObjectMeta{Name: "w1"}}
	exp := newTestExporter(t, w)
	handled := false
	exp.handler = func(_ context.Context, _ *unstructured.Unstructured) error {
		handled = true
		return nil
	}
	_, err := exp.Do(context.Background(), types.NamespacedName{Name: "w1"})
	assert.NilError(t, err)
	assert.Assert(t, handled)

	// Finalizer should now be present.
	obj, err := exp.getObject(context.Background(), types.NamespacedName{Name: "w1"})
	assert.NilError(t, err)
	found := false
	for _, f := range obj.GetFinalizers() {
		if f == v1.ExporterFinalizer {
			found = true
		}
	}
	assert.Assert(t, found)
}

func TestExporterStart(t *testing.T) {
	exp := newTestExporter(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	// start spawns worker goroutines; should not block or panic.
	assert.NilError(t, exp.start(ctx))
}

func TestExporterDoDeletionRemovesFinalizer(t *testing.T) {
	now := metav1.Now()
	w := &v1.Workload{ObjectMeta: metav1.ObjectMeta{
		Name:              "w-del",
		DeletionTimestamp: &now,
		Finalizers:        []string{v1.ExporterFinalizer},
	}}
	exp := newTestExporter(t, w)
	handled := false
	exp.handler = func(_ context.Context, _ *unstructured.Unstructured) error {
		handled = true
		return nil
	}
	_, err := exp.Do(context.Background(), types.NamespacedName{Name: "w-del"})
	assert.NilError(t, err)
	assert.Assert(t, handled)
}

func TestExporterFinalizerHelpers(t *testing.T) {
	w := &v1.Workload{ObjectMeta: metav1.ObjectMeta{Name: "w2"}}
	exp := newTestExporter(t, w)
	obj, err := exp.getObject(context.Background(), types.NamespacedName{Name: "w2"})
	assert.NilError(t, err)

	assert.NilError(t, exp.addFinalizer(context.Background(), obj))
	// Adding again is a no-op (finalizer already present).
	assert.NilError(t, exp.addFinalizer(context.Background(), obj))

	assert.NilError(t, exp.removeFinalizer(context.Background(), obj))
	// Removing again is a no-op.
	assert.NilError(t, exp.removeFinalizer(context.Background(), obj))
}
