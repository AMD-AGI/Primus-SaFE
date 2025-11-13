/*
 * Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package ops_job

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/containers/image/v5/copy"
	"github.com/containers/image/v5/signature"
	"github.com/containers/image/v5/transports/alltransports"
	"github.com/containers/image/v5/types"
	corev1 "k8s.io/api/core/v1"
	apitypes "k8s.io/apimachinery/pkg/types"
	"k8s.io/klog/v2"
	ctrlruntime "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	commonconfig "github.com/AMD-AIG-AIMA/SAFE/common/pkg/config"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/controller"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/crypto"
	dbclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client/model"
	commonerrors "github.com/AMD-AIG-AIMA/SAFE/common/pkg/errors"
)

const (
	// Default concurrent workers for image export
	exportImageDefaultConcurrent = 3

	// Default namespace for resources
	defaultNamespace = "primus-safe"

	// Secret name containing Harbor authentication
	imageImportSecretName = "primus-safe-image-import-reg-cred"

	// Registry project name
	registryProject = "custom"
)

// ExportImageJobReconciler reconciles image export jobs using containers/image library
type ExportImageJobReconciler struct {
	*OpsJobBaseReconciler
	dbClient dbclient.Interface
	*controller.Controller[string]
}

// SetupExportImageJobController initializes and registers ExportImageJobReconciler with the controller manager
func SetupExportImageJobController(ctx context.Context, mgr manager.Manager) error {
	// Check if database is enabled
	if !commonconfig.IsDBEnable() {
		klog.Infof("Database is not enabled, skip ExportImageJobController setup")
		return nil
	}

	// Create reconciler instance
	r := &ExportImageJobReconciler{
		OpsJobBaseReconciler: &OpsJobBaseReconciler{
			Client: mgr.GetClient(),
		},
		dbClient: dbclient.NewClient(),
	}

	// Verify database client initialization
	if r.dbClient == nil {
		return fmt.Errorf("failed to initialize database client for ExportImageJobController")
	}

	// Initialize worker controller for parallel processing
	r.Controller = controller.NewController[string](r, exportImageDefaultConcurrent)
	r.start(ctx)

	// Register controller to watch OpsJob resources
	err := ctrlruntime.NewControllerManagedBy(mgr).
		For(&v1.OpsJob{}, builder.WithPredicates(predicate.Or(
			predicate.GenerationChangedPredicate{},
			onFirstPhaseChangedPredicate(),
		))).
		Complete(r)

	if err != nil {
		return fmt.Errorf("failed to setup ExportImageJobController: %w", err)
	}

	klog.Infof("Setup ExportImageJobController successfully with %d workers", exportImageDefaultConcurrent)
	return nil
}

// start initializes and runs the worker routines for processing export jobs
func (r *ExportImageJobReconciler) start(ctx context.Context) {
	for i := 0; i < r.MaxConcurrent; i++ {
		r.Run(ctx)
	}
}

// Reconcile is the main reconciliation loop for ExportImageJob resources
func (r *ExportImageJobReconciler) Reconcile(ctx context.Context, req ctrlruntime.Request) (ctrlruntime.Result, error) {
	return r.OpsJobBaseReconciler.Reconcile(ctx, req, r)
}

// observe checks if the job is still processing or completed
func (r *ExportImageJobReconciler) observe(ctx context.Context, job *v1.OpsJob) (bool, error) {
	// For direct execution model, observe doesn't need to check external Job status
	return false, nil
}

// filter determines whether to process the OpsJob, returns true to skip
func (r *ExportImageJobReconciler) filter(_ context.Context, job *v1.OpsJob) bool {
	return job.Spec.Type != v1.OpsJobExportImageType
}

// handle processes pending export jobs by adding them to worker queue
func (r *ExportImageJobReconciler) handle(ctx context.Context, job *v1.OpsJob) (ctrlruntime.Result, error) {
	if job.IsPending() {
		// Update job phase to running
		if err := r.setJobPhase(ctx, job, v1.OpsJobRunning); err != nil {
			return ctrlruntime.Result{}, fmt.Errorf("failed to set job phase to running: %w", err)
		}

		// Add to worker queue for async processing
		r.Add(job.Name)

		// Ensure job will be reconciled on timeout
		return newRequeueAfterResult(job), nil
	}
	return ctrlruntime.Result{}, nil
}

// Do processes the export job by SSH to node and using nerdctl
func (r *ExportImageJobReconciler) Do(ctx context.Context, jobName string) (ctrlruntime.Result, error) {
	// Get the OpsJob
	job := &v1.OpsJob{}
	if err := r.Get(ctx, client.ObjectKey{Name: jobName}, job); err != nil {
		klog.ErrorS(err, "failed to get OpsJob", "jobName", jobName)
		return ctrlruntime.Result{}, err
	}

	// Skip if job is already completed
	if job.IsEnd() {
		return ctrlruntime.Result{}, nil
	}

	// Extract workload ID and source image
	workloadId := getWorkloadIdFromJob(job)
	if workloadId == "" {
		err := commonerrors.NewBadRequest("workload ID is empty")
		return ctrlruntime.Result{}, r.setJobCompleted(ctx, job, v1.OpsJobFailed, err.Error(), nil)
	}

	sourceImage := getSourceImageFromJob(job)
	if sourceImage == "" {
		err := commonerrors.NewBadRequest("source image is empty")
		return ctrlruntime.Result{}, r.setJobCompleted(ctx, job, v1.OpsJobFailed, err.Error(), nil)
	}

	// Query default Harbor registry
	defaultRegistry, err := r.dbClient.GetDefaultRegistryInfo(ctx)
	if err != nil {
		klog.ErrorS(err, "failed to get default registry info")
		return ctrlruntime.Result{}, r.setJobCompleted(ctx, job, v1.OpsJobFailed, "failed to get default registry", nil)
	}
	if defaultRegistry == nil {
		err := commonerrors.NewBadRequest("default push registry not exist, please contact your administrator")
		return ctrlruntime.Result{}, r.setJobCompleted(ctx, job, v1.OpsJobFailed, err.Error(), nil)
	}

	// Generate target image name (without registry host)
	targetImage, err := generateTargetImageName(sourceImage)
	if err != nil {
		klog.ErrorS(err, "failed to generate target image name")
		return ctrlruntime.Result{}, r.setJobCompleted(ctx, job, v1.OpsJobFailed, err.Error(), nil)
	}

	// Create image record in database
	dbImage := &model.Image{
		Tag:            targetImage,
		Description:    fmt.Sprintf("Exported from workload %s, source: %s", workloadId, sourceImage),
		CreatedBy:      v1.GetUserName(job),
		CreatedAt:      time.Now().UTC(),
		Status:         common.ImageImportingStatus,
		Source:         "export",
		RelationDigest: map[string]interface{}{},
	}

	if err := r.dbClient.UpsertImage(ctx, dbImage); err != nil {
		klog.ErrorS(err, "failed to create image record in database")
		return ctrlruntime.Result{}, r.setJobCompleted(ctx, job, v1.OpsJobFailed, "failed to create image record", nil)
	}

	klog.Infof("Starting image export: workload=%s, source=%s, target=%s, imageId=%d",
		workloadId, sourceImage, targetImage, dbImage.ID)

	// Execute image copy using containers/image library (running in controller Pod, K8s cluster network)
	fullTargetImage := fmt.Sprintf("%s/%s", defaultRegistry.URL, targetImage)
	if err := r.copyImageDirectly(ctx, sourceImage, fullTargetImage, defaultRegistry); err != nil {
		klog.ErrorS(err, "failed to copy image", "source", sourceImage, "target", fullTargetImage)
		return ctrlruntime.Result{}, r.handleExportFailure(ctx, job, dbImage, err.Error())
	}

	// Update image status to ready
	dbImage.Status = "ready"
	dbImage.UpdatedAt = time.Now().UTC()
	if err := r.dbClient.UpsertImage(ctx, dbImage); err != nil {
		klog.ErrorS(err, "failed to update image status to ready", "imageId", dbImage.ID)
	}

	klog.Infof("Successfully exported image: workload=%s, target=%s, imageId=%d",
		workloadId, targetImage, dbImage.ID)

	// Update OpsJob status to succeeded
	outputs := []v1.Parameter{
		{Name: "status", Value: "completed"},
		{Name: "target", Value: targetImage},
		{Name: "message", Value: "Image exported successfully"},
	}
	return ctrlruntime.Result{}, r.setJobCompleted(ctx, job, v1.OpsJobSucceeded, "Image exported successfully", outputs)
}

// handleExportFailure handles export failure by updating database and job status
func (r *ExportImageJobReconciler) handleExportFailure(ctx context.Context, job *v1.OpsJob, dbImage *model.Image, errMsg string) error {
	// Update image status to failed
	dbImage.Status = "failed"
	dbImage.Description = fmt.Sprintf("%s (Error: %s)", dbImage.Description, errMsg)
	dbImage.UpdatedAt = time.Now().UTC()
	_ = r.dbClient.UpsertImage(ctx, dbImage)

	// Update OpsJob status to failed
	return r.setJobCompleted(ctx, job, v1.OpsJobFailed, errMsg, nil)
}

// copyImageDirectly copies image from source to Harbor using containers/image library
// This method runs in the controller Pod (K8s cluster network), same as Import image
func (r *ExportImageJobReconciler) copyImageDirectly(
	ctx context.Context,
	sourceImage string,
	targetImage string,
	registry *model.RegistryInfo,
) error {
	klog.Infof("Starting direct image copy: %s -> %s", sourceImage, targetImage)

	// Step 1: Parse source and destination image references
	srcRef, err := alltransports.ParseImageName(fmt.Sprintf("docker://%s", sourceImage))
	if err != nil {
		return fmt.Errorf("failed to parse source image: %w", err)
	}

	destRef, err := alltransports.ParseImageName(fmt.Sprintf("docker://%s", targetImage))
	if err != nil {
		return fmt.Errorf("failed to parse destination image: %w", err)
	}

	// Step 2: Create policy context (for signature verification)
	policyContext, err := signature.NewPolicyContext(&signature.Policy{
		Default: []signature.PolicyRequirement{signature.NewPRInsecureAcceptAnything()},
	})
	if err != nil {
		return fmt.Errorf("failed to create policy context: %w", err)
	}
	defer policyContext.Destroy()

	// Step 3: Setup source system context (for pulling from source registry)
	sourceCtx := &types.SystemContext{
		DockerInsecureSkipTLSVerify: types.OptionalBoolTrue,
	}

	// Step 4: Setup destination system context (for pushing to Harbor)
	destCtx, err := r.getHarborSystemContext(ctx, registry)
	if err != nil {
		return fmt.Errorf("failed to get Harbor system context: %w", err)
	}

	// Step 5: Execute image copy (registry-to-registry direct copy)
	klog.V(4).Infof("Copying image from %s to %s", sourceImage, targetImage)
	_, err = copy.Image(ctx, policyContext, destRef, srcRef, &copy.Options{
		SourceCtx:      sourceCtx,
		DestinationCtx: destCtx,
		ImageListSelection: copy.CopySystemImage,
		ReportWriter: nil, // Could add progress reporting here
	})
	if err != nil {
		return fmt.Errorf("image copy failed: %w", err)
	}

	klog.Infof("Successfully copied image %s to %s", sourceImage, targetImage)
	return nil
}

// getHarborSystemContext creates system context with Harbor authentication
func (r *ExportImageJobReconciler) getHarborSystemContext(ctx context.Context, registry *model.RegistryInfo) (*types.SystemContext, error) {
	// Decrypt username and password from database
	username, err := crypto.NewCrypto().Decrypt(registry.Username)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt Harbor username: %w", err)
	}

	password, err := crypto.NewCrypto().Decrypt(registry.Password)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt Harbor password: %w", err)
	}

	return &types.SystemContext{
		DockerInsecureSkipTLSVerify: types.OptionalBoolTrue,
		DockerAuthConfig: &types.DockerAuthConfig{
			Username: username,
			Password: password,
		},
	}, nil
}

// generateTargetImageName generates the target image name without registry host
// Format: Custom/namespace/repository:YYYYMMDD
func generateTargetImageName(sourceImage string) (string, error) {
	// Remove tag from source image
	imageWithoutTag := sourceImage
	if colonIndex := strings.LastIndex(sourceImage, ":"); colonIndex != -1 {
		imageWithoutTag = sourceImage[:colonIndex]
	}

	// Split by "/" to extract namespace/repository structure
	parts := strings.Split(imageWithoutTag, "/")
	if len(parts) == 0 {
		return "", fmt.Errorf("invalid source image format: %s", sourceImage)
	}

	var namespace, repository string
	
	if len(parts) == 1 {
		// Single part: "nginx" -> Custom/library/nginx
		namespace = "library"
		repository = parts[0]
	} else if len(parts) == 2 {
		// Two parts: "rocm/7.0-preview" -> Custom/rocm/7.0-preview
		namespace = parts[0]
		repository = parts[1]
	} else {
		// Three or more parts: "docker.io/library/nginx" -> Custom/library/nginx
		// Skip the registry host (first part), use the rest
		namespace = parts[len(parts)-2]
		repository = parts[len(parts)-1]
	}

	// Generate timestamp tag (YYYYMMDD format)
	timestamp := time.Now().Format("20060102")

	// Create target image path: custom/namespace/repository:YYYYMMDD
	// Convert to lowercase as Harbor requires lowercase repository names
	targetImage := fmt.Sprintf("%s/%s/%s:%s",
		strings.ToLower(registryProject), // "custom" (lowercase)
		strings.ToLower(namespace),
		strings.ToLower(repository),
		timestamp)

	return targetImage, nil
}

// getWorkloadIdFromJob extracts workload ID from OpsJob parameters
func getWorkloadIdFromJob(job *v1.OpsJob) string {
	param := job.GetParameter(v1.ParameterWorkload)
	if param != nil {
		return param.Value
	}
	return ""
}

// getSourceImageFromJob extracts source image from OpsJob parameters
func getSourceImageFromJob(job *v1.OpsJob) string {
	for _, param := range job.Spec.Inputs {
		if param.Name == "image" {
			return param.Value
		}
	}
	return ""
}
