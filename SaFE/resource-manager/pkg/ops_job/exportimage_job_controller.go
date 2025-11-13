/*
 * Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package ops_job

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
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
	dbclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client/model"
	commonerrors "github.com/AMD-AIG-AIMA/SAFE/common/pkg/errors"
	rmutils "github.com/AMD-AIG-AIMA/SAFE/resource-manager/pkg/utils"
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

// RegistryAuth represents Docker registry authentication structure
type RegistryAuth struct {
	Auths map[string]RegistryAuthItem `json:"auths"`
}

// RegistryAuthItem represents authentication item for a registry
type RegistryAuthItem struct {
	Auth string `json:"auth"`
}

// ExportImageJobReconciler reconciles image export jobs using SSH + nerdctl
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

	// Get workload to find the node
	workload := &v1.Workload{}
	if err := r.Get(ctx, client.ObjectKey{Name: workloadId}, workload); err != nil {
		klog.ErrorS(err, "failed to get workload", "workloadId", workloadId)
		return ctrlruntime.Result{}, r.setJobCompleted(ctx, job, v1.OpsJobFailed, "failed to get workload", nil)
	}

	// Get node information from workload pods
	if len(workload.Status.Pods) == 0 {
		err := commonerrors.NewBadRequest("workload has no pods")
		return ctrlruntime.Result{}, r.setJobCompleted(ctx, job, v1.OpsJobFailed, err.Error(), nil)
	}

	// Use the first pod's admin node name
	adminNodeName := workload.Status.Pods[0].AdminNodeName
	if adminNodeName == "" {
		err := commonerrors.NewBadRequest("workload pod is not scheduled to any node")
		return ctrlruntime.Result{}, r.setJobCompleted(ctx, job, v1.OpsJobFailed, err.Error(), nil)
	}

	node := &v1.Node{}
	if err := r.Get(ctx, client.ObjectKey{Name: adminNodeName}, node); err != nil {
		klog.ErrorS(err, "failed to get node", "nodeName", adminNodeName)
		return ctrlruntime.Result{}, r.setJobCompleted(ctx, job, v1.OpsJobFailed, "failed to get node", nil)
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

	klog.Infof("Starting image export: workload=%s, source=%s, target=%s, imageId=%d, node=%s",
		workloadId, sourceImage, targetImage, dbImage.ID, node.Name)

	// Get Harbor credentials from Secret
	harborAuth, err := r.getHarborCredentials(ctx, defaultRegistry.URL)
	if err != nil {
		klog.ErrorS(err, "failed to get Harbor credentials")
		return ctrlruntime.Result{}, r.handleExportFailure(ctx, job, dbImage, err.Error())
	}

	// Execute image export via SSH
	if err := r.exportImageViaSSH(ctx, node, sourceImage, targetImage, defaultRegistry.URL, harborAuth); err != nil {
		klog.ErrorS(err, "failed to export image", "source", sourceImage, "target", targetImage)
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

// getHarborCredentials retrieves Harbor credentials from Secret
// This function reads the same Secret format as used in image.go (ImageImportSecretName)
// Secret structure: {"auths": {"registry_url": {"auth": "base64(username:password)"}}}
func (r *ExportImageJobReconciler) getHarborCredentials(ctx context.Context, registryURL string) (string, error) {
	// Get Secret
	secret := &corev1.Secret{}
	if err := r.Get(ctx, apitypes.NamespacedName{
		Name:      imageImportSecretName,
		Namespace: defaultNamespace,
	}, secret); err != nil {
		return "", fmt.Errorf("failed to get secret %s: %w", imageImportSecretName, err)
	}

	// Parse config.json
	configData, ok := secret.Data["config.json"]
	if !ok {
		return "", fmt.Errorf("config.json not found in secret")
	}

	var registryAuth RegistryAuth
	if err := json.Unmarshal(configData, &registryAuth); err != nil {
		return "", fmt.Errorf("failed to parse registry auth: %w", err)
	}

	// Find auth for the registry
	authItem, ok := registryAuth.Auths[registryURL]
	if !ok {
		return "", fmt.Errorf("no authentication found for registry %s", registryURL)
	}

	// Return base64 encoded auth string (format: base64(username:password))
	// This is the same format as Docker config.json uses
	return authItem.Auth, nil
}

// exportImageViaSSH exports image using SSH connection and nerdctl commands
func (r *ExportImageJobReconciler) exportImageViaSSH(
	ctx context.Context,
	node *v1.Node,
	sourceImage string,
	targetImage string,
	registry string,
	harborAuth string,
) error {
	// Establish SSH connection
	sshClient, err := rmutils.GetSSHClient(ctx, r.Client, node)
	if err != nil {
		return fmt.Errorf("failed to create SSH client: %w", err)
	}
	defer sshClient.Close()

	klog.Infof("SSH connected to node %s", node.Name)

	// Ensure registry uses HTTPS protocol for Harbor authentication
	// Remove any existing protocol prefix first, then add https://
	registryClean := strings.TrimPrefix(registry, "https://")
	registryClean = strings.TrimPrefix(registryClean, "http://")
	registryWithHTTPS := "https://" + registryClean

	klog.Infof("Using HTTPS for Harbor registry: %s", registryWithHTTPS)

	// Step 1: Login to Harbor (use HTTPS URL)
	if err := r.loginHarbor(sshClient, registryWithHTTPS, harborAuth); err != nil {
		return fmt.Errorf("failed to login Harbor: %w", err)
	}

	// Construct full image path with registry for tag and push operations
	// Note: Use registry without protocol for docker tag/push commands
	fullTargetImage := fmt.Sprintf("%s/%s", registryClean, targetImage)

	// Step 2: Tag image
	if err := r.tagImage(sshClient, sourceImage, fullTargetImage); err != nil {
		return fmt.Errorf("failed to tag image: %w", err)
	}

	// Step 3: Push image
	if err := r.pushImage(sshClient, fullTargetImage); err != nil {
		return fmt.Errorf("failed to push image: %w", err)
	}

	klog.Infof("Successfully pushed image %s to registry %s", targetImage, registryClean)
	return nil
}

// loginHarbor logs into Harbor registry using nerdctl
// It creates the Docker config.json file directly instead of using nerdctl login command
func (r *ExportImageJobReconciler) loginHarbor(sshClient *ssh.Client, registry string, auth string) error {
	session, err := sshClient.NewSession()
	if err != nil {
		return fmt.Errorf("failed to create SSH session: %w", err)
	}
	defer session.Close()

	// Extract registry host without protocol for config.json key
	registryHost := strings.TrimPrefix(registry, "https://")
	registryHost = strings.TrimPrefix(registryHost, "http://")

	// Create config.json with the auth
	configJSON := fmt.Sprintf(`{"auths":{"%s":{"auth":"%s"},"%s":{"auth":"%s"}}}`, 
		registryHost, auth,  // Key without protocol (primary)
		registry, auth)       // Key with protocol (fallback)
	
	// Write config.json to /root/.docker/ (nerdctl will read from there)
	cmd := fmt.Sprintf(`sudo mkdir -p /root/.docker && echo '%s' | sudo tee /root/.docker/config.json > /dev/null`, configJSON)

	klog.V(4).Infof("Configuring Docker auth for registry: %s (host: %s)", registry, registryHost)

	output, err := session.CombinedOutput(cmd)
	if err != nil {
		return fmt.Errorf("failed to configure auth: %s, error: %w", string(output), err)
	}

	klog.Infof("Successfully configured auth for Harbor registry %s", registryHost)
	return nil
}

// tagImage tags the source image with target name
func (r *ExportImageJobReconciler) tagImage(sshClient *ssh.Client, sourceImage string, targetImage string) error {
	session, err := sshClient.NewSession()
	if err != nil {
		return fmt.Errorf("failed to create SSH session: %w", err)
	}
	defer session.Close()

	cmd := fmt.Sprintf("sudo nerdctl tag %s %s", sourceImage, targetImage)
	klog.V(4).Infof("Tagging image: %s -> %s", sourceImage, targetImage)

	output, err := session.CombinedOutput(cmd)
	if err != nil {
		return fmt.Errorf("tag failed: %s, error: %w", string(output), err)
	}

	klog.Infof("Successfully tagged image %s as %s", sourceImage, targetImage)
	return nil
}

// pushImage pushes the image to registry using nerdctl
func (r *ExportImageJobReconciler) pushImage(sshClient *ssh.Client, imageName string) error {
	session, err := sshClient.NewSession()
	if err != nil {
		return fmt.Errorf("failed to create SSH session: %w", err)
	}
	defer session.Close()

	cmd := fmt.Sprintf("sudo nerdctl push %s", imageName)
	klog.V(4).Infof("Pushing image: %s", imageName)

	output, err := session.CombinedOutput(cmd)
	if err != nil {
		return fmt.Errorf("push failed: %s, error: %w", string(output), err)
	}

	klog.Infof("Successfully pushed image %s", imageName)
	return nil
}

// generateTargetImageName generates the target image name without registry host
// Format: Custom/namespace/repository:YYYYMMDD
// Example: "rocm/7.0-preview:tag" -> "Custom/rocm/7.0-preview:20250112"
func generateTargetImageName(sourceImage string) (string, error) {
	// Remove tag from source image
	imageWithoutTag := sourceImage
	if colonIndex := strings.LastIndex(sourceImage, ":"); colonIndex != -1 {
		imageWithoutTag = sourceImage[:colonIndex]
	}

	// Split by "/" to extract namespace/repository structure
	// Examples:
	//   "rocm/7.0-preview" -> ["rocm", "7.0-preview"]
	//   "docker.io/library/nginx" -> ["docker.io", "library", "nginx"]
	//   "nginx" -> ["nginx"]
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
