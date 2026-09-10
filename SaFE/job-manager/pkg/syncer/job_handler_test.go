/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package syncer

import (
	"context"
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/agiledragon/gomonkey/v2"
	"gotest.tools/assert"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	ctrlruntime "sigs.k8s.io/controller-runtime"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	ctrlfake "sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	commonclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/k8sclient"
	commonworkload "github.com/AMD-AIG-AIMA/SAFE/common/pkg/workload"
	jobutils "github.com/AMD-AIG-AIMA/SAFE/job-manager/pkg/utils"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/timeutil"
)

// syncerScheme builds a scheme with the amd/v1 types registered for fake clients.
func syncerScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	s := runtime.NewScheme()
	if err := v1.AddToScheme(s); err != nil {
		t.Fatal(err)
	}
	return s
}

// monkeyClientSets returns a client set backed by a nil data-plane client, used
// by tests that patch the helpers touching the data plane.
func monkeyClientSets() *ClusterClientSets {
	return &ClusterClientSets{
		dataClientFactory: commonclient.NewClientFactoryWithOnlyClient(context.Background(), "c", nil),
	}
}

func TestHandleJobWorkloadNotFound(t *testing.T) {
	cl := ctrlfake.NewClientBuilder().WithScheme(syncerScheme(t)).Build()
	r := &SyncerReconciler{Client: cl}
	_, err := r.handleJob(context.Background(), &resourceMessage{workloadId: "missing"}, nil)
	assert.NilError(t, err)
}

func TestHandleJobNamespaceMismatch(t *testing.T) {
	w := &v1.Workload{ObjectMeta: metav1.ObjectMeta{Name: "w"}}
	w.Spec.Workspace = "ws"
	cl := ctrlfake.NewClientBuilder().WithScheme(syncerScheme(t)).WithObjects(w).Build()
	r := &SyncerReconciler{Client: cl}
	res, err := r.handleJob(context.Background(), &resourceMessage{workloadId: "w", namespace: "other"}, nil)
	assert.NilError(t, err)
	assert.Equal(t, res.RequeueAfter.Nanoseconds(), int64(0))
}

func TestHandleJobNotDispatched(t *testing.T) {
	w := &v1.Workload{ObjectMeta: metav1.ObjectMeta{Name: "w"}}
	w.Spec.Workspace = "ws"
	cl := ctrlfake.NewClientBuilder().WithScheme(syncerScheme(t)).WithObjects(w).Build()
	r := &SyncerReconciler{Client: cl}
	res, err := r.handleJob(context.Background(),
		&resourceMessage{workloadId: "w", namespace: "ws", gvk: schema.GroupVersionKind{Kind: "Job"}}, nil)
	assert.NilError(t, err)
	// Not dispatched -> requeue after a second.
	assert.Assert(t, res.RequeueAfter > 0)
}

func TestGetK8sObjectStatusDeleted(t *testing.T) {
	r := &SyncerReconciler{}
	msg := &resourceMessage{
		action: ResourceDel,
		name:   "obj",
		gvk:    schema.GroupVersionKind{Kind: "Job"},
	}
	status, err := r.getK8sObjectStatus(context.Background(), msg, nil, &v1.Workload{})
	assert.NilError(t, err)
	assert.Assert(t, status != nil)
	assert.Equal(t, status.Phase, string(v1.K8sDeleted))
}

// TestGetK8sObjectStatusNotFoundTreatedAsDeleted verifies that when the managed
// data-plane object is gone (e.g. failover deleted it for a restart), a NotFound is
// reported as K8sDeleted and the action is rewritten to ResourceDel, so the workload
// reschedules instead of being marked Failed and reaped by the TTL controller.
func TestGetK8sObjectStatusNotFoundTreatedAsDeleted(t *testing.T) {
	patches := gomonkey.ApplyFunc(jobutils.GetObject,
		func(context.Context, *commonclient.ClientFactory, string, string, schema.GroupVersionKind) (*unstructured.Unstructured, error) {
			return nil, apierrors.NewNotFound(schema.GroupResource{Resource: "autoscalingrunnersets"}, "w")
		})
	defer patches.Reset()

	r := &SyncerReconciler{}
	msg := &resourceMessage{
		name:      "w",
		namespace: "ns",
		action:    ResourceUpdate,
		gvk:       schema.GroupVersionKind{Kind: "AutoscalingRunnerSet"},
	}
	status, err := r.getK8sObjectStatus(context.Background(), msg, monkeyClientSets(), &v1.Workload{})
	assert.NilError(t, err)
	assert.Assert(t, status != nil)
	assert.Equal(t, status.Phase, string(v1.K8sDeleted))
	assert.Equal(t, msg.action, ResourceDel)
}

// TestGetK8sObjectStatusOtherErrorPropagated verifies non-NotFound errors are still
// surfaced as errors and the action is left untouched.
func TestGetK8sObjectStatusOtherErrorPropagated(t *testing.T) {
	patches := gomonkey.ApplyFunc(jobutils.GetObject,
		func(context.Context, *commonclient.ClientFactory, string, string, schema.GroupVersionKind) (*unstructured.Unstructured, error) {
			return nil, errors.New("boom")
		})
	defer patches.Reset()

	r := &SyncerReconciler{}
	msg := &resourceMessage{
		name:      "w",
		namespace: "ns",
		action:    ResourceUpdate,
		gvk:       schema.GroupVersionKind{Kind: "AutoscalingRunnerSet"},
	}
	status, err := r.getK8sObjectStatus(context.Background(), msg, monkeyClientSets(), &v1.Workload{})
	assert.Assert(t, err != nil)
	assert.Assert(t, status == nil)
	assert.Equal(t, msg.action, ResourceUpdate)
}

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

// TestWaitJobDeletedEmpty patches getObjectsByWorkload (via GetObject) to return no
// objects, exercising the "all deleted" path.
func TestWaitJobDeletedEmpty(t *testing.T) {
	patches := gomonkey.ApplyFunc(jobutils.GetObject,
		func(_ context.Context, _ *commonclient.ClientFactory, name, ns string, gvk schema.GroupVersionKind) (*unstructured.Unstructured, error) {
			// IgnoreNotFound path: getObjectsByWorkload treats not-found as empty.
			return &unstructured.Unstructured{}, nil
		})
	defer patches.Reset()
	// Patch IsTorchFT to false so the single-object branch is taken.
	p2 := gomonkey.ApplyFunc(commonworkload.IsTorchFT, func(*v1.Workload) bool { return false })
	defer p2.Reset()

	r := &SyncerReconciler{}
	w := &v1.Workload{}
	// One object returned without deletion timestamp -> not yet deleted.
	ok, err := r.waitJobDeleted(context.Background(), w,
		&resourceMessage{name: "obj", namespace: "ns", gvk: schema.GroupVersionKind{Kind: "Job"}}, monkeyClientSets())
	assert.NilError(t, err)
	assert.Equal(t, ok, false)
}

// TestGetObjectsByWorkloadNonTorchFT patches jobutils.GetObject to avoid the real
// dynamic client and verifies the single-object path.
func TestGetObjectsByWorkloadNonTorchFT(t *testing.T) {
	patches := gomonkey.ApplyFunc(jobutils.GetObject,
		func(_ context.Context, _ *commonclient.ClientFactory, name, ns string, gvk schema.GroupVersionKind) (*unstructured.Unstructured, error) {
			obj := &unstructured.Unstructured{}
			obj.SetName(name)
			return obj, nil
		})
	defer patches.Reset()

	r := &SyncerReconciler{}
	w := &v1.Workload{}
	objs, err := r.getObjectsByWorkload(context.Background(), w,
		&resourceMessage{name: "obj", namespace: "ns", gvk: schema.GroupVersionKind{Kind: "Job"}}, monkeyClientSets())
	assert.NilError(t, err)
	assert.Equal(t, len(objs), 1)
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

func TestUpdateAdminWorkloadByJobDeleted(t *testing.T) {
	w := &v1.Workload{ObjectMeta: metav1.ObjectMeta{Name: "w"}}
	cl := ctrlfake.NewClientBuilder().
		WithScheme(syncerScheme(t)).
		WithObjects(w).
		WithStatusSubresource(&v1.Workload{}).
		Build()
	r := &SyncerReconciler{Client: cl}

	// ResourceDel action -> getK8sObjectStatus returns a K8sDeleted status without
	// needing the data-plane client factory, so updateAdminWorkloadByJob can run.
	msg := &resourceMessage{
		action:        ResourceDel,
		name:          "obj",
		dispatchCount: 1,
		gvk:           schema.GroupVersionKind{Kind: "Job"},
	}
	out, err := r.updateAdminWorkloadByJob(context.Background(), nil, w, msg)
	assert.NilError(t, err)
	assert.Assert(t, out != nil)
}

// TestUpdateAdminWorkloadByJobRunning patches the dynamic + template helpers so both
// getK8sObjectStatus and updateAdminWorkloadByJob run their full non-CICD update path
// against a fake admin client.
func TestUpdateAdminWorkloadByJobRunning(t *testing.T) {
	patches := gomonkey.NewPatches()
	defer patches.Reset()
	patches.ApplyFunc(jobutils.GetObject,
		func(context.Context, *commonclient.ClientFactory, string, string, schema.GroupVersionKind) (*unstructured.Unstructured, error) {
			return &unstructured.Unstructured{}, nil
		})
	patches.ApplyFunc(commonworkload.IsTorchFT, func(*v1.Workload) bool { return false })
	patches.ApplyFunc(commonworkload.IsMonarchJob, func(*v1.Workload) bool { return false })
	patches.ApplyFunc(commonworkload.IsCICDScalingRunnerSet, func(*v1.Workload) bool { return false })
	patches.ApplyFunc(commonworkload.GetResourceTemplate,
		func(context.Context, ctrlclient.Client, *v1.Workload) (*v1.ResourceTemplate, error) {
			return &v1.ResourceTemplate{}, nil
		})
	patches.ApplyFunc(jobutils.GetK8sObjectStatus,
		func(*unstructured.Unstructured, *v1.ResourceTemplate) (*jobutils.K8sObjectStatus, error) {
			return &jobutils.K8sObjectStatus{Phase: string(v1.K8sRunning)}, nil
		})

	w := &v1.Workload{ObjectMeta: metav1.ObjectMeta{Name: "w"}}
	cl := ctrlfake.NewClientBuilder().
		WithScheme(syncerScheme(t)).
		WithObjects(w).
		WithStatusSubresource(&v1.Workload{}).
		Build()
	r := &SyncerReconciler{Client: cl}

	out, err := r.updateAdminWorkloadByJob(context.Background(), monkeyClientSets(), w,
		&resourceMessage{name: "o", namespace: "ns", dispatchCount: 1, gvk: schema.GroupVersionKind{Kind: "Job"}})
	assert.NilError(t, err)
	assert.Assert(t, out != nil)
}

// TestUpdateAdminWorkloadByJobKeepsOffloadedDetailOutOfEtcd covers an offloaded
// workload whose pod detail reached this copy through hydration. The phase
// transition owns neither the per-pod arrays nor the aggregate: writing the
// whole status back would restore the arrays to etcd against the object size
// limit, and would republish an aggregate this path never recomputed.
func TestUpdateAdminWorkloadByJobKeepsOffloadedDetailOutOfEtcd(t *testing.T) {
	patches := gomonkey.NewPatches()
	defer patches.Reset()
	patches.ApplyFunc(jobutils.GetObject,
		func(context.Context, *commonclient.ClientFactory, string, string, schema.GroupVersionKind) (*unstructured.Unstructured, error) {
			return &unstructured.Unstructured{}, nil
		})
	patches.ApplyFunc(commonworkload.IsTorchFT, func(*v1.Workload) bool { return false })
	patches.ApplyFunc(commonworkload.IsMonarchJob, func(*v1.Workload) bool { return false })
	patches.ApplyFunc(commonworkload.IsCICDScalingRunnerSet, func(*v1.Workload) bool { return false })
	patches.ApplyFunc(commonworkload.GetResourceTemplate,
		func(context.Context, ctrlclient.Client, *v1.Workload) (*v1.ResourceTemplate, error) {
			return &v1.ResourceTemplate{}, nil
		})
	patches.ApplyFunc(jobutils.GetK8sObjectStatus,
		func(*unstructured.Unstructured, *v1.ResourceTemplate) (*jobutils.K8sObjectStatus, error) {
			return &jobutils.K8sObjectStatus{Phase: string(v1.K8sRunning)}, nil
		})

	// etcd holds the aggregate alone, the shape an offloaded workload is stored in.
	stored := &v1.Workload{ObjectMeta: metav1.ObjectMeta{
		Name:        "w",
		Annotations: map[string]string{v1.WorkloadStatusOffloadAnnotation: v1.TrueStr},
	}}
	stored.Status.NodeUsage = []v1.NodePodUsage{{
		Node:    "n1",
		Active:  map[string]int{"0": 1},
		Running: map[string]int{"0": 1},
	}}
	cl := ctrlfake.NewClientBuilder().
		WithScheme(syncerScheme(t)).
		WithObjects(stored).
		WithStatusSubresource(&v1.Workload{}).
		Build()
	r := &SyncerReconciler{Client: cl}

	hydrated := &v1.Workload{}
	assert.NilError(t, cl.Get(context.Background(), ctrlclient.ObjectKey{Name: "w"}, hydrated))
	hydrated.Status.Pods = []v1.WorkloadPod{{PodId: "p1", AdminNodeName: "n1", Phase: corev1.PodRunning}}
	hydrated.Status.Nodes = [][]string{{"n1"}}
	hydrated.Status.Ranks = [][]string{{"0"}}

	out, err := r.updateAdminWorkloadByJob(context.Background(), monkeyClientSets(), hydrated,
		&resourceMessage{name: "o", namespace: "ns", dispatchCount: 1, gvk: schema.GroupVersionKind{Kind: "Job"}})
	assert.NilError(t, err)
	assert.Assert(t, out != nil)

	fresh := &v1.Workload{}
	assert.NilError(t, cl.Get(context.Background(), ctrlclient.ObjectKey{Name: "w"}, fresh))
	assert.Equal(t, string(fresh.Status.Phase), string(v1.WorkloadRunning))
	assert.Equal(t, len(fresh.Status.Pods), 0)
	assert.Equal(t, len(fresh.Status.Nodes), 0)
	assert.Equal(t, len(fresh.Status.Ranks), 0)
	assert.Equal(t, len(fresh.Status.NodeUsage), 1)
	assert.Equal(t, fresh.Status.NodeUsage[0].Node, "n1")
}

// A TorchFT workload tracks a phase per group. That map is set only once the
// groups report, so the status patch has to carry it when it exists.
func TestUpdateAdminWorkloadByJobCarriesTorchFTPhase(t *testing.T) {
	patches := gomonkey.NewPatches()
	defer patches.Reset()
	patches.ApplyFunc(jobutils.GetObject,
		func(context.Context, *commonclient.ClientFactory, string, string, schema.GroupVersionKind) (*unstructured.Unstructured, error) {
			return &unstructured.Unstructured{}, nil
		})
	patches.ApplyFunc(commonworkload.IsTorchFT, func(*v1.Workload) bool { return false })
	patches.ApplyFunc(commonworkload.IsMonarchJob, func(*v1.Workload) bool { return false })
	patches.ApplyFunc(commonworkload.IsCICDScalingRunnerSet, func(*v1.Workload) bool { return false })
	patches.ApplyFunc(commonworkload.GetResourceTemplate,
		func(context.Context, ctrlclient.Client, *v1.Workload) (*v1.ResourceTemplate, error) {
			return &v1.ResourceTemplate{}, nil
		})
	patches.ApplyFunc(jobutils.GetK8sObjectStatus,
		func(*unstructured.Unstructured, *v1.ResourceTemplate) (*jobutils.K8sObjectStatus, error) {
			return &jobutils.K8sObjectStatus{Phase: string(v1.K8sRunning)}, nil
		})

	stored := &v1.Workload{ObjectMeta: metav1.ObjectMeta{Name: "w"}}
	cl := ctrlfake.NewClientBuilder().
		WithScheme(syncerScheme(t)).
		WithObjects(stored).
		WithStatusSubresource(&v1.Workload{}).
		Build()

	w := &v1.Workload{}
	assert.NilError(t, cl.Get(context.Background(), ctrlclient.ObjectKey{Name: "w"}, w))
	w.Status.TorchFTPhase = map[string]v1.WorkloadPhase{"1": v1.WorkloadRunning}

	r := &SyncerReconciler{Client: cl}
	out, err := r.updateAdminWorkloadByJob(context.Background(), monkeyClientSets(), w,
		&resourceMessage{name: "o", namespace: "ns", dispatchCount: 1, gvk: schema.GroupVersionKind{Kind: "Job"}})
	assert.NilError(t, err)
	assert.Assert(t, out != nil)

	fresh := &v1.Workload{}
	assert.NilError(t, cl.Get(context.Background(), ctrlclient.ObjectKey{Name: "w"}, fresh))
	assert.Equal(t, string(fresh.Status.TorchFTPhase["1"]), string(v1.WorkloadRunning))
}

// The workload can be deleted between the two reads that bracket the pod status
// lock, and the second read is the one that decides there is nothing to sync.
func TestHandleJobStopsWhenWorkloadVanishesUnderLock(t *testing.T) {
	w := &v1.Workload{ObjectMeta: metav1.ObjectMeta{Name: "w"}}
	cl := ctrlfake.NewClientBuilder().WithScheme(syncerScheme(t)).WithObjects(w).Build()

	patches := gomonkey.NewPatches()
	defer patches.Reset()
	reads := 0
	patches.ApplyPrivateMethod(reflect.TypeOf(&SyncerReconciler{}), "getAdminWorkload",
		func(_ *SyncerReconciler, _ context.Context, _ string) (*v1.Workload, error) {
			reads++
			if reads == 1 {
				return w, nil
			}
			return nil, nil
		})

	r := &SyncerReconciler{Client: cl}
	res, err := r.handleJob(context.Background(), &resourceMessage{workloadId: "w"}, nil)
	assert.NilError(t, err)
	assert.Equal(t, res.RequeueAfter.Nanoseconds(), int64(0))
	assert.Equal(t, reads, 2)
}

func TestUpdateAdminWorkloadPhase(t *testing.T) {
	r := &SyncerReconciler{}
	msg := &resourceMessage{dispatchCount: 1}

	cases := []struct {
		k8sPhase string
		want     v1.WorkloadPhase
		maxRetry int
		count    int
	}{
		{string(v1.K8sPending), v1.WorkloadPending, 3, 1},
		{string(v1.K8sSucceeded), v1.WorkloadSucceeded, 3, 1},
		{string(v1.K8sNotReady), v1.WorkloadNotReady, 3, 1},
		{string(v1.K8sRunning), v1.WorkloadRunning, 3, 1},
		{string(v1.K8sUpdating), v1.WorkloadUpdating, 3, 1},
		{string(v1.AdminStopped), v1.WorkloadStopped, 3, 1},
		// Failed beyond retry budget -> Failed.
		{string(v1.K8sFailed), v1.WorkloadFailed, 1, 99},
	}
	for _, c := range cases {
		w := &v1.Workload{}
		w.Spec.MaxRetry = c.maxRetry
		m := &resourceMessage{dispatchCount: c.count}
		r.updateAdminWorkloadPhase(w, &jobutils.K8sObjectStatus{Phase: c.k8sPhase}, m)
		assert.Equal(t, string(w.Status.Phase), string(c.want))
	}
	_ = msg
}

func TestUpdateWorkloadCondition(t *testing.T) {
	w := &v1.Workload{}
	cond := jobutils.NewCondition("TypeA", "msg1", "reason1")

	// First insert appends.
	updateWorkloadCondition(w, cond)
	assert.Equal(t, len(w.Status.Conditions), 1)

	// Same type+reason but different message updates in place.
	cond2 := jobutils.NewCondition("TypeA", "msg2", "reason1")
	updateWorkloadCondition(w, cond2)
	assert.Equal(t, len(w.Status.Conditions), 1)
	assert.Equal(t, w.Status.Conditions[0].Message, "msg2")

	// Different reason appends a new condition.
	cond3 := jobutils.NewCondition("TypeA", "msg", "reason2")
	updateWorkloadCondition(w, cond3)
	assert.Equal(t, len(w.Status.Conditions), 2)
}

func TestReSchedule(t *testing.T) {
	w := &v1.Workload{ObjectMeta: metav1.ObjectMeta{
		Name:        "w",
		Annotations: map[string]string{v1.WorkloadDispatchedAnnotation: "true", v1.WorkloadScheduledAnnotation: "true"},
	}}
	w.Status.Phase = v1.WorkloadRunning
	w.Status.Pods = []v1.WorkloadPod{{PodId: "p"}}
	cl := ctrlfake.NewClientBuilder().
		WithScheme(syncerScheme(t)).
		WithObjects(w).
		WithStatusSubresource(&v1.Workload{}).
		Build()
	r := &SyncerReconciler{Client: cl}

	err := r.reSchedule(context.Background(), w, 1)
	assert.NilError(t, err)
	assert.Equal(t, string(w.Status.Phase), string(v1.WorkloadPending))
	// Dispatched annotation removed during reschedule.
	assert.Equal(t, v1.IsWorkloadDispatched(w), false)
}

// unregisteredRunnerSet builds a dispatched CICD scaling runner set that ARC has
// not registered with GitHub, dispatched `since` ago.
func unregisteredRunnerSet(since time.Duration) *v1.Workload {
	w := &v1.Workload{ObjectMeta: metav1.ObjectMeta{Name: "rs"}}
	w.Spec.GroupVersionKind = v1.GroupVersionKind{Version: "v1", Kind: common.CICDScaleRunnerSetKind}
	v1.SetAnnotation(w, v1.WorkloadDispatchedAnnotation,
		timeutil.FormatRFC3339(time.Now().UTC().Add(-since)))
	return w
}

// The pod reconcile runs on the way out of an event, and it is bookkeeping: its
// failure must not fail the event, and the result the event computed must survive.
func TestHandleJobToleratesVanishedPodReconcileError(t *testing.T) {
	w := &v1.Workload{ObjectMeta: metav1.ObjectMeta{Name: "w"}}
	w.Spec.Workspace = "ws"
	v1.SetAnnotation(w, v1.WorkloadDispatchedAnnotation, timeutil.FormatRFC3339(time.Now().UTC()))
	cl := ctrlfake.NewClientBuilder().WithScheme(syncerScheme(t)).WithObjects(w).Build()
	r := &SyncerReconciler{Client: cl}

	patches := gomonkey.NewPatches()
	defer patches.Reset()
	patches.ApplyPrivateMethod(reflect.TypeOf(&SyncerReconciler{}), "handleJobImpl",
		func(_ *SyncerReconciler, _ context.Context, _ *resourceMessage, _ *v1.Workload,
			_ *ClusterClientSets) (ctrlruntime.Result, error) {
			return ctrlruntime.Result{RequeueAfter: time.Second}, nil
		})
	reconciled := false
	patches.ApplyPrivateMethod(reflect.TypeOf(&SyncerReconciler{}), "reconcileVanishedPods",
		func(_ *SyncerReconciler, _ context.Context, _ *ClusterClientSets, _ *v1.Workload,
			_ *resourceMessage) error {
			reconciled = true
			return errors.New("conflict")
		})

	result, err := r.handleJob(context.Background(),
		&resourceMessage{workloadId: "w", namespace: "ws"}, nil)

	assert.NilError(t, err)
	assert.Equal(t, reconciled, true)
	assert.Equal(t, result.RequeueAfter, time.Second, "the event's own result is preserved")
}

// A registration failure is invisible on the object, so a wait that has gone on too
// long is the only signal to act on.
func TestCheckRunnerSetRegistrationFailsPastDeadline(t *testing.T) {
	w := unregisteredRunnerSet(runnerSetRegistrationTimeout + time.Minute)

	result, err := checkRunnerSetRegistration(w, &resourceMessage{action: ResourceAdd})

	assert.Assert(t, err != nil)
	// Unrecoverable is what makes handleJob mark it Failed without a retry, which
	// is what releases the queue.
	assert.Equal(t, jobutils.IsUnrecoverableError(err), true)
	assert.Equal(t, result.RequeueAfter, time.Duration(0))
}

func TestCheckRunnerSetRegistrationWaitsBeforeDeadline(t *testing.T) {
	w := unregisteredRunnerSet(time.Minute)

	result, err := checkRunnerSetRegistration(w, &resourceMessage{action: ResourceAdd})

	assert.NilError(t, err)
	// ARC does not touch the object while it retries, so the deadline has to be
	// reached by a re-queue rather than by the next event.
	assert.Assert(t, result.RequeueAfter > 0)
	assert.Assert(t, result.RequeueAfter <= runnerSetRegistrationTimeout-time.Minute)
}

func TestCheckRunnerSetRegistrationLeavesOtherStatesAlone(t *testing.T) {
	// Registered: the healthy path must not be touched however long it has run.
	registered := unregisteredRunnerSet(runnerSetRegistrationTimeout + time.Minute)
	registered.Status.RunnerScaleSetId = "95"
	result, err := checkRunnerSetRegistration(registered, &resourceMessage{action: ResourceAdd})
	assert.NilError(t, err)
	assert.Equal(t, result.RequeueAfter, time.Duration(0))

	// Teardown reaches this code with an empty id too, and must not be reported as
	// a registration failure.
	for _, action := range []string{ResourceDel, ResourceDeleting} {
		deleting := unregisteredRunnerSet(runnerSetRegistrationTimeout + time.Minute)
		result, err = checkRunnerSetRegistration(deleting, &resourceMessage{action: action})
		assert.NilError(t, err)
		assert.Equal(t, result.RequeueAfter, time.Duration(0))
	}

	// No dispatch annotation: no evidence of how long the wait has been, so nothing
	// may be concluded from it.
	undispatched := unregisteredRunnerSet(runnerSetRegistrationTimeout + time.Minute)
	v1.RemoveAnnotation(undispatched, v1.WorkloadDispatchedAnnotation)
	result, err = checkRunnerSetRegistration(undispatched, &resourceMessage{action: ResourceAdd})
	assert.NilError(t, err)
	assert.Equal(t, result.RequeueAfter, time.Duration(0))

	// A workload of any other kind is out of scope.
	pytorch := unregisteredRunnerSet(runnerSetRegistrationTimeout + time.Minute)
	pytorch.Spec.GroupVersionKind.Kind = common.PytorchJobKind
	result, err = checkRunnerSetRegistration(pytorch, &resourceMessage{action: ResourceAdd})
	assert.NilError(t, err)
	assert.Equal(t, result.RequeueAfter, time.Duration(0))
}

func TestShouldWorkloadStopRetry(t *testing.T) {
	// Disable failover -> stop retry.
	w := &v1.Workload{}
	w.Annotations = map[string]string{v1.WorkloadDisableFailoverAnnotation: v1.TrueStr}
	w.Spec.MaxRetry = 3
	assert.Equal(t, shouldWorkloadStopRetry(w, 1), true)

	// MaxRetry <= 0 -> stop retry.
	w2 := &v1.Workload{}
	assert.Equal(t, shouldWorkloadStopRetry(w2, 1), true)

	// count > MaxRetry -> stop retry.
	w3 := &v1.Workload{}
	w3.Spec.MaxRetry = 2
	assert.Equal(t, shouldWorkloadStopRetry(w3, 3), true)

	// Within retry budget -> keep retrying.
	w4 := &v1.Workload{}
	w4.Spec.MaxRetry = 3
	assert.Equal(t, shouldWorkloadStopRetry(w4, 1), false)
}

func TestShouldTerminateWorkload(t *testing.T) {
	w := &v1.Workload{}
	w.Spec.MaxRetry = 3

	// Succeeded -> terminate.
	assert.Equal(t, shouldTerminateWorkload(w, &jobutils.K8sObjectStatus{Phase: string(v1.K8sSucceeded)}, 1), true)

	// Failed within retry budget -> do not terminate.
	assert.Equal(t, shouldTerminateWorkload(w, &jobutils.K8sObjectStatus{Phase: string(v1.K8sFailed)}, 1), false)

	// Failed beyond retry budget -> terminate.
	assert.Equal(t, shouldTerminateWorkload(w, &jobutils.K8sObjectStatus{Phase: string(v1.K8sFailed)}, 99), true)

	// Preempted workload is never terminated here.
	wp := &v1.Workload{}
	wp.Annotations = map[string]string{v1.WorkloadPreemptedAnnotation: "true"}
	assert.Equal(t, shouldTerminateWorkload(wp, &jobutils.K8sObjectStatus{Phase: string(v1.K8sSucceeded)}, 1), false)
}

func TestIsTorchFTGroupFailed(t *testing.T) {
	w := &v1.Workload{}
	w.Spec.Env = map[string]string{
		common.ReplicaCount:    "4",
		common.MinReplicaCount: "2",
	}
	// 3 of 4 failed -> remaining 1 < min 2 -> failed.
	w.Status.TorchFTPhase = map[string]v1.WorkloadPhase{
		"1": v1.WorkloadFailed, "2": v1.WorkloadFailed, "3": v1.WorkloadFailed,
	}
	assert.Equal(t, isTorchFTGroupFailed(w), true)

	// Only 1 failed -> remaining 3 >= min 2 -> not failed.
	w.Status.TorchFTPhase = map[string]v1.WorkloadPhase{"1": v1.WorkloadFailed}
	assert.Equal(t, isTorchFTGroupFailed(w), false)
}

func TestHandleTorchFTGroupStatusSingleGroup(t *testing.T) {
	// No replica count env -> treated as single group, returns the phase as-is.
	w := &v1.Workload{}
	got := handleTorchFTGroupStatus(w, "1", v1.WorkloadRunning)
	assert.Equal(t, string(got), string(v1.WorkloadRunning))
}

func TestHandleTorchFTGroupStatusFailed(t *testing.T) {
	w := &v1.Workload{}
	w.Spec.Env = map[string]string{
		common.ReplicaCount:    "2",
		common.MinReplicaCount: "2",
	}
	// First group fails; remaining (1) < min (2) -> overall failed.
	got := handleTorchFTGroupStatus(w, "1", v1.WorkloadFailed)
	assert.Equal(t, string(got), string(v1.WorkloadFailed))
}

func TestHandleTorchFTGroupStatusInvalidGroupId(t *testing.T) {
	w := &v1.Workload{}
	w.Spec.Env = map[string]string{
		common.ReplicaCount:    "2",
		common.MinReplicaCount: "1",
	}
	// Group id beyond total groups -> empty phase.
	got := handleTorchFTGroupStatus(w, "5", v1.WorkloadRunning)
	assert.Equal(t, string(got), "")
}

func TestGetFailedPodInfo(t *testing.T) {
	// No failed pods -> empty string.
	w := &v1.Workload{}
	w.Status.Pods = []v1.WorkloadPod{{Phase: corev1.PodRunning}}
	assert.Equal(t, getFailedPodInfo(w), "")

	// Failed pod -> JSON with details.
	w.Status.Pods = []v1.WorkloadPod{{
		Phase:         corev1.PodFailed,
		PodId:         "p1",
		AdminNodeName: "node-1",
		FailedMessage: "oom",
		Containers:    []v1.Container{{ExitCode: 137}},
	}}
	out := getFailedPodInfo(w)
	assert.Assert(t, out != "")
}

// TestHandleJobFullPath patches the two heavy helpers so handleJob + handleJobImpl run
// their orchestration for a dispatched, namespace-matched workload.
func TestHandleJobFullPath(t *testing.T) {
	patches := gomonkey.NewPatches()
	defer patches.Reset()
	patches.ApplyPrivateMethod(reflect.TypeOf(&SyncerReconciler{}), "updateAdminWorkloadByJob",
		func(_ *SyncerReconciler, _ context.Context, _ *ClusterClientSets, w *v1.Workload, _ *resourceMessage) (*v1.Workload, error) {
			return w, nil
		})
	patches.ApplyPrivateMethod(reflect.TypeOf(&SyncerReconciler{}), "shouldReSchedule",
		func(_ *SyncerReconciler, _ context.Context, _ *v1.Workload, _ *resourceMessage, _ *ClusterClientSets) (bool, error) {
			return false, nil
		})

	w := &v1.Workload{ObjectMeta: metav1.ObjectMeta{
		Name:        "w",
		Annotations: map[string]string{v1.WorkloadDispatchedAnnotation: "true"},
	}}
	w.Spec.Workspace = "ns"
	cl := ctrlfake.NewClientBuilder().WithScheme(syncerScheme(t)).WithObjects(w).Build()
	r := &SyncerReconciler{Client: cl}

	_, err := r.handleJob(context.Background(),
		&resourceMessage{workloadId: "w", namespace: "ns", gvk: schema.GroupVersionKind{Kind: "Job"}},
		monkeyClientSets())
	assert.NilError(t, err)
}

func runnerFailureFixture(t *testing.T, elapsed time.Duration, conditions interface{}) (*SyncerReconciler, *v1.Workload, *resourceMessage, *unstructured.Unstructured) {
	t.Helper()
	w := unregisteredRunnerSet(elapsed)
	w.Name = "example-set"
	w.UID = "example-set-uid"
	w.Spec.Workspace = "test-workspace"
	w.Status.Phase = v1.WorkloadPending
	w.Status.Message = "waiting for capacity"
	v1.SetLabel(w, v1.WorkloadDispatchCntLabel, "1")
	v1.SetLabel(w, v1.ClusterIdLabel, "test-cluster")
	obj := &unstructured.Unstructured{Object: map[string]interface{}{}}
	rt := jobutils.TestCICDScaleSetResourceTemplate.DeepCopy()
	if conditions != nil {
		rt.Spec.ResourceStatus = v1.ResourceStatus{PrePaths: []string{"status", "conditions"}, MessagePaths: []string{"message"},
			Phases: []v1.PhaseExpression{{MatchExpressions: map[string]string{"type": "Failed", "status": "True"}, Phase: string(v1.K8sFailed)}}}
		obj.Object["status"] = map[string]interface{}{"conditions": conditions}
	}
	cli := ctrlfake.NewClientBuilder().WithScheme(syncerScheme(t)).WithObjects(rt).WithStatusSubresource(w).Build()
	assert.NilError(t, cli.Create(context.Background(), w))
	r := &SyncerReconciler{Client: cli}
	r.newCICDFailureWorker()
	patches := gomonkey.ApplyFunc(jobutils.GetObject, func(context.Context, *commonclient.ClientFactory, string, string, schema.GroupVersionKind) (*unstructured.Unstructured, error) {
		return obj.DeepCopy(), nil
	})
	t.Cleanup(patches.Reset)
	msg := &resourceMessage{workloadId: w.Name, name: w.Name, namespace: w.Spec.Workspace, dispatchCount: 1, action: ResourceUpdate, gvk: rt.ToSchemaGVK()}
	return r, w, msg, obj
}

func TestUpdateRunnerSetFailure_WithRegisteredID(t *testing.T) {
	conditions := []interface{}{map[string]interface{}{"type": "Failed", "status": "True", "message": "controller registration failure"}}
	r, w, msg, obj := runnerFailureFixture(t, time.Minute, conditions)
	obj.SetAnnotations(map[string]string{v1.CICDScaleSetIdAnnotation: "1"})
	result, err := r.updateAdminWorkloadByJob(context.Background(), monkeyClientSets(), w, msg)
	assert.NilError(t, err)
	assert.Equal(t, result.Status.RunnerScaleSetId, "1")
	assert.Assert(t, result.Status.StartTime != nil)
	assert.Equal(t, result.Status.Phase, v1.WorkloadFailed)
	assert.Equal(t, result.Status.Message, "controller registration failure")
	assert.Equal(t, result.Status.Conditions[0].Message, result.Status.Message)
	assert.Equal(t, r.cicdFailureLogs.GetQueueSize(), 1)
}

func TestUpdateRunnerSetFailureUsesWorkloadDispatchCount(t *testing.T) {
	conditions := []interface{}{map[string]interface{}{"type": "Failed", "status": "True", "message": "current dispatch failure"}}
	r, workload, message, _ := runnerFailureFixture(t, time.Minute, conditions)
	v1.SetLabel(workload, v1.WorkloadDispatchCntLabel, "2")
	assert.NilError(t, r.Update(context.Background(), workload))
	message.dispatchCount = 1

	result, err := r.updateAdminWorkloadByJob(context.Background(), monkeyClientSets(), workload, message)

	assert.NilError(t, err)
	assert.Equal(t, result.Status.Message, "current dispatch failure")
	assert.Equal(t, result.Status.Conditions[0].Reason, commonworkload.GenerateDispatchReason(2))
	assert.Equal(t, r.cicdFailureLogs.GetQueueSize(), 1)
}

func TestUpdateRunnerSetFailure_RespectsRetry(t *testing.T) {
	conditions := []interface{}{map[string]interface{}{"type": "Failed", "status": "True", "message": "retryable controller failure"}}
	r, w, msg, _ := runnerFailureFixture(t, time.Minute, conditions)
	w.Spec.MaxRetry = 3
	result, err := r.updateAdminWorkloadByJob(context.Background(), monkeyClientSets(), w, msg)
	assert.NilError(t, err)
	assert.Equal(t, result.Status.Phase, v1.WorkloadPending)
	assert.Equal(t, result.Status.Message, "waiting for capacity")
	assert.Equal(t, r.cicdFailureLogs.GetQueueSize(), 0)
}

func TestRunnerSetRegistrationTimeoutDiagnosticUsesConstant(t *testing.T) {
	source, err := parser.ParseFile(token.NewFileSet(), "job_handler.go", nil, 0)
	assert.NilError(t, err)
	for _, declaration := range source.Decls {
		function, ok := declaration.(*ast.FuncDecl)
		if !ok || function.Name.Name != "checkRunnerSetRegistration" {
			continue
		}
		for _, statement := range function.Body.List {
			assignment, ok := statement.(*ast.AssignStmt)
			if !ok || len(assignment.Lhs) != 1 || len(assignment.Rhs) != 1 {
				continue
			}
			name, ok := assignment.Lhs[0].(*ast.Ident)
			if !ok || name.Name != "diagnostic" {
				continue
			}
			call, ok := assignment.Rhs[0].(*ast.CallExpr)
			assert.Assert(t, ok, "registration timeout diagnostic must be formatted from its timeout constant")
			formatter, ok := call.Fun.(*ast.SelectorExpr)
			assert.Assert(t, ok)
			pkg, ok := formatter.X.(*ast.Ident)
			assert.Assert(t, ok)
			assert.Equal(t, pkg.Name, "fmt")
			assert.Equal(t, formatter.Sel.Name, "Sprintf")
			assert.Equal(t, len(call.Args), 2)
			timeout, ok := call.Args[1].(*ast.Ident)
			assert.Assert(t, ok)
			assert.Equal(t, timeout.Name, "runnerSetRegistrationTimeout")
			return
		}
	}
	t.Fatal("registration timeout diagnostic assignment not found")
}

func TestRunnerSetProxyRegistrationTimeout_Message(t *testing.T) {
	for _, elapsed := range []time.Duration{time.Minute, runnerSetRegistrationTimeout, runnerSetRegistrationTimeout + time.Minute} {
		t.Run(elapsed.String(), func(t *testing.T) {
			r, w, msg, _ := runnerFailureFixture(t, elapsed, nil)
			v1.SetAnnotation(w, v1.CICDProxyManagedAnnotation, v1.TrueStr)
			w.Spec.Env = map[string]string{common.ProxyUrl: "http://proxy.example.com"}
			assert.NilError(t, r.Update(context.Background(), w))
			result, err := r.handleJob(context.Background(), msg, monkeyClientSets())
			assert.NilError(t, err)
			current := &v1.Workload{}
			assert.NilError(t, r.Get(context.Background(), ctrlclient.ObjectKeyFromObject(w), current))
			if elapsed < runnerSetRegistrationTimeout {
				assert.Assert(t, result.RequeueAfter > 0)
				assert.Equal(t, r.cicdFailureLogs.GetQueueSize(), 0)
				return
			}
			assert.Equal(t, current.Status.Phase, v1.WorkloadFailed)
			assert.Assert(t, strings.Contains(current.Status.Message, fmt.Sprintf("Runner scale set registration timed out after %s", runnerSetRegistrationTimeout)))
			assert.Assert(t, strings.Contains(current.Status.Message, "Proxy configuration is enabled"))
			assert.Equal(t, current.Status.Conditions[0].Message, current.Status.Message)
			assert.Equal(t, r.cicdFailureLogs.GetQueueSize(), 1)
		})
	}
}

func TestRunnerSetRegistrationTimeout_PreservesControllerDetail(t *testing.T) {
	w := unregisteredRunnerSet(runnerSetRegistrationTimeout)
	w.Status.Conditions = []metav1.Condition{*jobutils.NewCondition(string(v1.K8sFailed), "controller failure detail", commonworkload.GenerateDispatchReason(1))}
	_, err := checkRunnerSetRegistration(w, &resourceMessage{action: ResourceUpdate})
	assert.ErrorContains(t, err, "controller failure detail")
	w.Status.Conditions[0].Reason = commonworkload.GenerateDispatchReason(2)
	_, err = checkRunnerSetRegistration(w, &resourceMessage{action: ResourceUpdate})
	assert.Assert(t, !strings.Contains(err.Error(), "controller failure detail"))
}

func TestRunnerSetRegistrationTimeout_UnreadableStatus(t *testing.T) {
	for _, elapsed := range []time.Duration{time.Minute, runnerSetRegistrationTimeout} {
		for _, registered := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/%t", elapsed, registered), func(t *testing.T) {
				r, w, msg, obj := runnerFailureFixture(t, elapsed, "malformed conditions")
				if registered {
					obj.SetAnnotations(map[string]string{v1.CICDScaleSetIdAnnotation: "1"})
				}
				_, err := r.handleJobImpl(context.Background(), msg, w, monkeyClientSets())
				assert.Assert(t, err != nil)
				if registered || elapsed < runnerSetRegistrationTimeout {
					assert.Assert(t, !jobutils.IsUnrecoverableError(err))
				} else {
					assert.ErrorContains(t, err, "ARC status could not be read")
					assert.Assert(t, jobutils.IsUnrecoverableError(err))
				}
				if registered {
					current := &v1.Workload{}
					assert.NilError(t, r.Get(context.Background(), ctrlclient.ObjectKeyFromObject(w), current))
					assert.Equal(t, current.Status.RunnerScaleSetId, "1")
				}
			})
		}
	}
}

func TestRunnerSetFailure_NoProxy(t *testing.T) {
	r, w, msg, _ := runnerFailureFixture(t, runnerSetRegistrationTimeout, nil)
	_, err := r.handleJob(context.Background(), msg, monkeyClientSets())
	assert.NilError(t, err)
	current := &v1.Workload{}
	assert.NilError(t, r.Get(context.Background(), ctrlclient.ObjectKeyFromObject(w), current))
	assert.Equal(t, current.Status.Phase, v1.WorkloadFailed)
	assert.Assert(t, strings.TrimSpace(current.Status.Message) != "")
	assert.Assert(t, !strings.Contains(current.Status.Message, "Proxy configuration is enabled"))
}

func TestRunnerSetFailure_ConflictAndRestart(t *testing.T) {
	conditions := []interface{}{map[string]interface{}{"type": "Failed", "status": "True", "message": "controller failure detail"}}
	r, w, msg, obj := runnerFailureFixture(t, time.Minute, conditions)
	obj.SetAnnotations(map[string]string{v1.CICDScaleSetIdAnnotation: "1"})
	stale := w.DeepCopy()
	v1.SetAnnotation(w, "example.com/change", "concurrent")
	assert.NilError(t, r.Update(context.Background(), w))
	_, err := r.updateAdminWorkloadByJob(context.Background(), monkeyClientSets(), stale, msg)
	assert.Assert(t, apierrors.IsConflict(err))
	restarted := &SyncerReconciler{Client: r.Client}
	current := &v1.Workload{}
	assert.NilError(t, r.Get(context.Background(), ctrlclient.ObjectKeyFromObject(w), current))
	current, err = restarted.updateAdminWorkloadByJob(context.Background(), monkeyClientSets(), current, msg)
	assert.NilError(t, err)
	assert.Equal(t, current.Status.RunnerScaleSetId, "1")
	assert.Equal(t, current.Status.Message, "controller failure detail")
	before := current.DeepCopy()
	obj.Object = map[string]interface{}{}
	msg.action = ResourceDel
	current, err = restarted.updateAdminWorkloadByJob(context.Background(), nil, current, msg)
	assert.NilError(t, err)
	assert.DeepEqual(t, current.Status, before.Status)
}
