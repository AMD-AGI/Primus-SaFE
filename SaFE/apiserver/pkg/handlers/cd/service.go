/*
 * Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package cd

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"
	"k8s.io/utils/ptr"

	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	dbclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
	dbutils "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/utils"
	commonutils "github.com/AMD-AIG-AIMA/SAFE/common/pkg/utils"
)

// Service handles business logic for CD
type Service struct {
	dbClient  *dbclient.Client
	clientSet kubernetes.Interface
}

func NewService(dbClient *dbclient.Client, clientSet kubernetes.Interface) *Service {
	return &Service{
		dbClient:  dbClient,
		clientSet: clientSet,
	}
}

const (
	JobNamespace = common.PrimusSafeNamespace // primus-safe system namespace
	JobImage     = "bitnami/kubectl:latest"   // Image with bash and necessary tools
)

type JobParams struct {
	Name          string
	Namespace     string
	Image         string
	ComponentTags string
	NodeAgentTags string
	EnvFileConfig string
}

// ExecuteDeployment simulates the deployment process
func (s *Service) ExecuteDeployment(ctx context.Context, req *dbclient.DeploymentRequest) (string, error) {
	klog.Infof("Starting deployment for request %d: %s", req.Id, req.DeployName)

	// 1. Parse config
	var config DeploymentConfig
	if err := json.Unmarshal([]byte(req.EnvConfig), &config); err != nil {
		return "", fmt.Errorf("failed to parse config: %v", err)
	}

	// Validate expected components
	expectedComponents := []string{
		"apiserver", "resource_manager", "job_manager",
		"webhooks", "web", "preprocess", "node_agent",
	}

	// Build tags string for sed/yq
	componentTags := ""
	nodeAgentTags := ""

	for _, comp := range expectedComponents {
		if tag, ok := config.ImageVersions[comp]; ok {
			if comp == "node_agent" {
				// node_agent format in values.yaml is "node_agent.image"
				// Assuming input is just the tag like "node-agent:latest"
				// We will pass "image: <tag>" to replace
				nodeAgentTags += fmt.Sprintf("%s=%s;", "image", tag)
			} else {
				// others format: "component.image"
				componentTags += fmt.Sprintf("%s.image=%s;", comp, tag)
			}
		}
	}

	// 2. Prepare Job Params
	jobName := commonutils.GenerateName(fmt.Sprintf("cd-upgrade-%d", req.Id))

	params := JobParams{
		Name:          jobName,
		Namespace:     JobNamespace,
		Image:         JobImage,
		ComponentTags: componentTags,
		NodeAgentTags: nodeAgentTags,
		EnvFileConfig: config.EnvFileConfig,
	}

	// 3. Create Job
	if err := s.createK8sJob(ctx, params); err != nil {
		return "", fmt.Errorf("failed to create k8s job: %v", err)
	}

	// 4. Update DB status to deploying
	req.Status = StatusDeploying
	// Store job name in description or separate field if possible, using Description for now
	desc := req.Description.String
	if desc == "" {
		desc = fmt.Sprintf("Job: %s", jobName)
	} else {
		desc = fmt.Sprintf("%s | Job: %s", desc, jobName)
	}
	req.Description = dbutils.NullString(desc)

	if err := s.dbClient.UpdateDeploymentRequest(ctx, req); err != nil {
		return jobName, err
	}

	return jobName, nil
}

func (s *Service) createK8sJob(ctx context.Context, params JobParams) error {
	// Prepare .env file content (base64 encoded for safe passing)
	envFileBase64 := ""
	if params.EnvFileConfig != "" {
		envFileBase64 = base64.StdEncoding.EncodeToString([]byte(params.EnvFileConfig))
	}

	argsScript := fmt.Sprintf(`
set -e
PRIMUS_VALUES="/primus/Primus-SaFE/SaFE/charts/primus-safe/values.yaml"
NODE_AGENT_VALUES="/primus/Primus-SaFE/SaFE/node-agent/charts/node-agent/values.yaml"
ENV_FILE="/primus/Primus-SaFE/SaFE/bootstrap/.env"

# Helper function to update yaml using sed (simple implementation)
# Usage: update_yaml "key.subkey" "new_value" "file"
update_yaml() {
    local key=$1
    local value=$2
    local file=$3
    # Escape slashes in value for sed
    local escaped_value=$(echo $value | sed 's/\//\\\//g')
    
    # Split key by dot to find parent block and child key
    # This simple sed assumes indentation is 2 spaces and structure matches
    # e.g. apiserver:\n  image: "old"
    local parent=$(echo $key | cut -d. -f1)
    local child=$(echo $key | cut -d. -f2)
    
    if [ "$parent" = "$child" ]; then
        # Top level key not supported by this simple sed yet or not needed for this use case
        echo "Top level key update not implemented for $key"
    else
        # Look for parent block, then update child key within it
        # This is a heuristic replacement and assumes standard formatting
        sed -i "/^$parent:/,/^[^ ]/ s/^[[:space:]]*$child:.*/  $child: \"$escaped_value\"/" "$file"
    fi
}

# Update .env file if provided
ENV_CONTENT="%s"
if [ -n "$ENV_CONTENT" ]; then
    echo "Updating .env file..."
    echo "$ENV_CONTENT" | base64 -d > "$ENV_FILE"
    echo "✓ .env file updated"
fi

# Update components
IFS=';' read -ra COMPS <<< "%s"
for comp in "${COMPS[@]}"; do
    if [ -n "$comp" ]; then
        KEY=$(echo $comp | cut -d= -f1)
        VAL=$(echo $comp | cut -d= -f2)
        update_yaml "$KEY" "$VAL" "$PRIMUS_VALUES"
        echo "✓ Updated $KEY in primus-safe/values.yaml"
    fi
done

# Update node-agent
IFS=';' read -ra AGENTS <<< "%s"
for agent in "${AGENTS[@]}"; do
    if [ -n "$agent" ]; then
        KEY=$(echo $agent | cut -d= -f1)
        VAL=$(echo $agent | cut -d= -f2)
        # node_agent block in node-agent values.yaml
        update_yaml "node_agent.$KEY" "$VAL" "$NODE_AGENT_VALUES"
        echo "✓ Updated node_agent.$KEY in node-agent/values.yaml"
    fi
done

echo "=========================================="
echo "Starting upgrade script..."
echo "=========================================="
/bin/bash /primus/Primus-SaFE/SaFE/bootstrap/newUpgrade.sh
`, envFileBase64, params.ComponentTags, params.NodeAgentTags)

	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      params.Name,
			Namespace: params.Namespace,
		},
		Spec: batchv1.JobSpec{
			TTLSecondsAfterFinished: ptr.To(int32(3600)),
			BackoffLimit:            ptr.To(int32(3)),
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					RestartPolicy: corev1.RestartPolicyOnFailure,
					Containers: []corev1.Container{
						{
							Name:    "upgrade-task",
							Image:   params.Image,
							Command: []string{"/bin/bash", "-c"},
							Args:    []string{argsScript},
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "primus-dir",
									MountPath: "/primus",
								},
							},
						},
					},
					Volumes: []corev1.Volume{
						{
							Name: "primus-dir",
							VolumeSource: corev1.VolumeSource{
								HostPath: &corev1.HostPathVolumeSource{
									Path: "/primus",
									Type: ptr.To(corev1.HostPathDirectory),
								},
							},
						},
					},
				},
			},
		},
	}

	_, err := s.clientSet.BatchV1().Jobs(params.Namespace).Create(ctx, job, metav1.CreateOptions{})
	return err
}

// WaitForJobCompletion waits for the K8s job to complete or fail
func (s *Service) WaitForJobCompletion(ctx context.Context, jobName, namespace string) error {
	// Simple polling mechanism
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	// Wait up to 30 minutes
	timeout := time.After(30 * time.Minute)

	for {
		select {
		case <-timeout:
			return fmt.Errorf("job execution timed out")
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			job, err := s.clientSet.BatchV1().Jobs(namespace).Get(ctx, jobName, metav1.GetOptions{})
			if err != nil {
				klog.ErrorS(err, "Failed to get job status", "job", jobName)
				continue
			}

			if job.Status.Succeeded > 0 {
				klog.Infof("Job %s succeeded", jobName)
				return nil
			}

			if job.Status.Failed > 0 && job.Status.Failed >= *job.Spec.BackoffLimit+1 {
				return fmt.Errorf("job execution failed")
			}
		}
	}
}

// VerifyDeploymentRollout checks if the workloads defined in config are actually running
func (s *Service) VerifyDeploymentRollout(ctx context.Context, envConfig string) error {
	// 1. Parse config to know which components to check
	var config DeploymentConfig
	if err := json.Unmarshal([]byte(envConfig), &config); err != nil {
		return fmt.Errorf("failed to parse config during verification: %v", err)
	}

	// 2. Define component mapping: key from ImageVersions -> K8s Deployment Name
	// Based on "kgd -n primus-safe"
	componentMap := map[string]string{
		"apiserver":        "primus-safe-apiserver",
		"resource_manager": "primus-safe-resource-manager",
		"job_manager":      "primus-safe-job-manager",
		"webhooks":         "primus-safe-webhooks",
		"web":              "primus-safe-web",
		// Add other mappings if needed
	}

	// 3. Poll for readiness
	// We wait up to 5 minutes for the pods to be ready
	timeout := time.After(5 * time.Minute)
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	klog.Infof("Starting deployment rollout verification...")

	for {
		select {
		case <-timeout:
			return fmt.Errorf("deployment verification timed out: workloads did not become ready in time")
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			allReady := true
			notReadyList := []string{}

			for comp, _ := range config.ImageVersions {
				// Skip if not in our mapping (e.g. node_agent is DaemonSet, handled differently or ignored for now)
				deploymentName, ok := componentMap[comp]
				if !ok {
					continue
				}

				if err := s.checkDeploymentStatus(ctx, deploymentName, JobNamespace); err != nil {
					// If it's a critical image error, fail immediately
					if strings.Contains(err.Error(), "ImagePullBackOff") || strings.Contains(err.Error(), "ErrImagePull") {
						return fmt.Errorf("deployment failed for %s: %v", deploymentName, err)
					}
					// Otherwise just not ready yet
					allReady = false
					notReadyList = append(notReadyList, deploymentName)
				}
			}

			if allReady {
				klog.Infof("All deployed components are ready.")
				return nil
			} else {
				klog.V(4).Infof("Waiting for components to be ready: %v", notReadyList)
			}
		}
	}
}

// checkDeploymentStatus checks if a deployment is fully available and free of image errors
func (s *Service) checkDeploymentStatus(ctx context.Context, name, namespace string) error {
	// 1. Check Deployment status
	deploy, err := s.clientSet.AppsV1().Deployments(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("failed to get deployment %s: %v", name, err)
	}

	// Check if generation matches (ensure we are observing the new version)
	if deploy.Generation != deploy.Status.ObservedGeneration {
		return fmt.Errorf("deployment generation mismatch")
	}

	// Check readiness
	if deploy.Status.AvailableReplicas == *deploy.Spec.Replicas {
		// Ready!
		return nil
	}

	// 2. Deep dive into Pods to check for Image errors
	// Select pods for this deployment
	labelSelector := metav1.FormatLabelSelector(deploy.Spec.Selector)
	pods, err := s.clientSet.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{
		LabelSelector: labelSelector,
	})
	if err != nil {
		return fmt.Errorf("failed to list pods for %s: %v", name, err)
	}

	for _, pod := range pods.Items {
		// Check container statuses
		for _, containerStatus := range pod.Status.ContainerStatuses {
			// Check Waiting state (ImagePullBackOff, CrashLoopBackOff, etc.)
			if !containerStatus.Ready && containerStatus.State.Waiting != nil {
				reason := containerStatus.State.Waiting.Reason
				if reason == "ImagePullBackOff" || reason == "ErrImagePull" || reason == "CrashLoopBackOff" {
					return fmt.Errorf("%s: %s (%s)", pod.Name, reason, containerStatus.State.Waiting.Message)
				}
			}

			// Check Terminated state (Container crashed)
			if containerStatus.State.Terminated != nil && containerStatus.State.Terminated.ExitCode != 0 {
				return fmt.Errorf("%s: Container terminated with exit code %d (%s)",
					pod.Name, containerStatus.State.Terminated.ExitCode, containerStatus.State.Terminated.Reason)
			}
		}
	}

	return fmt.Errorf("not ready")
}

func (s *Service) UpdateRequestStatus(ctx context.Context, reqId int64, status, reason string) error {
	req, err := s.dbClient.GetDeploymentRequest(ctx, reqId)
	if err != nil {
		klog.ErrorS(err, "Failed to get request for update", "id", reqId)
		return err
	}

	req.Status = status
	if reason != "" {
		desc := req.Description.String
		if desc != "" {
			desc += ". " + reason
		} else {
			desc = reason
		}
		req.Description = dbutils.NullString(desc)
	}

	return s.dbClient.UpdateDeploymentRequest(ctx, req)
}

// Rollback creates a new request based on a previous snapshot
func (s *Service) Rollback(ctx context.Context, reqId int64, username string) (int64, error) {
	// Find the snapshot associated with the request
	// In a real system, we might look up the snapshot by request ID, or find the *previous* successful deployment
	// For this design, we assume we rollback TO the state of reqId.

	targetReq, err := s.dbClient.GetDeploymentRequest(ctx, reqId)
	if err != nil {
		return 0, err
	}

	if targetReq.Status != StatusDeployed && targetReq.Status != StatusRolledBack {
		return 0, fmt.Errorf("cannot rollback to a request with status %s (must be %s or %s)",
			targetReq.Status, StatusDeployed, StatusRolledBack)
	}

	// Create a new request that applies the old config
	newReq := &dbclient.DeploymentRequest{
		DeployName:     username,
		Status:         StatusPendingApproval, // Or auto-approve for rollback?
		EnvConfig:      targetReq.EnvConfig,
		Description:    dbutils.NullString(fmt.Sprintf("Rollback to version from request %d", reqId)),
		RollbackFromId: sql.NullInt64{Int64: reqId, Valid: true},
	}

	return s.dbClient.CreateDeploymentRequest(ctx, newReq)
}

// GetCurrentEnvConfig reads the current .env file content
// It tries to read from the actual file first, and falls back to the latest snapshot if file read fails
func (s *Service) GetCurrentEnvConfig(ctx context.Context) (content string, err error) {
	envFilePath := "/primus/Primus-SaFE/SaFE/bootstrap/.env"

	// 1. Try to read actual file (recommended approach)
	fileContent, readErr := os.ReadFile(envFilePath)
	if readErr == nil {
		return string(fileContent), nil
	}

	klog.Warningf("Failed to read .env file at %s, falling back to latest snapshot: %v", envFilePath, readErr)

	// 2. Fallback: Get from the latest successful deployment snapshot
	// Query the most recent snapshot
	snapshots, err := s.dbClient.ListEnvironmentSnapshots(ctx, nil, []string{"created_at DESC"}, 1, 0)
	if err != nil {
		return "", fmt.Errorf("failed to read .env file and no snapshot available: file_error=%v, db_error=%v", readErr, err)
	}

	if len(snapshots) == 0 {
		return "", fmt.Errorf("failed to read .env file and no snapshot available: %v", readErr)
	}

	// Parse the snapshot config
	var config DeploymentConfig
	if err := json.Unmarshal([]byte(snapshots[0].EnvConfig), &config); err != nil {
		return "", fmt.Errorf("failed to parse snapshot config: %v", err)
	}

	if config.EnvFileConfig == "" {
		return "", fmt.Errorf("snapshot does not contain env_file_config")
	}

	return config.EnvFileConfig, nil
}

// CreateSnapshot creates a backup of the current FULL state
// It merges the new request config with the previous snapshot to ensure complete state record
func (s *Service) CreateSnapshot(ctx context.Context, reqId int64, newConfigStr string) error {
	// 1. Parse new config (partial or full)
	var newConfig DeploymentConfig
	if err := json.Unmarshal([]byte(newConfigStr), &newConfig); err != nil {
		return fmt.Errorf("failed to parse new config: %v", err)
	}

	// 2. Get latest snapshot to find previous state
	var finalConfig DeploymentConfig

	snapshots, err := s.dbClient.ListEnvironmentSnapshots(ctx, nil, []string{"created_at DESC"}, 1, 0)
	if err == nil && len(snapshots) > 0 {
		// Parse previous config
		if err := json.Unmarshal([]byte(snapshots[0].EnvConfig), &finalConfig); err != nil {
			klog.Warningf("Failed to parse previous snapshot config: %v", err)
			// If failed to parse previous, we start fresh
			finalConfig = DeploymentConfig{
				ImageVersions: make(map[string]string),
			}
		}
	} else {
		// No previous snapshot, initialize empty
		finalConfig = DeploymentConfig{
			ImageVersions: make(map[string]string),
		}
	}

	// 3. Merge Configs
	// 3.1 Merge Image Versions
	if finalConfig.ImageVersions == nil {
		finalConfig.ImageVersions = make(map[string]string)
	}
	for component, version := range newConfig.ImageVersions {
		finalConfig.ImageVersions[component] = version
	}

	// 3.2 Merge Env File Config
	// Only update if new config provides a non-empty env file content
	if newConfig.EnvFileConfig != "" {
		finalConfig.EnvFileConfig = newConfig.EnvFileConfig
	}
	// If newConfig.EnvFileConfig is empty, we keep finalConfig.EnvFileConfig (from previous snapshot)

	// 4. Marshal final merged config
	finalConfigJSON, err := json.Marshal(finalConfig)
	if err != nil {
		return fmt.Errorf("failed to marshal final config: %v", err)
	}

	// 5. Save to DB
	snapshot := &dbclient.EnvironmentSnapshot{
		DeploymentRequestId: reqId,
		EnvConfig:           string(finalConfigJSON),
	}
	_, err = s.dbClient.CreateEnvironmentSnapshot(ctx, snapshot)
	return err
}

func (s *Service) cvtDBRequestToItem(req *dbclient.DeploymentRequest) *DeploymentRequestItem {
	return &DeploymentRequestItem{
		Id:             req.Id,
		DeployName:     req.DeployName,
		Status:         req.Status,
		ApproverName:   dbutils.ParseNullString(req.ApproverName),
		ApprovalResult: dbutils.ParseNullString(req.ApprovalResult),
		Description:    dbutils.ParseNullString(req.Description),
		RollbackFromId: req.RollbackFromId.Int64,
		CreatedAt:      dbutils.ParseNullTimeToString(req.CreatedAt),
		UpdatedAt:      dbutils.ParseNullTimeToString(req.UpdatedAt),
		ApprovedAt:     dbutils.ParseNullTimeToString(req.ApprovedAt),
	}
}
