/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package ops_job

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
	ctrlruntime "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	commonconfig "github.com/AMD-AIG-AIMA/SAFE/common/pkg/config"
	dbclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
	commonutils "github.com/AMD-AIG-AIMA/SAFE/common/pkg/utils"
	commonworkspace "github.com/AMD-AIG-AIMA/SAFE/common/pkg/workspace"
)

const (
	// HFDatasetJobLabel is the label key for identifying HF dataset download Jobs
	HFDatasetJobLabel = "hf-dataset-job"
	// HFDatasetIdLabel is the label key for the dataset ID in HF download Jobs
	HFDatasetIdLabel = "hf-dataset-id"
)

// HFDatasetDownloadController watches K8s Jobs for HuggingFace dataset downloads
// and updates dataset status in database, then triggers S3 → PFS download.
type HFDatasetDownloadController struct {
	client.Client
	dbClient dbclient.Interface
}

// SetupHFDatasetDownloadController initializes the controller.
func SetupHFDatasetDownloadController(ctx context.Context, mgr manager.Manager) error {
	if !commonconfig.IsDBEnable() {
		klog.Info("Database is not enabled, skipping HFDatasetDownloadController setup")
		return nil
	}

	dbClient := dbclient.NewClient()
	if dbClient == nil {
		klog.Warning("Failed to create database client, skipping HFDatasetDownloadController setup")
		return nil
	}

	r := &HFDatasetDownloadController{
		Client:   mgr.GetClient(),
		dbClient: dbClient,
	}

	// Watch K8s Jobs with HF dataset label
	err := ctrlruntime.NewControllerManagedBy(mgr).
		For(&batchv1.Job{}, builder.WithPredicates(
			predicate.And(
				hfDatasetJobPredicate(),
				predicate.Or(
					predicate.GenerationChangedPredicate{},
					hfJobStatusChangedPredicate(),
				),
			),
		)).
		Complete(r)
	if err != nil {
		return err
	}

	klog.Info("Setup HFDatasetDownloadController successfully")
	return nil
}

// hfDatasetJobPredicate filters Jobs with hf-dataset-job label
func hfDatasetJobPredicate() predicate.Predicate {
	return predicate.NewPredicateFuncs(func(obj client.Object) bool {
		labels := obj.GetLabels()
		if labels == nil {
			return false
		}
		return labels[HFDatasetJobLabel] == "true"
	})
}

// hfJobStatusChangedPredicate triggers when Job status changes (succeeded/failed)
func hfJobStatusChangedPredicate() predicate.Predicate {
	return predicate.Funcs{
		UpdateFunc: func(e event.UpdateEvent) bool {
			oldJob, ok1 := e.ObjectOld.(*batchv1.Job)
			newJob, ok2 := e.ObjectNew.(*batchv1.Job)
			if !ok1 || !ok2 {
				return false
			}
			return oldJob.Status.Succeeded != newJob.Status.Succeeded ||
				oldJob.Status.Failed != newJob.Status.Failed
		},
	}
}

// Reconcile handles Job status changes for HF dataset downloads.
func (r *HFDatasetDownloadController) Reconcile(ctx context.Context, req ctrlruntime.Request) (ctrlruntime.Result, error) {
	job := &batchv1.Job{}
	if err := r.Get(ctx, req.NamespacedName, job); err != nil {
		return ctrlruntime.Result{}, client.IgnoreNotFound(err)
	}

	// Get dataset ID from label
	datasetId := job.Labels[HFDatasetIdLabel]
	if datasetId == "" {
		return ctrlruntime.Result{}, nil
	}

	// Get dataset from database
	dataset, err := r.dbClient.GetDataset(ctx, datasetId)
	if err != nil {
		klog.ErrorS(err, "Failed to get dataset", "datasetId", datasetId)
		return ctrlruntime.Result{}, err
	}

	// Handle Job completion
	if job.Status.Succeeded > 0 {
		return r.handleJobSucceeded(ctx, dataset, job)
	}

	// Handle Job failure (all retries exhausted)
	if job.Status.Failed > 0 && job.Status.Active == 0 {
		return r.handleJobFailed(ctx, dataset, job)
	}

	// Still in progress
	return ctrlruntime.Result{RequeueAfter: 10 * time.Second}, nil
}

// handleJobSucceeded handles successful HF → S3 download.
// It updates the dataset status and creates OpsJobs for S3 → PFS download.
func (r *HFDatasetDownloadController) handleJobSucceeded(ctx context.Context, dataset *dbclient.Dataset, job *batchv1.Job) (ctrlruntime.Result, error) {
	klog.InfoS("HF dataset download Job succeeded", "datasetId", dataset.DatasetId, "jobName", job.Name)

	// Initialize local paths based on workspace configuration
	localPaths, downloadTargets, err := r.initializeLocalPaths(ctx, dataset)
	if err != nil {
		klog.ErrorS(err, "Failed to initialize local paths", "datasetId", dataset.DatasetId)
	}

	// Update status to Downloading (S3 upload complete, starting local download)
	dataset.Status = dbclient.DatasetStatusDownloading
	dataset.Message = "HF download completed, starting local download"
	dataset.LocalPaths = localPaths
	if err := r.dbClient.UpsertDataset(ctx, dataset); err != nil {
		klog.ErrorS(err, "Failed to update dataset status", "datasetId", dataset.DatasetId)
		return ctrlruntime.Result{Requeue: true}, nil
	}

	// Create OpsJobs for S3 → PFS download
	if len(downloadTargets) > 0 {
		if err := r.createLocalDownloadOpsJobs(ctx, dataset, downloadTargets); err != nil {
			klog.ErrorS(err, "Failed to create local download OpsJobs", "datasetId", dataset.DatasetId)
		}
	}

	// Delete completed Job
	if err := r.Delete(ctx, job, client.PropagationPolicy(metav1.DeletePropagationBackground)); err != nil {
		klog.ErrorS(err, "Failed to delete completed job", "jobName", job.Name)
	}

	return ctrlruntime.Result{}, nil
}

// handleJobFailed handles failed HF download.
func (r *HFDatasetDownloadController) handleJobFailed(ctx context.Context, dataset *dbclient.Dataset, job *batchv1.Job) (ctrlruntime.Result, error) {
	failureReason := extractHFJobFailureReason(job)
	klog.ErrorS(nil, "HF dataset download Job failed",
		"datasetId", dataset.DatasetId, "jobName", job.Name, "reason", failureReason)

	dataset.Status = dbclient.DatasetStatusFailed
	dataset.Message = fmt.Sprintf("HF download failed: %s", failureReason)

	if err := r.dbClient.UpsertDataset(ctx, dataset); err != nil {
		klog.ErrorS(err, "Failed to update dataset status", "datasetId", dataset.DatasetId)
		return ctrlruntime.Result{Requeue: true}, nil
	}

	// Delete failed Job
	if err := r.Delete(ctx, job, client.PropagationPolicy(metav1.DeletePropagationBackground)); err != nil {
		klog.ErrorS(err, "Failed to delete failed job", "jobName", job.Name)
	}

	return ctrlruntime.Result{}, nil
}

// initializeLocalPaths initializes local paths based on workspace configuration.
// Returns the JSON string for local_paths and the download targets.
func (r *HFDatasetDownloadController) initializeLocalPaths(ctx context.Context, dataset *dbclient.Dataset) (string, []commonworkspace.DownloadTarget, error) {
	var targets []commonworkspace.DownloadTarget

	if dataset.Workspace != "" {
		// Private dataset: download to specific workspace
		ws := &v1.Workspace{}
		if err := r.Get(ctx, client.ObjectKey{Name: dataset.Workspace}, ws); err != nil {
			return "[]", nil, fmt.Errorf("failed to get workspace %s: %w", dataset.Workspace, err)
		}
		path := commonworkspace.GetNfsPathFromWorkspace(ws)
		if path == "" {
			return "[]", nil, fmt.Errorf("workspace %s has no volume configured", dataset.Workspace)
		}
		targets = []commonworkspace.DownloadTarget{{Workspace: ws.Name, Path: path}}
	} else {
		// Public dataset: download to all workspaces (deduplicated)
		workspaceList := &v1.WorkspaceList{}
		if err := r.List(ctx, workspaceList); err != nil {
			return "[]", nil, fmt.Errorf("failed to list workspaces: %w", err)
		}
		targets = commonworkspace.GetUniqueDownloadPaths(workspaceList.Items)
	}

	// Build local paths JSON
	localPaths := make([]dbclient.DatasetLocalPathDB, 0, len(targets))
	for _, target := range targets {
		localPaths = append(localPaths, dbclient.DatasetLocalPathDB{
			Workspace: target.Workspace,
			Path:      target.Path + "/datasets/" + dataset.DisplayName,
			Status:    dbclient.DatasetStatusPending,
		})
	}

	jsonBytes, err := json.Marshal(localPaths)
	if err != nil {
		return "[]", targets, fmt.Errorf("failed to marshal local paths: %w", err)
	}

	return string(jsonBytes), targets, nil
}

// createLocalDownloadOpsJobs creates OpsJobs to download from S3 to local PFS.
func (r *HFDatasetDownloadController) createLocalDownloadOpsJobs(ctx context.Context, dataset *dbclient.Dataset, targets []commonworkspace.DownloadTarget) error {
	if !commonconfig.IsS3Enable() {
		return fmt.Errorf("S3 storage is not enabled")
	}

	s3Endpoint := commonconfig.GetS3Endpoint()
	s3Bucket := commonconfig.GetS3Bucket()
	if s3Endpoint == "" || s3Bucket == "" {
		return fmt.Errorf("S3 configuration is incomplete")
	}

	// Normalize S3 endpoint
	s3Endpoint = strings.TrimSuffix(s3Endpoint, "/")
	if !strings.HasPrefix(s3Endpoint, "http://") && !strings.HasPrefix(s3Endpoint, "https://") {
		if strings.Contains(s3Endpoint, ".") || strings.Contains(s3Endpoint, ":") {
			s3Endpoint = "https://" + s3Endpoint
		}
	}

	// Construct S3 URL
	s3URL := fmt.Sprintf("%s/%s/%s", s3Endpoint, s3Bucket, dataset.S3Path)
	destPath := fmt.Sprintf("datasets/%s", dataset.DisplayName)
	image := commonconfig.GetDownloadJoImage()
	secretName := "primus-safe-s3"

	for _, target := range targets {
		// Get workspace to get cluster ID
		ws := &v1.Workspace{}
		if err := r.Get(ctx, client.ObjectKey{Name: target.Workspace}, ws); err != nil {
			klog.ErrorS(err, "Failed to get workspace for download job", "workspace", target.Workspace)
			continue
		}

		jobName := commonutils.GenerateName(fmt.Sprintf("dataset-dl-%s", dataset.DatasetId))

		opsJob := &v1.OpsJob{
			ObjectMeta: metav1.ObjectMeta{
				Name: jobName,
				Labels: map[string]string{
					v1.UserIdLabel:          common.UserSystem,
					v1.DisplayNameLabel:     jobName,
					v1.WorkspaceIdLabel:     target.Workspace,
					v1.ClusterIdLabel:       ws.Spec.Cluster,
					dbclient.DatasetIdLabel: dataset.DatasetId,
				},
				Annotations: map[string]string{
					v1.UserNameAnnotation: common.UserSystem,
				},
			},
			Spec: v1.OpsJobSpec{
				Type:  v1.OpsJobDownloadType,
				Image: &image,
				Inputs: []v1.Parameter{
					{Name: v1.ParameterEndpoint, Value: s3URL},
					{Name: v1.ParameterDestPath, Value: destPath},
					{Name: v1.ParameterSecret, Value: secretName},
					{Name: v1.ParameterWorkspace, Value: target.Workspace},
				},
				TTLSecondsAfterFinished: 300,  // Auto cleanup after 5 minutes
				TimeoutSecond:           3600, // 1 hour timeout
			},
		}

		if err := r.Create(ctx, opsJob); err != nil {
			klog.ErrorS(err, "Failed to create download OpsJob", "jobName", jobName, "workspace", target.Workspace)
			continue
		}

		klog.InfoS("Created local download OpsJob for HF dataset",
			"datasetId", dataset.DatasetId,
			"jobName", jobName,
			"workspace", target.Workspace)
	}

	return nil
}

// extractHFJobFailureReason extracts failure message from a batchv1.Job.
func extractHFJobFailureReason(job *batchv1.Job) string {
	for _, cond := range job.Status.Conditions {
		if cond.Type == batchv1.JobFailed && cond.Status == corev1.ConditionTrue {
			if cond.Message != "" {
				return fmt.Sprintf("%s: %s", cond.Reason, cond.Message)
			}
		}
	}

	if job.Spec.BackoffLimit != nil && job.Status.Failed >= *job.Spec.BackoffLimit {
		return "Maximum retry attempts exceeded"
	}

	return "Unknown error during download"
}

