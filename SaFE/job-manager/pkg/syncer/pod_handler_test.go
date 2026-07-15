/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package syncer

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/agiledragon/gomonkey/v2"
	tassert "github.com/stretchr/testify/assert"
	"gotest.tools/assert"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	ctrlruntime "sigs.k8s.io/controller-runtime"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	ctrlfake "sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	commonfaults "github.com/AMD-AIG-AIMA/SAFE/common/pkg/faults"
	commonclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/k8sclient"
	jobutils "github.com/AMD-AIG-AIMA/SAFE/job-manager/pkg/utils"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/timeutil"
)

// clientSetsWith returns a client set backed by an empty fake kubernetes client.
func clientSetsWith() *ClusterClientSets {
	cs := k8sfake.NewSimpleClientset()
	return &ClusterClientSets{
		dataClientFactory: commonclient.NewClientFactoryWithOnlyClient(context.Background(), "c", cs),
	}
}

// setupTestScheme creates a scheme with required types for testing.
func setupTestScheme() *runtime.Scheme {
	scheme := runtime.NewScheme()
	_ = v1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)
	return scheme
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

func TestGetK8sNodeEmptyName(t *testing.T) {
	r := &SyncerReconciler{}
	// Empty node name -> returns an empty node without touching the client.
	node, err := r.getK8sNode(context.Background(), nil, "")
	assert.NilError(t, err)
	assert.Assert(t, node != nil)
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

func TestDeletePodNilObject(t *testing.T) {
	r := &SyncerReconciler{}
	res, err := r.deletePod(context.Background(), nil, nil)
	assert.NilError(t, err)
	assert.Equal(t, res.RequeueAfter.Nanoseconds(), int64(0))
}

func TestDeletePodRecentDeletionRequeues(t *testing.T) {
	r := &SyncerReconciler{}
	obj := &unstructured.Unstructured{Object: map[string]interface{}{}}
	now := metav1.NewTime(time.Now())
	obj.SetDeletionTimestamp(&now)
	res, err := r.deletePod(context.Background(), obj, nil)
	assert.NilError(t, err)
	// Recently-deleted pod -> requeue, not yet force-deleted.
	assert.Assert(t, res.RequeueAfter > 0)
}

func TestConvertPodFromUnstructured(t *testing.T) {
	obj := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "v1",
		"kind":       "Pod",
		"metadata":   map[string]interface{}{"name": "p1"},
		"status":     map[string]interface{}{"phase": "Running"},
	}}
	pod := convertPodFromUnstructured(obj)
	assert.Assert(t, pod != nil)
	assert.Equal(t, pod.Name, "p1")
	assert.Equal(t, string(pod.Status.Phase), "Running")

	// Failed pod hits the failure-logging branch.
	failed := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "v1",
		"kind":       "Pod",
		"metadata":   map[string]interface{}{"name": "p2"},
		"status":     map[string]interface{}{"phase": "Failed"},
	}}
	pod2 := convertPodFromUnstructured(failed)
	assert.Assert(t, pod2 != nil)
	assert.Equal(t, string(pod2.Status.Phase), "Failed")
}

func TestUpdateCICDScalingRunnerSetPhase(t *testing.T) {
	mkPod := func(phase corev1.PodPhase) *corev1.Pod {
		return &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{appComponent: scaleSetListener}},
			Status:     corev1.PodStatus{Phase: phase},
		}
	}

	w := &v1.Workload{}
	updateCICDScalingRunnerSetPhase(w, mkPod(corev1.PodRunning))
	assert.Equal(t, string(w.Status.Phase), string(v1.WorkloadRunning))

	updateCICDScalingRunnerSetPhase(w, mkPod(corev1.PodPending))
	assert.Equal(t, string(w.Status.Phase), string(v1.WorkloadPending))

	updateCICDScalingRunnerSetPhase(w, mkPod(corev1.PodSucceeded))
	assert.Equal(t, string(w.Status.Phase), string(v1.WorkloadNotReady))

	// Pod without the listener label is ignored.
	w2 := &v1.Workload{}
	updateCICDScalingRunnerSetPhase(w2, &corev1.Pod{Status: corev1.PodStatus{Phase: corev1.PodRunning}})
	assert.Equal(t, string(w2.Status.Phase), "")
}

func TestCompareRayJobPodPriority(t *testing.T) {
	running := v1.WorkloadPod{Phase: corev1.PodRunning, PodId: "a"}
	pending := v1.WorkloadPod{Phase: corev1.PodPending, PodId: "b"}
	// Running has higher phase priority than pending.
	assert.Assert(t, compareRayJobPodPriority(running, pending) > 0)
	assert.Assert(t, compareRayJobPodPriority(pending, running) < 0)

	// Same phase, tie broken by start time (later wins).
	now := time.Now().UTC()
	early := v1.WorkloadPod{Phase: corev1.PodRunning, PodId: "a", StartTime: timeutil.FormatRFC3339(now.Add(-time.Hour))}
	late := v1.WorkloadPod{Phase: corev1.PodRunning, PodId: "a", StartTime: timeutil.FormatRFC3339(now)}
	assert.Assert(t, compareRayJobPodPriority(late, early) > 0)

	// Same phase and time, tie broken by pod id.
	p1 := v1.WorkloadPod{Phase: corev1.PodRunning, PodId: "a"}
	p2 := v1.WorkloadPod{Phase: corev1.PodRunning, PodId: "b"}
	assert.Assert(t, compareRayJobPodPriority(p2, p1) > 0)
	assert.Equal(t, compareRayJobPodPriority(p1, p1), 0)
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

func TestRemoveWorkloadPodEmptyId(t *testing.T) {
	r := &SyncerReconciler{}
	err := r.removeWorkloadPod(context.Background(), &resourceMessage{})
	assert.NilError(t, err)
}

func TestRemoveWorkloadPodNotFound(t *testing.T) {
	cl := ctrlfake.NewClientBuilder().WithScheme(syncerScheme(t)).Build()
	r := &SyncerReconciler{Client: cl}
	err := r.removeWorkloadPod(context.Background(), &resourceMessage{workloadId: "missing", name: "p"})
	assert.NilError(t, err)
}

func TestRemoveWorkloadPodEnded(t *testing.T) {
	w := &v1.Workload{ObjectMeta: metav1.ObjectMeta{Name: "w"}}
	w.Status.Phase = v1.WorkloadFailed
	cl := ctrlfake.NewClientBuilder().WithScheme(syncerScheme(t)).WithObjects(w).Build()
	r := &SyncerReconciler{Client: cl}
	err := r.removeWorkloadPod(context.Background(), &resourceMessage{workloadId: "w", name: "p"})
	assert.NilError(t, err)
}

func TestRemoveWorkloadPodNotInList(t *testing.T) {
	w := &v1.Workload{ObjectMeta: metav1.ObjectMeta{
		Name:        "w",
		Annotations: map[string]string{v1.WorkloadDispatchedAnnotation: "true"},
	}}
	w.Spec.MaxRetry = 3
	w.Status.Pods = []v1.WorkloadPod{{PodId: "other"}}
	cl := ctrlfake.NewClientBuilder().WithScheme(syncerScheme(t)).WithObjects(w).Build()
	r := &SyncerReconciler{Client: cl}
	err := r.removeWorkloadPod(context.Background(),
		&resourceMessage{workloadId: "w", name: "p", dispatchCount: 1})
	assert.NilError(t, err)
}

func TestRemoveWorkloadPodStopsLivePod(t *testing.T) {
	w := &v1.Workload{ObjectMeta: metav1.ObjectMeta{
		Name:        "w",
		Annotations: map[string]string{v1.WorkloadDispatchedAnnotation: "true"},
	}}
	w.Spec.MaxRetry = 3
	w.Status.Pods = []v1.WorkloadPod{{PodId: "p1"}, {PodId: "p2"}}
	cl := ctrlfake.NewClientBuilder().
		WithScheme(syncerScheme(t)).
		WithObjects(w).
		WithStatusSubresource(&v1.Workload{}).
		Build()
	r := &SyncerReconciler{Client: cl}
	err := r.removeWorkloadPod(context.Background(),
		&resourceMessage{workloadId: "w", name: "p1", dispatchCount: 1})
	assert.NilError(t, err)

	got := &v1.Workload{}
	assert.NilError(t, cl.Get(context.Background(), ctrlclient.ObjectKey{Name: "w"}, got))
	// Non-application workload: the pod entry is kept and a still-live pod is
	// flipped to Stopped instead of being removed.
	assert.Equal(t, len(got.Status.Pods), 2)
	assert.Equal(t, got.Status.Pods[0].PodId, "p1")
	assert.Equal(t, got.Status.Pods[0].Phase, corev1.PodPhase(v1.WorkloadStopped))
	assert.Equal(t, got.Status.Pods[1].PodId, "p2")
}

func TestRemoveWorkloadPodStopsLivePodDuringTeardown(t *testing.T) {
	now := metav1.Now()
	w := &v1.Workload{ObjectMeta: metav1.ObjectMeta{
		Name:              "w",
		Annotations:       map[string]string{v1.WorkloadDispatchedAnnotation: "true"},
		DeletionTimestamp: &now,
		Finalizers:        []string{"test/keep"},
	}}
	w.Spec.MaxRetry = 3
	w.Status.Pods = []v1.WorkloadPod{
		{PodId: "p1", Phase: corev1.PodRunning},
		{PodId: "p2", Phase: corev1.PodRunning},
	}
	cl := ctrlfake.NewClientBuilder().
		WithScheme(syncerScheme(t)).
		WithObjects(w).
		WithStatusSubresource(&v1.Workload{}).
		Build()
	r := &SyncerReconciler{Client: cl}
	err := r.removeWorkloadPod(context.Background(),
		&resourceMessage{workloadId: "w", name: "p1", dispatchCount: 1})
	assert.NilError(t, err)

	got := &v1.Workload{}
	assert.NilError(t, cl.Get(context.Background(), ctrlclient.ObjectKey{Name: "w"}, got))
	// Even while the workload is being torn down, the pod row is kept as history
	// and the still-live pod is recorded as Stopped rather than left at Running.
	assert.Equal(t, len(got.Status.Pods), 2)
	assert.Equal(t, got.Status.Pods[0].PodId, "p1")
	assert.Equal(t, got.Status.Pods[0].Phase, corev1.PodPhase(v1.WorkloadStopped))
}

func TestRemoveWorkloadPodKeepsHistoryOnTeardown(t *testing.T) {
	w := &v1.Workload{ObjectMeta: metav1.ObjectMeta{
		Name:        "w",
		Finalizers:  []string{"test/keep"},
		Annotations: map[string]string{v1.WorkloadDispatchedAnnotation: "true"},
	}}
	w.Spec.MaxRetry = 3
	w.Status.Pods = []v1.WorkloadPod{
		{PodId: "p1", Phase: corev1.PodRunning},
		{PodId: "p2", Phase: corev1.PodFailed},
	}
	cl := ctrlfake.NewClientBuilder().
		WithScheme(syncerScheme(t)).
		WithObjects(w).
		WithStatusSubresource(&v1.Workload{}).
		Build()
	// The finalizer keeps the object around with a deletion timestamp set,
	// simulating workload teardown.
	assert.NilError(t, cl.Delete(context.Background(), w.DeepCopy()))

	r := &SyncerReconciler{Client: cl}
	err := r.removeWorkloadPod(context.Background(),
		&resourceMessage{workloadId: "w", name: "p1", dispatchCount: 1})
	assert.NilError(t, err)

	got := &v1.Workload{}
	assert.NilError(t, cl.Get(context.Background(), ctrlclient.ObjectKey{Name: "w"}, got))
	// History is preserved during teardown: both entries remain, the live pod is
	// flipped to Stopped, the already-terminal pod keeps its final phase.
	assert.Equal(t, len(got.Status.Pods), 2)
	assert.Equal(t, got.Status.Pods[0].Phase, corev1.PodPhase(v1.WorkloadStopped))
	assert.Equal(t, got.Status.Pods[1].Phase, corev1.PodFailed)
}

func TestHandleRaySubmitterTimeoutNonRayJob(t *testing.T) {
	r := &SyncerReconciler{}
	w := &v1.Workload{}
	pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "p"}}
	ok, err := r.handleRaySubmitterTimeout(context.Background(), w, pod)
	assert.NilError(t, err)
	assert.Equal(t, ok, false)
}

func TestBuildPodTerminatedInfoRunningNoop(t *testing.T) {
	w := &v1.Workload{}
	pod := &corev1.Pod{Status: corev1.PodStatus{Phase: corev1.PodRunning}}
	wp := &v1.WorkloadPod{}
	buildPodTerminatedInfo(context.Background(), nil, w, pod, wp, "main")
	// Running pod -> no termination info recorded.
	assert.Equal(t, wp.EndTime, "")
	assert.Equal(t, len(wp.Containers), 0)
}

func TestBuildPodTerminatedInfoFailed(t *testing.T) {
	w := &v1.Workload{}
	pod := &corev1.Pod{Status: corev1.PodStatus{
		Phase:   corev1.PodFailed,
		Reason:  "OOMKilled",
		Message: "out of memory",
	}}
	wp := &v1.WorkloadPod{}
	buildPodTerminatedInfo(context.Background(), nil, w, pod, wp, "main")
	assert.Assert(t, wp.FailedMessage != "")
	assert.Assert(t, wp.EndTime != "")
}

func TestBuildPodTerminatedInfoSucceeded(t *testing.T) {
	w := &v1.Workload{}
	pod := &corev1.Pod{Status: corev1.PodStatus{
		Phase: corev1.PodSucceeded,
		ContainerStatuses: []corev1.ContainerStatus{{
			Name: "main",
			State: corev1.ContainerState{
				Terminated: &corev1.ContainerStateTerminated{ExitCode: 0},
			},
		}},
	}}
	wp := &v1.WorkloadPod{}
	buildPodTerminatedInfo(context.Background(), nil, w, pod, wp, "main")
	assert.Equal(t, len(wp.Containers), 1)
	assert.Assert(t, wp.EndTime != "")
}

func TestGenerateStickyFault(t *testing.T) {
	// Empty node id -> nil fault, no error.
	f, err := generateStickyFault(&v1.Workload{}, "", syncerScheme(t))
	assert.NilError(t, err)
	assert.Assert(t, f == nil)

	// Valid node id -> a fault is generated.
	w := &v1.Workload{ObjectMeta: metav1.ObjectMeta{Name: "w"}}
	f2, err := generateStickyFault(w, "node-1", syncerScheme(t))
	assert.NilError(t, err)
	assert.Assert(t, f2 != nil)
	assert.Equal(t, f2.Spec.Node.AdminName, "node-1")
}

// TestUpdateAdminWorkloadByPodPath patches the per-pod helpers so updateAdminWorkloadByPod
// runs its orchestration up to the "no update needed" early return.
func TestUpdateAdminWorkloadByPodPath(t *testing.T) {
	w := &v1.Workload{ObjectMeta: metav1.ObjectMeta{
		Name:        "w",
		Annotations: map[string]string{v1.WorkloadDispatchedAnnotation: "true"},
	}}

	patches := gomonkey.NewPatches()
	defer patches.Reset()
	patches.ApplyPrivateMethod(reflect.TypeOf(&SyncerReconciler{}), "getAdminWorkloadAndSyncPod",
		func(_ *SyncerReconciler, _ context.Context, _ *ClusterClientSets, _ *corev1.Pod, _ *resourceMessage) (*v1.Workload, error) {
			return w, nil
		})
	patches.ApplyPrivateMethod(reflect.TypeOf(&SyncerReconciler{}), "handleRaySubmitterTimeout",
		func(_ *SyncerReconciler, _ context.Context, _ *v1.Workload, _ *corev1.Pod) (bool, error) {
			return false, nil
		})
	patches.ApplyPrivateMethod(reflect.TypeOf(&SyncerReconciler{}), "getK8sNode",
		func(_ *SyncerReconciler, _ context.Context, _ *ClusterClientSets, _ string) (*corev1.Node, error) {
			return &corev1.Node{}, nil
		})
	patches.ApplyPrivateMethod(reflect.TypeOf(&SyncerReconciler{}), "updateWorkloadNodeAndPods",
		func(_ *SyncerReconciler, _ context.Context, _ *ClusterClientSets, _ *v1.Workload, _ *corev1.Pod, _ *corev1.Node) (v1.WorkloadPod, corev1.PodPhase, bool) {
			return v1.WorkloadPod{}, "", false
		})

	r := &SyncerReconciler{}
	obj := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "v1", "kind": "Pod",
		"metadata": map[string]interface{}{"name": "p1"},
		"status":   map[string]interface{}{"phase": "Running"},
	}}
	_, err := r.updateAdminWorkloadByPod(context.Background(), monkeyClientSets(), obj, &resourceMessage{workloadId: "w"})
	assert.NilError(t, err)
}

// TestHandlePodPath patches the informer lookup + per-pod update so handlePod runs the
// "object present" branch into updateAdminWorkloadByPod.
func TestHandlePodPath(t *testing.T) {
	patches := gomonkey.NewPatches()
	defer patches.Reset()
	patches.ApplyMethod(reflect.TypeOf(&ClusterClientSets{}), "GetResourceInformer",
		func(_ *ClusterClientSets, _ context.Context, _ schema.GroupVersionKind) (informers.GenericInformer, error) {
			return nil, nil
		})
	patches.ApplyFunc(jobutils.GetObjectByInformer,
		func(informers.GenericInformer, string, string) (*unstructured.Unstructured, error) {
			return &unstructured.Unstructured{}, nil
		})
	patches.ApplyPrivateMethod(reflect.TypeOf(&SyncerReconciler{}), "updateAdminWorkloadByPod",
		func(_ *SyncerReconciler, _ context.Context, _ *ClusterClientSets, _ *unstructured.Unstructured, _ *resourceMessage) (ctrlruntime.Result, error) {
			return ctrlruntime.Result{}, nil
		})

	r := &SyncerReconciler{}
	_, err := r.handlePod(context.Background(),
		&resourceMessage{name: "p1", namespace: "ns", gvk: schema.GroupVersionKind{Kind: "Pod"}},
		monkeyClientSets())
	assert.NilError(t, err)
}

// TestUpdateWorkloadNodeAndPodsAppend patches buildWorkloadPodInfo so the function
// appends a new pod entry and recomputes node assignments.
func TestUpdateWorkloadNodeAndPodsAppend(t *testing.T) {
	patches := gomonkey.NewPatches()
	defer patches.Reset()
	patches.ApplyPrivateMethod(reflect.TypeOf(&SyncerReconciler{}), "buildWorkloadPodInfo",
		func(_ *SyncerReconciler, _ context.Context, _ *ClusterClientSets, _ *v1.Workload, _ *corev1.Pod, _ *corev1.Node) v1.WorkloadPod {
			return v1.WorkloadPod{PodId: "p1", AdminNodeName: "n1"}
		})

	r := &SyncerReconciler{}
	w := &v1.Workload{ObjectMeta: metav1.ObjectMeta{
		Name:   "w",
		Labels: map[string]string{v1.WorkloadDispatchCntLabel: "1"},
	}}
	pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "p1"}}
	podInfo, _, updated := r.updateWorkloadNodeAndPods(context.Background(), monkeyClientSets(), w, pod, &corev1.Node{})
	assert.Equal(t, updated, true)
	assert.Equal(t, podInfo.PodId, "p1")
	assert.Equal(t, len(w.Status.Pods), 1)
}

// TestGetAdminWorkloadAndSyncPodNonMesh covers the non-mesh path: the admin workload is
// fetched directly by message.workloadId and stamped with the dispatch count.
func TestGetAdminWorkloadAndSyncPodNonMesh(t *testing.T) {
	w := &v1.Workload{ObjectMeta: metav1.ObjectMeta{Name: "w"}}
	cl := ctrlfake.NewClientBuilder().WithScheme(syncerScheme(t)).WithObjects(w).Build()
	r := &SyncerReconciler{Client: cl}

	pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "p1"}}
	got, err := r.getAdminWorkloadAndSyncPod(context.Background(), monkeyClientSets(), pod,
		&resourceMessage{workloadId: "w", dispatchCount: 2})
	assert.NilError(t, err)
	assert.Assert(t, got != nil)
	assert.Equal(t, v1.GetWorkloadDispatchCnt(got), 2)
}

// TestGetAdminWorkloadAndSyncPodMissingWorkload reproduces the nil-pointer panic:
// when the admin workload is deleted (NotFound), getAdminWorkload returns
// (nil, nil) and the sync must return (nil, nil) instead of dereferencing nil in
// SetLabel.
func TestGetAdminWorkloadAndSyncPodMissingWorkload(t *testing.T) {
	cl := ctrlfake.NewClientBuilder().WithScheme(syncerScheme(t)).Build()
	r := &SyncerReconciler{Client: cl}

	pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "p1"}}
	got, err := r.getAdminWorkloadAndSyncPod(context.Background(), monkeyClientSets(), pod,
		&resourceMessage{workloadId: "missing", dispatchCount: 1})
	assert.NilError(t, err)
	assert.Assert(t, got == nil)
}

// TestBuildWorkloadPodInfo patches buildPodTerminatedInfo (the only clientSet user) so
// buildWorkloadPodInfo can assemble pod metadata without a live cluster.
func TestBuildWorkloadPodInfo(t *testing.T) {
	patches := gomonkey.NewPatches()
	defer patches.Reset()
	patches.ApplyFunc(buildPodTerminatedInfo,
		func(context.Context, kubernetes.Interface, *v1.Workload, *corev1.Pod, *v1.WorkloadPod, string) {})

	r := &SyncerReconciler{}
	w := &v1.Workload{ObjectMeta: metav1.ObjectMeta{Name: "w"}}
	pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "p1"}}
	pod.Status.HostIP = "1.2.3.4"
	info := r.buildWorkloadPodInfo(context.Background(), monkeyClientSets(), w, pod, &corev1.Node{})
	assert.Equal(t, info.PodId, "p1")
	assert.Equal(t, info.HostIp, "1.2.3.4")
}

// TestGetMainContainerRank tests extraction of RANK environment variable
func TestGetMainContainerRank(t *testing.T) {
	tests := []struct {
		name         string
		workload     *v1.Workload
		pod          *corev1.Pod
		expectedRank string
	}{
		{
			name: "pod with RANK env variable",
			workload: &v1.Workload{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						v1.MainContainerAnnotation: "main",
					},
				},
				Spec: v1.WorkloadSpec{
					Images: []string{"pytorch:latest"},
				},
			},
			pod: &corev1.Pod{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name: "main",
							Env: []corev1.EnvVar{
								{Name: "RANK", Value: "0"},
								{Name: "WORLD_SIZE", Value: "4"},
							},
						},
					},
				},
			},
			expectedRank: "0",
		},
		{
			name: "pod with multiple containers",
			workload: &v1.Workload{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						v1.MainContainerAnnotation: "worker",
					},
				},
				Spec: v1.WorkloadSpec{
					Images: []string{"pytorch:latest"},
				},
			},
			pod: &corev1.Pod{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name: "sidecar",
							Env: []corev1.EnvVar{
								{Name: "RANK", Value: "999"}, // Wrong container
							},
						},
						{
							Name: "worker",
							Env: []corev1.EnvVar{
								{Name: "RANK", Value: "2"},
							},
						},
					},
				},
			},
			expectedRank: "2",
		},
		{
			name: "pod without RANK env",
			workload: &v1.Workload{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						v1.MainContainerAnnotation: "main",
					},
				},
				Spec: v1.WorkloadSpec{
					Images: []string{"pytorch:latest"},
				},
			},
			pod: &corev1.Pod{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name: "main",
							Env: []corev1.EnvVar{
								{Name: "OTHER_VAR", Value: "value"},
							},
						},
					},
				},
			},
			expectedRank: "",
		},
		{
			name: "empty pod",
			workload: &v1.Workload{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						v1.MainContainerAnnotation: "main",
					},
				},
				Spec: v1.WorkloadSpec{
					Images: []string{"pytorch:latest"},
				},
			},
			pod: &corev1.Pod{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{},
				},
			},
			expectedRank: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			name := getMainContainerName(tt.workload, tt.pod)
			rank := getMainContainerRank(name, tt.pod)
			tassert.Equal(t, tt.expectedRank, rank)
		})
	}
}

// TestCreateStickyNodeFaults tests the createStickyNodeFaults function
func TestCreateStickyNodeFaults(t *testing.T) {
	ctx := context.Background()
	scheme := setupTestScheme()

	t.Run("sticky nodes not enabled - should skip", func(t *testing.T) {
		workload := &v1.Workload{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-workload",
				Labels: map[string]string{
					v1.WorkloadDispatchCntLabel: "1",
				},
			},
		}
		cli := ctrlfake.NewClientBuilder().WithScheme(scheme).Build()
		r := &SyncerReconciler{Client: cli}

		err := r.createStickyNodeFaults(ctx, workload)
		tassert.NoError(t, err)

		// Verify no fault was created
		faultList := &v1.FaultList{}
		err = cli.List(ctx, faultList)
		tassert.NoError(t, err)
		tassert.Empty(t, faultList.Items)
	})

	t.Run("count is zero - should skip", func(t *testing.T) {
		workload := &v1.Workload{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-workload",
				Labels: map[string]string{
					v1.WorkloadDispatchCntLabel: "0",
				},
				Annotations: map[string]string{
					v1.RetryOnOriginalNodesAnnotation: v1.TrueStr,
				},
			},
		}
		cli := ctrlfake.NewClientBuilder().WithScheme(scheme).Build()
		r := &SyncerReconciler{Client: cli}

		err := r.createStickyNodeFaults(ctx, workload)
		tassert.NoError(t, err)

		// Verify no fault was created
		faultList := &v1.FaultList{}
		err = cli.List(ctx, faultList)
		tassert.NoError(t, err)
		tassert.Empty(t, faultList.Items)
	})

	t.Run("sticky nodes enabled with count=1 - should create faults", func(t *testing.T) {
		workload := &v1.Workload{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-workload",
				UID:  "test-uid",
				Labels: map[string]string{
					v1.WorkloadDispatchCntLabel: "1",
				},
				Annotations: map[string]string{
					v1.RetryOnOriginalNodesAnnotation: v1.TrueStr,
				},
			},
			Spec: v1.WorkloadSpec{
				MaxRetry: 3,
			},
			Status: v1.WorkloadStatus{
				Nodes: [][]string{
					{"node-1", "node-2"},
				},
				Pods: []v1.WorkloadPod{
					{AdminNodeName: "node-1"},
					{AdminNodeName: "node-2"},
				},
			},
		}
		cli := ctrlfake.NewClientBuilder().WithScheme(scheme).Build()
		r := &SyncerReconciler{Client: cli}

		err := r.createStickyNodeFaults(ctx, workload)
		tassert.NoError(t, err)

		// Verify faults were created for both nodes
		faultList := &v1.FaultList{}
		err = cli.List(ctx, faultList)
		tassert.NoError(t, err)
		tassert.Len(t, faultList.Items, 2)

		// Verify fault IDs
		faultIds := make(map[string]bool)
		for _, f := range faultList.Items {
			faultIds[f.Name] = true
		}
		expectedFault1 := commonfaults.GenerateFaultId("node-1", v1.StickyNodesMonitorId)
		expectedFault2 := commonfaults.GenerateFaultId("node-2", v1.StickyNodesMonitorId)
		tassert.True(t, faultIds[expectedFault1], "fault for node-1 should exist")
		tassert.True(t, faultIds[expectedFault2], "fault for node-2 should exist")
	})

	t.Run("sticky nodes enabled with count=2 - should add new and delete old faults", func(t *testing.T) {
		workload := &v1.Workload{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-workload",
				UID:  "test-uid",
				Labels: map[string]string{
					v1.WorkloadDispatchCntLabel: "2",
				},
				Annotations: map[string]string{
					v1.RetryOnOriginalNodesAnnotation: v1.TrueStr,
				},
			},
			Spec: v1.WorkloadSpec{
				MaxRetry: 3,
			},
			Status: v1.WorkloadStatus{
				Nodes: [][]string{
					{"node-1", "node-2"}, // previous nodes
					{"node-2", "node-3"}, // current nodes (node-1 removed, node-3 added)
				},
				Pods: []v1.WorkloadPod{
					{AdminNodeName: "node-2"},
					{AdminNodeName: "node-3"},
				},
			},
		}

		// Pre-create fault for node-1 (which should be deleted)
		existingFault := &v1.Fault{
			ObjectMeta: metav1.ObjectMeta{
				Name: commonfaults.GenerateFaultId("node-1", v1.StickyNodesMonitorId),
			},
			Spec: v1.FaultSpec{
				MonitorId: v1.StickyNodesMonitorId,
			},
		}
		cli := ctrlfake.NewClientBuilder().WithScheme(scheme).WithObjects(existingFault).Build()
		r := &SyncerReconciler{Client: cli}

		err := r.createStickyNodeFaults(ctx, workload)
		tassert.NoError(t, err)

		// Verify fault for node-3 was created
		expectedFault3 := commonfaults.GenerateFaultId("node-3", v1.StickyNodesMonitorId)
		fault3 := &v1.Fault{}
		err = cli.Get(ctx, ctrlclient.ObjectKey{Name: expectedFault3}, fault3)
		tassert.NoError(t, err, "fault for node-3 should be created")

		// Verify fault for node-1 was deleted
		expectedFault1 := commonfaults.GenerateFaultId("node-1", v1.StickyNodesMonitorId)
		fault1 := &v1.Fault{}
		err = cli.Get(ctx, ctrlclient.ObjectKey{Name: expectedFault1}, fault1)
		tassert.True(t, apierrors.IsNotFound(err), "fault for node-1 should be deleted")
	})
}

// TestSortWorkloadPods tests sorting of workload pods by IP and ID
func TestSortWorkloadPods(t *testing.T) {
	tests := []struct {
		name          string
		inputPods     []v1.WorkloadPod
		expectedOrder []string // Pod IDs in expected order
	}{
		{
			name: "sort by different IPs",
			inputPods: []v1.WorkloadPod{
				{PodId: "pod-1", HostIp: "192.168.1.1"},
				{PodId: "pod-2", HostIp: "192.168.1.100"},
				{PodId: "pod-3", HostIp: "192.168.1.50"},
			},
			expectedOrder: []string{"pod-1", "pod-3", "pod-2"}, // Sorted by IP descending
		},
		{
			name: "sort by pod ID when same IP",
			inputPods: []v1.WorkloadPod{
				{PodId: "pod-c", HostIp: "192.168.1.1"},
				{PodId: "pod-a", HostIp: "192.168.1.1"},
				{PodId: "pod-b", HostIp: "192.168.1.1"},
			},
			expectedOrder: []string{"pod-a", "pod-b", "pod-c"}, // Sorted by pod ID ascending
		},
		{
			name: "mixed IPs and IDs",
			inputPods: []v1.WorkloadPod{
				{PodId: "pod-2", HostIp: "10.0.0.5"},
				{PodId: "pod-1", HostIp: "10.0.0.5"},
				{PodId: "pod-4", HostIp: "10.0.0.10"},
				{PodId: "pod-3", HostIp: "10.0.0.10"},
			},
			expectedOrder: []string{"pod-1", "pod-2", "pod-3", "pod-4"},
		},
		{
			name: "single pod",
			inputPods: []v1.WorkloadPod{
				{PodId: "pod-1", HostIp: "192.168.1.1"},
			},
			expectedOrder: []string{"pod-1"},
		},
		{
			name:          "empty pods",
			inputPods:     []v1.WorkloadPod{},
			expectedOrder: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			workload := &v1.Workload{
				Spec: v1.WorkloadSpec{
					GroupVersionKind: v1.GroupVersionKind{Kind: common.PytorchJobKind},
				},
				Status: v1.WorkloadStatus{
					Pods: tt.inputPods,
				},
			}

			sortWorkloadPods(workload)

			tassert.Equal(t, len(tt.expectedOrder), len(workload.Status.Pods))
			for i, expectedPodId := range tt.expectedOrder {
				tassert.Equal(t, expectedPodId, workload.Status.Pods[i].PodId,
					"Pod at index %d should be %s", i, expectedPodId)
			}
		})
	}
}

// TestSortWorkloadPodsRayJob tests RayJob pod sorting: submitter first, then head, then worker by name
func TestSortWorkloadPodsRayJob(t *testing.T) {
	inputPods := []v1.WorkloadPod{
		{PodId: "rdma-bench-sleep-fwlts-rfz2g-1-worker-jddbx"},
		{PodId: "rdma-bench-sleep-fwlts-rfz2g-head-4cbqm"},
		{PodId: "rdma-bench-sleep-fwlts-zqndk"},
		{PodId: "rdma-bench-sleep-fwlts-rfz2g-2-worker-jddbx"},
	}
	expectedOrder := []string{
		"rdma-bench-sleep-fwlts-zqndk",                // submitter (no -head-/-worker-)
		"rdma-bench-sleep-fwlts-rfz2g-head-4cbqm",     // head
		"rdma-bench-sleep-fwlts-rfz2g-1-worker-jddbx", // worker 1
		"rdma-bench-sleep-fwlts-rfz2g-2-worker-jddbx", // worker 2
	}

	workload := &v1.Workload{
		Spec: v1.WorkloadSpec{
			GroupVersionKind: v1.GroupVersionKind{Kind: common.RayJobKind},
		},
		Status: v1.WorkloadStatus{
			Pods: inputPods,
		},
	}

	sortWorkloadPods(workload)

	tassert.Equal(t, len(expectedOrder), len(workload.Status.Pods))
	for i, expectedPodId := range expectedOrder {
		tassert.Equal(t, expectedPodId, workload.Status.Pods[i].PodId,
			"Pod at index %d should be %s", i, expectedPodId)
	}
}

// TestGetRayJobPodSlotKey tests RayJob pod slot key extraction
func TestGetRayJobPodSlotKey(t *testing.T) {
	tests := []struct {
		podId    string
		expected string
	}{
		{"rdma-bench-sleep-fwlts-zqndk", "submitter"},
		{"rdma-bench-sleep-fwlts-rfz2g-head-4cbqm", "head"},
		// Each worker pod owns its slot (full pod name) so concurrent replicas
		// in the same worker group are never collapsed into one slot.
		{"rdma-bench-sleep-fwlts-rfz2g-1-worker-jddbx", "rdma-bench-sleep-fwlts-rfz2g-1-worker-jddbx"},
		{"rdma-bench-sleep-fwlts-rfz2g-1-worker-abcde", "rdma-bench-sleep-fwlts-rfz2g-1-worker-abcde"},
		{"rdma-bench-sleep-fwlts-rfz2g-2-worker-jddbx", "rdma-bench-sleep-fwlts-rfz2g-2-worker-jddbx"},
	}
	for _, tt := range tests {
		t.Run(tt.podId, func(t *testing.T) {
			tassert.Equal(t, tt.expected, getRayJobPodSlotKey(tt.podId))
		})
	}
}

// TestPruneStaleRayJobPods tests removal of historical RayJob pods after restart
func TestPruneStaleRayJobPods(t *testing.T) {
	tests := []struct {
		name           string
		inputPods      []v1.WorkloadPod
		expectedPodIds []string
	}{
		{
			name: "keep running head over failed head",
			inputPods: []v1.WorkloadPod{
				{PodId: "job-rfz2g-head-old123", Phase: corev1.PodFailed, StartTime: "2025-01-01T00:00:00Z"},
				{PodId: "job-rfz2g-head-new456", Phase: corev1.PodRunning, StartTime: "2025-01-01T01:00:00Z"},
				{PodId: "job-submitter", Phase: corev1.PodRunning},
			},
			expectedPodIds: []string{"job-rfz2g-head-new456", "job-submitter"},
		},
		{
			// Worker pods each own their slot, so distinct workers are never
			// collapsed (even those sharing the same worker-group index).
			name: "keep all distinct workers, dedup only head",
			inputPods: []v1.WorkloadPod{
				{PodId: "job-rfz2g-head-old123", Phase: corev1.PodFailed, StartTime: "2025-01-01T00:00:00Z"},
				{PodId: "job-rfz2g-head-new456", Phase: corev1.PodRunning, StartTime: "2025-01-01T01:00:00Z"},
				{PodId: "job-rfz2g-1-worker-aaa", Phase: corev1.PodRunning},
				{PodId: "job-rfz2g-1-worker-bbb", Phase: corev1.PodRunning},
				{PodId: "job-rfz2g-2-worker-abc", Phase: corev1.PodRunning},
			},
			expectedPodIds: []string{
				"job-rfz2g-head-new456",
				"job-rfz2g-1-worker-aaa",
				"job-rfz2g-1-worker-bbb",
				"job-rfz2g-2-worker-abc",
			},
		},
		{
			// Regression for #590: a single worker group with many replicas
			// (all named "<cluster>-1-worker-<random>") must keep every pod.
			name: "multi-replica worker group keeps all replicas",
			inputPods: []v1.WorkloadPod{
				{PodId: "miles-9node-fn4lb-2964j-head-8dx95", Phase: corev1.PodRunning},
				{PodId: "miles-9node-fn4lb-2964j-1-worker-2kfz5", Phase: corev1.PodRunning},
				{PodId: "miles-9node-fn4lb-2964j-1-worker-2q4dq", Phase: corev1.PodRunning},
				{PodId: "miles-9node-fn4lb-2964j-1-worker-b9f2m", Phase: corev1.PodRunning},
				{PodId: "miles-9node-fn4lb-2964j-1-worker-bvspp", Phase: corev1.PodRunning},
				{PodId: "miles-9node-fn4lb-wd9sd", Phase: corev1.PodRunning},
			},
			expectedPodIds: []string{
				"miles-9node-fn4lb-2964j-head-8dx95",
				"miles-9node-fn4lb-2964j-1-worker-2kfz5",
				"miles-9node-fn4lb-2964j-1-worker-2q4dq",
				"miles-9node-fn4lb-2964j-1-worker-b9f2m",
				"miles-9node-fn4lb-2964j-1-worker-bvspp",
				"miles-9node-fn4lb-wd9sd",
			},
		},
		{
			name: "no pruning when each slot has one pod",
			inputPods: []v1.WorkloadPod{
				{PodId: "job-submitter", Phase: corev1.PodRunning},
				{PodId: "job-rfz2g-head-abc", Phase: corev1.PodRunning},
				{PodId: "job-rfz2g-1-worker-abc", Phase: corev1.PodRunning},
			},
			expectedPodIds: []string{"job-submitter", "job-rfz2g-head-abc", "job-rfz2g-1-worker-abc"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := pruneStaleRayJobPods(tt.inputPods)
			tassert.Equal(t, len(tt.expectedPodIds), len(result))
			resultIds := make(map[string]bool)
			for _, pod := range result {
				resultIds[pod.PodId] = true
			}
			for _, expectedId := range tt.expectedPodIds {
				tassert.True(t, resultIds[expectedId], "expected pod %s to be kept", expectedId)
			}
		})
	}
}
