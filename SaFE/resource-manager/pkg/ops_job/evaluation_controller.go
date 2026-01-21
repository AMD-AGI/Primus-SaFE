/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package ops_job

import (
	"context"

	"k8s.io/klog/v2"
	ctrlruntime "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	commonconfig "github.com/AMD-AIG-AIMA/SAFE/common/pkg/config"
	dbclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
)

// EvaluationJobController watches OpsJob resources and updates evaluation task status in database.
type EvaluationJobController struct {
	client.Client
	dbClient dbclient.Interface
}

// SetupEvaluationJobController initializes and registers the EvaluationJobController with the controller manager.
func SetupEvaluationJobController(ctx context.Context, mgr manager.Manager) error {
	// Only setup if database is enabled
	if !commonconfig.IsDBEnable() {
		klog.Info("Database is not enabled, skipping EvaluationJobController setup")
		return nil
	}

	dbClient := dbclient.NewClient()
	if dbClient == nil {
		klog.Warning("Failed to create database client, skipping EvaluationJobController setup")
		return nil
	}

	r := &EvaluationJobController{
		Client:   mgr.GetClient(),
		dbClient: dbClient,
	}

	// Watch OpsJob with predicate to filter evaluation-related OpsJobs
	err := ctrlruntime.NewControllerManagedBy(mgr).
		For(&v1.OpsJob{}, builder.WithPredicates(
			predicate.And(
				evaluationOpsJobPredicate(),
				predicate.Or(
					predicate.GenerationChangedPredicate{},
					evaluationOpsJobPhaseChangedPredicate(),
				),
			),
		)).
		Complete(r)
	if err != nil {
		return err
	}

	klog.Info("Setup EvaluationJobController successfully")
	return nil
}

// evaluationOpsJobPredicate filters OpsJobs that have evaluation-task-id label
func evaluationOpsJobPredicate() predicate.Predicate {
	return predicate.NewPredicateFuncs(func(obj client.Object) bool {
		labels := obj.GetLabels()
		if labels == nil {
			return false
		}
		_, hasEvalTaskId := labels[dbclient.EvaluationTaskIdLabel]
		return hasEvalTaskId
	})
}

// evaluationOpsJobPhaseChangedPredicate triggers when OpsJob phase changes
func evaluationOpsJobPhaseChangedPredicate() predicate.Predicate {
	return predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			return true
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			oldJob, ok1 := e.ObjectOld.(*v1.OpsJob)
			newJob, ok2 := e.ObjectNew.(*v1.OpsJob)
			if !ok1 || !ok2 {
				return false
			}
			// Trigger if phase changed
			return oldJob.Status.Phase != newJob.Status.Phase
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			return false
		},
	}
}

// Reconcile handles OpsJob status changes and updates evaluation task status.
func (r *EvaluationJobController) Reconcile(ctx context.Context, req ctrlruntime.Request) (ctrlruntime.Result, error) {
	// Get the OpsJob
	job := &v1.OpsJob{}
	if err := r.Get(ctx, req.NamespacedName, job); err != nil {
		// Job may have been deleted, ignore
		return ctrlruntime.Result{}, client.IgnoreNotFound(err)
	}

	// Get evaluation task ID from labels
	taskId := job.Labels[dbclient.EvaluationTaskIdLabel]
	if taskId == "" {
		// No task ID, skip
		return ctrlruntime.Result{}, nil
	}

	// Map OpsJob phase to evaluation task status
	status, progress := mapOpsJobPhaseToEvaluationStatus(job.Status.Phase)
	if status == "" {
		// Unknown phase, skip
		return ctrlruntime.Result{}, nil
	}

	// Update status based on phase
	switch job.Status.Phase {
	case v1.OpsJobRunning:
		// Update start time and status
		if err := r.dbClient.UpdateEvaluationTaskStartTime(ctx, taskId); err != nil {
			klog.ErrorS(err, "failed to update evaluation task start time",
				"taskId", taskId,
				"opsJobName", job.Name)
			return ctrlruntime.Result{Requeue: true}, nil
		}
	case v1.OpsJobSucceeded:
		// Update status to succeeded
		if err := r.dbClient.UpdateEvaluationTaskStatus(ctx, taskId, status, progress); err != nil {
			klog.ErrorS(err, "failed to update evaluation task status",
				"taskId", taskId,
				"opsJobName", job.Name,
				"status", status)
			return ctrlruntime.Result{Requeue: true}, nil
		}
		// Try to get and store report path from outputs
		r.updateReportPath(ctx, taskId, job)
	case v1.OpsJobFailed:
		// Mark task as failed with error message
		message := extractEvaluationFailureMessage(job)
		if err := r.dbClient.SetEvaluationTaskFailed(ctx, taskId, message); err != nil {
			klog.ErrorS(err, "failed to set evaluation task failed",
				"taskId", taskId,
				"opsJobName", job.Name)
			return ctrlruntime.Result{Requeue: true}, nil
		}
	default:
		// Update status and progress
		if err := r.dbClient.UpdateEvaluationTaskStatus(ctx, taskId, status, progress); err != nil {
			klog.ErrorS(err, "failed to update evaluation task status",
				"taskId", taskId,
				"opsJobName", job.Name,
				"status", status)
			return ctrlruntime.Result{Requeue: true}, nil
		}
	}

	klog.InfoS("updated evaluation task status",
		"taskId", taskId,
		"opsJobName", job.Name,
		"opsJobPhase", job.Status.Phase,
		"status", status,
		"progress", progress)

	return ctrlruntime.Result{}, nil
}

// mapOpsJobPhaseToEvaluationStatus converts OpsJob phase to evaluation task status.
func mapOpsJobPhaseToEvaluationStatus(phase v1.OpsJobPhase) (dbclient.EvaluationTaskStatus, int) {
	switch phase {
	case v1.OpsJobPending:
		return dbclient.EvaluationTaskStatusPending, 0
	case v1.OpsJobRunning:
		return dbclient.EvaluationTaskStatusRunning, 50
	case v1.OpsJobSucceeded:
		return dbclient.EvaluationTaskStatusSucceeded, 100
	case v1.OpsJobFailed:
		return dbclient.EvaluationTaskStatusFailed, 100
	default:
		return dbclient.EvaluationTaskStatusPending, 0
	}
}

// extractEvaluationFailureMessage extracts failure message from OpsJob conditions
func extractEvaluationFailureMessage(job *v1.OpsJob) string {
	for _, cond := range job.Status.Conditions {
		if cond.Type == "Failed" && cond.Message != "" {
			return cond.Message
		}
	}
	return "Evaluation failed"
}

// updateReportPath tries to extract and update the report path from job outputs
func (r *EvaluationJobController) updateReportPath(ctx context.Context, taskId string, job *v1.OpsJob) {
	// Look for report path in outputs
	for _, output := range job.Status.Outputs {
		if output.Name == v1.ParameterEvalReportPath && output.Value != "" {
			// TODO: Also parse and store result summary from the report
			if err := r.dbClient.UpdateEvaluationTaskResult(ctx, taskId, "", output.Value); err != nil {
				klog.ErrorS(err, "failed to update evaluation task result",
					"taskId", taskId,
					"reportPath", output.Value)
			}
			break
		}
	}
}

