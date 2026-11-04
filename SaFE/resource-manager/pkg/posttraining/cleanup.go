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
	cleanupRetry        = 5 * time.Minute
	// checkpointsCleanedEvent is the stage_history event of a finished checkpoint cleanup.
	checkpointsCleanedEvent = "checkpoints-cleaned"
)

// checkpointCleanup is the cleanup_request column the Post Training API writes.
type checkpointCleanup struct {
	Keep     string   `json:"keep"`
	Paths    []string `json:"paths"`
	KeptStep *int     `json:"keptStep,omitempty"`
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
// counts the run directory again, stores the usage, records the cleanup and clears the request.
func (r *RunReconciler) reconcileCleanup(ctx context.Context, run *dbclient.PostTrainingRun) (ctrlruntime.Result, error) {
	var plan checkpointCleanup
	if err := json.Unmarshal([]byte(run.CleanupRequest), &plan); err != nil {
		klog.ErrorS(err, "dropped an unreadable checkpoint cleanup", "run", run.RunID)
		return ctrlruntime.Result{}, r.db.UpdatePostTrainingRun(ctx, run.RunID, map[string]interface{}{"cleanup_request": ""})
	}
	ws := &v1.Workspace{}
	if err := r.Get(ctx, client.ObjectKey{Name: run.Workspace}, ws); err != nil {
		return ctrlruntime.Result{}, err
	}
	var jobs []*v1.OpsJob
	running := false
	for _, dir := range plan.Paths {
		job := &v1.OpsJob{}
		err := r.Get(ctx, client.ObjectKey{Name: resource.DirectoryCleanupJobName(run.Workspace, dir)}, job)
		if apierrors.IsNotFound(err) {
			if job, err = resource.NewDirectoryCleanupOpsJob(ws, dir, map[string]string{v1.PostTrainingRunIdLabel: run.RunID}); err != nil {
				return ctrlruntime.Result{}, err
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
		switch job.Status.Phase {
		case v1.OpsJobSucceeded:
			jobs = append(jobs, job)
		case v1.OpsJobFailed:
			klog.Warningf("cleanup of %s for post training run %s failed, retrying", dir, run.RunID)
			return ctrlruntime.Result{RequeueAfter: cleanupRetry}, client.IgnoreNotFound(r.Delete(ctx, job))
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
		Event: checkpointsCleanedEvent, Message: fmt.Sprintf("kept: %s", plan.Keep), KeptStep: plan.KeptStep})
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

// countRunDir counts a run directory with a model-inspect job and returns the usage JSON once the job is done.
func (r *RunReconciler) countRunDir(ctx context.Context, run *dbclient.PostTrainingRun) (string, bool, error) {
	job := &v1.OpsJob{}
	err := r.Get(ctx, client.ObjectKey{Name: diskJobName(run.RunID)}, job)
	if apierrors.IsNotFound(err) {
		return "", false, client.IgnoreAlreadyExists(r.Create(ctx, newRunDirJob(run, diskJobName(run.RunID))))
	}
	if err != nil {
		return "", false, err
	}
	if job.Status.Phase != v1.OpsJobSucceeded && job.Status.Phase != v1.OpsJobFailed {
		return "", false, nil
	}
	disk := ""
	if _, res := parseVerifyJob(job); res != nil && len(res.Disk) > 0 {
		disk = string(res.Disk)
	}
	return disk, true, client.IgnoreNotFound(r.Delete(ctx, job))
}
