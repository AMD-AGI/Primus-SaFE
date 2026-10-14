/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package syncer

import (
	"context"
	"reflect"
	"strings"
	"testing"

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

// syncedInformer is a pod informer whose cache holds exactly the given pods.
type syncedInformer struct{ indexer cache.Indexer }

type syncedSharedInformer struct{ cache.SharedIndexInformer }

func (syncedSharedInformer) HasSynced() bool { return true }

func (s syncedInformer) Informer() cache.SharedIndexInformer { return syncedSharedInformer{} }
func (s syncedInformer) Lister() cache.GenericLister {
	return cache.NewGenericLister(s.indexer, schema.GroupResource{Resource: "pods"})
}

func cachedPod(t *testing.T, name string, phase corev1.PodPhase, conditions ...corev1.PodCondition) *unstructured.Unstructured {
	t.Helper()
	pod := &corev1.Pod{
		TypeMeta: metav1.TypeMeta{APIVersion: "v1", Kind: "Pod"},
		ObjectMeta: metav1.ObjectMeta{
			Name: name, Namespace: "ns",
			Labels: map[string]string{v1.WorkloadIdLabel: "w"},
		},
		Status: corev1.PodStatus{Phase: phase, Conditions: conditions},
	}
	m, err := runtime.DefaultUnstructuredConverter.ToUnstructured(pod)
	assert.NilError(t, err)
	return &unstructured.Unstructured{Object: m}
}

// runObjectSucceeded runs the job status path for an object that reports
// K8sSucceeded, with the pod informer cache holding cached, and returns the
// stored workload.
func runObjectSucceeded(t *testing.T, records []v1.WorkloadPod, cached ...*unstructured.Unstructured) *v1.Workload {
	t.Helper()
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
	for _, obj := range cached {
		assert.NilError(t, indexer.Add(obj))
	}
	patches.ApplyMethod(reflect.TypeOf(&ClusterClientSets{}), "GetResourceInformer",
		func(_ *ClusterClientSets, _ context.Context, gvk schema.GroupVersionKind) (informers.GenericInformer, error) {
			assert.Equal(t, gvk, podResourceGVK)
			return syncedInformer{indexer: indexer}, nil
		})

	w := &v1.Workload{ObjectMeta: metav1.ObjectMeta{
		Name:   "w",
		Labels: map[string]string{v1.ClusterIdLabel: "c"},
	}}
	w.Status.Phase = v1.WorkloadRunning
	cl := ctrlfake.NewClientBuilder().
		WithScheme(syncerScheme(t)).
		WithObjects(w).
		WithStatusSubresource(&v1.Workload{}).
		Build()
	r := &SyncerReconciler{Client: cl}
	hydrated := &v1.Workload{}
	assert.NilError(t, cl.Get(context.Background(), ctrlclient.ObjectKey{Name: "w"}, hydrated))
	hydrated.Status.Pods = records

	cs := monkeyClientSets()
	cs.name = "c"
	_, err := r.updateAdminWorkloadByJob(context.Background(), cs, hydrated,
		&resourceMessage{name: "o", namespace: "ns", dispatchCount: 1, gvk: schema.GroupVersionKind{Kind: "Sandbox"}})
	assert.NilError(t, err)
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

func assertDisruptedFailed(t *testing.T, w *v1.Workload) {
	t.Helper()
	assert.Equal(t, w.Status.Phase, v1.WorkloadFailed)
	assert.Assert(t, conditionOf(w, v1.K8sSucceeded) == nil)
	failed := conditionOf(w, v1.K8sFailed)
	assert.Assert(t, failed != nil)
	assert.Equal(t, failed.Message, objectSucceededMessage+"; "+wantEviction())
	assert.Assert(t, strings.Contains(w.Status.Message, wantEviction()), w.Status.Message)
}

// The Eviction API took the pod; its containers exited 0 on SIGTERM, so the pod
// and then the object report Succeeded. The delete path had already recorded
// the DisruptionTarget on the pod record: the workload failed.
func TestDisruptedSuccessFromPodRecord(t *testing.T) {
	w := runObjectSucceeded(t, []v1.WorkloadPod{{
		PodId: "p1", Phase: corev1.PodPhase(v1.WorkloadStopped), FailedMessage: wantEviction(),
	}})
	assertDisruptedFailed(t, w)
}

// The object can report success before the pod's last event is recorded; the
// pod in the informer cache carries the condition.
func TestDisruptedSuccessFromInformerCache(t *testing.T) {
	w := runObjectSucceeded(t, []v1.WorkloadPod{{PodId: "p1", Phase: corev1.PodRunning}},
		cachedPod(t, "p1", corev1.PodSucceeded, evictionCondition()))
	assertDisruptedFailed(t, w)
}

// A pod that ended Succeeded with no DisruptionTarget is a clean success.
func TestCleanSuccessStaysSucceeded(t *testing.T) {
	notTrue := evictionCondition()
	notTrue.Status = corev1.ConditionFalse
	w := runObjectSucceeded(t, []v1.WorkloadPod{{PodId: "p1", Phase: corev1.PodSucceeded}},
		cachedPod(t, "p1", corev1.PodSucceeded, notTrue))
	assert.Equal(t, w.Status.Phase, v1.WorkloadSucceeded)
	assert.Assert(t, conditionOf(w, v1.K8sFailed) == nil)
	assert.Equal(t, conditionOf(w, v1.K8sSucceeded).Message, objectSucceededMessage)
}

// One pod was evicted and another completed (a Job replaces an evicted pod):
// the object's success stands, from either source of the clean pod.
func TestSuccessAfterReplacedDisruptedPodStands(t *testing.T) {
	evicted := v1.WorkloadPod{PodId: "p1", Phase: corev1.PodPhase(v1.WorkloadStopped), FailedMessage: wantEviction()}
	w := runObjectSucceeded(t, []v1.WorkloadPod{evicted, {PodId: "p2", Phase: corev1.PodSucceeded}})
	assert.Equal(t, w.Status.Phase, v1.WorkloadSucceeded)

	w = runObjectSucceeded(t, []v1.WorkloadPod{evicted},
		cachedPod(t, "p2", corev1.PodSucceeded))
	assert.Equal(t, w.Status.Phase, v1.WorkloadSucceeded)
}

// A pod record that reads Succeeded is not a clean success when the same pod
// in the cache carries the condition.
func TestDisruptedSuccessCacheOverridesRecordOfSamePod(t *testing.T) {
	w := runObjectSucceeded(t, []v1.WorkloadPod{{PodId: "p1", Phase: corev1.PodSucceeded}},
		cachedPod(t, "p1", corev1.PodSucceeded, evictionCondition()))
	assertDisruptedFailed(t, w)
}

// Once the workload has ended, a later pod of the same name (a controller
// recreating it after the end) does not overwrite the record of how the
// workload's pod ended.
func TestEndedPodRecordNotOverwrittenAfterWorkloadEnd(t *testing.T) {
	record := v1.WorkloadPod{PodId: "p1", Phase: corev1.PodPhase(v1.WorkloadStopped),
		StartTime: "2026-01-02T00:00:00", FailedMessage: wantEviction()}
	replacement := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "p1", Namespace: "ns"},
		Status:     corev1.PodStatus{Phase: corev1.PodPending},
	}
	r := &SyncerReconciler{}

	ended := &v1.Workload{ObjectMeta: metav1.ObjectMeta{Name: "w"}}
	ended.Status.Phase = v1.WorkloadFailed
	ended.Status.Pods = []v1.WorkloadPod{record}
	_, _, updated := r.updateWorkloadNodeAndPods(context.Background(), monkeyClientSets(), ended, replacement, nil)
	assert.Equal(t, updated, false)
	assert.DeepEqual(t, ended.Status.Pods, []v1.WorkloadPod{record})

	// While the workload runs, a pod restarted under the same name replaces
	// the record as before.
	running := &v1.Workload{ObjectMeta: metav1.ObjectMeta{Name: "w"}}
	running.Status.Phase = v1.WorkloadRunning
	running.Status.Pods = []v1.WorkloadPod{record}
	_, _, updated = r.updateWorkloadNodeAndPods(context.Background(), monkeyClientSets(), running, replacement, nil)
	assert.Equal(t, updated, true)
	assert.Equal(t, running.Status.Pods[0].Phase, corev1.PodPending)
	assert.Equal(t, running.Status.Pods[0].FailedMessage, "")
}
