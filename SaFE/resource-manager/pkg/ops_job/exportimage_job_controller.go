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

	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/utils"
	"golang.org/x/crypto/ssh"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	apitypes "k8s.io/apimachinery/pkg/types"
	"k8s.io/klog/v2"
	ctrlruntime "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	commonconfig "github.com/AMD-AIG-AIMA/SAFE/common/pkg/config"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/controller"
	dbclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
	commonerrors "github.com/AMD-AIG-AIMA/SAFE/common/pkg/errors"
	rmutils "github.com/AMD-AIG-AIMA/SAFE/resource-manager/pkg/utils"
)

const (
	// Default concurrent workers for image export
	exportImageDefaultConcurrent = 3

	// Registry project name
	registryProject = "Custom"
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
			Client:        mgr.GetClient(),
			clientManager: utils.NewObjectManagerSingleton(),
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

	klog.Infof("Starting image export: workload=%s, source=%s, target=%s, node=%s",
		workloadId, sourceImage, targetImage, node.Name)

	// Get Harbor credentials from Secret
	harborUsername, harborPassword, err := r.getHarborCredentials(ctx, defaultRegistry.URL)
	if err != nil {
		klog.ErrorS(err, "failed to get Harbor credentials")
		return ctrlruntime.Result{}, r.setJobCompleted(ctx, job, v1.OpsJobFailed, err.Error(), nil)
	}

	// Get Pod name and namespace from workload
	podName := workload.Status.Pods[0].PodId
	if podName == "" {
		err := commonerrors.NewBadRequest("workload pod name is empty")
		return ctrlruntime.Result{}, r.setJobCompleted(ctx, job, v1.OpsJobFailed, err.Error(), nil)
	}

	namespace := v1.GetWorkspaceId(workload) // Pod namespace is the workspace ID
	cluster := v1.GetClusterId(workload)

	// Get container ID from Pod
	containerID, err := r.getContainerIDFromPod(ctx, podName, cluster, namespace)
	if err != nil {
		klog.ErrorS(err, "failed to get container ID", "pod", podName, "namespace", namespace)
		return ctrlruntime.Result{}, r.setJobCompleted(ctx, job, v1.OpsJobFailed, fmt.Sprintf("failed to get container ID: %v", err), nil)
	}

	// Execute image export via SSH
	if err := r.exportImageViaSSH(ctx, node, targetImage, containerID, defaultRegistry.URL, harborUsername, harborPassword); err != nil {
		klog.ErrorS(err, "failed to export image", "source", sourceImage, "target", targetImage)
		return ctrlruntime.Result{}, r.setJobCompleted(ctx, job, v1.OpsJobFailed, err.Error(), nil)
	}

	// Construct full target image path (with registry) for outputs
	fullTargetImage := fmt.Sprintf("%s/%s", defaultRegistry.URL, targetImage)

	klog.Infof("Successfully exported image: workload=%s, target=%s", workloadId, fullTargetImage)

	// Update OpsJob status to succeeded with full image path
	outputs := []v1.Parameter{
		{Name: "status", Value: "completed"},
		{Name: "target", Value: fullTargetImage}, // Store full path with registry
		{Name: "message", Value: "Image exported successfully"},
	}
	return ctrlruntime.Result{}, r.setJobCompleted(ctx, job, v1.OpsJobSucceeded, "Image exported successfully", outputs)
}

// getContainerIDFromPod retrieves the container ID from a Kubernetes Pod
// Returns the container ID without the runtime prefix (e.g., removes "containerd://")
func (r *ExportImageJobReconciler) getContainerIDFromPod(ctx context.Context, podName, cluster, namespace string) (string, error) {
	// Get Pod from Kubernetes
	pod := &corev1.Pod{}
	k8sClients, err := rmutils.GetK8sClientFactory(r.clientManager, cluster)
	if err != nil {
		return "", err
	}
	pod, err = k8sClients.ClientSet().CoreV1().Pods(namespace).Get(ctx, podName, metav1.GetOptions{})
	if err != nil {
		return "", fmt.Errorf("failed to get Pod %s/%s: %w", namespace, podName, err)
	}

	// Check if Pod has container statuses
	if len(pod.Status.ContainerStatuses) == 0 {
		return "", fmt.Errorf("pod %s has no container statuses", podName)
	}

	// Get the first container's ID
	containerID := pod.Status.ContainerStatuses[0].ContainerID
	if containerID == "" {
		return "", fmt.Errorf("container ID is empty for Pod %s", podName)
	}

	// Remove runtime prefix (e.g., "containerd://6bf0a2bc63a5...")
	if strings.Contains(containerID, "://") {
		parts := strings.SplitN(containerID, "://", 2)
		if len(parts) == 2 {
			containerID = parts[1]
		}
	}

	klog.V(4).Infof("Retrieved container ID for Pod %s: %s", podName, containerID)
	return containerID, nil
}

// getHarborCredentials retrieves Harbor username and password from Secret
// This function reads the same Secret format as used in image.go (ImageImportSecretName)
// Secret structure: {"auths": {"registry_url": {"auth": "base64(username:password)"}}}
// Returns decoded username and password for nerdctl login
func (r *ExportImageJobReconciler) getHarborCredentials(ctx context.Context, registryURL string) (username, password string, err error) {
	// Get Secret
	secret := &corev1.Secret{}
	if err := r.Get(ctx, apitypes.NamespacedName{
		Name:      common.ImageImportSecretName,
		Namespace: common.DefaultNamespace,
	}, secret); err != nil {
		return "", "", fmt.Errorf("failed to get secret %s: %w", common.ImageImportSecretName, err)
	}

	// Parse config.json
	configData, ok := secret.Data["config.json"]
	if !ok {
		return "", "", fmt.Errorf("config.json not found in secret")
	}

	var registryAuth RegistryAuth
	if err := json.Unmarshal(configData, &registryAuth); err != nil {
		return "", "", fmt.Errorf("failed to parse registry auth: %w", err)
	}

	// Find auth for the registry
	authItem, ok := registryAuth.Auths[registryURL]
	if !ok {
		return "", "", fmt.Errorf("no authentication found for registry %s", registryURL)
	}

	// Decode base64 auth to get username:password
	// auth is base64 encoded string: base64("username:password")
	authDecoded, err := base64.StdEncoding.DecodeString(authItem.Auth)
	if err != nil {
		return "", "", fmt.Errorf("failed to decode auth for registry %s: %w", registryURL, err)
	}

	// Split username:password
	credentials := string(authDecoded)
	parts := strings.SplitN(credentials, ":", 2)
	if len(parts) != 2 {
		return "", "", fmt.Errorf("invalid auth format for registry %s, expected username:password", registryURL)
	}

	klog.V(4).Infof("Retrieved credentials for registry %s, username: %s", registryURL, parts[0])
	return parts[0], parts[1], nil
}

// exportImageViaSSH exports image using SSH connection and nerdctl commands
func (r *ExportImageJobReconciler) exportImageViaSSH(
	ctx context.Context,
	node *v1.Node,
	targetImage string,
	containerID string,
	registry string,
	harborUsername string,
	harborPassword string,
) error {
	// Establish SSH connection
	sshClient, err := rmutils.GetSSHClient(ctx, r.Client, node)
	if err != nil {
		return fmt.Errorf("failed to create SSH client: %w", err)
	}
	defer sshClient.Close()

	klog.Infof("SSH connected to node %s", node.Name)

	// Construct full image path with registry
	fullTargetImage := fmt.Sprintf("%s/%s", registry, targetImage)

	// Step 1: Commit container to image (creates image directly with target name)
	if err := r.commitContainerToImage(sshClient, containerID, fullTargetImage); err != nil {
		return fmt.Errorf("failed to commit container: %w", err)
	}

	// Step 2: Login to Harbor
	if err := r.loginHarbor(sshClient, registry, harborUsername, harborPassword); err != nil {
		return fmt.Errorf("failed to login Harbor: %w", err)
	}

	// Step 3: Push image
	if err := r.pushImage(sshClient, fullTargetImage); err != nil {
		return fmt.Errorf("failed to push image: %w", err)
	}

	// Step 4: Clean up local image (optional, to save node storage)
	if err := r.deleteImage(ctx, sshClient, fullTargetImage); err != nil {
		// Log warning but don't fail the job if cleanup fails
		klog.Warningf("Failed to delete local image %s: %v ", fullTargetImage, err)
	}

	klog.Infof("Successfully pushed image %s to registry %s", targetImage, registry)
	return nil
}

// commitContainerToImage commits a container to an image by container ID
// Uses the real container ID obtained from Kubernetes Pod status
func (r *ExportImageJobReconciler) commitContainerToImage(sshClient *ssh.Client, containerID, targetImage string) error {
	session, err := sshClient.NewSession()
	if err != nil {
		return fmt.Errorf("failed to create SSH session: %w", err)
	}
	defer session.Close()

	klog.Infof("Committing container %s to image %s", containerID, targetImage)

	// Use nerdctl commit with container ID
	// containerID should already have the runtime prefix removed (e.g., "6bf0a2bc63a5...")
	commitCmd := fmt.Sprintf("sudo nerdctl commit %s %s", containerID, targetImage)
	klog.V(4).Infof("Executing: %s", commitCmd)

	output, err := session.CombinedOutput(commitCmd)
	if err != nil {
		return fmt.Errorf("commit failed: %s, error: %w", string(output), err)
	}

	klog.Infof("Successfully committed container %s to image %s", containerID, targetImage)
	return nil
}

// loginHarbor logs into Harbor registry using nerdctl login command
func (r *ExportImageJobReconciler) loginHarbor(sshClient *ssh.Client, registry, username, password string) error {
	session, err := sshClient.NewSession()
	if err != nil {
		return fmt.Errorf("failed to create SSH session: %w", err)
	}
	defer session.Close()

	klog.Infof("Logging into Harbor registry %s as user %s", registry, username)

	// Use nerdctl login with password stdin for security
	// Password won't appear in process list or command history
	cmd := fmt.Sprintf("echo '%s' | sudo nerdctl login %s -u %s --password-stdin", password, registry, username)

	klog.V(4).Infof("Executing nerdctl login for registry %s", registry)

	output, err := session.CombinedOutput(cmd)
	outputStr := string(output)

	if err != nil {
		return fmt.Errorf("nerdctl login failed: %s, error: %w", outputStr, err)
	}

	// Check if login succeeded
	if !strings.Contains(strings.ToLower(outputStr), "login succeeded") &&
		!strings.Contains(strings.ToLower(outputStr), "logged in") {
		return fmt.Errorf("nerdctl login returned unexpected output: %s", outputStr)
	}

	klog.Infof("Successfully logged into Harbor registry %s", registry)
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

// deleteImage deletes an image from the node using nerdctl rmi
func (r *ExportImageJobReconciler) deleteImage(ctx context.Context, sshClient *ssh.Client, imageName string) error {
	session, err := sshClient.NewSession()
	if err != nil {
		return fmt.Errorf("failed to create SSH session: %w", err)
	}
	defer session.Close()

	klog.Infof("Deleting image: %s", imageName)

	// Use nerdctl rmi to remove image
	cmd := fmt.Sprintf("sudo nerdctl rmi %s", imageName)
	klog.V(4).Infof("Executing: %s", cmd)

	output, err := session.CombinedOutput(cmd)
	if err != nil {
		return fmt.Errorf("delete image failed: %s, error: %w", string(output), err)
	}

	klog.Infof("Successfully deleted image %s", imageName)
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

	// Generate timestamp tag (YYYYMMDDHHmmss format - precise to seconds)
	timestamp := time.Now().Format("200601021504")

	// Create target image path: custom/namespace/repository:YYYYMMDDHHmmss
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
