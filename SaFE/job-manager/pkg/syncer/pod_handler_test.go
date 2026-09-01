/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package syncer

import (
	"context"
	"errors"
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
	"k8s.io/client-go/tools/cache"
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

// A runner set gets one ephemeral pod per CI job, so keeping a row per deleted
// pod grows without bound and every reconcile hydrates the whole list back from
// the DB. The job's detail lives in GitHub, so the row is dropped rather than
// kept as history.
func TestRemoveWorkloadPodDropsCICDEntry(t *testing.T) {
	for _, kind := range []string{common.CICDScaleRunnerSetKind, common.CICDEphemeralRunnerKind} {
		w := &v1.Workload{ObjectMeta: metav1.ObjectMeta{
			Name:        "w",
			Annotations: map[string]string{v1.WorkloadDispatchedAnnotation: "true"},
		}}
		w.Spec.GroupVersionKind = v1.GroupVersionKind{Version: "v1", Kind: kind}
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
		assert.Equal(t, len(got.Status.Pods), 1, kind)
		assert.Equal(t, got.Status.Pods[0].PodId, "p2", kind)
	}
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

// A container the kernel killed for memory is the case the pod-level reason
// cannot describe: status.reason carries the kill decisions made *above* the
// container (Evicted, Preempted, NodeLost) and is empty for an OOM, so consumers
// were left inferring it from exit code 137 -- which is any SIGKILL, and therefore
// also every eviction and every deliberate stop.
func TestBuildPodTerminatedInfoCarriesContainerReason(t *testing.T) {
	w := &v1.Workload{}
	pod := &corev1.Pod{Status: corev1.PodStatus{
		Phase: corev1.PodFailed,
		ContainerStatuses: []corev1.ContainerStatus{{
			Name: "main",
			State: corev1.ContainerState{
				Terminated: &corev1.ContainerStateTerminated{
					Reason:   "OOMKilled",
					ExitCode: 137,
					Message:  "",
				},
			},
		}},
	}}
	wp := &v1.WorkloadPod{}
	buildPodTerminatedInfo(context.Background(), nil, w, pod, wp, "main")
	assert.Equal(t, len(wp.Containers), 1)
	assert.Equal(t, wp.Containers[0].Reason, "OOMKilled")
	assert.Equal(t, wp.Containers[0].ExitCode, int32(137))
	// The pod-level field says nothing here, which is exactly why the container
	// one had to be carried.
	assert.Equal(t, wp.FailedMessage, "")
}

// The ordinary case still reports its reason, so a consumer can tell "the process
// exited badly" from "something killed it" without a second source.
func TestBuildPodTerminatedInfoCarriesOrdinaryReason(t *testing.T) {
	w := &v1.Workload{}
	pod := &corev1.Pod{Status: corev1.PodStatus{
		Phase: corev1.PodFailed,
		ContainerStatuses: []corev1.ContainerStatus{{
			Name: "main",
			State: corev1.ContainerState{
				Terminated: &corev1.ContainerStateTerminated{Reason: "Error", ExitCode: 1},
			},
		}},
	}}
	wp := &v1.WorkloadPod{}
	buildPodTerminatedInfo(context.Background(), nil, w, pod, wp, "main")
	assert.Equal(t, wp.Containers[0].Reason, "Error")
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

// podRecord builds a pod record started `age` ago, or with no start time at all
// when age is zero, which is what a pod that never ran leaves behind.
func podRecord(podId, node string, phase corev1.PodPhase, age time.Duration) v1.WorkloadPod {
	p := v1.WorkloadPod{PodId: podId, AdminNodeName: node, Phase: phase}
	if age > 0 {
		p.StartTime = timeutil.FormatRFC3339(time.Now().UTC().Add(-age))
	}
	return p
}

func dispatchedWorkloadWithPods(name string, dispatchedAgo time.Duration, pods ...v1.WorkloadPod) *v1.Workload {
	w := &v1.Workload{ObjectMeta: metav1.ObjectMeta{Name: name}}
	w.Spec.Workspace = "ws"
	// Matches newTestClientSets: the cache read refuses a workload from elsewhere.
	v1.SetLabel(w, v1.ClusterIdLabel, "c1")
	v1.SetAnnotation(w, v1.WorkloadDispatchedAnnotation,
		timeutil.FormatRFC3339(time.Now().UTC().Add(-dispatchedAgo)))
	w.Status.Pods = pods
	return w
}

// storedWorkload puts the workload behind a fake client and hands back the stored
// copy. The status patch is resourceVersion-guarded, so a hand-built object would
// conflict rather than write.
func storedWorkload(t *testing.T, w *v1.Workload) (ctrlclient.Client, *v1.Workload) {
	t.Helper()
	cl := ctrlfake.NewClientBuilder().
		WithScheme(syncerScheme(t)).
		WithObjects(w.DeepCopy()).
		WithStatusSubresource(&v1.Workload{}).
		Build()
	stored := &v1.Workload{}
	assert.NilError(t, cl.Get(context.Background(), ctrlclient.ObjectKey{Name: w.Name}, stored))
	return cl, stored
}

// A record left at Running because its delete event was lost keeps charging the
// workspace, so it is what the reconcile has to find.
func TestVanishedPodIdsReleasesRecordsWithoutAPod(t *testing.T) {
	w := dispatchedWorkloadWithPods("w", time.Hour,
		podRecord("gone", "n1", corev1.PodRunning, time.Hour),
		podRecord("alive", "n1", corev1.PodRunning, time.Hour),
	)

	gone := vanishedPodIds(w, map[string]struct{}{"alive": {}})

	assert.Equal(t, len(gone), 1)
	assert.Equal(t, gone[0], "gone")
}

func TestVanishedPodIdsLeavesRecordsItCannotJudge(t *testing.T) {
	w := dispatchedWorkloadWithPods("w", time.Hour,
		// Already terminal: costs nothing and keeps its final phase.
		podRecord("succeeded", "n1", corev1.PodSucceeded, time.Hour),
		podRecord("stopped", "n1", corev1.PodPhase(v1.WorkloadStopped), time.Hour),
		// Started inside the grace period: a pod this new may belong to another
		// process that has seen it while this cache has not.
		podRecord("fresh", "n1", corev1.PodRunning, time.Second),
	)

	assert.Equal(t, len(vanishedPodIds(w, map[string]struct{}{})), 0)
}

// A pod that never ran has no start time, so its age can only be inferred from
// the workload's dispatch.
func TestVanishedPodIdsJudgesStartlessRecordsByDispatch(t *testing.T) {
	pending := podRecord("pending", "", corev1.PodPending, 0)

	old := dispatchedWorkloadWithPods("w", time.Hour, pending)
	gone := vanishedPodIds(old, map[string]struct{}{})
	assert.Equal(t, len(gone), 1)
	assert.Equal(t, gone[0], "pending")

	// Dispatched moments ago: the pod may simply not be scheduled yet.
	fresh := dispatchedWorkloadWithPods("w", time.Second, pending)
	assert.Equal(t, len(vanishedPodIds(fresh, map[string]struct{}{})), 0)
}

// livePodsStub is the answer a patched livePodNames gives, plus how many times it
// was asked. Both answers may be changed between calls, which is how a test drives
// a failed comparison followed by a successful one.
type livePodsStub struct {
	live  map[string]struct{}
	ok    bool
	calls int
}

// patchLivePods replaces the informer read so the reconcile's own decisions can be
// tested without informer machinery. The read itself is covered by
// TestLivePodNames* below, which exercises the real one against a real cache.
func patchLivePods(patches *gomonkey.Patches, stub *livePodsStub) {
	patches.ApplyPrivateMethod(reflect.TypeOf(&SyncerReconciler{}), "livePodNames",
		func(_ *SyncerReconciler, _ context.Context, _ *ClusterClientSets,
			_ *v1.Workload) (map[string]struct{}, bool) {
			stub.calls++
			return stub.live, stub.ok
		})
}

// fakeSharedIndexInformer answers HasSynced and nothing else, which is all
// livePodNames asks of the informer.
type fakeSharedIndexInformer struct {
	cache.SharedIndexInformer
	synced bool
}

func (f *fakeSharedIndexInformer) HasSynced() bool { return f.synced }

type fakeGenericInformer struct {
	informer cache.SharedIndexInformer
	lister   cache.GenericLister
}

func (f fakeGenericInformer) Informer() cache.SharedIndexInformer { return f.informer }
func (f fakeGenericInformer) Lister() cache.GenericLister         { return f.lister }

// podInCache builds the unstructured pod a dynamic informer would hold.
func podInCache(namespace, name, workloadId string) *unstructured.Unstructured {
	pod := &unstructured.Unstructured{Object: map[string]any{"apiVersion": "v1", "kind": "Pod"}}
	pod.SetNamespace(namespace)
	pod.SetName(name)
	if workloadId != "" {
		pod.SetLabels(map[string]string{v1.WorkloadIdLabel: workloadId})
	}
	return pod
}

// clientSetsWithPodCache registers a pod informer whose cache holds the given pods.
func clientSetsWithPodCache(t *testing.T, synced bool, pods ...*unstructured.Unstructured) *ClusterClientSets {
	t.Helper()
	indexer := cache.NewIndexer(cache.MetaNamespaceKeyFunc,
		cache.Indexers{cache.NamespaceIndex: cache.MetaNamespaceIndexFunc})
	for _, p := range pods {
		assert.NilError(t, indexer.Add(p))
	}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	cs := newTestClientSets()
	assert.NilError(t, cs.resourceInformers.Add(podResourceGVK.String(), &resourceInformer{
		GenericInformer: fakeGenericInformer{
			informer: &fakeSharedIndexInformer{synced: synced},
			lister:   cache.NewGenericLister(indexer, schema.GroupResource{Resource: "pods"}),
		},
		context: ctx,
		cancel:  cancel,
	}))
	return cs
}

// A runner set owns its runners in the workspace and the ARC listener in the
// controller's namespace, and both have pod records. Scoping the cache read to the
// workspace reported the listener as vanished and released a record for a pod that
// was running.
func TestLivePodNamesSeesPodsInAnyNamespace(t *testing.T) {
	w := dispatchedWorkloadWithPods("rs", time.Hour)
	clientSets := clientSetsWithPodCache(t, true,
		podInCache("ws", "rs-runner-1", "rs"),
		podInCache("arc-systems", "rs-b9bccbd6-listener", "rs"),
		podInCache("ws", "other-master-0", "other"),
	)
	r := &SyncerReconciler{}

	live, ok := r.livePodNames(context.Background(), clientSets, w)

	assert.Equal(t, ok, true)
	assert.Equal(t, len(live), 2)
	_, hasRunner := live["rs-runner-1"]
	assert.Equal(t, hasRunner, true)
	_, hasListener := live["rs-b9bccbd6-listener"]
	assert.Equal(t, hasListener, true, "the listener runs outside the workspace and still counts")
	_, hasOther := live["other-master-0"]
	assert.Equal(t, hasOther, false, "another workload's pod is not this one's")
}

// A Monarch mesh pod carries neither SaFE label -- MonarchMesh exposes a bare PodSpec
// with no metadata to stamp -- while its record is attributed through the mesh object.
// Deciding existence by label alone would report a running mesh as vanished and
// release every one of its records.
func TestLivePodNamesResolvesMonarchMeshPods(t *testing.T) {
	w := dispatchedWorkloadWithPods("mj", time.Hour,
		podRecord("mj-client-0", "n1", corev1.PodRunning, time.Hour),
		podRecord("mj-mesh-0-worker-0", "n1", corev1.PodRunning, time.Hour))
	w.Spec.GroupVersionKind = v1.GroupVersionKind{Version: "v1", Kind: common.MonarchJob}

	meshPod := podInCache("ws", "mj-mesh-0-worker-0", "")
	meshPod.SetLabels(map[string]string{monarchMeshLabel: "mj-mesh-0"})
	otherMeshPod := podInCache("ws", "other-mesh-0-worker-0", "")
	otherMeshPod.SetLabels(map[string]string{monarchMeshLabel: "other-mesh-0"})
	clientSets := clientSetsWithPodCache(t, true,
		podInCache("ws", "mj-client-0", "mj"), meshPod, otherMeshPod)

	patches := gomonkey.NewPatches()
	defer patches.Reset()
	meshLookups := 0
	patches.ApplyPrivateMethod(reflect.TypeOf(&SyncerReconciler{}), "getMonarchMesh",
		func(_ *SyncerReconciler, _ context.Context, _ *ClusterClientSets,
			name, namespace string) (*unstructured.Unstructured, error) {
			meshLookups++
			owner := "mj"
			if name != "mj-mesh-0" {
				owner = "another-monarch-job"
			}
			mesh := podInCache(namespace, name, owner)
			return mesh, nil
		})

	r := &SyncerReconciler{}
	live, ok := r.livePodNames(context.Background(), clientSets, w)

	assert.Equal(t, ok, true)
	assert.Equal(t, len(live), 2)
	_, hasClient := live["mj-client-0"]
	assert.Equal(t, hasClient, true, "the client pod is labelled and found directly")
	_, hasMesh := live["mj-mesh-0-worker-0"]
	assert.Equal(t, hasMesh, true, "the mesh pod is found through its mesh object")
	_, hasOther := live["other-mesh-0-worker-0"]
	assert.Equal(t, hasOther, false, "another job's mesh is not this one's")
	assert.Equal(t, meshLookups, 1,
		"only a mesh named by a record is resolved, once, not once per pod")
}

// Mesh teardown is routine, and an unrelated job's is not this workload's problem:
// no record names those pods, so the mesh is never read and cannot make the answer
// unknown.
func TestLivePodNamesIgnoresAStrangerMeshItCannotRead(t *testing.T) {
	w := dispatchedWorkloadWithPods("mj", time.Hour,
		podRecord("mj-client-0", "n1", corev1.PodRunning, time.Hour))
	w.Spec.GroupVersionKind = v1.GroupVersionKind{Version: "v1", Kind: common.MonarchJob}

	strangerPod := podInCache("ws", "other-mesh-0-worker-0", "")
	strangerPod.SetLabels(map[string]string{monarchMeshLabel: "other-mesh-0"})
	clientSets := clientSetsWithPodCache(t, true,
		podInCache("ws", "mj-client-0", "mj"), strangerPod)

	patches := gomonkey.NewPatches()
	defer patches.Reset()
	meshLookups := 0
	patches.ApplyPrivateMethod(reflect.TypeOf(&SyncerReconciler{}), "getMonarchMesh",
		func(_ *SyncerReconciler, _ context.Context, _ *ClusterClientSets,
			_, _ string) (*unstructured.Unstructured, error) {
			meshLookups++
			return nil, errors.New("mesh being torn down")
		})

	r := &SyncerReconciler{}
	live, ok := r.livePodNames(context.Background(), clientSets, w)

	assert.Equal(t, ok, true)
	assert.Equal(t, meshLookups, 0, "a pod no record names is never resolved")
	_, hasClient := live["mj-client-0"]
	assert.Equal(t, hasClient, true)
}

// A mesh that cannot be read may be this workload's own, so the answer is unknown
// rather than "those pods are gone".
func TestLivePodNamesRefusesWhenAMeshCannotBeRead(t *testing.T) {
	w := dispatchedWorkloadWithPods("mj", time.Hour,
		podRecord("mj-mesh-0-worker-0", "n1", corev1.PodRunning, time.Hour))
	w.Spec.GroupVersionKind = v1.GroupVersionKind{Version: "v1", Kind: common.MonarchJob}
	meshPod := podInCache("ws", "mj-mesh-0-worker-0", "")
	meshPod.SetLabels(map[string]string{monarchMeshLabel: "mj-mesh-0"})
	clientSets := clientSetsWithPodCache(t, true, meshPod)

	patches := gomonkey.NewPatches()
	defer patches.Reset()
	patches.ApplyPrivateMethod(reflect.TypeOf(&SyncerReconciler{}), "getMonarchMesh",
		func(_ *SyncerReconciler, _ context.Context, _ *ClusterClientSets,
			_, _ string) (*unstructured.Unstructured, error) {
			return nil, errors.New("mesh unreachable")
		})

	r := &SyncerReconciler{}
	live, ok := r.livePodNames(context.Background(), clientSets, w)

	assert.Equal(t, ok, false)
	assert.Equal(t, len(live), 0)
}

// "Not found out" and "none exist" are the same bytes out of an unsynced cache, so
// the two must not be the same answer.
func TestLivePodNamesReportsWhatItCouldNotEstablish(t *testing.T) {
	w := dispatchedWorkloadWithPods("rs", time.Hour)
	r := &SyncerReconciler{}

	// A cache that holds the pods but has not synced: acting on it would stop
	// every record of every workload.
	unsynced := clientSetsWithPodCache(t, false, podInCache("ws", "rs-runner-1", "rs"))
	live, ok := r.livePodNames(context.Background(), unsynced, w)
	assert.Equal(t, ok, false)
	assert.Equal(t, len(live), 0)

	// No pod informer on this cluster at all.
	live, ok = r.livePodNames(context.Background(), newTestClientSets(), w)
	assert.Equal(t, ok, false)
	assert.Equal(t, len(live), 0)

	live, ok = r.livePodNames(context.Background(), nil, w)
	assert.Equal(t, ok, false)
	assert.Equal(t, len(live), 0)
}

func TestReconcileVanishedPodsStopsThemAndRunsOnce(t *testing.T) {
	cl, w := storedWorkload(t, dispatchedWorkloadWithPods("w", time.Hour,
		podRecord("gone1", "n1", corev1.PodRunning, time.Hour),
		podRecord("alive", "n1", corev1.PodRunning, time.Hour),
		podRecord("gone2", "n2", corev1.PodRunning, time.Hour),
	))
	r := &SyncerReconciler{Client: cl}

	patches := gomonkey.NewPatches()
	defer patches.Reset()
	stub := &livePodsStub{live: map[string]struct{}{"alive": {}}, ok: true}
	patchLivePods(patches, stub)

	assert.NilError(t, r.reconcileVanishedPods(context.Background(), monkeyClientSets(), w, &resourceMessage{action: ResourceAdd}))

	got := &v1.Workload{}
	assert.NilError(t, cl.Get(context.Background(), ctrlclient.ObjectKey{Name: "w"}, got))
	// History is kept for this kind, so the records stay and stop counting by phase
	// rather than by being removed.
	assert.Equal(t, len(got.Status.Pods), 3)
	byId := map[string]corev1.PodPhase{}
	for _, p := range got.Status.Pods {
		byId[p.PodId] = p.Phase
	}
	assert.Equal(t, byId["gone1"], corev1.PodPhase(v1.WorkloadStopped))
	assert.Equal(t, byId["gone2"], corev1.PodPhase(v1.WorkloadStopped))
	assert.Equal(t, byId["alive"], corev1.PodRunning)

	// The drift this repairs is opened by a process ending, so a second event for
	// the same workload must not pay for the comparison again.
	assert.NilError(t, r.reconcileVanishedPods(context.Background(), monkeyClientSets(), w, &resourceMessage{action: ResourceAdd}))
	assert.Equal(t, stub.calls, 1)
}

// The re-read yields no workload when it was deleted between the event and this
// pass, and none either when the Get fails. Both reach the same exit, which must not
// dereference it -- a panic there is not contained and ends the process.
func TestReconcileVanishedPodsSurvivesAVanishedWorkload(t *testing.T) {
	w := dispatchedWorkloadWithPods("w", time.Hour,
		podRecord("gone", "n1", corev1.PodRunning, time.Hour))
	// The client holds no workload, so the re-read is a not-found the Get swallows.
	cl := ctrlfake.NewClientBuilder().WithScheme(syncerScheme(t)).
		WithStatusSubresource(&v1.Workload{}).Build()
	r := &SyncerReconciler{Client: cl}

	assert.NilError(t, r.reconcileVanishedPods(context.Background(), monkeyClientSets(), w,
		&resourceMessage{action: ResourceAdd}))

	// Re-armed rather than remembered: nothing was compared, so a later event for a
	// workload that comes back still gets its pass.
	_, remembered := r.vanishedPodsChecked.Load("w")
	assert.Equal(t, remembered, false)
}

func TestReconcileVanishedPodsDropsCICDRecords(t *testing.T) {
	rs := dispatchedWorkloadWithPods("rs", time.Hour,
		podRecord("gone", "n1", corev1.PodRunning, time.Hour),
		podRecord("alive", "n1", corev1.PodRunning, time.Hour),
	)
	rs.Spec.GroupVersionKind = v1.GroupVersionKind{Version: "v1", Kind: common.CICDScaleRunnerSetKind}
	cl, w := storedWorkload(t, rs)
	r := &SyncerReconciler{Client: cl}

	patches := gomonkey.NewPatches()
	defer patches.Reset()
	patchLivePods(patches, &livePodsStub{live: map[string]struct{}{"alive": {}}, ok: true})

	assert.NilError(t, r.reconcileVanishedPods(context.Background(), monkeyClientSets(), w, &resourceMessage{action: ResourceAdd}))

	got := &v1.Workload{}
	assert.NilError(t, cl.Get(context.Background(), ctrlclient.ObjectKey{Name: "rs"}, got))
	// A CI runner keeps no history, so the record goes rather than being kept as a
	// stopped row -- the same policy the delete-event path applies.
	assert.Equal(t, len(got.Status.Pods), 1)
	assert.Equal(t, got.Status.Pods[0].PodId, "alive")
}

// Records left over from a round that is being re-scheduled must not be judged:
// reSchedule drops the dispatch annotation, and without it there is no clock to
// measure a record against.
func TestReconcileVanishedPodsSkipsUndispatchedWorkloads(t *testing.T) {
	undispatched := dispatchedWorkloadWithPods("w", time.Hour,
		podRecord("gone", "n1", corev1.PodRunning, time.Hour),
	)
	v1.RemoveAnnotation(undispatched, v1.WorkloadDispatchedAnnotation)
	cl, w := storedWorkload(t, undispatched)
	r := &SyncerReconciler{Client: cl}

	patches := gomonkey.NewPatches()
	defer patches.Reset()
	stub := &livePodsStub{live: map[string]struct{}{}, ok: true}
	patchLivePods(patches, stub)

	assert.NilError(t, r.reconcileVanishedPods(context.Background(), monkeyClientSets(), w, &resourceMessage{action: ResourceAdd}))

	assert.Equal(t, stub.calls, 0, "nothing is compared before the workload is dispatched")
	got := &v1.Workload{}
	assert.NilError(t, cl.Get(context.Background(), ctrlclient.ObjectKey{Name: "w"}, got))
	assert.Equal(t, len(got.Status.Pods), 1)
	assert.Equal(t, got.Status.Pods[0].Phase, corev1.PodRunning)
}

// The event that triggers this may have written the status through a deep copy,
// leaving the caller's pointer a version behind. The pass re-reads, so the release
// still lands instead of writing the database from a stale list and then having the
// guarded etcd patch rejected.
func TestReconcileVanishedPodsWorksFromAStaleCaller(t *testing.T) {
	cl, w := storedWorkload(t, dispatchedWorkloadWithPods("w", time.Hour,
		podRecord("gone", "n1", corev1.PodRunning, time.Hour),
	))
	// Something else writes the workload, so the stored version moves past this copy.
	bumped := w.DeepCopy()
	v1.SetLabel(bumped, "test/bumped", v1.TrueStr)
	assert.NilError(t, cl.Update(context.Background(), bumped))

	r := &SyncerReconciler{Client: cl}
	patches := gomonkey.NewPatches()
	defer patches.Reset()
	stub := &livePodsStub{live: map[string]struct{}{}, ok: true}
	patchLivePods(patches, stub)

	assert.NilError(t, r.reconcileVanishedPods(context.Background(), monkeyClientSets(), w,
		&resourceMessage{action: ResourceAdd}))

	got := &v1.Workload{}
	assert.NilError(t, cl.Get(context.Background(), ctrlclient.ObjectKey{Name: "w"}, got))
	assert.Equal(t, got.Status.Pods[0].Phase, corev1.PodPhase(v1.WorkloadStopped))
}

// A write that fails for any other reason must not spend the workload's one pass.
func TestReconcileVanishedPodsReleasesTheEntryOnAWriteFailure(t *testing.T) {
	cl, w := storedWorkload(t, dispatchedWorkloadWithPods("w", time.Hour,
		podRecord("gone", "n1", corev1.PodRunning, time.Hour),
	))
	r := &SyncerReconciler{Client: cl}

	patches := gomonkey.NewPatches()
	defer patches.Reset()
	patchLivePods(patches, &livePodsStub{live: map[string]struct{}{}, ok: true})
	patches.ApplyPrivateMethod(reflect.TypeOf(&SyncerReconciler{}), "patchWorkloadPodStatus",
		func(_ *SyncerReconciler, _ context.Context, _ *v1.Workload, _ map[string]any) error {
			return apierrors.NewConflict(schema.GroupResource{Resource: "workloads"}, "w", errors.New("stale"))
		})

	err := r.reconcileVanishedPods(context.Background(), monkeyClientSets(), w,
		&resourceMessage{action: ResourceAdd})

	assert.Assert(t, err != nil)
	assert.Equal(t, apierrors.IsConflict(err), true)
	_, remembered := r.vanishedPodsChecked.Load("w")
	assert.Equal(t, remembered, false, "a failed write must not spend the one pass")

	got := &v1.Workload{}
	assert.NilError(t, cl.Get(context.Background(), ctrlclient.ObjectKey{Name: "w"}, got))
	assert.Equal(t, got.Status.Pods[0].Phase, corev1.PodRunning, "nothing is half-written")
}

func TestIndexOfPod(t *testing.T) {
	pods := []v1.WorkloadPod{{PodId: "a"}, {PodId: "b"}}
	assert.Equal(t, indexOfPod(pods, "a"), 0)
	assert.Equal(t, indexOfPod(pods, "b"), 1)
	assert.Equal(t, indexOfPod(pods, "missing"), -1)
	assert.Equal(t, indexOfPod(nil, "a"), -1)
}

// Teardown deletes pods on purpose, so it is not evidence that records should be
// released -- and it must not spend the one pass the workload gets, since a
// re-schedule tears the old objects down the same way before dispatching again.
func TestReconcileVanishedPodsSkipsTeardown(t *testing.T) {
	cl, w := storedWorkload(t, dispatchedWorkloadWithPods("w", time.Hour,
		podRecord("gone", "n1", corev1.PodRunning, time.Hour),
	))
	r := &SyncerReconciler{Client: cl}

	patches := gomonkey.NewPatches()
	defer patches.Reset()
	stub := &livePodsStub{live: map[string]struct{}{}, ok: true}
	patchLivePods(patches, stub)

	for _, action := range []string{ResourceDel, ResourceDeleting} {
		assert.NilError(t, r.reconcileVanishedPods(context.Background(), monkeyClientSets(), w,
			&resourceMessage{action: action}))
		assert.Equal(t, stub.calls, 0, action)
		_, remembered := r.vanishedPodsChecked.Load("w")
		assert.Equal(t, remembered, false, action)
	}

	got := &v1.Workload{}
	assert.NilError(t, cl.Get(context.Background(), ctrlclient.ObjectKey{Name: "w"}, got))
	assert.Equal(t, got.Status.Pods[0].Phase, corev1.PodRunning)
}

// An ended workload costs no quota, so there is nothing to reconcile -- and
// forgetting it is what keeps the memo to the live set instead of growing by one
// entry per CI job for the life of the process.
func TestReconcileVanishedPodsForgetsEndedWorkloads(t *testing.T) {
	cl, w := storedWorkload(t, dispatchedWorkloadWithPods("w", time.Hour,
		podRecord("gone", "n1", corev1.PodRunning, time.Hour),
	))
	r := &SyncerReconciler{Client: cl}

	patches := gomonkey.NewPatches()
	defer patches.Reset()
	stub := &livePodsStub{live: map[string]struct{}{}, ok: true}
	patchLivePods(patches, stub)

	// First pass while it runs: the record is released and the workload remembered.
	assert.NilError(t, r.reconcileVanishedPods(context.Background(), monkeyClientSets(), w, &resourceMessage{action: ResourceAdd}))
	assert.Equal(t, stub.calls, 1)
	_, remembered := r.vanishedPodsChecked.Load("w")
	assert.Equal(t, remembered, true)

	w.Status.Phase = v1.WorkloadSucceeded
	assert.NilError(t, r.reconcileVanishedPods(context.Background(), monkeyClientSets(), w, &resourceMessage{action: ResourceAdd}))
	assert.Equal(t, stub.calls, 1, "an ended workload is not compared again")
	_, remembered = r.vanishedPodsChecked.Load("w")
	assert.Equal(t, remembered, false, "the entry is dropped once the workload ends")
}

// The one way this could destroy live state: an unsynced cache answers "no pods"
// and every record is stopped. Not knowing has to mean not acting, and the next
// event has to try again.
func TestReconcileVanishedPodsDoesNothingWhenNothingIsKnown(t *testing.T) {
	cl, w := storedWorkload(t, dispatchedWorkloadWithPods("w", time.Hour,
		podRecord("p1", "n1", corev1.PodRunning, time.Hour),
	))
	r := &SyncerReconciler{Client: cl}

	patches := gomonkey.NewPatches()
	defer patches.Reset()
	stub := &livePodsStub{ok: false}
	patchLivePods(patches, stub)

	assert.NilError(t, r.reconcileVanishedPods(context.Background(), monkeyClientSets(), w, &resourceMessage{action: ResourceAdd}))

	got := &v1.Workload{}
	assert.NilError(t, cl.Get(context.Background(), ctrlclient.ObjectKey{Name: "w"}, got))
	assert.Equal(t, len(got.Status.Pods), 1)
	assert.Equal(t, got.Status.Pods[0].Phase, corev1.PodRunning)
	_, remembered := r.vanishedPodsChecked.Load("w")
	assert.Equal(t, remembered, false, "a failed comparison must not spend the one pass")

	// So the next event tries again, and this time it establishes the answer.
	stub.live, stub.ok = map[string]struct{}{}, true
	assert.NilError(t, r.reconcileVanishedPods(context.Background(), monkeyClientSets(), w, &resourceMessage{action: ResourceAdd}))
	assert.Equal(t, stub.calls, 2)
	assert.NilError(t, cl.Get(context.Background(), ctrlclient.ObjectKey{Name: "w"}, got))
	assert.Equal(t, got.Status.Pods[0].Phase, corev1.PodPhase(v1.WorkloadStopped))
}
