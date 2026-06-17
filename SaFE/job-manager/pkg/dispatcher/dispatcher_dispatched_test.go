/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package dispatcher

import (
	"context"
	"testing"

	"gotest.tools/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
)

func TestMarkAsDispatchedRootWorkload(t *testing.T) {
	r := &DispatcherReconciler{}
	w := &v1.Workload{ObjectMeta: metav1.ObjectMeta{Name: "child"}}
	v1.SetLabel(w, v1.RootWorkloadIdLabel, "root")
	// Child workloads of a root are skipped.
	err := r.markAsDispatched(context.Background(), w)
	assert.NilError(t, err)
}

func TestMarkAsDispatched(t *testing.T) {
	scheme, err := genMockScheme()
	assert.NilError(t, err)
	w := &v1.Workload{ObjectMeta: metav1.ObjectMeta{Name: "w"}}
	cl := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(w).
		WithStatusSubresource(&v1.Workload{}).
		Build()
	r := &DispatcherReconciler{Client: cl}

	err = r.markAsDispatched(context.Background(), w)
	assert.NilError(t, err)
	assert.Equal(t, v1.IsWorkloadDispatched(w), true)
}

func TestIsResourceChangedErrorReturnsFalse(t *testing.T) {
	w := &v1.Workload{}
	w.Spec.Resources = []v1.WorkloadResource{{Replica: 1, CPU: "2"}}
	obj := &unstructured.Unstructured{Object: map[string]interface{}{}}
	rt := &v1.ResourceTemplate{}
	// With an empty object/template, GetResources fails and the change check is false.
	assert.Equal(t, isResourceChanged(w, obj, rt), false)
}
