/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package syncer

import (
	"context"
	"errors"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/agiledragon/gomonkey/v2"
	"gotest.tools/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/tools/cache"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	ctrlfake "sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	commonclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/k8sclient"
	commonworkload "github.com/AMD-AIG-AIMA/SAFE/common/pkg/workload"
	jobutils "github.com/AMD-AIG-AIMA/SAFE/job-manager/pkg/utils"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/timeutil"
)

// What a sandbox-style object reports when its pod ended Succeeded.
const objectSucceededMessage = "Pod phase is Succeeded; the container exited 0"

func evictionCondition() corev1.PodCondition {
	return corev1.PodCondition{
		Type:               corev1.DisruptionTarget,
		Status:             corev1.ConditionTrue,
		Reason:             "EvictionByEvictionAPI",
		Message:            "Eviction API: evicting",
		LastTransitionTime: disruptionTime,
	}
}

func wantEviction() string {
	return "DisruptionTarget: EvictionByEvictionAPI (" +
		timeutil.FormatRFC3339(disruptionTime.Time) + "): Eviction API: evicting"
}

// syncedInformer is a pod informer whose cache holds exactly the given pods,
// and reports it has synced unless unsynced is set.
type syncedInformer struct {
	indexer  cache.Indexer
	unsynced bool
}

type syncedSharedInformer struct {
	cache.SharedIndexInformer
	synced bool
}

func (s syncedSharedInformer) HasSynced() bool { return s.synced }

func (s syncedInformer) Informer() cache.SharedIndexInformer {
	return syncedSharedInformer{synced: !s.unsynced}
}
func (s syncedInformer) Lister() cache.GenericLister {
	return cache.NewGenericLister(s.indexer, schema.GroupResource{Resource: "pods"})
}

func cachedPod(t *testing.T, name string, phase corev1.PodPhase, conditions ...corev1.PodCondition) *unstructured.Unstructured {
	t.Helper()
	return cachedPodWith(t, name, phase, nil, conditions...)
}

// cachedPodWith is cachedPod with the given container statuses.
func cachedPodWith(t *testing.T, name string, phase corev1.PodPhase, containers []corev1.ContainerStatus,
	conditions ...corev1.PodCondition) *unstructured.Unstructured {
	t.Helper()
	pod := &corev1.Pod{
		TypeMeta: metav1.TypeMeta{APIVersion: "v1", Kind: "Pod"},
		ObjectMeta: metav1.ObjectMeta{
			Name: name, Namespace: "ns",
			Labels: map[string]string{v1.WorkloadIdLabel: "w", v1.WorkloadDispatchCntLabel: "1"},
		},
		Status: corev1.PodStatus{Phase: phase, Conditions: conditions, ContainerStatuses: containers},
	}
	m, err := runtime.DefaultUnstructuredConverter.ToUnstructured(pod)
	assert.NilError(t, err)
	return &unstructured.Unstructured{Object: m}
}

// objectSucceededRun describes one run of the job status path for an object
// that reports K8sSucceeded.
type objectSucceededRun struct {
	replicas int
	records  []v1.WorkloadPod
	cached   []*unstructured.Unstructured
	// The workload's dispatch count, 1 when unset, and the one the event carries,
	// the workload's when unset.
	dispatch, eventDispatch int
	maxRetry                int
	// The pod cache cannot answer: no informer, or one that has not synced.
	noInformer, unsynced bool
	// How long this process has waited to tell whether a disruption caused the
	// success, when it has.
	waited *time.Duration
}

// lastObjectSucceededErr is what updateAdminWorkloadByJob returned in the last
// runObjectSucceededWith.
var lastObjectSucceededErr error

// runObjectSucceeded runs the job status path for an object that reports
// K8sSucceeded, with the pod informer cache holding cached, and returns the
// stored workload.
func runObjectSucceeded(t *testing.T, replicas int, records []v1.WorkloadPod, cached ...*unstructured.Unstructured) *v1.Workload {
	t.Helper()
	return runObjectSucceededWith(t, objectSucceededRun{replicas: replicas, records: records, cached: cached})
}

func runObjectSucceededWith(t *testing.T, run objectSucceededRun) *v1.Workload {
	t.Helper()
	if run.dispatch == 0 {
		run.dispatch = 1
	}
	if run.eventDispatch == 0 {
		run.eventDispatch = run.dispatch
	}
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
			return &jobutils.K8sObjectStatus{Phase: string(v1.K8sSucceeded), Message: objectSucceededMessage}, nil
		})
	indexer := cache.NewIndexer(cache.MetaNamespaceKeyFunc, cache.Indexers{})
	for _, obj := range run.cached {
		assert.NilError(t, indexer.Add(obj))
	}
	patches.ApplyMethod(reflect.TypeOf(&ClusterClientSets{}), "GetResourceInformer",
		func(_ *ClusterClientSets, _ context.Context, gvk schema.GroupVersionKind) (informers.GenericInformer, error) {
			assert.Equal(t, gvk, podResourceGVK)
			if run.noInformer {
				return nil, errors.New("pod informer not started")
			}
			return syncedInformer{indexer: indexer, unsynced: run.unsynced}, nil
		})

	w := &v1.Workload{ObjectMeta: metav1.ObjectMeta{
		Name: "w",
		Labels: map[string]string{v1.ClusterIdLabel: "c",
			v1.WorkloadDispatchCntLabel: strconv.Itoa(run.dispatch)},
		Annotations: map[string]string{v1.WorkloadDispatchedAnnotation: timeutil.FormatRFC3339(dispatchedAt)},
	}}
	w.Status.Phase = v1.WorkloadRunning
	w.Spec.Resources = []v1.WorkloadResource{{Replica: run.replicas}}
	w.Spec.MaxRetry = run.maxRetry
	cl := ctrlfake.NewClientBuilder().
		WithScheme(syncerScheme(t)).
		WithObjects(w).
		WithStatusSubresource(&v1.Workload{}).
		Build()
	r := &SyncerReconciler{Client: cl}
	if run.waited != nil {
		r.successVerdictWaits.Store("w", successVerdictWait{dispatch: run.dispatch, since: time.Now().Add(-*run.waited)})
	}
	hydrated := &v1.Workload{}
	assert.NilError(t, cl.Get(context.Background(), ctrlclient.ObjectKey{Name: "w"}, hydrated))
	hydrated.Status.Pods = run.records

	cs := monkeyClientSets()
	cs.name = "c"
	_, err := r.updateAdminWorkloadByJob(context.Background(), cs, hydrated,
		&resourceMessage{name: "o", namespace: "ns", dispatchCount: run.eventDispatch,
			gvk: schema.GroupVersionKind{Kind: "Sandbox"}})
	lastObjectSucceededErr = err
	if !errors.Is(err, errSuccessVerdictPending) {
		assert.NilError(t, err)
	}
	_, waiting := r.successVerdictWaits.Load("w")
	assert.Equal(t, waiting, errors.Is(err, errSuccessVerdictPending), "a wait is kept only while pending")
	fresh := &v1.Workload{}
	assert.NilError(t, cl.Get(context.Background(), ctrlclient.ObjectKey{Name: "w"}, fresh))
	return fresh
}

func conditionOf(w *v1.Workload, condType v1.WorkloadConditionType) *metav1.Condition {
	for i := range w.Status.Conditions {
		if w.Status.Conditions[i].Type == string(condType) {
			return &w.Status.Conditions[i]
		}
	}
	return nil
}

// The current dispatch's time, and a start time after it as a pod record holds
// it: local wall time with no zone.
var (
	dispatchedAt      = time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)
	recordStartTime   = timeutil.FormatRFC3339(dispatchedAt.Add(time.Hour).Local())
	earlierStartTime  = timeutil.FormatRFC3339(dispatchedAt.Add(-time.Hour).Local())
	wantFailedMessage = "pod p1 was taken away by a disruption, " + wantEviction() +
		"; the object reported success: " + objectSucceededMessage
)

// assertDisruptedFailed asserts the workload failed, its failure naming the
// disruption first and then what the object reported.
func assertDisruptedFailed(t *testing.T, w *v1.Workload) {
	t.Helper()
	assert.Equal(t, w.Status.Phase, v1.WorkloadFailed)
	assert.Assert(t, conditionOf(w, v1.K8sSucceeded) == nil)
	failed := conditionOf(w, v1.K8sFailed)
	assert.Assert(t, failed != nil)
	assert.Equal(t, failed.Message, wantFailedMessage)
	assert.Assert(t, strings.Contains(w.Status.Message, wantFailedMessage), w.Status.Message)
}

func evictedRecord() v1.WorkloadPod {
	return v1.WorkloadPod{PodId: "p1", Phase: corev1.PodPhase(v1.WorkloadStopped), StartTime: recordStartTime,
		FailedMessage: wantEviction()}
}

func assertSucceeded(t *testing.T, w *v1.Workload) {
	t.Helper()
	assert.NilError(t, lastObjectSucceededErr)
	assert.Equal(t, w.Status.Phase, v1.WorkloadSucceeded)
	assert.Assert(t, conditionOf(w, v1.K8sFailed) == nil)
	assert.Equal(t, conditionOf(w, v1.K8sSucceeded).Message, objectSucceededMessage)
}

// assertPending asserts the object's success was left unapplied, to be read
// again: the workload has not ended, and holds no condition of the success.
func assertPending(t *testing.T, w *v1.Workload) {
	t.Helper()
	assert.Assert(t, errors.Is(lastObjectSucceededErr, errSuccessVerdictPending), lastObjectSucceededErr)
	assert.Equal(t, w.Status.Phase, v1.WorkloadRunning)
	assert.Assert(t, conditionOf(w, v1.K8sSucceeded) == nil)
	assert.Assert(t, conditionOf(w, v1.K8sFailed) == nil)
}

// The Eviction API took the pod; its containers exited 0 on SIGTERM, so the pod
// and then the object report Succeeded. The delete path has recorded the pod as
// Stopped with the condition, and the cache still holds the Succeeded pod.
func TestDisruptedSuccessFromInformerCache(t *testing.T) {
	assertDisruptedFailed(t, runObjectSucceeded(t, 1, []v1.WorkloadPod{evictedRecord()},
		cachedPod(t, "p1", corev1.PodSucceeded, evictionCondition())))
	// The object can report success before the pod's delete event is recorded.
	assertDisruptedFailed(t, runObjectSucceeded(t, 1, []v1.WorkloadPod{{PodId: "p1", Phase: corev1.PodRunning}},
		cachedPod(t, "p1", corev1.PodSucceeded, evictionCondition())))
}

// The pod has left the cache; its record carries the condition.
func TestDisruptedSuccessFromPodRecord(t *testing.T) {
	assertDisruptedFailed(t, runObjectSucceeded(t, 1, []v1.WorkloadPod{evictedRecord()}))
	// A record that has not ended is a pod whose delete is not recorded yet: the
	// success waits for it.
	running := evictedRecord()
	running.Phase = corev1.PodRunning
	assertPending(t, runObjectSucceeded(t, 1, []v1.WorkloadPod{running}))
	// As it does when nothing of the pod is recorded or cached yet.
	assertPending(t, runObjectSucceeded(t, 1, nil))
}

// A pod that ended Succeeded with no True DisruptionTarget is a clean success.
func TestCleanSuccessStaysSucceeded(t *testing.T) {
	notTrue := evictionCondition()
	notTrue.Status = corev1.ConditionFalse
	assertSucceeded(t, runObjectSucceeded(t, 1, []v1.WorkloadPod{{PodId: "p1", Phase: corev1.PodSucceeded}},
		cachedPod(t, "p1", corev1.PodSucceeded, notTrue)))
	assertSucceeded(t, runObjectSucceeded(t, 1, []v1.WorkloadPod{{PodId: "p1", Phase: corev1.PodSucceeded,
		StartTime: recordStartTime}}))
	assertSucceeded(t, runObjectSucceeded(t, 1, []v1.WorkloadPod{{PodId: "p1",
		Phase: corev1.PodPhase(v1.WorkloadStopped), StartTime: recordStartTime}}))
}

// A later pod of the same name that completed cleanly is what the object
// reports, though the earlier one's record carries the condition.
func TestSameNameReplacementSuccessStands(t *testing.T) {
	assertSucceeded(t, runObjectSucceeded(t, 1, []v1.WorkloadPod{evictedRecord()},
		cachedPod(t, "p1", corev1.PodSucceeded)))
	// Nor is the record read while the cache holds a later pod still running:
	// the success waits for the cache.
	assertPending(t, runObjectSucceeded(t, 1, []v1.WorkloadPod{evictedRecord()},
		cachedPod(t, "p1", corev1.PodRunning)))
}

// A disrupted pod that ended Failed is not what the object counted as success:
// a Job replaced it, and the replacement may not be in the cache yet.
func TestDisruptedPodThatFailedIsNotTheSuccess(t *testing.T) {
	failed := evictedRecord()
	failed.Phase = corev1.PodFailed
	assertSucceeded(t, runObjectSucceeded(t, 1, []v1.WorkloadPod{failed},
		cachedPod(t, "p1", corev1.PodFailed, evictionCondition())))
	// Nor when the pod has left the cache and only its record says so.
	assertSucceeded(t, runObjectSucceeded(t, 1, []v1.WorkloadPod{failed}))
	// A record that ended Succeeded with the condition is the candidate.
	succeeded := evictedRecord()
	succeeded.Phase = corev1.PodSucceeded
	assertDisruptedFailed(t, runObjectSucceeded(t, 1, []v1.WorkloadPod{succeeded}))
}

// With more than one pod the object's success cannot be tied to the disrupted
// one: neither a second pod name nor more than one replica is read.
func TestMultiPodWorkloadIsNotRead(t *testing.T) {
	assertSucceeded(t, runObjectSucceeded(t, 1, []v1.WorkloadPod{evictedRecord()},
		cachedPod(t, "p1", corev1.PodSucceeded, evictionCondition()),
		cachedPod(t, "p2", corev1.PodRunning)))
	assertSucceeded(t, runObjectSucceeded(t, 1, []v1.WorkloadPod{evictedRecord(), {PodId: "p2", Phase: corev1.PodRunning}}))
	assertSucceeded(t, runObjectSucceeded(t, 2, []v1.WorkloadPod{evictedRecord()},
		cachedPod(t, "p1", corev1.PodSucceeded, evictionCondition())))
}

// Once the workload has ended, a later pod of the same name (a controller
// recreating it before the object is deleted) is recorded like any other: its
// node and devices are in use, whatever the record it replaces said.
func TestSameNamePodAfterWorkloadEndIsRecorded(t *testing.T) {
	record := evictedRecord()
	replacement := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "p1", Namespace: "ns"},
		Status:     corev1.PodStatus{Phase: corev1.PodPending},
	}
	r := &SyncerReconciler{}
	for _, phase := range []v1.WorkloadPhase{v1.WorkloadFailed, v1.WorkloadStopped, v1.WorkloadRunning} {
		w := &v1.Workload{ObjectMeta: metav1.ObjectMeta{Name: "w"}}
		w.Status.Phase = phase
		w.Status.Pods = []v1.WorkloadPod{record}
		_, _, updated := r.updateWorkloadNodeAndPods(context.Background(), monkeyClientSets(), w, replacement, nil)
		assert.Equal(t, updated, true, phase)
		assert.Equal(t, w.Status.Pods[0].Phase, corev1.PodPending, phase)
	}
}

// A pod cache that cannot answer is not a pod that left it: the record is not
// read, and the success waits for the cache. After a restart the object's
// informer can replay its success before the pod informer has synced, while
// the pod, a later one of the same name, is still in the cluster.
func TestUnansweredPodCacheWaits(t *testing.T) {
	records := []v1.WorkloadPod{evictedRecord()}
	assertPending(t, runObjectSucceededWith(t, objectSucceededRun{replicas: 1, records: records, unsynced: true,
		cached: []*unstructured.Unstructured{cachedPod(t, "p1", corev1.PodSucceeded, evictionCondition())}}))
	assertPending(t, runObjectSucceededWith(t, objectSucceededRun{replicas: 1, records: records, unsynced: true}))
	assertPending(t, runObjectSucceededWith(t, objectSucceededRun{replicas: 1, records: records, noInformer: true}))
	// The same record with a cache that has synced and holds no pod decides.
	assertDisruptedFailed(t, runObjectSucceededWith(t, objectSucceededRun{replicas: 1, records: records}))
}

// The wait is bounded: past maxSuccessVerdictWait of the dispatch the success
// stands as reported. A wait begun for an earlier dispatch starts over.
func TestSuccessVerdictWaitIsBounded(t *testing.T) {
	within, past := maxSuccessVerdictWait-time.Second, maxSuccessVerdictWait
	assertPending(t, runObjectSucceededWith(t, objectSucceededRun{replicas: 1, unsynced: true, waited: &within}))
	assertSucceeded(t, runObjectSucceededWith(t, objectSucceededRun{replicas: 1, unsynced: true, waited: &past}))

	r := &SyncerReconciler{}
	r.successVerdictWaits.Store("w", successVerdictWait{dispatch: 1, since: time.Now().Add(-past)})
	assert.Equal(t, r.awaitSuccessVerdict("w", 2), true)
	assert.Equal(t, r.awaitSuccessVerdict("w", 2), true)
	r.forgetWorkloadChecks("w")
	_, waiting := r.successVerdictWaits.Load("w")
	assert.Equal(t, waiting, false)
}

// A pod of an earlier dispatch still in the cache is not the current dispatch's
// pod, and does not decide how the current one ended.
func TestEarlierDispatchPodDoesNotDecide(t *testing.T) {
	assertPending(t, runObjectSucceededWith(t, objectSucceededRun{replicas: 1, dispatch: 2,
		records: []v1.WorkloadPod{{PodId: "p1", Phase: corev1.PodSucceeded}},
		cached:  []*unstructured.Unstructured{cachedPod(t, "p1", corev1.PodSucceeded, evictionCondition())}}))
	// The current dispatch's pod decides.
	current := cachedPod(t, "p1", corev1.PodSucceeded, evictionCondition())
	labels := current.GetLabels()
	labels[v1.WorkloadDispatchCntLabel] = "2"
	current.SetLabels(labels)
	assertDisruptedFailed(t, runObjectSucceededWith(t, objectSucceededRun{replicas: 1, dispatch: 2,
		records: []v1.WorkloadPod{{PodId: "p1", Phase: corev1.PodRunning}},
		cached:  []*unstructured.Unstructured{current}}))
	// A cache holding only an earlier dispatch's pod cannot tell whether the
	// current pod has left it, and the record is not read.
	assertPending(t, runObjectSucceededWith(t, objectSucceededRun{replicas: 1, dispatch: 2,
		records: []v1.WorkloadPod{evictedRecord()},
		cached:  []*unstructured.Unstructured{cachedPod(t, "p1", corev1.PodSucceeded)}}))
}

// A MonarchJob or TorchFT workload is not read: its mesh pods carry the
// workload label only after their first sync.
func TestMeshWorkloadsAreNotRead(t *testing.T) {
	disruptedSuccessOf := func(monarch, torchFT bool) string {
		patches := gomonkey.NewPatches()
		defer patches.Reset()
		patches.ApplyFunc(commonworkload.IsTorchFT, func(*v1.Workload) bool { return torchFT })
		patches.ApplyFunc(commonworkload.IsMonarchJob, func(*v1.Workload) bool { return monarch })
		indexer := cache.NewIndexer(cache.MetaNamespaceKeyFunc, cache.Indexers{})
		assert.NilError(t, indexer.Add(cachedPod(t, "p1", corev1.PodSucceeded, evictionCondition())))
		patches.ApplyMethod(reflect.TypeOf(&ClusterClientSets{}), "GetResourceInformer",
			func(*ClusterClientSets, context.Context, schema.GroupVersionKind) (informers.GenericInformer, error) {
				return syncedInformer{indexer: indexer}, nil
			})
		w := &v1.Workload{ObjectMeta: metav1.ObjectMeta{Name: "w",
			Labels:      map[string]string{v1.ClusterIdLabel: "c", v1.WorkloadDispatchCntLabel: "1"},
			Annotations: map[string]string{v1.WorkloadDispatchedAnnotation: timeutil.FormatRFC3339(dispatchedAt)}}}
		w.Spec.Resources = []v1.WorkloadResource{{Replica: 1}}
		w.Status.Pods = []v1.WorkloadPod{evictedRecord()}
		cs := monkeyClientSets()
		cs.name = "c"
		_, disruption, _ := (&SyncerReconciler{}).disruptedSuccess(context.Background(), cs, w)
		return disruption
	}
	assert.Equal(t, disruptedSuccessOf(false, false), wantEviction())
	assert.Equal(t, disruptedSuccessOf(true, false), "")
	assert.Equal(t, disruptedSuccessOf(false, true), "")
}

// The failure a disrupted success becomes ends the workload once failover will
// not act, whatever dispatch the event carries. Failover counts the workload's
// dispatches: a late event of dispatch 1 arriving during dispatch 4 of a
// workload allowed 3 retries must not leave it past its limit and running.
func TestDisruptedSuccessOfLateEventEndsWorkloadPastRetryLimit(t *testing.T) {
	current := cachedPod(t, "p1", corev1.PodSucceeded, evictionCondition())
	labels := current.GetLabels()
	labels[v1.WorkloadDispatchCntLabel] = "4"
	current.SetLabels(labels)
	w := runObjectSucceededWith(t, objectSucceededRun{replicas: 1, dispatch: 4, eventDispatch: 1, maxRetry: 3,
		records: []v1.WorkloadPod{{PodId: "p1", Phase: corev1.PodRunning}},
		cached:  []*unstructured.Unstructured{current}})
	assertDisruptedFailed(t, w)

	// Within the limit the workload is left to failover, which acts on the
	// K8sFailed condition of its current dispatch.
	current = cachedPod(t, "p1", corev1.PodSucceeded, evictionCondition())
	labels = current.GetLabels()
	labels[v1.WorkloadDispatchCntLabel] = "2"
	current.SetLabels(labels)
	w = runObjectSucceededWith(t, objectSucceededRun{replicas: 1, dispatch: 2, eventDispatch: 1, maxRetry: 3,
		records: []v1.WorkloadPod{{PodId: "p1", Phase: corev1.PodRunning}},
		cached:  []*unstructured.Unstructured{current}})
	assert.Equal(t, w.Status.Phase, v1.WorkloadRunning)
	failed := conditionOf(w, v1.K8sFailed)
	assert.Assert(t, failed != nil)
	assert.Equal(t, failed.Reason, commonworkload.GenerateDispatchReason(2))
	assert.Assert(t, jobutils.FindFailedCondition(w))
}

// The object can report success before the pod cache has seen its pod end.
// The pod's DisruptionTarget, set while it still ran, is the disruption that
// ended it.
func TestDisruptedPodNotYetEndedInCacheFailsSuccess(t *testing.T) {
	for _, phase := range []corev1.PodPhase{corev1.PodRunning, corev1.PodPending, ""} {
		assertDisruptedFailed(t, runObjectSucceeded(t, 1, []v1.WorkloadPod{{PodId: "p1", Phase: corev1.PodRunning}},
			cachedPod(t, "p1", phase, evictionCondition())))
	}
	// Without the condition the cache cannot say yet whether the pod was
	// disrupted, and the success waits for it to see the pod end.
	for _, phase := range []corev1.PodPhase{corev1.PodRunning, corev1.PodPending, ""} {
		assertPending(t, runObjectSucceeded(t, 1, []v1.WorkloadPod{{PodId: "p1", Phase: corev1.PodRunning}},
			cachedPod(t, "p1", phase)))
	}
}

func terminatedAt(at time.Time) []corev1.ContainerStatus {
	return []corev1.ContainerStatus{{Name: "main", State: corev1.ContainerState{
		Terminated: &corev1.ContainerStateTerminated{ExitCode: 0, Reason: "Completed",
			FinishedAt: metav1.NewTime(at)}}}}
}

// The taint manager and a drain also mark a pod that had already completed. A
// condition set after every container had finished did not end the pod, and
// does not fail the success; one set before, or in the same second, did.
func TestDisruptionAfterPodEndedDoesNotFailSuccess(t *testing.T) {
	finishedEarlier := terminatedAt(disruptionTime.Add(-time.Minute))
	assertSucceeded(t, runObjectSucceeded(t, 1, []v1.WorkloadPod{{PodId: "p1", Phase: corev1.PodSucceeded}},
		cachedPodWith(t, "p1", corev1.PodSucceeded, finishedEarlier, evictionCondition())))
	assert.Equal(t, disruptionTargetOf(cachedPodWith(t, "p1", corev1.PodSucceeded, finishedEarlier,
		evictionCondition())), "")

	for _, finished := range []time.Time{disruptionTime.Time, disruptionTime.Add(time.Second)} {
		assertDisruptedFailed(t, runObjectSucceeded(t, 1, []v1.WorkloadPod{{PodId: "p1", Phase: corev1.PodSucceeded}},
			cachedPodWith(t, "p1", corev1.PodSucceeded, terminatedAt(finished), evictionCondition())))
	}
	// A container still running had not ended.
	running := append(terminatedAt(disruptionTime.Add(-time.Minute)), corev1.ContainerStatus{Name: "side",
		State: corev1.ContainerState{Running: &corev1.ContainerStateRunning{}}})
	assert.Equal(t, disruptionTargetOf(cachedPodWith(t, "p1", corev1.PodRunning, running, evictionCondition())),
		wantEviction())

	// Nor is it recorded on the pod's record.
	pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "p1"}, Status: corev1.PodStatus{
		Phase: corev1.PodSucceeded, Conditions: []corev1.PodCondition{evictionCondition()},
		ContainerStatuses: finishedEarlier}}
	record := v1.WorkloadPod{}
	buildPodTerminatedInfo(context.Background(), nil, &v1.Workload{}, pod, &record, "")
	assert.Equal(t, record.FailedMessage, "")
	pod.Status.ContainerStatuses = terminatedAt(disruptionTime.Time)
	record = v1.WorkloadPod{}
	buildPodTerminatedInfo(context.Background(), nil, &v1.Workload{}, pod, &record, "")
	assert.Equal(t, record.FailedMessage, wantEviction())
}

// A record is read only when it started in the current dispatch: one left from
// an earlier dispatch, as the cache's pods of one are not read, does not say how
// the current pod ended, and the success waits for the current pod's.
func TestRecordOfEarlierDispatchDoesNotDecide(t *testing.T) {
	earlier := evictedRecord()
	earlier.StartTime = earlierStartTime
	assertPending(t, runObjectSucceededWith(t, objectSucceededRun{replicas: 1, dispatch: 2,
		records: []v1.WorkloadPod{earlier}}))
	failedEarlier := earlier
	failedEarlier.Phase = corev1.PodFailed
	assertPending(t, runObjectSucceededWith(t, objectSucceededRun{replicas: 1, dispatch: 2,
		records: []v1.WorkloadPod{failedEarlier}}))
	neverStarted := evictedRecord()
	neverStarted.StartTime = ""
	assertPending(t, runObjectSucceededWith(t, objectSucceededRun{replicas: 1, dispatch: 2,
		records: []v1.WorkloadPod{neverStarted}}))
	assertDisruptedFailed(t, runObjectSucceededWith(t, objectSucceededRun{replicas: 1, dispatch: 2,
		records: []v1.WorkloadPod{evictedRecord()}}))
}

// A record holds whole seconds: a pod that started in the second it was
// dispatched in started in that dispatch.
func TestRecordStartedInDispatchSecond(t *testing.T) {
	w := &v1.Workload{ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{
		v1.WorkloadDispatchedAnnotation: dispatchedAt.Add(700 * time.Millisecond).Format(timeutil.TimeRFC3339Milli)}}}
	same := &v1.WorkloadPod{StartTime: timeutil.FormatRFC3339(dispatchedAt.Local())}
	assert.Equal(t, startedInCurrentDispatch(w, same), true)
	before := &v1.WorkloadPod{StartTime: timeutil.FormatRFC3339(dispatchedAt.Add(-time.Second).Local())}
	assert.Equal(t, startedInCurrentDispatch(w, before), false)
}

// A success left unapplied is read again: the job event is requeued, and none
// of what follows a status update runs on it.
func TestPendingSuccessVerdictRequeuesTheEvent(t *testing.T) {
	patches := gomonkey.NewPatches()
	defer patches.Reset()
	patches.ApplyPrivateMethod(reflect.TypeOf(&SyncerReconciler{}), "updateAdminWorkloadByJob",
		func(_ *SyncerReconciler, _ context.Context, _ *ClusterClientSets, w *v1.Workload,
			_ *resourceMessage) (*v1.Workload, error) {
			return w, errSuccessVerdictPending
		})
	rescheduled := false
	patches.ApplyPrivateMethod(reflect.TypeOf(&SyncerReconciler{}), "shouldReSchedule",
		func(*SyncerReconciler, context.Context, *v1.Workload, *resourceMessage, *ClusterClientSets) (bool, error) {
			rescheduled = true
			return false, nil
		})
	w := &v1.Workload{ObjectMeta: metav1.ObjectMeta{Name: "w"}}
	w.Status.Phase = v1.WorkloadRunning
	result, err := (&SyncerReconciler{}).handleJobImpl(context.Background(),
		&resourceMessage{name: "o", namespace: "ns"}, w, monkeyClientSets())
	assert.NilError(t, err)
	assert.Equal(t, result.RequeueAfter, successVerdictRetry)
	assert.Equal(t, rescheduled, false)
}
