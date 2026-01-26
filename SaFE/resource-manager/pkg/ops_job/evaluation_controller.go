/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package ops_job

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
	"k8s.io/utils/pointer"
	ctrlruntime "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	commonconfig "github.com/AMD-AIG-AIMA/SAFE/common/pkg/config"
	dbclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
	commonjob "github.com/AMD-AIG-AIMA/SAFE/common/pkg/ops_job"
	commons3 "github.com/AMD-AIG-AIMA/SAFE/common/pkg/s3"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/timeutil"
)

const (
	// Default resource requirements for evaluation workload
	DefaultEvalCPU    = "4"
	DefaultEvalMemory = "16Gi"
)

// EvaluationJobReconciler reconciles evaluation type OpsJobs.
type EvaluationJobReconciler struct {
	*OpsJobBaseReconciler
	dbClient dbclient.Interface
	s3Client commons3.Interface
	sync.RWMutex
}

// SetupEvaluationJobController initializes and registers the EvaluationJobReconciler with the controller manager.
func SetupEvaluationJobController(ctx context.Context, mgr manager.Manager) error {
	var dbClient dbclient.Interface
	if commonconfig.IsDBEnable() {
		dbClient = dbclient.NewClient()
		if dbClient == nil {
			klog.Warning("Failed to create database client for EvaluationJobController")
		}
	}

	var s3Client commons3.Interface
	if commonconfig.IsS3Enable() {
		var err error
		s3Client, err = commons3.NewClient(ctx, commons3.Option{ExpireDay: commonconfig.GetS3ExpireDay()})
		if err != nil {
			klog.ErrorS(err, "Failed to create S3 client for EvaluationJobController, report upload will be disabled")
		}
	}

	r := &EvaluationJobReconciler{
		OpsJobBaseReconciler: &OpsJobBaseReconciler{
			Client: mgr.GetClient(),
		},
		dbClient: dbClient,
		s3Client: s3Client,
	}

	err := ctrlruntime.NewControllerManagedBy(mgr).
		For(&v1.OpsJob{}, builder.WithPredicates(predicate.Or(
			predicate.GenerationChangedPredicate{}, onFirstPhaseChangedPredicate()))).
		Watches(&v1.Workload{}, r.handleWorkloadEvent()).
		Complete(r)
	if err != nil {
		return err
	}

	klog.Info("Setup EvaluationJobController successfully")
	return nil
}

// handleWorkloadEvent creates an event handler that watches Workload resource events.
func (r *EvaluationJobReconciler) handleWorkloadEvent() handler.EventHandler {
	return handler.Funcs{
		CreateFunc: func(ctx context.Context, evt event.CreateEvent, q v1.RequestWorkQueue) {
			workload, ok := evt.Object.(*v1.Workload)
			if !ok || !isEvaluationWorkload(workload) {
				return
			}
			r.handleWorkloadEventImpl(ctx, workload)
		},
		UpdateFunc: func(ctx context.Context, evt event.UpdateEvent, q v1.RequestWorkQueue) {
			oldWorkload, ok1 := evt.ObjectOld.(*v1.Workload)
			newWorkload, ok2 := evt.ObjectNew.(*v1.Workload)
			if !ok1 || !ok2 || !isEvaluationWorkload(newWorkload) {
				return
			}
			if (!oldWorkload.IsEnd() && newWorkload.IsEnd()) ||
				(!oldWorkload.IsRunning() && newWorkload.IsRunning()) {
				r.handleWorkloadEventImpl(ctx, newWorkload)
			}
		},
	}
}

// isEvaluationWorkload checks if a workload is an evaluation job workload.
func isEvaluationWorkload(workload *v1.Workload) bool {
	return v1.GetOpsJobId(workload) != "" &&
		v1.GetOpsJobType(workload) == string(v1.OpsJobEvaluationType)
}

// Reconcile is the main control loop for EvaluationJob resources.
func (r *EvaluationJobReconciler) Reconcile(ctx context.Context, req ctrlruntime.Request) (ctrlruntime.Result, error) {
	clearFuncs := []ClearFunc{r.cleanupJobRelatedInfo}
	return r.OpsJobBaseReconciler.Reconcile(ctx, req, r, clearFuncs...)
}

// cleanupJobRelatedInfo cleans up job-related resources.
func (r *EvaluationJobReconciler) cleanupJobRelatedInfo(ctx context.Context, job *v1.OpsJob) error {
	return commonjob.CleanupJobRelatedResource(ctx, r.Client, job.Name)
}

// observe the job status. Returns true if the expected state is met (no handling required), false otherwise.
func (r *EvaluationJobReconciler) observe(_ context.Context, job *v1.OpsJob) (bool, error) {
	return job.IsEnd(), nil
}

// filter determines if the job should be processed by this evaluation job reconciler.
func (r *EvaluationJobReconciler) filter(_ context.Context, job *v1.OpsJob) bool {
	return job.Spec.Type != v1.OpsJobEvaluationType
}

// handle processes the evaluation job by creating a corresponding workload.
func (r *EvaluationJobReconciler) handle(ctx context.Context, job *v1.OpsJob) (ctrlruntime.Result, error) {
	if job.Status.Phase == "" {
		originalJob := client.MergeFrom(job.DeepCopy())
		job.Status.Phase = v1.OpsJobPending
		if err := r.Status().Patch(ctx, job, originalJob); err != nil {
			return ctrlruntime.Result{}, err
		}
		// Update database status
		r.updateDBStatus(ctx, job, dbclient.EvaluationTaskStatusPending, 0)
		return newRequeueAfterResult(job), nil
	}

	// Check if workload already exists
	workload := &v1.Workload{}
	if r.Get(ctx, client.ObjectKey{Name: job.Name}, workload) == nil {
		return ctrlruntime.Result{}, nil
	}

	// Generate and create workload
	var err error
	workload, err = r.generateEvaluationWorkload(ctx, job)
	if err != nil {
		klog.ErrorS(err, "failed to generate evaluation workload", "job", job.Name)
		return ctrlruntime.Result{}, err
	}

	if err = r.Create(ctx, workload); err != nil {
		return ctrlruntime.Result{}, client.IgnoreAlreadyExists(err)
	}

	klog.Infof("Created evaluation workload %s for job %s", workload.Name, job.Name)
	return ctrlruntime.Result{}, nil
}

// getParamValue safely extracts string value from Parameter
func getParamValue(param *v1.Parameter) string {
	if param == nil {
		return ""
	}
	return param.Value
}

// generateEvaluationWorkload generates an evaluation workload based on the job specification.
func (r *EvaluationJobReconciler) generateEvaluationWorkload(ctx context.Context, job *v1.OpsJob) (*v1.Workload, error) {
	// Extract parameters from job inputs
	taskId := getParamValue(job.GetParameter(v1.ParameterEvalTaskId))
	modelEndpoint := getParamValue(job.GetParameter(v1.ParameterModelEndpoint))
	modelName := getParamValue(job.GetParameter(v1.ParameterModelName))
	benchmarksJSON := getParamValue(job.GetParameter(v1.ParameterEvalBenchmarks))
	paramsJSON := getParamValue(job.GetParameter(v1.ParameterEvalParams))
	workspace := getParamValue(job.GetParameter(v1.ParameterWorkspace))
	clusterId := getParamValue(job.GetParameter(v1.ParameterCluster))

	if taskId == "" {
		taskId = job.Labels[dbclient.EvaluationTaskIdLabel]
	}
	if clusterId == "" {
		clusterId = v1.GetClusterId(job)
	}

	// Generate S3 presigned PUT URL for report upload
	var s3PresignedPutURL, s3ReportKey string
	if r.s3Client != nil {
		s3ReportKey = fmt.Sprintf("evaluations/%s/report.json", taskId)
		var err error
		s3PresignedPutURL, err = r.s3Client.GeneratePresignedPutURL(ctx, s3ReportKey, 24) // 24 hours expiry
		if err != nil {
			klog.ErrorS(err, "Failed to generate S3 presigned PUT URL", "taskId", taskId)
		}
	}

	// Build evalscope command with upload script
	entryPoint, err := r.buildEvalCommand(ctx, modelEndpoint, modelName, benchmarksJSON, paramsJSON, taskId, s3PresignedPutURL, s3ReportKey)
	if err != nil {
		return nil, fmt.Errorf("failed to build eval command: %w", err)
	}

	// Base64 encode the entry point
	encodedEntryPoint := base64.StdEncoding.EncodeToString([]byte(entryPoint))

	workload := &v1.Workload{
		ObjectMeta: metav1.ObjectMeta{
			Name: job.Name,
			Labels: map[string]string{
				v1.ClusterIdLabel:              clusterId,
				v1.UserIdLabel:                 v1.GetUserId(job),
				v1.OpsJobIdLabel:               job.Name,
				v1.OpsJobTypeLabel:             string(job.Spec.Type),
				v1.DisplayNameLabel:            v1.GetDisplayName(job),
				dbclient.EvaluationTaskIdLabel: taskId,
			},
			Annotations: map[string]string{
				v1.UserNameAnnotation:          v1.GetUserName(job),
				v1.WorkloadScheduledAnnotation: timeutil.FormatRFC3339(time.Now().UTC()),
				v1.DescriptionAnnotation:       "Model evaluation task",
			},
		},
		Spec: v1.WorkloadSpec{
			EntryPoints: []string{encodedEntryPoint},
			GroupVersionKind: v1.GroupVersionKind{
				Version: common.DefaultVersion,
				Kind:    common.JobKind,
			},
			IsTolerateAll: job.Spec.IsTolerateAll,
			Priority:      common.LowPriorityInt,
			Workspace:     workspace,
			Images:        []string{commonconfig.GetEvalScopeImage()},
			Resources: []v1.WorkloadResource{
				{
					CPU:              DefaultEvalCPU,
					Memory:           DefaultEvalMemory,
					EphemeralStorage: "50Gi",
					Replica:          1,
				},
			},
		},
	}

	// Set workspace default
	if workload.Spec.Workspace == "" {
		workload.Spec.Workspace = corev1.NamespaceDefault
	}

	// Set timeout
	if job.Spec.TimeoutSecond > 0 {
		workload.Spec.Timeout = pointer.Int(job.Spec.TimeoutSecond)
	}

	// Set TTL
	if job.Spec.TTLSecondsAfterFinished > 0 {
		workload.Spec.TTLSecondsAfterFinished = pointer.Int(job.Spec.TTLSecondsAfterFinished)
	}

	// Set owner reference
	if err := controllerutil.SetControllerReference(job, workload, r.Client.Scheme()); err != nil {
		return nil, err
	}

	return workload, nil
}

// BenchmarkConfig represents benchmark configuration from API
type BenchmarkConfig struct {
	DatasetId       string `json:"datasetId"`
	DatasetName     string `json:"datasetName"`     // Dataset displayName, used as evalscope benchmark name
	DatasetLocalDir string `json:"datasetLocalDir"` // Full local path to dataset, e.g. /apps/datasets/math_500
	EvalType        string `json:"evalType"`
	Limit           int    `json:"limit,omitempty"`
}

// EvalParams represents evaluation parameters from API
type EvalParams struct {
	FewShot   int `json:"fewShot,omitempty"`
	MaxTokens int `json:"maxTokens,omitempty"`
}

// buildEvalCommand builds the evalscope command based on parameters
func (r *EvaluationJobReconciler) buildEvalCommand(ctx context.Context, modelEndpoint, modelName, benchmarksJSON, paramsJSON, taskId, s3PresignedPutURL, s3ReportKey string) (string, error) {
	// Parse benchmarks
	var benchmarks []BenchmarkConfig
	if err := json.Unmarshal([]byte(benchmarksJSON), &benchmarks); err != nil {
		return "", fmt.Errorf("failed to parse benchmarks: %w", err)
	}

	if len(benchmarks) == 0 {
		return "", fmt.Errorf("no benchmarks specified")
	}

	// Parse eval params
	var evalParams EvalParams
	if paramsJSON != "" {
		if err := json.Unmarshal([]byte(paramsJSON), &evalParams); err != nil {
			klog.Warningf("failed to parse eval params, using defaults: %v", err)
		}
	}

	// Build dataset list and dataset directories from enriched benchmark config
	// DatasetName: evalscope benchmark name (e.g. math_500)
	// DatasetLocalDir: full local path to dataset (e.g. /wekafs/datasets/math_500)
	var datasetNames []string
	var datasetDirs []string
	for _, b := range benchmarks {
		name := b.DatasetName
		if name == "" {
			// Fallback to datasetId if DatasetName not set (shouldn't happen)
			name = b.DatasetId
		}
		datasetNames = append(datasetNames, name)

		// Use DatasetLocalDir if provided, otherwise fallback
		localDir := b.DatasetLocalDir
		if localDir == "" {
			// Fallback (shouldn't happen if apiserver enriches properly)
			localDir = fmt.Sprintf("/wekafs/datasets/%s", name)
		}
		datasetDirs = append(datasetDirs, localDir)
	}

	// Build evalscope command arguments
	var evalArgs []string
	evalArgs = append(evalArgs, "evalscope", "eval")

	// Model configuration
	evalArgs = append(evalArgs, "--model", modelName)

	// API endpoint for remote/local inference
	if modelEndpoint != "" {
		evalArgs = append(evalArgs, "--api-url", modelEndpoint)
	}

	// Datasets (benchmark names)
	evalArgs = append(evalArgs, "--datasets", strings.Join(datasetNames, ","))

	// Dataset directory (use first one for now, evalscope supports single --dataset-dir)
	if len(datasetDirs) > 0 {
		evalArgs = append(evalArgs, "--dataset-dir", datasetDirs[0])
	}

	// Limit (use first benchmark's limit if specified)
	if len(benchmarks) > 0 && benchmarks[0].Limit > 0 {
		evalArgs = append(evalArgs, "--limit", fmt.Sprintf("%d", benchmarks[0].Limit))
	}

	// Note: evalscope doesn't support --few-shot and --max-tokens directly
	// These can be passed via --generation-config if needed in future

	// Output directory (for report)
	outputDir := fmt.Sprintf("/outputs/%s", taskId)
	evalArgs = append(evalArgs, "--work-dir", outputDir)

	// Build full command - evalscope is pre-installed in the image
	evalCommand := strings.Join(evalArgs, " ")

	// If S3 presigned URL is provided, add upload script after evalscope
	if s3PresignedPutURL != "" {
		uploadScript := r.buildReportUploadScript(outputDir, s3PresignedPutURL)
		evalCommand = fmt.Sprintf("%s && %s", evalCommand, uploadScript)
	}

	return evalCommand, nil
}

// buildReportUploadScript builds a Python script to upload evaluation report to S3
func (r *EvaluationJobReconciler) buildReportUploadScript(outputDir, presignedURL string) string {
	// Python script to find and upload report file
	script := fmt.Sprintf(`python3 -c "
import glob, urllib.request, sys

# Find report files
report_files = glob.glob('%s/*/reports/*/*.json')
if not report_files:
    print('No report files found, skipping upload')
    sys.exit(0)

# Read the first report file
with open(report_files[0], 'rb') as f:
    report_data = f.read()

print(f'Uploading report: {report_files[0]} ({len(report_data)} bytes)')

# Upload to S3 using presigned PUT URL
try:
    req = urllib.request.Request('%s', data=report_data, method='PUT')
    req.add_header('Content-Type', 'application/json')
    urllib.request.urlopen(req, timeout=60)
    print('Report uploaded successfully')
except Exception as e:
    print(f'Failed to upload report: {e}')
    sys.exit(1)
"`, outputDir, presignedURL)

	return script
}

// handleWorkloadEventImpl handles workload events and updates job/database status
func (r *EvaluationJobReconciler) handleWorkloadEventImpl(ctx context.Context, workload *v1.Workload) {
	// Use base reconciler's implementation to update OpsJob status
	r.OpsJobBaseReconciler.handleWorkloadEventImpl(ctx, workload)

	// Additionally update database status
	taskId := workload.Labels[dbclient.EvaluationTaskIdLabel]
	if taskId == "" || r.dbClient == nil {
		return
	}

	var status dbclient.EvaluationTaskStatus
	var progress int

	switch {
	case workload.IsEnd():
		if workload.Status.Phase == v1.WorkloadSucceeded {
			status = dbclient.EvaluationTaskStatusSucceeded
			progress = 100
			// Try to get report path
			r.updateReportPath(ctx, taskId, workload)
		} else {
			status = dbclient.EvaluationTaskStatusFailed
			progress = 100
			message := getWorkloadCompletionMessage(workload)
			if err := r.dbClient.SetEvaluationTaskFailed(ctx, taskId, message); err != nil {
				klog.ErrorS(err, "failed to set evaluation task failed", "taskId", taskId)
			}
			return
		}
	case workload.IsRunning():
		status = dbclient.EvaluationTaskStatusRunning
		progress = 50
		// Update start time
		if err := r.dbClient.UpdateEvaluationTaskStartTime(ctx, taskId); err != nil {
			klog.ErrorS(err, "failed to update evaluation task start time", "taskId", taskId)
		}
	default:
		status = dbclient.EvaluationTaskStatusPending
		progress = 0
	}

	r.updateDBStatus(ctx, workload, status, progress)
}

// updateDBStatus updates the evaluation task status in database
func (r *EvaluationJobReconciler) updateDBStatus(ctx context.Context, obj client.Object, status dbclient.EvaluationTaskStatus, progress int) {
	if r.dbClient == nil {
		return
	}

	var taskId string
	switch o := obj.(type) {
	case *v1.OpsJob:
		taskId = o.Labels[dbclient.EvaluationTaskIdLabel]
	case *v1.Workload:
		taskId = o.Labels[dbclient.EvaluationTaskIdLabel]
	}

	if taskId == "" {
		return
	}

	if err := r.dbClient.UpdateEvaluationTaskStatus(ctx, taskId, status, progress); err != nil {
		klog.ErrorS(err, "failed to update evaluation task status",
			"taskId", taskId,
			"status", status,
			"progress", progress)
	}
}

// updateReportPath tries to extract report path from workload and update database
func (r *EvaluationJobReconciler) updateReportPath(ctx context.Context, taskId string, workload *v1.Workload) {
	if r.dbClient == nil {
		return
	}

	// S3 key for the evaluation report (same format as in generateEvaluationWorkload)
	s3Key := fmt.Sprintf("evaluations/%s/report.json", taskId)

	// Pass "{}" for empty result_summary since the field is JSONB type
	if err := r.dbClient.UpdateEvaluationTaskResult(ctx, taskId, "{}", s3Key); err != nil {
		klog.ErrorS(err, "failed to update evaluation task result",
			"taskId", taskId,
			"s3Key", s3Key)
	}
}
