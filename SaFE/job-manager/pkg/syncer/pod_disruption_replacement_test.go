/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package syncer

import (
	"context"
	"testing"
	"time"

	"gotest.tools/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/timeutil"
)

var (
	firstStart  = metav1.NewTime(time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC))
	secondStart = metav1.NewTime(time.Date(2026, 1, 2, 3, 5, 0, 0, time.UTC))
)

// samePod is a pod named p1 in the given phase, started at start (unset when nil).
func samePod(phase corev1.PodPhase, start *metav1.Time, conditions ...corev1.PodCondition) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "p1", Namespace: "ns"},
		Status:     corev1.PodStatus{Phase: phase, StartTime: start, Conditions: conditions},
	}
}

// seePod runs a live pod's update through the pod record path and stores the
// result, as updateAdminWorkloadByPod does.
func seePod(t *testing.T, r *SyncerReconciler, cl ctrlclient.Client, pod *corev1.Pod) {
	t.Helper()
	w := &v1.Workload{}
	assert.NilError(t, cl.Get(context.Background(), ctrlclient.ObjectKey{Name: "w"}, w))
	if _, _, updated := r.updateWorkloadNodeAndPods(context.Background(), monkeyClientSets(), w, pod, nil); updated {
		assert.NilError(t, r.patchWorkloadPodStatus(context.Background(), w, nil))
	}
}

// The ordering a controller that recreates its pod produces: the Eviction API
// takes the pod, the controller creates a pod of the same name, and the owner
// then stops the workload. The workload ends Stopped, and its pod record must
// still say a disruption took the pod.
func TestEvictedPodReplacedThenStoppedKeepsDisruption(t *testing.T) {
	cl := disruptionWorkload(t, v1.WorkloadPod{PodId: "p1", Phase: corev1.PodRunning,
		StartTime: timeutil.FormatRFC3339(firstStart.Time)})
	r := &SyncerReconciler{Client: cl}

	evicting := disruptedPodObject(t, evictionCondition())
	now := metav1.Now()
	evicting.SetDeletionTimestamp(&now)
	handlePodWithInformer(t, r, eventMessage(t, evicting, ResourceUpdate), evicting)
	pods := getWorkloadPods(t, cl)
	assert.Equal(t, pods[0].Phase, corev1.PodPhase(v1.WorkloadStopped))
	assert.Equal(t, pods[0].FailedMessage, wantEviction())

	// The replacement is seen pending, then running; neither update repeats
	// or loses the earlier pod's condition.
	seePod(t, r, cl, samePod(corev1.PodPending, nil))
	seePod(t, r, cl, samePod(corev1.PodRunning, &secondStart))
	seePod(t, r, cl, samePod(corev1.PodRunning, &secondStart, corev1.PodCondition{
		Type: corev1.PodReady, Status: corev1.ConditionTrue}))
	pods = getWorkloadPods(t, cl)
	assert.Equal(t, len(pods), 1)
	assert.Equal(t, pods[0].Phase, corev1.PodRunning)
	assert.Equal(t, pods[0].StartTime, timeutil.FormatRFC3339(secondStart.Time))
	assert.Equal(t, pods[0].FailedMessage, earlierPodPrefix+wantEviction())

	// The owner stops the workload: the replacement is deleted with no
	// disruption of its own.
	handlePodWithInformer(t, r, eventMessage(t, disruptedPodObject(t), ResourceDel), nil)
	pods = getWorkloadPods(t, cl)
	assert.Equal(t, pods[0].Phase, corev1.PodPhase(v1.WorkloadStopped))
	assert.Equal(t, pods[0].FailedMessage, earlierPodPrefix+wantEviction())
}

// A pod that is restarted under the same name and stopped, with no disruption
// anywhere, ends as a clean stop.
func TestSameNameRestartWithoutDisruptionStaysClean(t *testing.T) {
	cl := disruptionWorkload(t, v1.WorkloadPod{PodId: "p1", Phase: corev1.PodRunning,
		StartTime: timeutil.FormatRFC3339(firstStart.Time)})
	r := &SyncerReconciler{Client: cl}

	handlePodWithInformer(t, r, eventMessage(t, disruptedPodObject(t), ResourceDel), nil)
	seePod(t, r, cl, samePod(corev1.PodRunning, &secondStart))
	handlePodWithInformer(t, r, eventMessage(t, disruptedPodObject(t), ResourceDel), nil)

	pods := getWorkloadPods(t, cl)
	assert.Equal(t, pods[0].Phase, corev1.PodPhase(v1.WorkloadStopped))
	assert.Equal(t, pods[0].FailedMessage, "")
}

// A replacement that is itself disrupted records its own condition next to the
// earlier pod's, and a later pod carries only the latest.
func TestDisruptedReplacementRecordsItsOwn(t *testing.T) {
	cl := disruptionWorkload(t, v1.WorkloadPod{PodId: "p1", Phase: corev1.PodPhase(v1.WorkloadStopped),
		StartTime: timeutil.FormatRFC3339(firstStart.Time), FailedMessage: wantEviction()})
	r := &SyncerReconciler{Client: cl}

	seePod(t, r, cl, samePod(corev1.PodRunning, &secondStart))
	handlePodWithInformer(t, r, eventMessage(t, disruptedPodObject(t, taintManagerCondition()), ResourceDel), nil)
	pods := getWorkloadPods(t, cl)
	assert.Equal(t, pods[0].FailedMessage, earlierPodPrefix+wantEviction()+"; "+wantTaintManager())

	seePod(t, r, cl, samePod(corev1.PodPending, nil))
	pods = getWorkloadPods(t, cl)
	assert.Equal(t, pods[0].FailedMessage, earlierPodPrefix+wantTaintManager())
}

// Further updates of the pod that ended with a disruption are the same pod:
// its own condition is not turned into an earlier pod's.
func TestSamePodIsNotItsOwnEarlierPod(t *testing.T) {
	old := &v1.WorkloadPod{PodId: "p1", Phase: corev1.PodSucceeded,
		StartTime: timeutil.FormatRFC3339(firstStart.Time), FailedMessage: wantEviction()}
	cur := &v1.WorkloadPod{PodId: "p1", Phase: corev1.PodSucceeded,
		StartTime: timeutil.FormatRFC3339(firstStart.Time), FailedMessage: wantEviction()}
	carryEarlierDisruption(old, cur)
	assert.Equal(t, cur.FailedMessage, wantEviction())

	// A running record is not an ended pod either.
	old.Phase = corev1.PodRunning
	cur = &v1.WorkloadPod{PodId: "p1", Phase: corev1.PodRunning}
	carryEarlierDisruption(old, cur)
	assert.Equal(t, cur.FailedMessage, "")
}

// A later pod that has already ended when it is first seen is told apart from
// the earlier one by its start time.
func TestEndedLaterPodCarriesEarlierDisruption(t *testing.T) {
	old := &v1.WorkloadPod{PodId: "p1", Phase: corev1.PodSucceeded,
		StartTime: timeutil.FormatRFC3339(firstStart.Time), FailedMessage: wantEviction()}
	cur := &v1.WorkloadPod{PodId: "p1", Phase: corev1.PodFailed,
		StartTime: timeutil.FormatRFC3339(secondStart.Time), FailedMessage: "Error, exit 1"}
	carryEarlierDisruption(old, cur)
	assert.Equal(t, cur.FailedMessage, "Error, exit 1; "+earlierPodPrefix+wantEviction())
}

// A later pod first seen in the phase the old record ended in, on the same
// node, is still told apart by its start time rather than skipped as unchanged.
func TestLaterPodInSamePhaseIsNotSkipped(t *testing.T) {
	w := &v1.Workload{ObjectMeta: metav1.ObjectMeta{Name: "w"}}
	w.Status.Phase = v1.WorkloadRunning
	w.Status.Pods = []v1.WorkloadPod{{PodId: "p1", Phase: corev1.PodSucceeded,
		StartTime: timeutil.FormatRFC3339(firstStart.Time), FailedMessage: wantEviction()}}
	r := &SyncerReconciler{}
	_, _, updated := r.updateWorkloadNodeAndPods(context.Background(), monkeyClientSets(), w,
		samePod(corev1.PodSucceeded, &secondStart), nil)
	assert.Equal(t, updated, true)
	assert.Equal(t, w.Status.Pods[0].StartTime, timeutil.FormatRFC3339(secondStart.Time))
	assert.Equal(t, w.Status.Pods[0].FailedMessage, earlierPodPrefix+wantEviction())

	// The same pod seen again is unchanged.
	_, _, updated = r.updateWorkloadNodeAndPods(context.Background(), monkeyClientSets(), w,
		samePod(corev1.PodSucceeded, &secondStart), nil)
	assert.Equal(t, updated, false)
}

// An earlier pod's condition says nothing about how a later one ended: a
// single-pod object reporting success from a record that carries only that is
// a success.
func TestEarlierPodDisruptionDoesNotFailASuccess(t *testing.T) {
	assertSucceeded(t, runObjectSucceeded(t, 1, []v1.WorkloadPod{{PodId: "p1",
		Phase: corev1.PodSucceeded, FailedMessage: earlierPodPrefix + wantEviction()}}))
	// The record's own condition still decides.
	assertDisruptedFailed(t, runObjectSucceeded(t, 1, []v1.WorkloadPod{{PodId: "p1",
		Phase:         corev1.PodPhase(v1.WorkloadStopped),
		FailedMessage: earlierPodPrefix + wantTaintManager() + "; " + wantEviction()}}))
}

func TestPodDisruptions(t *testing.T) {
	for _, tc := range []struct{ message, own, earlier string }{
		{"", "", ""},
		{"OOMKilled, out of memory", "", ""},
		{wantEviction(), wantEviction(), ""},
		{"Error, exit 1; " + wantEviction(), wantEviction(), ""},
		{earlierPodPrefix + wantEviction(), "", wantEviction()},
		{earlierPodPrefix + wantEviction() + "; " + wantTaintManager(), wantTaintManager(), wantEviction()},
		{wantTaintManager() + "; " + earlierPodPrefix + wantEviction(), wantTaintManager(), wantEviction()},
	} {
		own, earlier := podDisruptions(tc.message)
		assert.Equal(t, own, tc.own, tc.message)
		assert.Equal(t, earlier, tc.earlier, tc.message)
	}
}
