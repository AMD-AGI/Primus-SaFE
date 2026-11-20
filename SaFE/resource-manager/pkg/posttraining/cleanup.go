/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package posttraining

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"path/filepath"
	"regexp"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
	ctrlruntime "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	dbclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
	"github.com/AMD-AIG-AIMA/SAFE/resource-manager/pkg/resource"
)

const (
	cleanupPollInterval = time.Minute
	// A failed cleanup job is kept for cleanupRetry before the cleanup runs again, and a cleanup that failed
	// cleanupAttempts times is given up.
	cleanupRetry    = 5 * time.Minute
	cleanupAttempts = 3
	// checkpointsCleanedEvent is the stage_history event of a finished checkpoint cleanup, and
	// checkpointsCleanupFailedEvent the one of a cleanup given up, with the reason.
	checkpointsCleanedEvent       = "checkpoints-cleaned"
	checkpointsCleanupFailedEvent = "checkpoints-cleanup-failed"
	// leftoverWait is how long the workload of a deleted job may take to go before a job of its name is made
	// all the same.
	leftoverWait = 10 * time.Minute
)

// iterDirPattern is the name of one saved iteration in a checkpoint directory.
var iterDirPattern = regexp.MustCompile(`^iter_\d{7}$`)

// checkpointCleanup is the cleanup_request column the Post Training API writes; the controller counts the failed
// attempts in it.
type checkpointCleanup struct {
	Keep     string   `json:"keep"`
	Paths    []string `json:"paths"`
	KeptStep *int     `json:"keptStep,omitempty"`
	// CleanedSteps are the steps of the checkpoints in Paths, for a cleanup that keeps one.
	CleanedSteps []int `json:"cleanedSteps,omitempty"`
	Attempts     int   `json:"attempts,omitempty"`
}

// runCleanupPolls queues the runs with a requested checkpoint cleanup every minute until ctx ends; the API only
// writes the request to the database.
func (r *RunReconciler) runCleanupPolls(ctx context.Context) error {
	ticker := time.NewTicker(cleanupPollInterval)
	defer ticker.Stop()
	for {
		runs, _, err := r.db.ListPostTrainingRuns(ctx, dbclient.PostTrainingRunFilter{CleanupRequested: true})
		if err != nil {
			klog.ErrorS(err, "failed to list the Post Training runs to clean up")
		}
		for _, run := range runs {
			obj := &metav1.PartialObjectMetadata{ObjectMeta: metav1.ObjectMeta{Name: run.RunID}}
			select {
			case r.recovered <- event.GenericEvent{Object: obj}:
			default:
			}
		}
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}

// diskJobName is the name of the job that counts a run directory's usage after a checkpoint cleanup.
func diskJobName(runID string) string {
	sum := sha256.Sum256([]byte(runID))
	return "pt-disk-" + hex.EncodeToString(sum[:8])
}

// reconcileCleanup removes the directories of a requested checkpoint cleanup with model-cleanup jobs, then
// counts the run directory again, stores the usage, records the cleanup and clears the request. A failed job is
// tried again after cleanupRetry; a cleanup that cannot run, or failed cleanupAttempts times, is given up.
func (r *RunReconciler) reconcileCleanup(ctx context.Context, run *dbclient.PostTrainingRun) (ctrlruntime.Result, error) {
	var plan checkpointCleanup
	if err := json.Unmarshal([]byte(run.CleanupRequest), &plan); err != nil {
		klog.ErrorS(err, "dropped an unreadable checkpoint cleanup", "run", run.RunID)
		return ctrlruntime.Result{}, r.db.UpdatePostTrainingRun(ctx, run.RunID, map[string]interface{}{"cleanup_request": ""})
	}
	for _, dir := range plan.Paths {
		if !cleanupPathAllowed(dir, parseRecipe(run).Output.Base) {
			return ctrlruntime.Result{}, r.giveUpCleanup(ctx, run, plan,
				fmt.Sprintf("%s is outside the run's checkpoints", dir))
		}
	}
	if exporting, err := r.exporting(ctx, run.RunID); err != nil || exporting {
		return ctrlruntime.Result{RequeueAfter: pendingRequeue}, err
	}
	ws := &v1.Workspace{}
	if err := r.Get(ctx, client.ObjectKey{Name: run.Workspace}, ws); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrlruntime.Result{}, r.giveUpCleanup(ctx, run, plan, "the workspace no longer exists")
		}
		return ctrlruntime.Result{}, err
	}
	var jobs []*v1.OpsJob
	running := false
	for _, dir := range plan.Paths {
		job := &v1.OpsJob{}
		name := resource.DirectoryCleanupJobName(run.Workspace, dir)
		err := r.Get(ctx, client.ObjectKey{Name: name}, job)
		if apierrors.IsNotFound(err) {
			if left, werr := r.jobWorkloadLeft(ctx, name); werr != nil {
				return ctrlruntime.Result{}, werr
			} else if left {
				running = true
				continue
			}
			if job, err = resource.NewDirectoryCleanupOpsJob(ws, dir, map[string]string{v1.PostTrainingRunIdLabel: run.RunID}); err != nil {
				return ctrlruntime.Result{}, r.giveUpCleanup(ctx, run, plan, err.Error())
			}
			if err = r.Create(ctx, job); client.IgnoreAlreadyExists(err) != nil {
				return ctrlruntime.Result{}, err
			}
			running = true
			continue
		}
		if err != nil {
			return ctrlruntime.Result{}, err
		}
		if !job.GetDeletionTimestamp().IsZero() {
			// The job's finalizer keeps it listed for a while; its outcome was handled when it was deleted.
			running = true
			continue
		}
		switch {
		case job.Status.Phase == v1.OpsJobSucceeded:
			jobs = append(jobs, job)
		// A job that is unfinished after its timeout has failed: its controller keeps the timer of the timeout
		// in memory and loses it in a restart.
		case job.Status.Phase == v1.OpsJobFailed, job.IsTimeout():
			if wait := cleanupRetry - time.Since(jobEnd(job)); wait > 0 {
				return ctrlruntime.Result{RequeueAfter: wait}, nil
			}
			if plan.Attempts++; plan.Attempts >= cleanupAttempts {
				return ctrlruntime.Result{}, r.giveUpCleanup(ctx, run, plan, jobFailure(job))
			}
			klog.Warningf("cleanup of %s for post training run %s failed, retrying", dir, run.RunID)
			data, _ := json.Marshal(plan)
			if err = r.db.UpdatePostTrainingRun(ctx, run.RunID, map[string]interface{}{"cleanup_request": string(data)}); err != nil {
				return ctrlruntime.Result{}, err
			}
			return ctrlruntime.Result{RequeueAfter: time.Second}, client.IgnoreNotFound(r.Delete(ctx, job))
		default:
			running = true
		}
	}
	if running {
		return ctrlruntime.Result{RequeueAfter: pendingRequeue}, nil
	}
	disk, done, err := r.countRunDir(ctx, run)
	if err != nil || !done {
		return ctrlruntime.Result{RequeueAfter: pendingRequeue}, err
	}
	u := newRunUpdate(run)
	u.history = append(u.history, stageEntry{Stage: run.Stage, Time: u.now.Format(time.RFC3339),
		Event: checkpointsCleanedEvent, Message: fmt.Sprintf("kept: %s", plan.Keep), KeptStep: plan.KeptStep,
		CleanedSteps: plan.CleanedSteps})
	u.historyChanged = true
	u.fields["cleanup_request"] = ""
	if disk != "" {
		u.set("disk_usage", run.DiskUsage, disk)
	}
	if err = r.save(ctx, u); err != nil {
		return ctrlruntime.Result{}, err
	}
	for _, job := range jobs {
		if err = r.Delete(ctx, job); client.IgnoreNotFound(err) != nil {
			klog.ErrorS(err, "failed to delete a finished cleanup job", "job", job.Name)
		}
	}
	return ctrlruntime.Result{}, nil
}

// cleanupPathAllowed reports whether a cleanup path is a checkpoint directory of the run under base, or one
// iteration in it.
func cleanupPathAllowed(path, base string) bool {
	if base == "" {
		return false
	}
	path, base = filepath.Clean(path), filepath.Clean(base)
	for _, sub := range []string{"checkpoints", "critic_checkpoints"} {
		dir := filepath.Join(base, sub)
		if path == dir || (filepath.Dir(path) == dir && iterDirPattern.MatchString(filepath.Base(path))) {
			return true
		}
	}
	return false
}

// exporting reports whether a checkpoint export of the run has not finished, as it reads a checkpoint. It asks the
// API server: the cache may not show an export job made a moment ago, which the cleanup would delete under.
func (r *RunReconciler) exporting(ctx context.Context, runID string) (bool, error) {
	jobs := &v1.WorkloadList{}
	if err := r.reader.List(ctx, jobs, client.MatchingLabels{v1.PostTrainingRunIdLabel: runID,
		v1.PostTrainingRoleLabel: v1.PostTrainingRoleExport}); err != nil {
		return false, err
	}
	for i := range jobs.Items {
		if !jobs.Items[i].IsEnd() {
			return true, nil
		}
	}
	return false, nil
}

// giveUpCleanup drops a checkpoint cleanup that cannot finish: it records why in the stage history, clears the
// request so another can be made, and removes the cleanup's jobs.
func (r *RunReconciler) giveUpCleanup(ctx context.Context, run *dbclient.PostTrainingRun, plan checkpointCleanup, reason string) error {
	klog.Warningf("gave up the checkpoint cleanup of post training run %s: %s", run.RunID, reason)
	u := newRunUpdate(run)
	u.history = append(u.history, stageEntry{Stage: run.Stage, Time: u.now.Format(time.RFC3339),
		Event: checkpointsCleanupFailedEvent, Message: reason})
	u.historyChanged = true
	u.fields["cleanup_request"] = ""
	if err := r.save(ctx, u); err != nil {
		return err
	}
	for _, dir := range plan.Paths {
		job := &v1.OpsJob{ObjectMeta: metav1.ObjectMeta{Name: resource.DirectoryCleanupJobName(run.Workspace, dir)}}
		if err := r.Delete(ctx, job); client.IgnoreNotFound(err) != nil {
			klog.ErrorS(err, "failed to delete a cleanup job", "job", job.Name)
		}
	}
	return nil
}

// jobEnd returns when an OpsJob finished; for one with no finish time, when its timeout ran out, or when it was
// created if it has not.
func jobEnd(job *v1.OpsJob) time.Time {
	if job.Status.FinishedAt != nil {
		return job.Status.FinishedAt.Time
	}
	if job.IsTimeout() {
		return job.CreationTimestamp.Add(time.Duration(job.Spec.TimeoutSecond) * time.Second)
	}
	return job.CreationTimestamp.Time
}

// jobFailure returns the reason a failed OpsJob gives: the message of its failed completion condition.
func jobFailure(job *v1.OpsJob) string {
	for _, c := range job.Status.Conditions {
		if c.Reason == "JobFailed" && c.Message != "" {
			return c.Message
		}
	}
	return "the cleanup job failed"
}

// jobWorkloadLeft reports whether a workload of an OpsJob's name still holds a job of that name back. A job that
// was deleted leaves its workload, of the same name, until that is torn down, and a job of the name made meanwhile
// would take that workload for its own and never get one: it is made once the workload is gone. A workload that
// has been deleted for longer than leftoverWait is stuck and holds nothing back: the job is made and ends on its
// timeout.
func (r *RunReconciler) jobWorkloadLeft(ctx context.Context, name string) (bool, error) {
	w := &v1.Workload{}
	if err := r.Get(ctx, client.ObjectKey{Name: name}, w); err != nil {
		return false, client.IgnoreNotFound(err)
	}
	return w.GetDeletionTimestamp().IsZero() || time.Since(w.GetDeletionTimestamp().Time) < leftoverWait, nil
}

// countRunDir counts a run directory with a model-inspect job and returns the usage JSON once the job is done.
func (r *RunReconciler) countRunDir(ctx context.Context, run *dbclient.PostTrainingRun) (string, bool, error) {
	job := &v1.OpsJob{}
	err := r.Get(ctx, client.ObjectKey{Name: diskJobName(run.RunID)}, job)
	if apierrors.IsNotFound(err) {
		if left, werr := r.jobWorkloadLeft(ctx, diskJobName(run.RunID)); werr != nil || left {
			return "", false, werr
		}
		return "", false, client.IgnoreAlreadyExists(r.Create(ctx, newRunDirJob(run, diskJobName(run.RunID))))
	}
	if err != nil {
		return "", false, err
	}
	if job.Status.Phase != v1.OpsJobSucceeded && job.Status.Phase != v1.OpsJobFailed && !job.IsTimeout() {
		return "", false, nil
	}
	disk := ""
	if _, res := parseVerifyJob(job); res != nil && len(res.Disk) > 0 {
		disk = string(res.Disk)
	}
	return disk, true, client.IgnoreNotFound(r.Delete(ctx, job))
}
