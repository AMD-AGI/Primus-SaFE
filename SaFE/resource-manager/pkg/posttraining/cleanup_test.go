/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package posttraining

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrlruntime "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	dbclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
	"github.com/AMD-AIG-AIMA/SAFE/resource-manager/pkg/resource"
)

// A checkpoint cleanup deletes the planned paths, then counts the run directory and removes its jobs.
func TestCheckpointCleanup(t *testing.T) {
	ws := &v1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: "ws1"}, Spec: v1.WorkspaceSpec{Cluster: "c1",
		Volumes: []v1.WorkspaceVolume{{Type: v1.PFS, MountPath: "/wekafs"}}}}
	run := testRunRow("Succeeded", "done")
	run.OutputPath = testRunDir
	run.CleanupRequest = `{"keep":"best","paths":["` + testRunDir + `/checkpoints/iter_0000010"],"keptStep":20,` +
		`"cleanedSteps":[10]}`
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
	require.Contains(t, last["stage_history"], `"cleanedSteps":[10]`, "the event names the checkpoints removed")
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

	// The cache may not show an export job made a moment ago: the API server is asked.
	shown := r.Client
	r, _, _ = newTestReconciler(t, run, ws)
	r.reader = shown
	require.Equal(t, pendingRequeue, reconcileRun(t, r).RequeueAfter)
	require.NoError(t, r.List(context.Background(), jobs))
	require.Empty(t, jobs.Items, "the cleanup waits for an export that the cache does not show yet")
}

// A cleanup job that was deleted leaves its workload, of the same name, for a while. A job of the name made
// meanwhile would take that workload for its own and never get one, so none is made before the workload is gone;
// the same holds for the job that counts the run directory.
func TestNoCleanupJobIsMadeWhileTheWorkloadOfADeletedOneIsLeft(t *testing.T) {
	ctx := context.Background()
	ws := &v1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: "ws1"}, Spec: v1.WorkspaceSpec{Cluster: "c1",
		Volumes: []v1.WorkspaceVolume{{Type: v1.PFS, MountPath: "/wekafs"}}}}
	dir := testRunDir + "/checkpoints/iter_0000010"
	run := testRunRow("Succeeded", "done")
	run.OutputPath = testRunDir
	run.CleanupRequest = `{"keep":"none","paths":["` + dir + `"],"attempts":1}`
	left := func(name string) *v1.Workload {
		return &v1.Workload{ObjectMeta: metav1.ObjectMeta{Name: name, Finalizers: []string{v1.WorkloadFinalizer},
			DeletionTimestamp: &metav1.Time{Time: time.Now()}}}
	}

	// The retry of a failed cleanup job waits for the workload of the job that was deleted.
	cleanup := client.ObjectKey{Name: resource.DirectoryCleanupJobName("ws1", dir)}
	old := left(cleanup.Name)
	r, _, _ := newTestReconciler(t, run, ws, old)
	require.Equal(t, pendingRequeue, reconcileRun(t, r).RequeueAfter)
	require.Error(t, r.Get(ctx, cleanup, &v1.OpsJob{}), "the workload of the deleted job is still there")
	old.Finalizers = nil
	require.NoError(t, r.Update(ctx, old))
	require.Equal(t, pendingRequeue, reconcileRun(t, r).RequeueAfter)
	job := &v1.OpsJob{}
	require.NoError(t, r.Get(ctx, cleanup, job), "the job is made once the workload is gone")

	// So does the job that counts the run directory after the cleanup.
	job.Status.Phase = v1.OpsJobSucceeded
	require.NoError(t, r.Update(ctx, job))
	disk := client.ObjectKey{Name: diskJobName(testRun)}
	old = left(disk.Name)
	require.NoError(t, r.Create(ctx, old))
	require.NoError(t, r.Delete(ctx, old))
	require.Equal(t, pendingRequeue, reconcileRun(t, r).RequeueAfter)
	require.Error(t, r.Get(ctx, disk, &v1.OpsJob{}))
	require.NoError(t, r.Get(ctx, disk, old))
	old.Finalizers = nil
	require.NoError(t, r.Update(ctx, old))
	require.Equal(t, pendingRequeue, reconcileRun(t, r).RequeueAfter)
	require.NoError(t, r.Get(ctx, disk, &v1.OpsJob{}))
}

// A workload that has been deleted for longer than leftoverWait is stuck and no longer holds a job of its name
// back: the job is made. One that is not deleted yet is waited for: it goes with the job that owns it.
func TestAStuckWorkloadDoesNotHoldACleanupJobBackForever(t *testing.T) {
	ctx := context.Background()
	ws := &v1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: "ws1"}, Spec: v1.WorkspaceSpec{Cluster: "c1",
		Volumes: []v1.WorkspaceVolume{{Type: v1.PFS, MountPath: "/wekafs"}}}}
	dir := testRunDir + "/checkpoints/iter_0000010"
	cleanup := client.ObjectKey{Name: resource.DirectoryCleanupJobName("ws1", dir)}
	deleted := func(ago time.Duration) *v1.Workload {
		return &v1.Workload{ObjectMeta: metav1.ObjectMeta{Name: cleanup.Name, Finalizers: []string{v1.WorkloadFinalizer},
			DeletionTimestamp: &metav1.Time{Time: time.Now().Add(-ago)}}}
	}
	for label, c := range map[string]struct {
		left *v1.Workload
		made bool
	}{
		"deleted within the wait": {deleted(leftoverWait - time.Minute), false},
		"deleted before the wait": {deleted(leftoverWait + time.Minute), true},
		"not deleted":             {&v1.Workload{ObjectMeta: metav1.ObjectMeta{Name: cleanup.Name}}, false},
	} {
		run := testRunRow("Succeeded", "done")
		run.OutputPath = testRunDir
		run.CleanupRequest = `{"keep":"none","paths":["` + dir + `"],"attempts":1}`
		r, _, _ := newTestReconciler(t, run, ws, c.left)
		require.Equal(t, pendingRequeue, reconcileRun(t, r).RequeueAfter, label)
		require.Equal(t, c.made, r.Get(ctx, cleanup, &v1.OpsJob{}) == nil, label)
	}
}

// A workload read that fails leaves it unknown whether one holds the job back: the cleanup stops with the error and
// makes no job.
func TestAFailedReadOfTheWorkloadMakesNoCleanupJob(t *testing.T) {
	ctx := context.Background()
	ws := &v1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: "ws1"}, Spec: v1.WorkspaceSpec{Cluster: "c1",
		Volumes: []v1.WorkspaceVolume{{Type: v1.PFS, MountPath: "/wekafs"}}}}
	dir := testRunDir + "/checkpoints/iter_0000010"
	cleanup := client.ObjectKey{Name: resource.DirectoryCleanupJobName("ws1", dir)}
	run := testRunRow("Succeeded", "done")
	run.OutputPath = testRunDir
	run.CleanupRequest = `{"keep":"none","paths":["` + dir + `"],"attempts":1}`
	r, _, _ := newTestReconciler(t, run, ws)
	inner := r.Client
	r.Client = interceptor.NewClient(inner.(client.WithWatch), interceptor.Funcs{
		Get: func(ctx context.Context, c client.WithWatch, key client.ObjectKey, obj client.Object,
			opts ...client.GetOption) error {
			if _, ok := obj.(*v1.Workload); ok && key.Name == cleanup.Name {
				return errors.New("the cache is not ready")
			}
			return c.Get(ctx, key, obj, opts...)
		}})
	_, err := r.Reconcile(ctx, ctrlruntime.Request{NamespacedName: client.ObjectKey{Name: testRun}})
	require.ErrorContains(t, err, "the cache is not ready")
	require.Error(t, inner.Get(ctx, cleanup, &v1.OpsJob{}))
}

// A cleanup job that is unfinished after its timeout, as when its controller lost the timer of it in a restart, has
// failed: it counts as an attempt once cleanupRetry has passed since the timeout. The job that counts the run
// directory is over at its timeout, without a count.
func TestAJobLeftUnfinishedPastItsTimeoutHasFailed(t *testing.T) {
	ctx := context.Background()
	ws := &v1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: "ws1"}, Spec: v1.WorkspaceSpec{Cluster: "c1",
		Volumes: []v1.WorkspaceVolume{{Type: v1.PFS, MountPath: "/wekafs"}}}}
	dir := testRunDir + "/checkpoints/iter_0000010"
	cleanup := client.ObjectKey{Name: resource.DirectoryCleanupJobName("ws1", dir)}
	aged := func(name string, timeout, age time.Duration) *v1.OpsJob {
		return &v1.OpsJob{ObjectMeta: metav1.ObjectMeta{Name: name,
			CreationTimestamp: metav1.NewTime(time.Now().Add(-age))},
			Spec: v1.OpsJobSpec{TimeoutSecond: int(timeout / time.Second)}}
	}
	cleaning := func() *dbclient.PostTrainingRun {
		run := testRunRow("Succeeded", "done")
		run.OutputPath = testRunDir
		run.CleanupRequest = `{"keep":"none","paths":["` + dir + `"],"attempts":1}`
		return run
	}
	for label, c := range map[string]struct {
		age     time.Duration
		counted bool
	}{
		"within its timeout":            {time.Hour - time.Minute, false},
		"within the wait after it":      {time.Hour + cleanupRetry - time.Minute, false},
		"past the timeout and the wait": {time.Hour + cleanupRetry + time.Minute, true},
	} {
		r, _, updates := newTestReconciler(t, cleaning(), ws, aged(cleanup.Name, time.Hour, c.age))
		require.Positive(t, reconcileRun(t, r).RequeueAfter, label)
		require.Equal(t, c.counted, r.Get(ctx, cleanup, &v1.OpsJob{}) != nil, "%s: the job is deleted", label)
		if c.counted {
			require.Len(t, *updates, 1, label)
			require.Contains(t, (*updates)[0]["cleanup_request"], `"attempts":2`, label)
		} else {
			require.Empty(t, *updates, label)
		}
	}

	cleaned := aged(cleanup.Name, time.Hour, time.Minute)
	cleaned.Status.Phase = v1.OpsJobSucceeded
	disk := client.ObjectKey{Name: diskJobName(testRun)}
	r, _, updates := newTestReconciler(t, cleaning(), ws, cleaned,
		aged(disk.Name, verifyTimeout, verifyTimeout+time.Minute))
	require.Zero(t, reconcileRun(t, r).RequeueAfter)
	require.Error(t, r.Get(ctx, disk, &v1.OpsJob{}))
	require.Equal(t, "", (*updates)[len(*updates)-1]["cleanup_request"], "the cleanup ends without a new count")
}

// A failed cleanup job counts as one failed attempt, however often the run is reconciled while the job is deleted.
func TestAFailedCleanupJobBeingDeletedIsCountedOnce(t *testing.T) {
	ws := &v1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: "ws1"}, Spec: v1.WorkspaceSpec{Cluster: "c1",
		Volumes: []v1.WorkspaceVolume{{Type: v1.PFS, MountPath: "/wekafs"}}}}
	run := testRunRow("Succeeded", "done")
	run.CleanupRequest = `{"keep":"best","paths":["` + testRunDir + `/checkpoints/iter_0000010"],"keptStep":20,` +
		`"cleanedSteps":[10]}`
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
	require.Contains(t, run.CleanupRequest, `"cleanedSteps":[10]`, "the retried request keeps its steps")
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
