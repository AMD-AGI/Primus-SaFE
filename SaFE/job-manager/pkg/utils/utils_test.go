/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package utils

import (
	"context"
	"errors"
	"testing"

	"github.com/agiledragon/gomonkey/v2"
	"gotest.tools/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	ctrlClient "sigs.k8s.io/controller-runtime/pkg/client"
	ctrlfake "sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	commonerrors "github.com/AMD-AIG-AIMA/SAFE/common/pkg/errors"
	commonclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/k8sclient"
	commonworkload "github.com/AMD-AIG-AIMA/SAFE/common/pkg/workload"
)

func TestListObjectsByWorkload(t *testing.T) {
	patches := gomonkey.NewPatches()
	defer patches.Reset()
	patches.ApplyFunc(commonworkload.IsTorchFT, func(*v1.Workload) bool { return false })
	patches.ApplyFunc(commonworkload.GetWorkloadGVK, func(*v1.Workload) []schema.GroupVersionKind {
		return []schema.GroupVersionKind{{Group: "batch", Version: "v1", Kind: "Job"}}
	})
	patches.ApplyFunc(commonworkload.GetResourceTemplateByGVK,
		func(context.Context, ctrlClient.Client, schema.GroupVersionKind) (*v1.ResourceTemplate, error) {
			return &v1.ResourceTemplate{}, nil
		})
	patches.ApplyFunc(ListObject,
		func(context.Context, *commonclient.ClientFactory, string, string, schema.GroupVersionKind) ([]unstructured.Unstructured, error) {
			return []unstructured.Unstructured{{}}, nil
		})

	w := &v1.Workload{}
	objs, err := ListObjectsByWorkload(context.Background(), nil, nil, w)
	assert.NilError(t, err)
	assert.Equal(t, len(objs), 1)
}

func TestDeleteObjectsByWorkload(t *testing.T) {
	patches := gomonkey.NewPatches()
	defer patches.Reset()
	patches.ApplyFunc(commonworkload.IsTorchFT, func(*v1.Workload) bool { return false })
	patches.ApplyFunc(commonworkload.GetWorkloadGVK, func(*v1.Workload) []schema.GroupVersionKind {
		return []schema.GroupVersionKind{{Group: "batch", Version: "v1", Kind: "Job"}}
	})
	patches.ApplyFunc(commonworkload.GetResourceTemplateByGVK,
		func(context.Context, ctrlClient.Client, schema.GroupVersionKind) (*v1.ResourceTemplate, error) {
			return &v1.ResourceTemplate{}, nil
		})
	patches.ApplyFunc(ListObject,
		func(context.Context, *commonclient.ClientFactory, string, string, schema.GroupVersionKind) ([]unstructured.Unstructured, error) {
			return []unstructured.Unstructured{{}}, nil
		})
	patches.ApplyFunc(DeleteObject,
		func(context.Context, *commonclient.ClientFactory, *unstructured.Unstructured) error { return nil })

	w := &v1.Workload{}
	found, err := DeleteObjectsByWorkload(context.Background(), nil, nil, w)
	assert.NilError(t, err)
	assert.Equal(t, found, true)
}

func TestDeleteObjectsByWorkloadEmpty(t *testing.T) {
	patches := gomonkey.NewPatches()
	defer patches.Reset()
	patches.ApplyFunc(commonworkload.IsTorchFT, func(*v1.Workload) bool { return false })
	patches.ApplyFunc(commonworkload.GetWorkloadGVK, func(*v1.Workload) []schema.GroupVersionKind {
		return []schema.GroupVersionKind{{Group: "batch", Version: "v1", Kind: "Job"}}
	})
	patches.ApplyFunc(commonworkload.GetResourceTemplateByGVK,
		func(context.Context, ctrlClient.Client, schema.GroupVersionKind) (*v1.ResourceTemplate, error) {
			return &v1.ResourceTemplate{}, nil
		})
	patches.ApplyFunc(ListObject,
		func(context.Context, *commonclient.ClientFactory, string, string, schema.GroupVersionKind) ([]unstructured.Unstructured, error) {
			return nil, nil
		})

	w := &v1.Workload{}
	found, err := DeleteObjectsByWorkload(context.Background(), nil, nil, w)
	assert.NilError(t, err)
	// No objects -> nothing deleted.
	assert.Equal(t, found, false)
}

// --- merged from utils_extra_test.go ---

func utilsScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	s := runtime.NewScheme()
	if err := v1.AddToScheme(s); err != nil {
		t.Fatal(err)
	}
	return s
}

func TestIsUnrecoverableError(t *testing.T) {
	assert.Equal(t, IsUnrecoverableError(nil), false)
	assert.Equal(t, IsUnrecoverableError(commonerrors.NewBadRequest("bad")), true)
	assert.Equal(t, IsUnrecoverableError(commonerrors.NewInternalError("oops")), true)
	assert.Equal(t, IsUnrecoverableError(commonerrors.NewNotFound("kind", "name")), true)
	assert.Equal(t, IsUnrecoverableError(errors.New("transient")), false)
}

func TestFindConditionAndNewCondition(t *testing.T) {
	cond := NewCondition("TypeA", "msg", "reason1")
	assert.Equal(t, cond.Type, "TypeA")
	assert.Equal(t, cond.Reason, "reason1")
	assert.Equal(t, string(cond.Status), string(metav1.ConditionTrue))

	w := &v1.Workload{}
	assert.Assert(t, FindCondition(w, cond) == nil)

	w.Status.Conditions = []metav1.Condition{*cond}
	assert.Assert(t, FindCondition(w, cond) != nil)

	other := NewCondition("TypeB", "m", "r")
	assert.Assert(t, FindCondition(w, other) == nil)
}

func TestSetWorkloadFailed(t *testing.T) {
	w := &v1.Workload{ObjectMeta: metav1.ObjectMeta{Name: "w"}}
	cl := ctrlfake.NewClientBuilder().
		WithScheme(utilsScheme(t)).
		WithObjects(w).
		WithStatusSubresource(&v1.Workload{}).
		Build()
	err := SetWorkloadFailed(context.Background(), cl, w, "boom")
	assert.NilError(t, err)
	assert.Equal(t, w.Status.Phase, v1.WorkloadFailed)
	assert.Assert(t, w.Status.EndTime != nil)
	assert.Assert(t, len(w.Status.Conditions) > 0)
}

// TestSetWorkloadFailedLeavesOffloadedDetailInDB covers a caller whose copy was
// hydrated from the DB. The transition owns phase, endTime and conditions; it
// must not carry the per-pod arrays back into etcd, whose object size limit is
// the reason the detail was offloaded in the first place.
func TestSetWorkloadFailedLeavesOffloadedDetailInDB(t *testing.T) {
	stored := &v1.Workload{ObjectMeta: metav1.ObjectMeta{Name: "w"}}
	stored.Status.NodeUsage = []v1.NodePodUsage{{Node: "n1"}}
	cl := ctrlfake.NewClientBuilder().
		WithScheme(utilsScheme(t)).
		WithObjects(stored).
		WithStatusSubresource(&v1.Workload{}).
		Build()

	hydrated := &v1.Workload{}
	assert.NilError(t, cl.Get(context.Background(), ctrlClient.ObjectKey{Name: "w"}, hydrated))
	hydrated.Status.Pods = []v1.WorkloadPod{{PodId: "p1", AdminNodeName: "n1"}}
	hydrated.Status.Nodes = [][]string{{"n1"}}
	hydrated.Status.Ranks = [][]string{{"0"}}

	assert.NilError(t, SetWorkloadFailed(context.Background(), cl, hydrated, "boom"))

	fresh := &v1.Workload{}
	assert.NilError(t, cl.Get(context.Background(), ctrlClient.ObjectKey{Name: "w"}, fresh))
	assert.Equal(t, fresh.Status.Phase, v1.WorkloadFailed)
	assert.Assert(t, fresh.Status.EndTime != nil)
	assert.Assert(t, len(fresh.Status.Conditions) > 0)
	assert.Equal(t, len(fresh.Status.Pods), 0)
	assert.Equal(t, len(fresh.Status.Nodes), 0)
	assert.Equal(t, len(fresh.Status.Ranks), 0)
	assert.Equal(t, len(fresh.Status.NodeUsage), 1)
	assert.Equal(t, fresh.Status.NodeUsage[0].Node, "n1")
}

func TestMarkWorkloadStopped(t *testing.T) {
	w := &v1.Workload{ObjectMeta: metav1.ObjectMeta{Name: "w"}}
	cl := ctrlfake.NewClientBuilder().
		WithScheme(utilsScheme(t)).
		WithObjects(w).
		WithStatusSubresource(&v1.Workload{}).
		Build()
	err := MarkWorkloadStopped(context.Background(), cl, w, StopReasonTimeout, "timed out")
	assert.NilError(t, err)
	assert.Equal(t, w.Status.Phase, v1.WorkloadStopped)
}

func TestMarkWorkloadStoppedAlreadyStopped(t *testing.T) {
	w := &v1.Workload{ObjectMeta: metav1.ObjectMeta{Name: "w"}}
	w.Status.Phase = v1.WorkloadStopped
	// No client interaction expected since it is already stopped.
	err := MarkWorkloadStopped(context.Background(), nil, w, StopReasonManual, "noop")
	assert.NilError(t, err)
}

func TestSetWorkloadTimeout(t *testing.T) {
	w := &v1.Workload{ObjectMeta: metav1.ObjectMeta{Name: "w"}}
	cl := ctrlfake.NewClientBuilder().
		WithScheme(utilsScheme(t)).
		WithObjects(w).
		WithStatusSubresource(&v1.Workload{}).
		Build()
	err := SetWorkloadTimeout(context.Background(), cl, w, "timeout")
	assert.NilError(t, err)
	assert.Equal(t, w.Status.Phase, v1.WorkloadStopped)
}

func TestFindFailedCondition(t *testing.T) {
	w := &v1.Workload{}
	assert.Equal(t, FindFailedCondition(w), false)
}

func TestIsWorkloadOrPod(t *testing.T) {
	for _, kind := range []string{"Pod", "Deployment", "StatefulSet", "Job", "MonarchMesh"} {
		assert.Equal(t, isWorkloadOrPod(schema.GroupVersionKind{Kind: kind}), true)
	}
	assert.Equal(t, isWorkloadOrPod(schema.GroupVersionKind{Kind: "ConfigMap"}), false)
}

func TestK8sObjectStatusIsPending(t *testing.T) {
	assert.Equal(t, (&K8sObjectStatus{Phase: ""}).IsPending(), true)
	assert.Equal(t, (&K8sObjectStatus{Phase: string(v1.K8sPending)}).IsPending(), true)
	assert.Equal(t, (&K8sObjectStatus{Phase: "Running"}).IsPending(), false)
}

func TestNestedBool(t *testing.T) {
	obj := map[string]interface{}{
		"spec": map[string]interface{}{
			"enabled": true,
			"wrong":   "notbool",
		},
	}
	v, found, err := NestedBool(obj, []string{"spec", "enabled"})
	assert.NilError(t, err)
	assert.Equal(t, found, true)
	assert.Equal(t, v, true)

	// Missing path -> not found.
	_, found, err = NestedBool(obj, []string{"spec", "missing"})
	assert.NilError(t, err)
	assert.Equal(t, found, false)

	// Wrong type -> error.
	_, _, err = NestedBool(obj, []string{"spec", "wrong"})
	assert.Assert(t, err != nil)
}

func TestGetLabels(t *testing.T) {
	u := &unstructured.Unstructured{Object: map[string]interface{}{
		"metadata": map[string]interface{}{
			"labels": map[string]interface{}{"app": "x"},
		},
	}}
	labels, err := GetLabels(u, v1.ResourceSpec{})
	assert.NilError(t, err)
	assert.Equal(t, labels["app"], "x")

	// No labels present -> nil, no error.
	u2 := &unstructured.Unstructured{Object: map[string]interface{}{"metadata": map[string]interface{}{}}}
	labels2, err := GetLabels(u2, v1.ResourceSpec{})
	assert.NilError(t, err)
	assert.Assert(t, labels2 == nil)
}
