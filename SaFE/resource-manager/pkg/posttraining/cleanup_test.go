/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package posttraining

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/resource-manager/pkg/resource"
)

// A checkpoint cleanup deletes the planned paths, then counts the run directory and removes its jobs.
func TestCheckpointCleanup(t *testing.T) {
	ws := &v1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: "ws1"}, Spec: v1.WorkspaceSpec{Cluster: "c1",
		Volumes: []v1.WorkspaceVolume{{Type: v1.PFS, MountPath: "/wekafs"}}}}
	run := testRunRow("Succeeded", "done")
	run.OutputPath = testRunDir
	run.CleanupRequest = `{"keep":"best","paths":["` + testRunDir + `/checkpoints/iter_0000010"],"keptStep":20}`
	r, _, updates := newTestReconciler(t, run, ws)
	ctx := context.Background()

	require.Equal(t, pendingRequeue, reconcileRun(t, r).RequeueAfter)
	cleanup := &v1.OpsJob{}
	name := resource.DirectoryCleanupJobName("ws1", testRunDir+"/checkpoints/iter_0000010")
	require.NoError(t, r.Get(ctx, client.ObjectKey{Name: name}, cleanup))
	require.Equal(t, testRun, cleanup.Labels[v1.PostTrainingRunIdLabel])

	cleanup.Status.Phase = v1.OpsJobSucceeded
	require.NoError(t, r.Update(ctx, cleanup))
	reconcileRun(t, r)
	disk := &v1.OpsJob{}
	require.NoError(t, r.Get(ctx, client.ObjectKey{Name: diskJobName(testRun)}, disk), "the run directory is counted again")
	require.Equal(t, testRunDir, disk.GetParameter(v1.ParameterDestPath).Value)

	disk.Status.Phase = v1.OpsJobSucceeded
	disk.Status.Outputs = []v1.Parameter{{Name: "result", Value: `["[SUCCESS] VERIFY={\"output\":\"x\",\"disk\":{\"checkpoints\":7,\"total\":7}}"]`}}
	require.NoError(t, r.Update(ctx, disk))
	reconcileRun(t, r)
	last := (*updates)[len(*updates)-1]
	require.Equal(t, "", last["cleanup_request"])
	require.Equal(t, `{"checkpoints":7,"total":7}`, last["disk_usage"])
	require.Contains(t, last["stage_history"], `"event":"checkpoints-cleaned"`)
	require.Contains(t, last["stage_history"], `"keptStep":20`)
	jobs := &v1.OpsJobList{}
	require.NoError(t, r.List(ctx, jobs))
	require.Empty(t, jobs.Items, "finished cleanup and count jobs are removed")
}

// A cleanup removes nothing outside the run's checkpoints and waits for exports reading them.
func TestACleanupStaysInTheRunDirectoryAndWaitsForExports(t *testing.T) {
	ws := &v1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: "ws1"}, Spec: v1.WorkspaceSpec{Cluster: "c1",
		Volumes: []v1.WorkspaceVolume{{Type: v1.PFS, MountPath: "/wekafs"}}}}
	run := testRunRow("Succeeded", "done")
	run.Recipe = `{"output":{"base":"` + testRunDir + `"}}`
	run.CleanupRequest = `{"keep":"none","paths":["/wekafs/custom/models/pt-other/checkpoints"]}`
	r, _, updates := newTestReconciler(t, run, ws)
	reconcileRun(t, r)
	last := (*updates)[len(*updates)-1]
	require.Equal(t, "", last["cleanup_request"], "a path outside the run's checkpoints is never removed")
	require.Contains(t, last["stage_history"], "outside the run's checkpoints")

	export := exportJob("pt-x20-h", v1.WorkloadRunning)
	run = testRunRow("Succeeded", "done")
	run.Recipe = `{"output":{"base":"` + testRunDir + `"}}`
	run.CleanupRequest = `{"keep":"none","paths":["` + testRunDir + `/checkpoints"]}`
	r, _, _ = newTestReconciler(t, run, ws, export)
	require.Equal(t, pendingRequeue, reconcileRun(t, r).RequeueAfter)
	jobs := &v1.OpsJobList{}
	require.NoError(t, r.List(context.Background(), jobs))
	require.Empty(t, jobs.Items, "the cleanup waits for the export that reads a checkpoint")
}

// A failed cleanup job counts as one failed attempt, however often the run is reconciled while the job is deleted.
func TestAFailedCleanupJobBeingDeletedIsCountedOnce(t *testing.T) {
	ws := &v1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: "ws1"}, Spec: v1.WorkspaceSpec{Cluster: "c1",
		Volumes: []v1.WorkspaceVolume{{Type: v1.PFS, MountPath: "/wekafs"}}}}
	run := testRunRow("Succeeded", "done")
	run.CleanupRequest = `{"keep":"none","paths":["` + testRunDir + `/checkpoints/iter_0000010"]}`
	r, _, updates := newTestReconciler(t, run, ws)
	ctx := context.Background()
	reconcileRun(t, r)
	job := &v1.OpsJob{}
	name := resource.DirectoryCleanupJobName("ws1", testRunDir+"/checkpoints/iter_0000010")
	require.NoError(t, r.Get(ctx, client.ObjectKey{Name: name}, job))
	ended := metav1.NewTime(time.Now().Add(-time.Hour))
	// The OpsJob webhook's finalizer keeps a deleted job listed until its controller let it go.
	job.Finalizers = append(job.Finalizers, v1.OpsJobFinalizer)
	job.Status.Phase, job.Status.FinishedAt = v1.OpsJobFailed, &ended
	require.NoError(t, r.Update(ctx, job))

	reconcileRun(t, r)
	counted := len(*updates)
	run.CleanupRequest = (*updates)[counted-1]["cleanup_request"].(string)
	require.Contains(t, run.CleanupRequest, `"attempts":1`)
	require.NoError(t, r.Get(ctx, client.ObjectKey{Name: name}, job))
	require.False(t, job.DeletionTimestamp.IsZero(), "the failed job is being deleted")
	for i := 0; i < cleanupAttempts; i++ {
		require.Positive(t, reconcileRun(t, r).RequeueAfter)
	}
	require.Len(t, *updates, counted, "the job that is being deleted is the failure already counted")
}

// A failing cleanup is retried after a wait and dropped after its attempts, so it can be asked again.
func TestAFailingCheckpointCleanupBacksOffAndGivesUp(t *testing.T) {
	ws := &v1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: "ws1"}, Spec: v1.WorkspaceSpec{Cluster: "c1",
		Volumes: []v1.WorkspaceVolume{{Type: v1.PFS, MountPath: "/wekafs"}}}}
	run := testRunRow("Succeeded", "done")
	run.CleanupRequest = `{"keep":"none","paths":["` + testRunDir + `/checkpoints/iter_0000010"]}`
	r, _, updates := newTestReconciler(t, run, ws)
	ctx := context.Background()
	name := resource.DirectoryCleanupJobName("ws1", testRunDir+"/checkpoints/iter_0000010")
	fail := func(ago time.Duration) {
		job := &v1.OpsJob{}
		require.NoError(t, r.Get(ctx, client.ObjectKey{Name: name}, job))
		ended := metav1.NewTime(time.Now().Add(-ago))
		job.Status.Phase, job.Status.FinishedAt = v1.OpsJobFailed, &ended
		job.Status.Conditions = []metav1.Condition{{Type: "JobCompleted", Reason: "JobFailed", Message: "rm: permission denied"}}
		require.NoError(t, r.Update(ctx, job))
	}

	for attempt := 1; attempt < cleanupAttempts; attempt++ {
		reconcileRun(t, r)
		fail(time.Minute)
		res := reconcileRun(t, r)
		require.Positive(t, res.RequeueAfter)
		require.NoError(t, r.Get(ctx, client.ObjectKey{Name: name}, &v1.OpsJob{}),
			"a failed cleanup waits before it runs again, attempt %d", attempt)
		fail(time.Hour)
		reconcileRun(t, r)
		require.Error(t, r.Get(ctx, client.ObjectKey{Name: name}, &v1.OpsJob{}))
		last := (*updates)[len(*updates)-1]
		run.CleanupRequest = last["cleanup_request"].(string)
		require.Contains(t, run.CleanupRequest, fmt.Sprintf(`"attempts":%d`, attempt))
	}
	reconcileRun(t, r)
	fail(time.Hour)
	reconcileRun(t, r)
	last := (*updates)[len(*updates)-1]
	require.Equal(t, "", last["cleanup_request"], "the request is dropped so the user can ask again")
	require.Contains(t, last["stage_history"], `"event":"checkpoints-cleanup-failed"`)
	require.Contains(t, last["stage_history"], "rm: permission denied")

	gone := testRunRow("Succeeded", "done")
	gone.Workspace = "ws-gone"
	gone.CleanupRequest = `{"keep":"none","paths":["` + testRunDir + `/checkpoints/iter_0000010"]}`
	r, _, updates = newTestReconciler(t, gone)
	reconcileRun(t, r)
	require.Equal(t, "", (*updates)[len(*updates)-1]["cleanup_request"], "a cleanup in a deleted workspace cannot run")
	require.Contains(t, (*updates)[len(*updates)-1]["stage_history"], "the workspace no longer exists")
}
