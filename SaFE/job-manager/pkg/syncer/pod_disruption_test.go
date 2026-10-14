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
	"gotest.tools/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/informers"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	ctrlfake "sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	jobutils "github.com/AMD-AIG-AIMA/SAFE/job-manager/pkg/utils"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/timeutil"
)

var disruptionTime = metav1.NewTime(time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC))

const taintManagerMessage = "Taint manager: deleting due to NoExecute taint"

func taintManagerCondition() corev1.PodCondition {
	return corev1.PodCondition{
		Type:               corev1.DisruptionTarget,
		Status:             corev1.ConditionTrue,
		Reason:             "DeletionByTaintManager",
		Message:            taintManagerMessage,
		LastTransitionTime: disruptionTime,
	}
}

func wantTaintManager() string {
	return "DisruptionTarget: DeletionByTaintManager (" +
		timeutil.FormatRFC3339(disruptionTime.Time) + "): " + taintManagerMessage
}

// disruptedPodObject is the pod as the informer delivers it: unstructured, with
// the workload labels the event handler routes on.
func disruptedPodObject(t *testing.T, conditions ...corev1.PodCondition) *unstructured.Unstructured {
	t.Helper()
	pod := &corev1.Pod{
		TypeMeta: metav1.TypeMeta{APIVersion: "v1", Kind: "Pod"},
		ObjectMeta: metav1.ObjectMeta{
			Name: "p1", Namespace: "ns", UID: "pod-uid",
			Labels: map[string]string{v1.WorkloadIdLabel: "w"},
		},
		Status: corev1.PodStatus{Phase: corev1.PodRunning, Conditions: conditions},
	}
	m, err := runtime.DefaultUnstructuredConverter.ToUnstructured(pod)
	assert.NilError(t, err)
	return &unstructured.Unstructured{Object: m}
}

func disruptionWorkload(t *testing.T, pods ...v1.WorkloadPod) ctrlclient.Client {
	t.Helper()
	w := &v1.Workload{ObjectMeta: metav1.ObjectMeta{
		Name:        "w",
		Annotations: map[string]string{v1.WorkloadDispatchedAnnotation: "true"},
	}}
	w.Spec.MaxRetry = 3
	w.Status.Pods = pods
	return ctrlfake.NewClientBuilder().
		WithScheme(syncerScheme(t)).
		WithObjects(w).
		WithStatusSubresource(&v1.Workload{}).
		Build()
}

// eventMessage runs an informer event through the real event handler and
// returns the message it queues.
func eventMessage(t *testing.T, obj *unstructured.Unstructured, action string) *resourceMessage {
	t.Helper()
	var got *resourceMessage
	cs := &ClusterClientSets{name: "c", handler: func(m *resourceMessage) { got = m }}
	cs.handleResource(context.Background(), obj, obj, action)
	assert.Assert(t, got != nil)
	return got
}

// handlePodWithInformer runs handlePod with the informer cache answering cached.
func handlePodWithInformer(t *testing.T, r *SyncerReconciler, msg *resourceMessage, cached *unstructured.Unstructured) {
	t.Helper()
	patches := gomonkey.NewPatches()
	defer patches.Reset()
	patches.ApplyMethod(reflect.TypeOf(&ClusterClientSets{}), "GetResourceInformer",
		func(_ *ClusterClientSets, _ context.Context, _ schema.GroupVersionKind) (informers.GenericInformer, error) {
			return nil, nil
		})
	patches.ApplyFunc(jobutils.GetObjectByInformer,
		func(informers.GenericInformer, string, string) (*unstructured.Unstructured, error) {
			return cached, nil
		})
	_, err := r.handlePod(context.Background(), msg, monkeyClientSets())
	assert.NilError(t, err)
}

func getWorkloadPods(t *testing.T, cl ctrlclient.Client) []v1.WorkloadPod {
	t.Helper()
	got := &v1.Workload{}
	assert.NilError(t, cl.Get(context.Background(), ctrlclient.ObjectKey{Name: "w"}, got))
	return got.Status.Pods
}

// The taint manager evicts a pod off a node taken out of service. By the time
// the delete event is processed the pod is gone from the informer cache, so the
// reason must come from the object the delete event carried.
func TestPodDisruptionTaintManagerEvictionOnDeletedPod(t *testing.T) {
	cl := disruptionWorkload(t, v1.WorkloadPod{PodId: "p1", Phase: corev1.PodRunning})
	r := &SyncerReconciler{Client: cl}

	msg := eventMessage(t, disruptedPodObject(t, taintManagerCondition()), ResourceDel)
	handlePodWithInformer(t, r, msg, nil)

	pods := getWorkloadPods(t, cl)
	assert.Equal(t, len(pods), 1)
	assert.Equal(t, pods[0].Phase, corev1.PodPhase(v1.WorkloadStopped))
	assert.Equal(t, pods[0].FailedMessage, wantTaintManager())
}

// While the pod is still terminating, the informer's current object is read.
func TestPodDisruptionTaintManagerEvictionOnTerminatingPod(t *testing.T) {
	cl := disruptionWorkload(t, v1.WorkloadPod{PodId: "p1", Phase: corev1.PodRunning})
	r := &SyncerReconciler{Client: cl}

	cached := disruptedPodObject(t, taintManagerCondition())
	now := metav1.Now()
	cached.SetDeletionTimestamp(&now)
	// The queued message predates the condition; the cache has it.
	msg := eventMessage(t, disruptedPodObject(t), ResourceUpdate)
	handlePodWithInformer(t, r, msg, cached)

	pods := getWorkloadPods(t, cl)
	assert.Equal(t, pods[0].Phase, corev1.PodPhase(v1.WorkloadStopped))
	assert.Equal(t, pods[0].FailedMessage, wantTaintManager())
}

// The kubelet terminates the pod itself and it ends Failed: the pod-level
// reason and the DisruptionTarget are both kept.
func TestPodDisruptionKubeletTerminationFailed(t *testing.T) {
	pod := &corev1.Pod{Status: corev1.PodStatus{
		Phase:   corev1.PodFailed,
		Reason:  "Terminated",
		Message: "Pod was terminated in response to imminent node shutdown.",
		Conditions: []corev1.PodCondition{{
			Type: corev1.PodReady, Status: corev1.ConditionFalse,
		}, {
			Type:               corev1.DisruptionTarget,
			Status:             corev1.ConditionTrue,
			Reason:             "TerminationByKubelet",
			Message:            "Pod was terminated in response to imminent node shutdown.",
			LastTransitionTime: disruptionTime,
		}},
	}}
	wp := &v1.WorkloadPod{}
	buildPodTerminatedInfo(context.Background(), nil, &v1.Workload{}, pod, wp, "main")
	assert.Equal(t, wp.FailedMessage,
		"Terminated, Pod was terminated in response to imminent node shutdown.; "+
			"DisruptionTarget: TerminationByKubelet ("+timeutil.FormatRFC3339(disruptionTime.Time)+"): "+
			"Pod was terminated in response to imminent node shutdown.")
}

// Without a True DisruptionTarget nothing changes, on either path.
func TestPodDisruptionAbsentLeavesMessageUnchanged(t *testing.T) {
	notTrue := taintManagerCondition()
	notTrue.Status = corev1.ConditionFalse

	pod := &corev1.Pod{Status: corev1.PodStatus{
		Phase: corev1.PodFailed, Reason: "OOMKilled", Message: "out of memory",
		Conditions: []corev1.PodCondition{notTrue},
	}}
	wp := &v1.WorkloadPod{}
	buildPodTerminatedInfo(context.Background(), nil, &v1.Workload{}, pod, wp, "main")
	assert.Equal(t, wp.FailedMessage, "OOMKilled, out of memory")

	running := &corev1.Pod{Status: corev1.PodStatus{
		Phase: corev1.PodRunning, Conditions: []corev1.PodCondition{taintManagerCondition()},
	}}
	wp = &v1.WorkloadPod{}
	buildPodTerminatedInfo(context.Background(), nil, &v1.Workload{}, running, wp, "main")
	assert.Equal(t, wp.FailedMessage, "")

	cl := disruptionWorkload(t, v1.WorkloadPod{PodId: "p1", Phase: corev1.PodRunning})
	r := &SyncerReconciler{Client: cl}
	msg := eventMessage(t, disruptedPodObject(t, notTrue), ResourceDel)
	assert.Equal(t, msg.disruption, "")
	handlePodWithInformer(t, r, msg, nil)
	pods := getWorkloadPods(t, cl)
	assert.Equal(t, pods[0].Phase, corev1.PodPhase(v1.WorkloadStopped))
	assert.Equal(t, pods[0].FailedMessage, "")
}

// A record that already ended with its own failedMessage gains the
// DisruptionTarget when the delete event carries one; both are kept, and a
// repeated event does not add it twice.
func TestPodDisruptionKeepsExistingFailedMessage(t *testing.T) {
	cl := disruptionWorkload(t,
		v1.WorkloadPod{PodId: "p1", Phase: corev1.PodFailed, FailedMessage: "Error, exit 1"})
	r := &SyncerReconciler{Client: cl}

	obj := disruptedPodObject(t, taintManagerCondition())
	handlePodWithInformer(t, r, eventMessage(t, obj, ResourceDel), nil)
	handlePodWithInformer(t, r, eventMessage(t, obj, ResourceDel), nil)

	pods := getWorkloadPods(t, cl)
	assert.Equal(t, pods[0].Phase, corev1.PodFailed)
	assert.Equal(t, pods[0].FailedMessage, "Error, exit 1; "+wantTaintManager())
}
