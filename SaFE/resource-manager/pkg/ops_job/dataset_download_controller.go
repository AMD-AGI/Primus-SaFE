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

// DatasetDownloadController watches OpsJob resources and updates dataset download status in database.
type DatasetDownloadController struct {
	client.Client
	dbClient dbclient.Interface
}

// SetupDatasetDownloadController initializes and registers the DatasetDownloadController with the controller manager.
func SetupDatasetDownloadController(mgr manager.Manager) error {
	// Only setup if database is enabled
	if !commonconfig.IsDBEnable() {
		klog.Info("Database is not enabled, skipping DatasetDownloadController setup")
		return nil
	}

	dbClient := dbclient.NewClient()
	if dbClient == nil {
		klog.Warning("Failed to create database client, skipping DatasetDownloadController setup")
		return nil
	}

	r := &DatasetDownloadController{
		Client:   mgr.GetClient(),
		dbClient: dbClient,
	}

	// Watch OpsJob with predicate to filter dataset-related OpsJobs
	err := ctrlruntime.NewControllerManagedBy(mgr).
		For(&v1.OpsJob{}, builder.WithPredicates(
			predicate.And(
				datasetOpsJobPredicate(),
				predicate.Or(
					predicate.GenerationChangedPredicate{},
					opsJobPhaseChangedPredicate(),
				),
			),
		)).
		Complete(r)
	if err != nil {
		return err
	}

	klog.Info("Setup DatasetDownloadController successfully")
	return nil
}

// datasetOpsJobPredicate filters OpsJobs that have dataset-id label
func datasetOpsJobPredicate() predicate.Predicate {
	return predicate.NewPredicateFuncs(func(obj client.Object) bool {
		labels := obj.GetLabels()
		if labels == nil {
			return false
		}
		_, hasDatasetId := labels[dbclient.DatasetIdLabel]
		return hasDatasetId
	})
}

// opsJobPhaseChangedPredicate triggers when OpsJob phase changes
func opsJobPhaseChangedPredicate() predicate.Predicate {
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

// Reconcile handles OpsJob status changes and updates dataset download status.
func (r *DatasetDownloadController) Reconcile(ctx context.Context, req ctrlruntime.Request) (ctrlruntime.Result, error) {
	// Get the OpsJob
	job := &v1.OpsJob{}
	if err := r.Get(ctx, req.NamespacedName, job); err != nil {
		// Job may have been deleted, ignore
		return ctrlruntime.Result{}, client.IgnoreNotFound(err)
	}

	// Get dataset ID from label
	datasetId := job.Labels[dbclient.DatasetIdLabel]
	if datasetId == "" {
		// No dataset ID, skip
		return ctrlruntime.Result{}, nil
	}

	// Map OpsJob phase to download status
	downloadStatus := mapOpsJobPhaseToDownloadStatus(job.Status.Phase)
	if downloadStatus == "" {
		// Unknown phase, skip
		return ctrlruntime.Result{}, nil
	}

	// Update database
	if err := r.dbClient.UpdateDatasetDownloadStatus(ctx, datasetId, downloadStatus); err != nil {
		klog.ErrorS(err, "failed to update dataset download status",
			"datasetId", datasetId,
			"opsJobName", job.Name,
			"opsJobPhase", job.Status.Phase,
			"downloadStatus", downloadStatus)
		// Requeue to retry
		return ctrlruntime.Result{Requeue: true}, nil
	}

	klog.InfoS("updated dataset download status",
		"datasetId", datasetId,
		"opsJobName", job.Name,
		"opsJobPhase", job.Status.Phase,
		"downloadStatus", downloadStatus)

	return ctrlruntime.Result{}, nil
}

// mapOpsJobPhaseToDownloadStatus converts OpsJob phase to dataset download status.
func mapOpsJobPhaseToDownloadStatus(phase v1.OpsJobPhase) string {
	switch phase {
	case v1.OpsJobPending:
		return dbclient.DatasetDownloadStatusPending
	case v1.OpsJobRunning:
		return dbclient.DatasetDownloadStatusDownloading
	case v1.OpsJobSucceeded:
		return dbclient.DatasetDownloadStatusReady
	case v1.OpsJobFailed:
		return dbclient.DatasetDownloadStatusFailed
	default:
		return dbclient.DatasetDownloadStatusPending
	}
}
