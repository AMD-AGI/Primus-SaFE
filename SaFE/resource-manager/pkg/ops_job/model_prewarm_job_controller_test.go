/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package ops_job

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	ctrlruntime "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	commonconfig "github.com/AMD-AIG-AIMA/SAFE/common/pkg/config"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/k8sclient"
	modelprewarm "github.com/AMD-AIG-AIMA/SAFE/common/pkg/model_prewarm"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/utils"
)

func TestModelPrewarmReconcileEntry(t *testing.T) {
	job := &v1.OpsJob{
		ObjectMeta: metav1.ObjectMeta{Name: "j1", Finalizers: []string{v1.OpsJobFinalizer}},
		Spec: v1.OpsJobSpec{
			Type: v1.OpsJobModelPrewarmType,
			Inputs: []v1.Parameter{
				{Name: v1.ParameterModelPath, Value: "/models/glm"},
				{Name: v1.ParameterNode, Value: "node-1"},
			},
		},
	}
	r := &ModelPrewarmJobReconciler{OpsJobBaseReconciler: newBaseWithObjs(t, job)}
	_, err := r.Reconcile(context.Background(), ctrlruntime.Request{NamespacedName: types.NamespacedName{Name: "j1"}})
	assert.NoError(t, err)
}

// runPrewarmPod runs the entry script of a prewarm pod on this host and returns the pod phase and the
// termination message the script wrote.
func runPrewarmPod(t *testing.T, modelPath string) (v1.WorkloadPhase, string) {
	t.Helper()
	terminationLog := filepath.Join(t.TempDir(), "termination-log")
	script := modelprewarm.PodEntryScript(modelPath, modelprewarm.DefaultGlob, 2)
	script = strings.ReplaceAll(script, "/dev/termination-log", terminationLog)
	phase := v1.WorkloadSucceeded
	if err := exec.Command("bash", "-c", script).Run(); err != nil {
		phase = v1.WorkloadFailed
	}
	message, err := os.ReadFile(terminationLog)
	require.NoError(t, err)
	return phase, string(message)
}

// externalPrewarmResult runs the prewarm pod of a simulated external node and reads back its result.
func externalPrewarmResult(t *testing.T, modelPath string) *modelprewarm.Result {
	t.Helper()
	job := &v1.OpsJob{ObjectMeta: metav1.ObjectMeta{Name: "j1"}}
	phase, message := runPrewarmPod(t, modelPath)
	workload := &v1.Workload{
		ObjectMeta: metav1.ObjectMeta{
			Name:        externalWorkloadName(job.Name, "admin-1"),
			Annotations: map[string]string{v1.MainContainerAnnotation: "main"},
		},
		Status: v1.WorkloadStatus{
			Phase: phase,
			Pods: []v1.WorkloadPod{{
				PodId:      "pod-1",
				Containers: []v1.Container{{Name: "main", Message: message}},
			}},
		},
	}
	r := &ModelPrewarmJobReconciler{OpsJobBaseReconciler: newBaseWithObjs(t, workload)}
	result := r.externalWorkloadResult(context.Background(), job, workload, nil)
	require.NotNil(t, result)
	return result
}

func writeWeightFile(t *testing.T, path string, size int) {
	t.Helper()
	require.NoError(t, os.WriteFile(path, bytes.Repeat([]byte("w"), size), 0o600))
}

func TestExternalPrewarmReportsBytesReadOnSuccess(t *testing.T) {
	dir := t.TempDir()
	writeWeightFile(t, filepath.Join(dir, "a.safetensors"), 1000)
	writeWeightFile(t, filepath.Join(dir, "b.safetensors"), 2345)

	result := externalPrewarmResult(t, dir)
	assert.Equal(t, modelprewarm.PhaseSucceeded, result.Phase)
	assert.Equal(t, int64(3345), result.BytesRead)
}

func TestExternalPrewarmFailsWhenNoWeightBytesRead(t *testing.T) {
	dir := t.TempDir()
	writeWeightFile(t, filepath.Join(dir, "a.safetensors"), 0)

	result := externalPrewarmResult(t, dir)
	assert.Equal(t, modelprewarm.PhaseFailed, result.Phase)
	assert.Contains(t, result.Message, "no weight bytes")
}

func TestExternalPrewarmFailsWithReasonOnMissingModelPath(t *testing.T) {
	result := externalPrewarmResult(t, filepath.Join(t.TempDir(), "missing"))
	assert.Equal(t, modelprewarm.PhaseFailed, result.Phase)
	assert.Contains(t, result.Message, "model path not found")
}

func TestPrewarmHostPathMountsDeclaredRoot(t *testing.T) {
	workspace := &v1.Workspace{
		ObjectMeta: metav1.ObjectMeta{Name: "ws"},
		Spec: v1.WorkspaceSpec{Volumes: []v1.WorkspaceVolume{
			{Id: 1, Type: v1.HOSTPATH, HostPath: "/data", MountPath: "/data"},
		}},
	}
	workload := &v1.Workload{Spec: v1.WorkloadSpec{
		GroupVersionKind: v1.GroupVersionKind{Kind: "Job"},
	}}

	path, err := prewarmHostPath(workspace, workload, "/data/models/glm")
	require.NoError(t, err)
	assert.Equal(t, "/data", path)

	path, err = prewarmHostPath(workspace, workload, "/data")
	require.NoError(t, err)
	assert.Equal(t, "/data", path)

	_, err = prewarmHostPath(workspace, workload, "/other/models")
	assert.Error(t, err)
}

// The prewarm workload of an external node opts out of user directory mounts, so its pod mounts the
// declared root even when external_execution.user_dir_mounts is on for the cluster.
func TestExternalPrewarmWorkloadOptsOutOfUserDirMounts(t *testing.T) {
	const key = "external_execution.user_dir_mounts"
	commonconfig.SetValue(key, "true")
	t.Cleanup(func() { commonconfig.UnsetValue(key) })

	hostname := "host-1"
	workspace := &v1.Workspace{
		ObjectMeta: metav1.ObjectMeta{Name: "ws", Labels: map[string]string{v1.WorkspaceExternalLabel: v1.TrueStr}},
		Spec: v1.WorkspaceSpec{Volumes: []v1.WorkspaceVolume{
			{Id: 1, Type: v1.HOSTPATH, HostPath: "/data", MountPath: "/data"},
		}},
	}
	node := &v1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "ext-1", Labels: map[string]string{v1.WorkspaceIdLabel: "ws"}},
		Spec: v1.NodeSpec{
			Hostname:      &hostname,
			LifecycleMode: v1.NodeLifecycleExternal,
			ExternalRef:   &v1.NodeExternalRef{Provider: "autopilot", AllocationId: "alloc-1"},
		},
	}
	job := &v1.OpsJob{ObjectMeta: metav1.ObjectMeta{Name: "j1"}, Spec: v1.OpsJobSpec{Type: v1.OpsJobModelPrewarmType}}
	r := &ModelPrewarmJobReconciler{OpsJobBaseReconciler: newBaseWithObjs(t, workspace, node, job)}
	cfg := &modelPrewarmConfig{modelPath: "/data/models/glm", glob: modelprewarm.DefaultGlob, parallelism: 2}

	workload, err := r.generateExternalWorkload(context.Background(), job, cfg, node, "j1-abc")
	require.NoError(t, err)
	assert.Equal(t, v1.TrueStr, v1.GetAnnotation(workload, v1.WorkloadUserDirMountsOffAnnotation))
	assert.Equal(t, []string{"/data"}, workload.Spec.Hostpath)
}

// The prewarm workload name must be a DNS label of at most MaxGeneratedNameLength characters, whatever
// the OpsJob name is, and two job names that differ only in their separators must not share a name.
func TestExternalWorkloadNameIsDNSLabel(t *testing.T) {
	nameRe := regexp.MustCompile(`^[a-z]([-a-z0-9]*[a-z0-9])?$`)
	const node = "autopilot-crsuse2-m2m-v3-027-c76cc2f0-g1"
	for _, jobName := range []string{"prewarm-glm-5.3-l6llf", "A.B_c", "1abc", strings.Repeat("x.", 40)} {
		name := externalWorkloadName(jobName, node)
		assert.LessOrEqual(t, len(name), utils.MaxGeneratedNameLength, jobName)
		assert.Regexp(t, nameRe, name, jobName)
	}
	assert.NotEqual(t, externalWorkloadName("a.b", node), externalWorkloadName("a-b", node))
}

// pendingPrewarmPod returns a pending prewarm pod of the workload, created age ago. An unscheduled pod
// states why in its PodScheduled condition; a scheduled one has that condition true since schedAt.
func pendingPrewarmPod(workloadName string, age time.Duration, scheduled corev1.ConditionStatus,
	schedAt time.Time) *corev1.Pod {
	message := ""
	if scheduled == corev1.ConditionFalse {
		message = "0/3 nodes are available: insufficient cpu"
	}
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:              workloadName + "-pod",
			Namespace:         "ws1",
			CreationTimestamp: metav1.NewTime(time.Now().Add(-age)),
			Labels:            map[string]string{v1.WorkloadIdLabel: workloadName},
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodPending,
			Conditions: []corev1.PodCondition{{
				Type: corev1.PodScheduled, Status: scheduled, Message: message,
				LastTransitionTime: metav1.NewTime(schedAt),
			}},
		},
	}
}

// prewarmWorkloadOf returns the prewarm workload of the node, created age ago in phase.
func prewarmWorkloadOf(job *v1.OpsJob, node string, age time.Duration, phase v1.WorkloadPhase) *v1.Workload {
	return &v1.Workload{
		ObjectMeta: metav1.ObjectMeta{
			Name:              externalWorkloadName(job.Name, node),
			Namespace:         "ws1",
			CreationTimestamp: metav1.NewTime(time.Now().Add(-age)),
			Labels:            map[string]string{v1.OpsJobIdLabel: job.Name},
		},
		Spec:   v1.WorkloadSpec{Workspace: "ws1"},
		Status: v1.WorkloadStatus{Phase: phase},
	}
}

// pendingResultReconciler returns a reconciler and the cluster clients holding the pods and events.
func pendingResultReconciler(t *testing.T, objs []client.Object, pods ...*corev1.Pod) (
	*ModelPrewarmJobReconciler, *k8sclient.ClientFactory) {
	t.Helper()
	cs := k8sfake.NewSimpleClientset()
	for _, pod := range pods {
		_, err := cs.CoreV1().Pods("ws1").Create(context.Background(), pod, metav1.CreateOptions{})
		require.NoError(t, err)
	}
	clients := k8sclient.NewClientFactoryWithOnlyClient(context.Background(), "c1", cs)
	return &ModelPrewarmJobReconciler{OpsJobBaseReconciler: newBaseWithObjs(t, objs...)}, clients
}

// setPendingTimeout configures the pending timeout of prewarm pods for the test.
func setPendingTimeout(t *testing.T, seconds int) {
	const key = "ops_job.model_prewarm.pending_timeout_second"
	commonconfig.SetValue(key, strconv.Itoa(seconds))
	t.Cleanup(func() { commonconfig.UnsetValue(key) })
}

// By default there is no pending timeout: a pod that stays unscheduled does not fail its node, and
// its workload is kept, so the job timeout is what bounds the wait.
func TestExternalPendingWithoutTimeoutWaitsIndefinitely(t *testing.T) {
	job := &v1.OpsJob{ObjectMeta: metav1.ObjectMeta{Name: "j1"}}
	workload := prewarmWorkloadOf(job, "admin-1", 24*time.Hour, v1.WorkloadPending)
	pod := pendingPrewarmPod(workload.Name, 24*time.Hour, corev1.ConditionFalse, time.Now())
	r, clients := pendingResultReconciler(t, []client.Object{workload}, pod)

	assert.Nil(t, r.externalWorkloadResult(context.Background(), job, workload, clients))
	assert.NoError(t, r.Get(context.Background(), client.ObjectKeyFromObject(workload), &v1.Workload{}))
}

// A pod that stays unscheduled past the pending timeout fails its node even when the workload reports
// Running, because an active job with a pending pod is not running.
func TestExternalPendingPodFailsWhileWorkloadReportsRunning(t *testing.T) {
	setPendingTimeout(t, 300)
	job := &v1.OpsJob{ObjectMeta: metav1.ObjectMeta{Name: "j1"}}
	workload := prewarmWorkloadOf(job, "admin-1", 400*time.Second, v1.WorkloadRunning)
	pod := pendingPrewarmPod(workload.Name, 400*time.Second, corev1.ConditionFalse, time.Now())
	r, clients := pendingResultReconciler(t, []client.Object{workload}, pod)

	result := r.externalWorkloadResult(context.Background(), job, workload, clients)
	require.NotNil(t, result)
	assert.Equal(t, modelprewarm.PhaseFailed, result.Phase)
	assert.Contains(t, result.Message, "insufficient cpu")
}

// The pending timeout counts from the pod creation, so time a workload spends queued is not counted.
func TestExternalPendingTimeoutCountsFromPodCreation(t *testing.T) {
	setPendingTimeout(t, 300)
	job := &v1.OpsJob{ObjectMeta: metav1.ObjectMeta{Name: "j1"}}
	workload := prewarmWorkloadOf(job, "admin-1", 3600*time.Second, v1.WorkloadPending)
	pod := pendingPrewarmPod(workload.Name, 10*time.Second, corev1.ConditionFalse, time.Now())
	r, clients := pendingResultReconciler(t, []client.Object{workload}, pod)

	assert.Nil(t, r.externalWorkloadResult(context.Background(), job, workload, clients))
}

// Once the pod is scheduled, a warning from before the scheduling is not the reason it is still pending.
func TestExternalPendingEventsBeforeScheduleAreIgnored(t *testing.T) {
	setPendingTimeout(t, 300)
	job := &v1.OpsJob{ObjectMeta: metav1.ObjectMeta{Name: "j1"}}
	workload := prewarmWorkloadOf(job, "admin-1", 400*time.Second, v1.WorkloadPending)
	scheduledAt := time.Now().Add(-380 * time.Second)
	pod := pendingPrewarmPod(workload.Name, 400*time.Second, corev1.ConditionTrue, scheduledAt)
	event := &corev1.Event{
		ObjectMeta:     metav1.ObjectMeta{Name: "old", Namespace: "ws1"},
		InvolvedObject: corev1.ObjectReference{Name: pod.Name},
		Type:           corev1.EventTypeWarning,
		Message:        "0/3 nodes are available",
		LastTimestamp:  metav1.NewTime(time.Now().Add(-390 * time.Second)),
	}
	r, clients := pendingResultReconciler(t, []client.Object{workload, event}, pod)
	// Events are not in the workload objects; they are created in the clientset below.
	_, err := clients.ClientSet().CoreV1().Events("ws1").Create(context.Background(), event, metav1.CreateOptions{})
	require.NoError(t, err)

	assert.Nil(t, r.externalWorkloadResult(context.Background(), job, workload, clients))
}

// A node whose pod timed out has its workload deleted, so the execution stops before the slot is released.
func TestExternalPendingTimeoutDeletesWorkload(t *testing.T) {
	setPendingTimeout(t, 300)
	job := &v1.OpsJob{ObjectMeta: metav1.ObjectMeta{Name: "j1"}}
	workload := prewarmWorkloadOf(job, "admin-1", 400*time.Second, v1.WorkloadPending)
	pod := pendingPrewarmPod(workload.Name, 400*time.Second, corev1.ConditionFalse, time.Now())
	r, clients := pendingResultReconciler(t, []client.Object{workload}, pod)

	result := r.externalWorkloadResult(context.Background(), job, workload, clients)
	require.NotNil(t, result)
	assert.Equal(t, modelprewarm.PhaseFailed, result.Phase)
	err := r.Get(context.Background(), client.ObjectKeyFromObject(workload), &v1.Workload{})
	assert.True(t, apierrors.IsNotFound(err), "%v", err)
}

// A node whose result is persisted on the job is neither redispatched nor recomputed, even after its
// workload is gone.
func TestExternalSyncDoesNotRedispatchPersistedResult(t *testing.T) {
	job := &v1.OpsJob{
		ObjectMeta: metav1.ObjectMeta{Name: "j1", Annotations: map[string]string{
			v1.OpsJobModelPrewarmExternalResultsAnnotation: `{"admin-1":{"OpsJobId":"j1","Phase":"Failed","Message":"done"}}`,
		}},
		Spec: v1.OpsJobSpec{Inputs: []v1.Parameter{
			{Name: v1.ParameterModelPath, Value: "/shared_nfs/models/m"},
		}},
	}
	node := &v1.Node{ObjectMeta: metav1.ObjectMeta{Name: "admin-1"}}
	r, clients := pendingResultReconciler(t, nil)

	results := r.syncExternalWorkloads(context.Background(), job, clients, []*v1.Node{node})
	require.NotNil(t, results["admin-1"])
	assert.Equal(t, "done", results["admin-1"].Message)
	workloads := &v1.WorkloadList{}
	require.NoError(t, r.List(context.Background(), workloads))
	assert.Empty(t, workloads.Items)
}

// Workloads of an ended job are deleted, so a timed-out job does not keep running its nodes.
func TestExternalWorkloadsCleanedWhenJobEnds(t *testing.T) {
	job := &v1.OpsJob{ObjectMeta: metav1.ObjectMeta{Name: "j1"}}
	finished := metav1.NewTime(time.Now())
	job.Status.Phase = v1.OpsJobFailed
	job.Status.FinishedAt = &finished
	workload := prewarmWorkloadOf(job, "admin-1", 100*time.Second, v1.WorkloadRunning)
	r := &ModelPrewarmJobReconciler{OpsJobBaseReconciler: newBaseWithObjs(t, workload)}

	quit, err := r.observe(context.Background(), job)
	require.NoError(t, err)
	assert.True(t, quit)
	err = r.Get(context.Background(), client.ObjectKeyFromObject(workload), &v1.Workload{})
	assert.True(t, apierrors.IsNotFound(err), "%v", err)
}

// Only terminal node results are persisted on the job, and they read back as they were saved.
func TestExternalResultsPersistOnlyTerminalPhases(t *testing.T) {
	job := &v1.OpsJob{ObjectMeta: metav1.ObjectMeta{Name: "j1"}}
	r := &ModelPrewarmJobReconciler{OpsJobBaseReconciler: newBaseWithObjs(t, job)}
	results := map[string]*modelprewarm.Result{
		"done":    {OpsJobId: "j1", Phase: modelprewarm.PhaseSucceeded, BytesRead: 7},
		"running": {OpsJobId: "j1", Phase: modelprewarm.PhaseRunning},
	}
	require.NoError(t, r.saveExternalResults(context.Background(), job, results))

	stored := &v1.OpsJob{}
	require.NoError(t, r.Get(context.Background(), client.ObjectKeyFromObject(job), stored))
	loaded := loadExternalResults(stored)
	assert.Len(t, loaded, 1)
	require.NotNil(t, loaded["done"])
	assert.Equal(t, int64(7), loaded["done"].BytesRead)
}
