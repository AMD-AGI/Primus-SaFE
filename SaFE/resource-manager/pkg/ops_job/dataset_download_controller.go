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
	// MaxDatasetFailoverAttempts is the maximum number of workspace failover attempts per path for dataset downloads.
	MaxDatasetFailoverAttempts = 3
)

// DatasetDownloadController watches OpsJob resources and updates dataset download status in database.
type DatasetDownloadController struct {
	client.Client
	dbClient dbclient.Interface
}

// SetupDatasetDownloadController initializes and registers the DatasetDownloadController with the controller manager.
func SetupDatasetDownloadController(_ context.Context, mgr manager.Manager) error {
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

// Reconcile handles OpsJob status changes and updates dataset status per workspace.
func (r *DatasetDownloadController) Reconcile(ctx context.Context, req ctrlruntime.Request) (ctrlruntime.Result, error) {
	// Get the OpsJob
	job := &v1.OpsJob{}
	if err := r.Get(ctx, req.NamespacedName, job); err != nil {
		// Job may have been deleted, ignore
		return ctrlruntime.Result{}, client.IgnoreNotFound(err)
	}

	// Get dataset ID and workspace from labels
	datasetId := job.Labels[dbclient.DatasetIdLabel]
	if datasetId == "" {
		// No dataset ID, skip
		return ctrlruntime.Result{}, nil
	}

	workspace := job.Labels[v1.WorkspaceIdLabel]
	if workspace == "" {
		// No workspace, skip
		return ctrlruntime.Result{}, nil
	}

	// Map OpsJob phase to dataset status
	status := mapOpsJobPhaseToDatasetStatus(job.Status.Phase)
	if status == "" {
		// Unknown phase, skip
		return ctrlruntime.Result{}, nil
	}

	// Get failure message if failed
	var message string
	if status == dbclient.DatasetStatusFailed {
		message = extractOpsJobFailureMessage(job)

		// Attempt failover to another workspace sharing the same storage path
		if failovered, err := r.tryDatasetFailover(ctx, datasetId, workspace, job); err != nil {
			klog.ErrorS(err, "failed to attempt dataset failover",
				"datasetId", datasetId, "workspace", workspace)
		} else if failovered {
			// Failover initiated - the failed workspace's localPath will be updated by the new OpsJob
			klog.InfoS("dataset download failover initiated",
				"datasetId", datasetId,
				"failedWorkspace", workspace,
				"opsJobName", job.Name)
			return ctrlruntime.Result{}, nil
		}
	}

	// Update per-workspace status in database (this also recalculates overall status)
	if err := r.dbClient.UpdateDatasetLocalPath(ctx, datasetId, workspace, status, message); err != nil {
		klog.ErrorS(err, "failed to update dataset local path status",
			"datasetId", datasetId,
			"workspace", workspace,
			"opsJobName", job.Name,
			"opsJobPhase", job.Status.Phase,
			"status", status)
		// Requeue to retry
		return ctrlruntime.Result{Requeue: true}, nil
	}

	klog.InfoS("updated dataset local path status",
		"datasetId", datasetId,
		"workspace", workspace,
		"opsJobName", job.Name,
		"opsJobPhase", job.Status.Phase,
		"status", status)

	return ctrlruntime.Result{}, nil
}

// mapOpsJobPhaseToDatasetStatus converts OpsJob phase to dataset status.
func mapOpsJobPhaseToDatasetStatus(phase v1.OpsJobPhase) dbclient.DatasetStatus {
	switch phase {
	case v1.OpsJobPending:
		return dbclient.DatasetStatusPending
	case v1.OpsJobRunning:
		return dbclient.DatasetStatusDownloading
	case v1.OpsJobSucceeded:
		return dbclient.DatasetStatusReady
	case v1.OpsJobFailed:
		return dbclient.DatasetStatusFailed
	default:
		return dbclient.DatasetStatusPending
	}
}

// extractOpsJobFailureMessage extracts failure message from OpsJob conditions
func extractOpsJobFailureMessage(job *v1.OpsJob) string {
	for _, cond := range job.Status.Conditions {
		if cond.Type == "Failed" && cond.Message != "" {
			return cond.Message
		}
	}
	return "Download failed"
}

// tryDatasetFailover attempts to switch the dataset download to another workspace sharing the same storage path.
// Returns (true, nil) if failover was initiated, (false, nil) if no failover possible, (false, err) on error.
func (r *DatasetDownloadController) tryDatasetFailover(ctx context.Context, datasetId, failedWorkspace string, failedJob *v1.OpsJob) (bool, error) {
	// Get dataset from database
	dataset, err := r.dbClient.GetDataset(ctx, datasetId)
	if err != nil {
		return false, fmt.Errorf("failed to get dataset: %w", err)
	}

	// Parse localPaths to find the base path for the failed workspace
	var localPaths []dbclient.DatasetLocalPathDB
	if dataset.LocalPaths != "" {
		if err := json.Unmarshal([]byte(dataset.LocalPaths), &localPaths); err != nil {
			return false, fmt.Errorf("failed to parse local_paths: %w", err)
		}
	}

	// Find the localPath entry for the failed workspace
	var failedPath string
	for _, lp := range localPaths {
		if lp.Workspace == failedWorkspace {
			failedPath = lp.Path
			break
		}
	}
	if failedPath == "" {
		return false, nil
	}

	// Extract base path (e.g., "/wekafs" from "/wekafs/datasets/xxx")
	basePath := extractDatasetBasePath(failedPath)
	if basePath == "" {
		return false, nil
	}

	// Get and update tried workspaces
	triedMap := parseTriedWorkspacesMap(dataset.TriedWorkspaces)
	tried := triedMap[basePath]
	tried = appendUniqueStr(tried, failedWorkspace)

	// Check max failover attempts
	if len(tried) > MaxDatasetFailoverAttempts {
		klog.InfoS("Exceeded max dataset failover attempts",
			"datasetId", datasetId, "basePath", basePath,
			"attempts", len(tried), "max", MaxDatasetFailoverAttempts)
		triedMap[basePath] = tried
		r.saveTriedWorkspaces(ctx, dataset, triedMap)
		return false, nil
	}

	// Find all workspaces sharing the same base path
	allWorkspaces, err := commonworkspace.GetWorkspacesWithSamePath(r.Client, basePath)
	if err != nil {
		return false, fmt.Errorf("failed to get workspaces with same path: %w", err)
	}

	// Filter out tried workspaces
	var candidates []string
	for _, ws := range allWorkspaces {
		if !containsStr(tried, ws) {
			candidates = append(candidates, ws)
		}
	}

	if len(candidates) == 0 {
		klog.InfoS("No more workspaces available for dataset failover",
			"datasetId", datasetId, "basePath", basePath, "triedWorkspaces", tried)
		triedMap[basePath] = tried
		r.saveTriedWorkspaces(ctx, dataset, triedMap)
		return false, nil
	}

	// Pick the next candidate
	nextWorkspace := candidates[0]
	klog.InfoS("Dataset failover to another workspace",
		"datasetId", datasetId,
		"failedWorkspace", failedWorkspace,
		"nextWorkspace", nextWorkspace,
		"triedWorkspaces", tried,
		"remainingCandidates", candidates)

	// Save tried workspaces
	triedMap[basePath] = tried
	r.saveTriedWorkspaces(ctx, dataset, triedMap)

	// Update the localPath entry: switch workspace and reset status
	for i := range localPaths {
		if localPaths[i].Workspace == failedWorkspace {
			localPaths[i].Workspace = nextWorkspace
			localPaths[i].Status = dbclient.DatasetStatusPending
			localPaths[i].Message = fmt.Sprintf("Failover: %s → %s (attempt %d/%d)",
				failedWorkspace, nextWorkspace, len(tried), MaxDatasetFailoverAttempts)
			break
		}
	}

	// Marshal updated localPaths
	localPathsJSON, err := json.Marshal(localPaths)
	if err != nil {
		return false, fmt.Errorf("failed to marshal local_paths: %w", err)
	}

	// Update dataset in database
	dataset.LocalPaths = string(localPathsJSON)
	dataset.Message = fmt.Sprintf("Failover: %s → %s", failedWorkspace, nextWorkspace)
	if err := r.dbClient.UpsertDataset(ctx, dataset); err != nil {
		return false, fmt.Errorf("failed to update dataset: %w", err)
	}

	// Create new OpsJob for the next workspace
	if err := r.createFailoverOpsJob(ctx, dataset, failedJob, nextWorkspace); err != nil {
		return false, fmt.Errorf("failed to create failover OpsJob: %w", err)
	}

	return true, nil
}

// createFailoverOpsJob creates a new download OpsJob for the failover workspace,
// copying the configuration from the failed job.
func (r *DatasetDownloadController) createFailoverOpsJob(ctx context.Context, dataset *dbclient.Dataset, failedJob *v1.OpsJob, nextWorkspace string) error {
	// Get workspace to get cluster ID
	ws := &v1.Workspace{}
	if err := r.Get(ctx, client.ObjectKey{Name: nextWorkspace}, ws); err != nil {
		return fmt.Errorf("failed to get workspace %s: %w", nextWorkspace, err)
	}

	// Generate new job name
	jobName := commonutils.GenerateName(fmt.Sprintf("dataset-dl-%s", dataset.DatasetId))

	// Copy inputs from failed job, update workspace parameter
	var inputs []v1.Parameter
	for _, param := range failedJob.Spec.Inputs {
		if param.Name == v1.ParameterWorkspace {
			inputs = append(inputs, v1.Parameter{Name: v1.ParameterWorkspace, Value: nextWorkspace})
		} else {
			inputs = append(inputs, param)
		}
	}

	// Get user info from failed job labels
	userId := failedJob.Labels[v1.UserIdLabel]
	userName := ""
	if failedJob.Annotations != nil {
		userName = failedJob.Annotations[v1.UserNameAnnotation]
	}
	if userId == "" {
		userId = common.UserSystem
	}
	if userName == "" {
		userName = common.UserSystem
	}

	newJob := &v1.OpsJob{
		ObjectMeta: failedJob.ObjectMeta,
	}
	// Reset metadata for the new job
	newJob.Name = jobName
	newJob.ResourceVersion = ""
	newJob.UID = ""
	newJob.CreationTimestamp = failedJob.CreationTimestamp // will be overwritten
	newJob.Labels = map[string]string{
		v1.UserIdLabel:          userId,
		v1.DisplayNameLabel:     jobName,
		v1.WorkspaceIdLabel:     nextWorkspace,
		v1.ClusterIdLabel:       ws.Spec.Cluster,
		dbclient.DatasetIdLabel: dataset.DatasetId,
	}
	newJob.Annotations = map[string]string{
		v1.UserNameAnnotation: userName,
	}
	newJob.Spec = v1.OpsJobSpec{
		Type:                    failedJob.Spec.Type,
		Image:                   failedJob.Spec.Image,
		Inputs:                  inputs,
		TTLSecondsAfterFinished: failedJob.Spec.TTLSecondsAfterFinished,
		TimeoutSecond:           failedJob.Spec.TimeoutSecond,
	}

	if err := r.Create(ctx, newJob); err != nil {
		return fmt.Errorf("failed to create OpsJob: %w", err)
	}

	klog.InfoS("Created failover OpsJob for dataset download",
		"datasetId", dataset.DatasetId,
		"jobName", jobName,
		"workspace", nextWorkspace)

	return nil
}

// saveTriedWorkspaces persists the tried workspaces map to the dataset's tried_workspaces field.
func (r *DatasetDownloadController) saveTriedWorkspaces(ctx context.Context, dataset *dbclient.Dataset, triedMap map[string][]string) {
	jsonBytes, err := json.Marshal(triedMap)
	if err != nil {
		klog.ErrorS(err, "Failed to marshal tried workspaces", "datasetId", dataset.DatasetId)
		return
	}
	dataset.TriedWorkspaces = string(jsonBytes)
	// Note: this will be persisted on the next UpsertDataset call
}

// extractDatasetBasePath extracts the base PFS path from a full dataset path.
// e.g., "/wekafs/datasets/my-dataset" -> "/wekafs"
func extractDatasetBasePath(fullPath string) string {
	idx := strings.Index(fullPath, "/datasets/")
	if idx > 0 {
		return fullPath[:idx]
	}
	return ""
}

// parseTriedWorkspacesMap parses the tried_workspaces JSON field into a map.
func parseTriedWorkspacesMap(data string) map[string][]string {
	if data == "" || data == "{}" || data == "[]" {
		return make(map[string][]string)
	}
	var result map[string][]string
	if err := json.Unmarshal([]byte(data), &result); err != nil {
		return make(map[string][]string)
	}
	return result
}

// appendUniqueStr appends a string to a slice if it's not already present.
func appendUniqueStr(slice []string, item string) []string {
	for _, s := range slice {
		if s == item {
			return slice
		}
	}
	return append(slice, item)
}

// containsStr checks if a string slice contains a specific item.
func containsStr(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}
