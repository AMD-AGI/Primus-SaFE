/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package ops_job

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	commonconfig "github.com/AMD-AIG-AIMA/SAFE/common/pkg/config"
	modelprewarm "github.com/AMD-AIG-AIMA/SAFE/common/pkg/model_prewarm"
)

// endedPrewarmWorkload returns a finished prewarm workload whose main container reports containerMessage.
func endedPrewarmWorkload(job *v1.OpsJob, phase v1.WorkloadPhase, containerMessage, statusMessage string) *v1.Workload {
	workload := prewarmWorkloadOf(job, "admin-1", 100*time.Second, phase)
	workload.Annotations = map[string]string{v1.MainContainerAnnotation: "main"}
	workload.Status.Message = statusMessage
	workload.Status.Pods = []v1.WorkloadPod{{
		PodId:      "pod-1",
		Containers: []v1.Container{{Name: "main", Message: containerMessage}},
	}}
	return workload
}

// The result of a finished prewarm pod depends on its phase and on the termination message it wrote.
func TestEndedWorkloadResultBranches(t *testing.T) {
	job := &v1.OpsJob{ObjectMeta: metav1.ObjectMeta{Name: "j1"}}
	cases := []struct {
		name             string
		phase            v1.WorkloadPhase
		containerMessage string
		statusMessage    string
		wantPhase        string
		wantMessage      string
	}{
		{"failed with error in termination message", v1.WorkloadFailed, `{"error":"boom"}`, "",
			modelprewarm.PhaseFailed, "boom"},
		{"failed with plain container message", v1.WorkloadFailed, "plain output", "",
			modelprewarm.PhaseFailed, "plain output"},
		{"failed without message falls back to status", v1.WorkloadFailed, "", "node lost",
			modelprewarm.PhaseFailed, "node lost"},
		{"failed without any message", v1.WorkloadFailed, "", "",
			modelprewarm.PhaseFailed, "workload Failed"},
		{"succeeded with unparsable message", v1.WorkloadSucceeded, "oops", "",
			modelprewarm.PhaseFailed, "failed to parse the result of the prewarm pod"},
		{"succeeded without bytes read", v1.WorkloadSucceeded, `{"bytesRead":0,"durationSeconds":1}`, "",
			modelprewarm.PhaseFailed, "no weight bytes were read"},
		{"succeeded with bytes read", v1.WorkloadSucceeded, `{"bytesRead":9,"durationSeconds":3}`, "",
			modelprewarm.PhaseSucceeded, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			workload := endedPrewarmWorkload(job, tc.phase, tc.containerMessage, tc.statusMessage)
			r := &ModelPrewarmJobReconciler{OpsJobBaseReconciler: newBaseWithObjs(t, workload)}

			result := r.endedWorkloadResult(context.Background(), job, workload)
			require.NotNil(t, result)
			assert.Equal(t, tc.wantPhase, result.Phase)
			if tc.wantMessage != "" {
				assert.Contains(t, result.Message, tc.wantMessage)
			}
			if tc.wantPhase == modelprewarm.PhaseSucceeded {
				assert.Equal(t, int64(9), result.BytesRead)
				assert.Equal(t, int64(3), result.DurationSeconds)
			}
		})
	}
}

// A failed result keeps its message, truncated to the annotation limit, and a refused dispatch is recognized.
func TestFailedResultTruncatesAndDispatchRefusal(t *testing.T) {
	job := &v1.OpsJob{ObjectMeta: metav1.ObjectMeta{Name: "j1"}}
	long := strings.Repeat("x", modelprewarm.MaxResultMessageLength+10)
	result := failedResult(job, long)
	assert.Equal(t, modelprewarm.PhaseFailed, result.Phase)
	assert.Equal(t, "j1", result.OpsJobId)
	assert.Len(t, result.Message, modelprewarm.MaxResultMessageLength)

	assert.True(t, isDispatchRejected(apierrors.NewBadRequest("refused")))
	assert.False(t, isDispatchRejected(errors.New("transient")))
}

// Nodes that are unknown to the cluster are not external nodes, so they yield no prewarm.
func TestGetExternalNodesSkipsUnknownNodes(t *testing.T) {
	job := &v1.OpsJob{ObjectMeta: metav1.ObjectMeta{Name: "j1"}}
	r := &ModelPrewarmJobReconciler{OpsJobBaseReconciler: newBaseWithObjs(t, job)}
	assert.Empty(t, r.getExternalNodes(context.Background(), []string{"missing-node"}))
}

// A job without a model path fails every node with the configuration error and dispatches nothing.
func TestSyncExternalWorkloadsReportsBadConfig(t *testing.T) {
	job := &v1.OpsJob{ObjectMeta: metav1.ObjectMeta{Name: "j1"}}
	node := &v1.Node{ObjectMeta: metav1.ObjectMeta{Name: "admin-1"}}
	r, clients := pendingResultReconciler(t, []client.Object{job})

	results := r.syncExternalWorkloads(context.Background(), job, clients, []*v1.Node{node})
	require.NotNil(t, results["admin-1"])
	assert.Equal(t, modelprewarm.PhaseFailed, results["admin-1"].Phase)
	assert.Contains(t, results["admin-1"].Message, "model.path is missing")
}

// When the concurrency limit is reached by a running node, a node still waiting is not dispatched.
func TestSyncExternalWorkloadsRespectsConcurrencyLimit(t *testing.T) {
	const key = "ops_job.model_prewarm.max_concurrent"
	commonconfig.SetValue(key, strconv.Itoa(1))
	t.Cleanup(func() { commonconfig.UnsetValue(key) })

	job := &v1.OpsJob{
		ObjectMeta: metav1.ObjectMeta{Name: "j1"},
		Spec: v1.OpsJobSpec{Inputs: []v1.Parameter{
			{Name: v1.ParameterModelPath, Value: "/shared_nfs/models/m"},
		}},
	}
	running := prewarmWorkloadOf(job, "node-a", 100*time.Second, v1.WorkloadRunning)
	r, clients := pendingResultReconciler(t, []client.Object{job, running})
	nodes := []*v1.Node{
		{ObjectMeta: metav1.ObjectMeta{Name: "node-a"}},
		{ObjectMeta: metav1.ObjectMeta{Name: "node-b"}},
	}

	results := r.syncExternalWorkloads(context.Background(), job, clients, nodes)
	require.NotNil(t, results["node-a"])
	assert.Equal(t, modelprewarm.PhaseRunning, results["node-a"].Phase)
	assert.NotContains(t, results, "node-b")
	err := r.Get(context.Background(), client.ObjectKey{Name: externalWorkloadName(job.Name, "node-b"),
		Namespace: "ws1"}, &v1.Workload{})
	assert.True(t, apierrors.IsNotFound(err), "%v", err)
}

// A node whose workspace or host name is missing cannot get a prewarm workload, and the refusal fails it.
func TestGenerateExternalWorkloadRefusesIncompleteNode(t *testing.T) {
	job := &v1.OpsJob{ObjectMeta: metav1.ObjectMeta{Name: "j1"}}
	cfg := &modelPrewarmConfig{modelPath: "/data/models/glm", glob: modelprewarm.DefaultGlob, parallelism: 2}
	hostname := "host-1"
	noVolumeWorkspace := &v1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: "ws-novol"}}

	cases := []struct {
		name string
		node *v1.Node
		objs []client.Object
	}{
		{"node without workspace", &v1.Node{ObjectMeta: metav1.ObjectMeta{Name: "n1"},
			Spec: v1.NodeSpec{Hostname: &hostname}}, nil},
		{"node without host name", &v1.Node{ObjectMeta: metav1.ObjectMeta{Name: "n2",
			Labels: map[string]string{v1.WorkspaceIdLabel: "ws-novol"}}}, []client.Object{noVolumeWorkspace}},
		{"workspace not found", &v1.Node{ObjectMeta: metav1.ObjectMeta{Name: "n3",
			Labels: map[string]string{v1.WorkspaceIdLabel: "ws-missing"}}, Spec: v1.NodeSpec{Hostname: &hostname}},
			nil},
		{"model path outside any volume", &v1.Node{ObjectMeta: metav1.ObjectMeta{Name: "n4",
			Labels: map[string]string{v1.WorkspaceIdLabel: "ws-novol"}}, Spec: v1.NodeSpec{Hostname: &hostname}},
			[]client.Object{noVolumeWorkspace}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			objs := append([]client.Object{job}, tc.objs...)
			r := &ModelPrewarmJobReconciler{OpsJobBaseReconciler: newBaseWithObjs(t, objs...)}
			workload, err := r.generateExternalWorkload(context.Background(), job, cfg, tc.node,
				externalWorkloadName(job.Name, tc.node.Name))
			assert.Error(t, err)
			assert.Nil(t, workload)
		})
	}

	// A refusal that will not change on retry fails the node instead of being retried.
	refused := &v1.Node{ObjectMeta: metav1.ObjectMeta{Name: "n5"}}
	r := &ModelPrewarmJobReconciler{OpsJobBaseReconciler: newBaseWithObjs(t, job)}
	_, err := r.generateExternalWorkload(context.Background(), job, cfg, refused, "w")
	assert.True(t, isDispatchRejected(err))
}

// Listing returns the prewarm workloads of the given job only, by name.
func TestListExternalWorkloadsOfJobOnly(t *testing.T) {
	job := &v1.OpsJob{ObjectMeta: metav1.ObjectMeta{Name: "j1"}}
	other := &v1.OpsJob{ObjectMeta: metav1.ObjectMeta{Name: "j2"}}
	mine := prewarmWorkloadOf(job, "admin-1", time.Second, v1.WorkloadRunning)
	mine2 := prewarmWorkloadOf(job, "admin-2", time.Second, v1.WorkloadRunning)
	theirs := prewarmWorkloadOf(other, "admin-1", time.Second, v1.WorkloadRunning)
	r := &ModelPrewarmJobReconciler{OpsJobBaseReconciler: newBaseWithObjs(t, mine, mine2, theirs)}

	listed, err := r.listExternalWorkloads(context.Background(), job)
	require.NoError(t, err)
	assert.Len(t, listed, 2)
	assert.Contains(t, listed, mine.Name)
	assert.Contains(t, listed, mine2.Name)
	assert.NotContains(t, listed, theirs.Name)
}

// Without a pending timeout, or without cluster clients, no pending reason is stated.
func TestUnscheduledReasonNeedsTimeoutAndClients(t *testing.T) {
	job := &v1.OpsJob{ObjectMeta: metav1.ObjectMeta{Name: "j1"}}
	workload := prewarmWorkloadOf(job, "admin-1", time.Hour, v1.WorkloadPending)
	r := &ModelPrewarmJobReconciler{OpsJobBaseReconciler: newBaseWithObjs(t, workload)}
	assert.Empty(t, r.unscheduledReason(context.Background(), workload, nil))

	setPendingTimeout(t, 300)
	assert.Empty(t, r.unscheduledReason(context.Background(), workload, nil))
}

// A workload whose pod was refused before creation states the reason in an event of the workload.
func TestExternalPendingWithoutPodReportsWorkloadEvent(t *testing.T) {
	setPendingTimeout(t, 300)
	job := &v1.OpsJob{ObjectMeta: metav1.ObjectMeta{Name: "j1"}}
	workload := prewarmWorkloadOf(job, "admin-1", 400*time.Second, v1.WorkloadPending)
	r, clients := pendingResultReconciler(t, []client.Object{workload})
	_, err := clients.ClientSet().CoreV1().Events("ws1").Create(context.Background(), &corev1.Event{
		ObjectMeta:     metav1.ObjectMeta{Name: "refused", Namespace: "ws1"},
		InvolvedObject: corev1.ObjectReference{Name: workload.Name},
		Type:           corev1.EventTypeWarning,
		Message:        "pod creation forbidden by quota",
		LastTimestamp:  metav1.NewTime(time.Now()),
	}, metav1.CreateOptions{})
	require.NoError(t, err)

	result := r.externalWorkloadResult(context.Background(), job, workload, clients)
	require.NotNil(t, result)
	assert.Equal(t, modelprewarm.PhaseFailed, result.Phase)
	assert.Contains(t, result.Message, "quota")
}

// The newest warning of an object counts, using its event time when it has no last timestamp; normal
// events and events not after the given time are ignored.
func TestLatestWarningEventSelection(t *testing.T) {
	_, clients := pendingResultReconciler(t, nil)
	events := clients.ClientSet().CoreV1().Events("ws1")
	now := time.Now()
	create := func(name, message, eventType string, last, eventTime time.Time) {
		ev := &corev1.Event{
			ObjectMeta:     metav1.ObjectMeta{Name: name, Namespace: "ws1"},
			InvolvedObject: corev1.ObjectReference{Name: "obj"},
			Type:           eventType,
			Message:        message,
		}
		if !last.IsZero() {
			ev.LastTimestamp = metav1.NewTime(last)
		}
		if !eventTime.IsZero() {
			ev.EventTime = metav1.NewMicroTime(eventTime)
		}
		_, err := events.Create(context.Background(), ev, metav1.CreateOptions{})
		require.NoError(t, err)
	}
	create("old", "old warning", corev1.EventTypeWarning, now.Add(-10*time.Minute), time.Time{})
	create("new", "new warning", corev1.EventTypeWarning, time.Time{}, now.Add(-1*time.Minute))
	create("normal", "normal event", corev1.EventTypeNormal, now, time.Time{})
	create("empty", "", corev1.EventTypeWarning, now, time.Time{})

	assert.Equal(t, "new warning", latestWarningEvent(context.Background(), events, "obj", time.Time{}))
	assert.Equal(t, "new warning", latestWarningEvent(context.Background(), events, "obj", now.Add(-5*time.Minute)))
	assert.Empty(t, latestWarningEvent(context.Background(), events, "obj", now))
}

// Persisted results are read back, invalid persisted data is ignored, and saving is idempotent.
func TestPersistedExternalResultsEdgeCases(t *testing.T) {
	invalid := &v1.OpsJob{ObjectMeta: metav1.ObjectMeta{Name: "j1", Annotations: map[string]string{
		v1.OpsJobModelPrewarmExternalResultsAnnotation: "{bad",
	}}}
	assert.Empty(t, loadExternalResults(invalid))
	assert.Empty(t, loadExternalResults(&v1.OpsJob{ObjectMeta: metav1.ObjectMeta{Name: "j2"}}))

	job := &v1.OpsJob{ObjectMeta: metav1.ObjectMeta{Name: "j3"}}
	r := &ModelPrewarmJobReconciler{OpsJobBaseReconciler: newBaseWithObjs(t, job)}
	running := map[string]*modelprewarm.Result{"a": {Phase: modelprewarm.PhaseRunning}}
	require.NoError(t, r.saveExternalResults(context.Background(), job, running))
	assert.Empty(t, v1.GetAnnotation(job, v1.OpsJobModelPrewarmExternalResultsAnnotation))

	done := map[string]*modelprewarm.Result{"b": {OpsJobId: "j3", Phase: modelprewarm.PhaseFailed, Message: "x"}}
	require.NoError(t, r.saveExternalResults(context.Background(), job, done))
	require.NoError(t, r.saveExternalResults(context.Background(), job, done))
	assert.Equal(t, "x", loadExternalResults(job)["b"].Message)
}

// Cleaning up deletes the prewarm workloads of the job and tolerates ones that are already gone.
func TestCleanupExternalWorkloadsRemovesJobWorkloads(t *testing.T) {
	job := &v1.OpsJob{ObjectMeta: metav1.ObjectMeta{Name: "j1"}}
	workload := prewarmWorkloadOf(job, "admin-1", time.Second, v1.WorkloadRunning)
	r := &ModelPrewarmJobReconciler{OpsJobBaseReconciler: newBaseWithObjs(t, workload)}

	require.NoError(t, r.cleanupExternalWorkloads(context.Background(), job))
	err := r.Get(context.Background(), client.ObjectKeyFromObject(workload), &v1.Workload{})
	assert.True(t, apierrors.IsNotFound(err), "%v", err)
	require.NoError(t, r.cleanupExternalWorkloads(context.Background(), job))
}
