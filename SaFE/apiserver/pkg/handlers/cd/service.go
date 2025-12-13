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
	"strings"
	"time"

	sqrl "github.com/Masterminds/squirrel"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"
	"k8s.io/utils/ptr"

	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	commonconfig "github.com/AMD-AIG-AIMA/SAFE/common/pkg/config"
	dbclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
	dbutils "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/utils"
	commonutils "github.com/AMD-AIG-AIMA/SAFE/common/pkg/utils"
)

// Service handles business logic for CD
type Service struct {
	dbClient      *dbclient.Client
	clientSet     kubernetes.Interface
	clientManager *commonutils.ObjectManager
	// onDeploymentFailure is called when a deployment fails (for email notification, etc.)
	onDeploymentFailure func(ctx context.Context, req *dbclient.DeploymentRequest, reason string)
}

func NewService(dbClient *dbclient.Client, clientSet kubernetes.Interface) *Service {
	return &Service{
		dbClient:      dbClient,
		clientSet:     clientSet,
		clientManager: commonutils.NewObjectManagerSingleton(),
	}
}

// SetDeploymentFailureCallback sets the callback function for deployment failures
func (s *Service) SetDeploymentFailureCallback(callback func(ctx context.Context, req *dbclient.DeploymentRequest, reason string)) {
	s.onDeploymentFailure = callback
}

// notifyDeploymentFailure calls the failure callback if set
func (s *Service) notifyDeploymentFailure(ctx context.Context, req *dbclient.DeploymentRequest, reason string) {
	if s.onDeploymentFailure != nil {
		s.onDeploymentFailure(ctx, req, reason)
	}
}

const (
	JobNamespace = common.PrimusSafeNamespace  // primus-safe system namespace
	JobImage     = "dtzar/helm-kubectl:latest" // Image with bash and necessary tools

	// Git repository URL for Primus-SaFE
	PrimusSaFERepoURL = "https://github.com/AMD-AGI/Primus-SaFE.git"
	// Container mount path for CD workspace
	ContainerMountPath = "/home/primus-safe-cd"
	// Host path on the node for persistent storage
	HostMountPath = "/mnt/primus-safe-cd"

	// AdminClusterID is the cluster.id of the admin/management cluster
	// This cluster doesn't need remote kubeconfig connection
	AdminClusterID = "tw-project2"
)

type JobParams struct {
	Name          string
	Namespace     string
	Image         string
	ComponentTags string
	NodeAgentTags string
	EnvFileConfig string
}

// RemoteClusterJobParams contains parameters for remote cluster update job
type RemoteClusterJobParams struct {
	Name             string
	Namespace        string
	Image            string
	NodeAgentImage   string // New node-agent image to deploy
	CICDRunnerImage  string // New cicd-runner image
	CICDUnifiedImage string // New cicd-unified-job image
	HasNodeAgent     bool
	HasCICD          bool
}

// DeploymentResult contains the result of ExecuteDeployment
type DeploymentResult struct {
	LocalJobName     string
	HasNodeAgent     bool
	HasCICD          bool
	NodeAgentImage   string
	CICDRunnerImage  string
	CICDUnifiedImage string
}

// ExecuteDeployment executes the deployment process and returns deployment result
func (s *Service) ExecuteDeployment(ctx context.Context, req *dbclient.DeploymentRequest) (*DeploymentResult, error) {
	klog.Infof("Starting deployment for request %d: %s", req.Id, req.DeployName)

	// 1. Parse current request config (user's incremental request)
	var requestConfig DeploymentConfig
	if err := json.Unmarshal([]byte(req.EnvConfig), &requestConfig); err != nil {
		return nil, fmt.Errorf("failed to parse config: %v", err)
	}

	// 2. Read latest snapshot and merge with current request for deployment
	// This ensures we have all historical image versions for the deployment
	// Note: We don't modify req.EnvConfig, it keeps the user's incremental request
	mergedConfig, err := s.mergeWithLatestSnapshot(ctx, requestConfig)
	if err != nil {
		klog.Warningf("Failed to merge with latest snapshot (will use request config only): %v", err)
		mergedConfig = requestConfig // Fallback to request config
	}

	// Get deployable components from config
	expectedComponents := commonconfig.GetComponents()

	// CICD components have special format in values.yaml: cicd.runner, cicd.unified_job
	cicdComponentsMap := map[string]string{
		"cicd_runner":      "cicd.runner",
		"cicd_unified_job": "cicd.unified_job",
	}

	// Build tags string for sed/yq and detect remote updates needed
	componentTags := ""
	nodeAgentTags := ""
	result := &DeploymentResult{}

	for _, comp := range expectedComponents {
		if tag, ok := mergedConfig.ImageVersions[comp]; ok {
			// Check if it's a CICD component with special format
			if yamlKey, isCICD := cicdComponentsMap[comp]; isCICD {
				componentTags += fmt.Sprintf("%s=%s;", yamlKey, tag)
				result.HasCICD = true
				if comp == "cicd_runner" {
					result.CICDRunnerImage = tag
				} else if comp == "cicd_unified_job" {
					result.CICDUnifiedImage = tag
				}
			} else if comp == "node_agent" {
				// node_agent format in values.yaml is "node_agent.image"
				nodeAgentTags += fmt.Sprintf("%s=%s;", "image", tag)
				result.HasNodeAgent = true
				result.NodeAgentImage = tag
			} else {
				// Standard format: "component.image"
				componentTags += fmt.Sprintf("%s.image=%s;", comp, tag)
			}
		}
	}

	// 3. Prepare Job Params
	jobName := commonutils.GenerateName(fmt.Sprintf("cd-upgrade-%d", req.Id))
	result.LocalJobName = jobName

	params := JobParams{
		Name:          jobName,
		Namespace:     JobNamespace,
		Image:         JobImage,
		ComponentTags: componentTags,
		NodeAgentTags: nodeAgentTags,
		EnvFileConfig: mergedConfig.EnvFileConfig,
	}

	// 3. Create Job
	if err := s.createK8sJob(ctx, params); err != nil {
		return nil, fmt.Errorf("failed to create k8s job: %v", err)
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
		return result, err
	}

	klog.Infof("Deployment result: hasNodeAgent=%v, hasCICD=%v", result.HasNodeAgent, result.HasCICD)
	return result, nil
}

func (s *Service) createK8sJob(ctx context.Context, params JobParams) error {
	// Prepare .env file content (base64 encoded for safe passing)
	envFileBase64 := ""
	if params.EnvFileConfig != "" {
		envFileBase64 = base64.StdEncoding.EncodeToString([]byte(params.EnvFileConfig))
	}

	argsScript := fmt.Sprintf(`
set -e

# Configuration - paths are relative to the mount point
MOUNT_DIR="%s"
REPO_URL="%s"
REPO_NAME="Primus-SaFE"
REPO_DIR="$MOUNT_DIR/$REPO_NAME"

PRIMUS_VALUES="$REPO_DIR/SaFE/charts/primus-safe/values.yaml"
NODE_AGENT_VALUES="$REPO_DIR/SaFE/node-agent/charts/node-agent/values.yaml"
ENV_FILE="$REPO_DIR/SaFE/bootstrap/.env"

echo "=========================================="
echo "Step 1: Preparing repository..."
echo "=========================================="

# Ensure mount directory exists
mkdir -p "$MOUNT_DIR"

# Always do a fresh clone to ensure we have the latest code
if [ -d "$REPO_DIR" ]; then
    echo "Removing existing repository at $REPO_DIR..."
    rm -rf "$REPO_DIR"
fi

echo "Cloning repository from $REPO_URL..."
git clone --depth 1 -b feature/chenyi/cicd_upgrad "$REPO_URL" "$REPO_DIR"
echo "✓ Repository cloned successfully"

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

echo "=========================================="
echo "Step 2: Updating configuration files..."
echo "=========================================="

# Create .env file from user request config
ENV_CONTENT="%s"
if [ -n "$ENV_CONTENT" ]; then
    echo "Creating .env file..."
    echo "$ENV_CONTENT" | base64 -d > "$ENV_FILE"
    echo "✓ .env file created"
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
echo "Step 3: Starting upgrade script..."
echo "=========================================="
cd "$REPO_DIR/SaFE/bootstrap/"
/bin/bash ./upgrade.sh
`, ContainerMountPath, PrimusSaFERepoURL, envFileBase64, params.ComponentTags, params.NodeAgentTags)

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
					ServiceAccountName: common.PrimusSafeNamespace,
					RestartPolicy:      corev1.RestartPolicyOnFailure,
					Containers: []corev1.Container{
						{
							Name:    "upgrade-task",
							Image:   params.Image,
							Command: []string{"/bin/bash", "-c"},
							Args:    []string{argsScript},
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "cd-workspace",
									MountPath: ContainerMountPath,
								},
							},
						},
					},
					Volumes: []corev1.Volume{
						{
							Name: "cd-workspace",
							VolumeSource: corev1.VolumeSource{
								HostPath: &corev1.HostPathVolumeSource{
									Path: HostMountPath,
									Type: ptr.To(corev1.HostPathDirectoryOrCreate),
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

// ExecuteRemoteClusterUpdates creates a job to update node-agent and/or cicd on remote clusters
func (s *Service) ExecuteRemoteClusterUpdates(ctx context.Context, reqId int64, result *DeploymentResult) (string, error) {
	klog.Infof("Creating remote cluster update job for request %d", reqId)

	jobName := commonutils.GenerateName(fmt.Sprintf("cd-remote-%d", reqId))

	// Build the script for remote cluster updates
	script := s.buildRemoteClusterScript(result)

	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      jobName,
			Namespace: JobNamespace,
		},
		Spec: batchv1.JobSpec{
			TTLSecondsAfterFinished: ptr.To(int32(3600)),
			BackoffLimit:            ptr.To(int32(1)), // Less retries for remote operations
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					ServiceAccountName: common.PrimusSafeNamespace,
					RestartPolicy:      corev1.RestartPolicyNever,
					Containers: []corev1.Container{
						{
							Name:    "remote-update",
							Image:   JobImage, // dtzar/helm-kubectl
							Command: []string{"/bin/bash", "-c"},
							Args:    []string{script},
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "cd-workspace",
									MountPath: ContainerMountPath,
								},
							},
						},
					},
					Volumes: []corev1.Volume{
						{
							Name: "cd-workspace",
							VolumeSource: corev1.VolumeSource{
								HostPath: &corev1.HostPathVolumeSource{
									Path: HostMountPath,
									Type: ptr.To(corev1.HostPathDirectoryOrCreate),
								},
							},
						},
					},
				},
			},
		},
	}

	_, err := s.clientSet.BatchV1().Jobs(JobNamespace).Create(ctx, job, metav1.CreateOptions{})
	if err != nil {
		return "", fmt.Errorf("failed to create remote cluster update job: %v", err)
	}

	klog.Infof("Remote cluster update job created: %s", jobName)
	return jobName, nil
}

// buildRemoteClusterScript builds the shell script for remote cluster updates
func (s *Service) buildRemoteClusterScript(result *DeploymentResult) string {
	script := fmt.Sprintf(`
set -e

echo "=========================================="
echo "Cluster Update Job (node-agent + CICD)"
echo "=========================================="
echo "NodeAgent update: %v (image: %s)"
echo "CICD update: %v (runner: %s, unified: %s)"
echo "=========================================="

# Configuration
WORK_DIR="%s"
REPO_DIR="$WORK_DIR/Primus-SaFE"
NODE_AGENT_CHART="$REPO_DIR/SaFE/node-agent/charts/node-agent"
HAS_NODE_AGENT=%t
HAS_CICD=%t
NODE_AGENT_IMAGE="%s"
CICD_RUNNER_IMAGE="%s"
CICD_UNIFIED_IMAGE="%s"
ADMIN_CLUSTER_ID="%s"

mkdir -p "$WORK_DIR"

# Clone repo if node-agent update needed (for helm chart)
if [ "$HAS_NODE_AGENT" = "true" ]; then
    echo "Cloning repository for node-agent chart..."
    if [ -d "$REPO_DIR" ]; then
        rm -rf "$REPO_DIR"
    fi
    git clone --depth 1 -b feature/chenyi/cicd_upgrad "%s" "$REPO_DIR"
    echo "✓ Repository cloned"
fi

echo "=========================================="
echo "Step 1: Discover clusters from Workloads"
echo "=========================================="

# Get all unique cluster IDs from AutoscalingRunnerSet workloads
if [ "$HAS_CICD" = "true" ]; then
    ALL_CLUSTER_IDS=$(kubectl get workload -l "primus-safe.workload.kind=AutoscalingRunnerSet" -o jsonpath='{.items[*].metadata.labels.primus-safe\.cluster\.id}' 2>/dev/null | tr ' ' '\n' | sort -u | tr '\n' ' ')
    echo "Found clusters with CICD workloads: $ALL_CLUSTER_IDS"
else
    # If only node-agent update, get all clusters
    ALL_CLUSTER_IDS=$(kubectl get cluster -o jsonpath='{.items[*].metadata.name}' 2>/dev/null || echo "")
    # Also add admin cluster for node-agent
    ALL_CLUSTER_IDS="$ADMIN_CLUSTER_ID $ALL_CLUSTER_IDS"
    echo "Found clusters for node-agent update: $ALL_CLUSTER_IDS"
fi

# Remove duplicates
ALL_CLUSTER_IDS=$(echo "$ALL_CLUSTER_IDS" | tr ' ' '\n' | sort -u | tr '\n' ' ')

for CLUSTER_ID in $ALL_CLUSTER_IDS; do
    [ -z "$CLUSTER_ID" ] && continue
    
    echo ""
    echo "=========================================="
    echo "Processing cluster: $CLUSTER_ID"
    echo "=========================================="
    
    # Check if this is the admin cluster (no remote connection needed)
    if [ "$CLUSTER_ID" = "$ADMIN_CLUSTER_ID" ]; then
        echo "This is the ADMIN cluster, using in-cluster config"
        KUBECONFIG_OPT=""
    else
        echo "This is a REMOTE cluster, generating kubeconfig..."
        
        # Get cluster info from Cluster resource
        CLUSTER_JSON=$(kubectl get cluster "$CLUSTER_ID" -o json 2>/dev/null || echo "")
        
        if [ -z "$CLUSTER_JSON" ]; then
            echo "⚠ Cluster resource not found for $CLUSTER_ID, skipping..."
            continue
        fi
        
        # Extract kubeconfig components from cluster status
        CA_DATA=$(echo "$CLUSTER_JSON" | jq -r '.status.controlPlaneStatus.CAData // empty')
        CERT_DATA=$(echo "$CLUSTER_JSON" | jq -r '.status.controlPlaneStatus.certData // empty')
        KEY_DATA=$(echo "$CLUSTER_JSON" | jq -r '.status.controlPlaneStatus.keyData // empty')
        ENDPOINT_RAW=$(echo "$CLUSTER_JSON" | jq -r '.status.controlPlaneStatus.endpoints[0] // empty')
        PHASE=$(echo "$CLUSTER_JSON" | jq -r '.status.controlPlaneStatus.phase // empty')
        
        # Extract host from endpoint and force use API Server port 6443
        # e.g., "https://10.32.80.60:2379" -> "https://10.32.80.60:6443"
        ENDPOINT_HOST=$(echo "$ENDPOINT_RAW" | sed 's|^\(https\?://[^:/]*\).*|\1|')
        ENDPOINTS="${ENDPOINT_HOST}:6443"
        
        # Skip if cluster is not ready or missing credentials
        if [ -z "$CA_DATA" ] || [ -z "$CERT_DATA" ] || [ -z "$KEY_DATA" ] || [ -z "$ENDPOINTS" ]; then
            echo "⚠ Skipping cluster $CLUSTER_ID: missing kubeconfig data"
            continue
        fi
        
        if [ "$PHASE" != "Ready" ]; then
            echo "⚠ Skipping cluster $CLUSTER_ID: not in Ready phase (phase: $PHASE)"
            continue
        fi
        
        # Generate kubeconfig for this cluster
        KUBECONFIG_FILE="$WORK_DIR/kubeconfig-$CLUSTER_ID"
        cat > "$KUBECONFIG_FILE" << EOF
apiVersion: v1
kind: Config
clusters:
- cluster:
    certificate-authority-data: $CA_DATA
    server: $ENDPOINTS
  name: $CLUSTER_ID
contexts:
- context:
    cluster: $CLUSTER_ID
    user: $CLUSTER_ID-admin
  name: $CLUSTER_ID
current-context: $CLUSTER_ID
users:
- name: $CLUSTER_ID-admin
  user:
    client-certificate-data: $CERT_DATA
    client-key-data: $KEY_DATA
EOF
        
        echo "✓ Generated kubeconfig for $CLUSTER_ID"
        
        # Test connection
        if ! kubectl --kubeconfig="$KUBECONFIG_FILE" get nodes > /dev/null 2>&1; then
            echo "⚠ Cannot connect to cluster $CLUSTER_ID, skipping..."
            continue
        fi
        echo "✓ Connected to cluster $CLUSTER_ID"
        
        KUBECONFIG_OPT="--kubeconfig=$KUBECONFIG_FILE"
    fi
    
    # Update node-agent if needed (skip admin cluster - handled by Job 1's upgrade.sh)
    if [ "$HAS_NODE_AGENT" = "true" ] && [ "$CLUSTER_ID" != "$ADMIN_CLUSTER_ID" ]; then
        echo "Updating node-agent on $CLUSTER_ID..."
        
        # Copy values.yaml to temporary file (like upgrade.sh does)
        NODE_AGENT_VALUES="$NODE_AGENT_CHART/.values.yaml"
        cp "$NODE_AGENT_CHART/values.yaml" "$NODE_AGENT_VALUES"
        
        # Update the image in temporary values.yaml
        sed -i "s|image: \".*\"|image: \"$NODE_AGENT_IMAGE\"|" "$NODE_AGENT_VALUES"
        
        # Helm upgrade (like upgrade.sh: helm upgrade -i node-agent ./node-agent -n primus-safe -f values.yaml)
        helm $KUBECONFIG_OPT upgrade -i node-agent "$NODE_AGENT_CHART" \
            -n primus-safe --create-namespace \
            -f "$NODE_AGENT_VALUES" \
            || echo "⚠ helm upgrade failed for $CLUSTER_ID, continuing..."
        
        # Cleanup temporary values file
        rm -f "$NODE_AGENT_VALUES"
        
        echo "✓ node-agent updated on $CLUSTER_ID"
    elif [ "$HAS_NODE_AGENT" = "true" ] && [ "$CLUSTER_ID" = "$ADMIN_CLUSTER_ID" ]; then
        echo "Skipping node-agent on admin cluster (already handled by upgrade.sh in Job 1)"
    fi
    
    # Update CICD if needed
    if [ "$HAS_CICD" = "true" ]; then
        echo "Updating CICD on $CLUSTER_ID..."
        
        # Get all AutoscalingRunnerSet workloads for this cluster (from admin cluster)
        WORKLOADS=$(kubectl get workload -l "primus-safe.workload.kind=AutoscalingRunnerSet,primus-safe.cluster.id=$CLUSTER_ID" -o json 2>/dev/null || echo '{"items":[]}')
        WORKLOAD_COUNT=$(echo "$WORKLOADS" | jq '.items | length')
        
        echo "Found $WORKLOAD_COUNT AutoscalingRunnerSet workloads for cluster $CLUSTER_ID"
        
        for i in $(seq 0 $((WORKLOAD_COUNT - 1))); do
            WORKLOAD_NAME=$(echo "$WORKLOADS" | jq -r ".items[$i].metadata.name")
            WORKSPACE=$(echo "$WORKLOADS" | jq -r ".items[$i].spec.workspace")
            
            # Extract UNIFIED_JOB_ENABLE from Workload's spec.env (in admin cluster)
            UNIFIED_JOB_ENABLE=$(echo "$WORKLOADS" | jq -r ".items[$i].spec.env.UNIFIED_JOB_ENABLE // \"false\"")
            [ -z "$UNIFIED_JOB_ENABLE" ] && UNIFIED_JOB_ENABLE="false"
            
            echo "  Processing workload: $WORKLOAD_NAME (namespace: $WORKSPACE, UNIFIED_JOB_ENABLE: $UNIFIED_JOB_ENABLE)"
            
            # Get current ARS to extract registry prefix from existing image
            CURRENT_ARS=$(kubectl $KUBECONFIG_OPT get autoscalingrunnersets "$WORKLOAD_NAME" -n "$WORKSPACE" -o json 2>/dev/null || echo "")
            
            if [ -z "$CURRENT_ARS" ]; then
                echo "  ⚠ AutoscalingRunnerSet not found, skipping..."
                continue
            fi
            
            # Get current runner image to extract registry prefix
            # e.g., "harbor.tw325.../primussafe/cicd-runner-proxy:旧版本"
            CURRENT_RUNNER_IMAGE=$(echo "$CURRENT_ARS" | jq -r '.spec.template.spec.containers[0].image')
            
            # Extract registry prefix (everything before the image name)
            # e.g., "harbor.tw325.../primussafe/"
            REGISTRY_PREFIX=$(echo "$CURRENT_RUNNER_IMAGE" | sed 's|/[^/]*$|/|')
            
            # Build full image path: registry prefix + new image name:tag
            # e.g., "harbor.tw325.../primussafe/" + "cicd-runner-proxy:202512111349"
            FULL_RUNNER_IMAGE="${REGISTRY_PREFIX}${CICD_RUNNER_IMAGE}"
            
            echo "  Current image: $CURRENT_RUNNER_IMAGE"
            echo "  New image: $FULL_RUNNER_IMAGE"
            
            # Patch runner container (containers[0])
            PATCH_RUNNER='[{"op": "replace", "path": "/spec/template/spec/containers/0/image", "value": "'"$FULL_RUNNER_IMAGE"'"}]'
            
            kubectl $KUBECONFIG_OPT patch autoscalingrunnersets "$WORKLOAD_NAME" \
                -n "$WORKSPACE" --type='json' -p="$PATCH_RUNNER" \
                || echo "  ⚠ Failed to patch runner for $WORKLOAD_NAME"
            
            echo "  ✓ Updated runner image"
            
            # If UNIFIED_JOB_ENABLE=true, also patch unified-job container (containers[1])
            if [ "$UNIFIED_JOB_ENABLE" = "true" ] && [ -n "$CICD_UNIFIED_IMAGE" ]; then
                # Build full unified-job image path with same registry prefix
                FULL_UNIFIED_IMAGE="${REGISTRY_PREFIX}${CICD_UNIFIED_IMAGE}"
                
                echo "  New unified-job image: $FULL_UNIFIED_IMAGE"
                
                PATCH_UNIFIED='[{"op": "replace", "path": "/spec/template/spec/containers/1/image", "value": "'"$FULL_UNIFIED_IMAGE"'"}]'
                kubectl $KUBECONFIG_OPT patch autoscalingrunnersets "$WORKLOAD_NAME" \
                    -n "$WORKSPACE" --type='json' -p="$PATCH_UNIFIED" \
                    || echo "  ⚠ Failed to patch unified-job for $WORKLOAD_NAME"
                
                echo "  ✓ Updated unified-job image"
            fi
            
            echo "  ✓ Completed $WORKLOAD_NAME"
        done
        
        echo "✓ CICD updated on $CLUSTER_ID"
    fi
    
    # Cleanup kubeconfig for remote cluster
    if [ "$CLUSTER_ID" != "$ADMIN_CLUSTER_ID" ] && [ -n "$KUBECONFIG_FILE" ]; then
        rm -f "$KUBECONFIG_FILE"
    fi
done

echo ""
echo "=========================================="
echo "✓ All cluster updates completed!"
echo "=========================================="
`,
		result.HasNodeAgent, result.NodeAgentImage,
		result.HasCICD, result.CICDRunnerImage, result.CICDUnifiedImage,
		ContainerMountPath,
		result.HasNodeAgent,
		result.HasCICD,
		result.NodeAgentImage,
		result.CICDRunnerImage,
		result.CICDUnifiedImage,
		AdminClusterID,
		PrimusSaFERepoURL,
	)

	return script
}

// WaitForJobCompletion waits for the K8s job to complete or fail
// It will delete the job after completion (success or failure)
func (s *Service) WaitForJobCompletion(ctx context.Context, jobName, namespace string) error {
	// Wait for job and get result
	jobErr := s.waitForJob(ctx, jobName, namespace)

	// Always delete the job after completion (success or failure)
	if err := s.DeleteJob(ctx, jobName, namespace); err != nil {
		klog.ErrorS(err, "Failed to delete job after completion", "job", jobName)
	}

	return jobErr
}

// waitForJob polls the job status until completion or timeout
func (s *Service) waitForJob(ctx context.Context, jobName, namespace string) error {
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

// DeleteJob deletes a Kubernetes Job and its associated pods
func (s *Service) DeleteJob(ctx context.Context, jobName, namespace string) error {
	klog.Infof("Deleting job %s in namespace %s", jobName, namespace)

	// Delete with propagation policy to also delete pods
	propagationPolicy := metav1.DeletePropagationBackground
	deleteOptions := metav1.DeleteOptions{
		PropagationPolicy: &propagationPolicy,
	}

	if err := s.clientSet.BatchV1().Jobs(namespace).Delete(ctx, jobName, deleteOptions); err != nil {
		return fmt.Errorf("failed to delete job %s: %v", jobName, err)
	}

	klog.Infof("Job %s deleted successfully", jobName)
	return nil
}

// VerifyDeploymentRollout checks if the workloads defined in config are actually running
func (s *Service) VerifyDeploymentRollout(ctx context.Context, envConfig string) error {
	// 1. Parse config to know which components to check
	var config DeploymentConfig
	if err := json.Unmarshal([]byte(envConfig), &config); err != nil {
		return fmt.Errorf("failed to parse config during verification: %v", err)
	}

	// 2. Verify CICD ConfigMap updates (if any CICD components were updated)
	if err := s.verifyCICDConfigMapUpdate(ctx, config.ImageVersions); err != nil {
		return fmt.Errorf("CICD ConfigMap verification failed: %v", err)
	}

	// 3. Components to skip (not Deployments)
	skipComponents := map[string]bool{
		"cicd_runner":      true, // Verified via ConfigMap
		"cicd_unified_job": true, // Verified via ConfigMap
		"node_agent":       true, // DaemonSet, verified in ticker loop
	}

	// Check if node_agent needs verification
	_, hasNodeAgent := config.ImageVersions["node_agent"]

	// 4. Poll for readiness
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

			// Check Deployments
			for comp := range config.ImageVersions {
				// Skip non-Deployment components
				if skipComponents[comp] {
					continue
				}

				// Generate deployment name: primus-safe-{component} (replace _ with -)
				deploymentName := "primus-safe-" + strings.ReplaceAll(comp, "_", "-")

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

			// Check node-agent DaemonSet on all clusters (if node_agent was updated)
			if hasNodeAgent {
				if err := s.verifyNodeAgentDaemonSet(ctx); err != nil {
					// If it's a critical image error, fail immediately
					if strings.Contains(err.Error(), "ImagePullBackOff") || strings.Contains(err.Error(), "ErrImagePull") {
						return fmt.Errorf("node-agent DaemonSet failed: %v", err)
					}
					// Otherwise just not ready yet
					allReady = false
					notReadyList = append(notReadyList, "node-agent-daemonset")
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

// verifyCICDConfigMapUpdate verifies that CICD component images are updated in the ConfigMap
func (s *Service) verifyCICDConfigMapUpdate(ctx context.Context, imageVersions map[string]string) error {
	// Check if any CICD components need verification
	runnerImage, hasRunner := imageVersions["cicd_runner"]
	unifiedJobImage, hasUnifiedJob := imageVersions["cicd_unified_job"]

	if !hasRunner && !hasUnifiedJob {
		// No CICD components to verify
		return nil
	}

	klog.Infof("Verifying CICD ConfigMap updates...")

	// Get the ConfigMap
	cm, err := s.clientSet.CoreV1().ConfigMaps(JobNamespace).Get(ctx, "github-scale-set-template", metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("failed to get github-scale-set-template ConfigMap: %v", err)
	}

	template, ok := cm.Data["template"]
	if !ok {
		return fmt.Errorf("ConfigMap does not contain 'template' key")
	}

	// Verify runner image
	if hasRunner {
		if !strings.Contains(template, runnerImage) {
			return fmt.Errorf("runner image '%s' not found in ConfigMap template", runnerImage)
		}
		klog.Infof("✓ CICD runner image verified: %s", runnerImage)
	}

	// Verify unified_job image
	if hasUnifiedJob {
		if !strings.Contains(template, unifiedJobImage) {
			return fmt.Errorf("unified_job image '%s' not found in ConfigMap template", unifiedJobImage)
		}
		klog.Infof("✓ CICD unified_job image verified: %s", unifiedJobImage)
	}

	return nil
}

// verifyNodeAgentDaemonSet verifies node-agent DaemonSet status on all clusters
// It checks if DESIRED == CURRENT for each cluster's DaemonSet
func (s *Service) verifyNodeAgentDaemonSet(ctx context.Context) error {
	klog.Infof("Verifying node-agent DaemonSet on all clusters...")

	const daemonSetName = "primus-safe-node-agent"
	const namespace = JobNamespace

	// Get all cluster IDs from client manager
	clusterIDs, _ := s.clientManager.GetAll()

	verifiedClusters := []string{}
	failedClusters := []string{}

	// Check all clusters
	for _, clusterID := range clusterIDs {
		// Get client factory for this cluster
		clientFactory, err := commonutils.GetK8sClientFactory(s.clientManager, clusterID)
		if err != nil {
			klog.V(4).Infof("Failed to get client for cluster %s: %v, skipping", clusterID, err)
			continue
		}

		if !clientFactory.IsValid() {
			klog.V(4).Infof("Client for cluster %s is not valid, skipping", clusterID)
			continue
		}

		clusterClientSet := clientFactory.ClientSet()
		if clusterClientSet == nil {
			klog.V(4).Infof("ClientSet for cluster %s is nil, skipping", clusterID)
			continue
		}

		if err := s.checkDaemonSetStatus(ctx, clusterClientSet, daemonSetName, namespace); err != nil {
			// If DaemonSet not found, it's ok (maybe not deployed on this cluster)
			if strings.Contains(err.Error(), "not found") {
				klog.V(4).Infof("node-agent DaemonSet not found on cluster %s, skipping", clusterID)
			} else {
				failedClusters = append(failedClusters, fmt.Sprintf("%s: %v", clusterID, err))
			}
		} else {
			verifiedClusters = append(verifiedClusters, clusterID)
		}
	}

	// Report results
	if len(verifiedClusters) > 0 {
		klog.Infof("✓ node-agent DaemonSet verified on clusters: %v", verifiedClusters)
	}

	if len(failedClusters) > 0 {
		return fmt.Errorf("node-agent DaemonSet verification failed on clusters: %v", failedClusters)
	}

	if len(verifiedClusters) == 0 {
		klog.Infof("No node-agent DaemonSet found on any cluster (might not be deployed)")
	}

	return nil
}

// checkDaemonSetStatus checks if a DaemonSet's DESIRED == CURRENT (ready)
func (s *Service) checkDaemonSetStatus(ctx context.Context, clientSet kubernetes.Interface, name, namespace string) error {
	ds, err := clientSet.AppsV1().DaemonSets(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("failed to get daemonset %s: %v", name, err)
	}

	desired := ds.Status.DesiredNumberScheduled
	current := ds.Status.CurrentNumberScheduled

	// Check if DESIRED == CURRENT
	if desired == 0 {
		return fmt.Errorf("daemonset %s has no desired pods scheduled", name)
	}

	if current != desired {
		return fmt.Errorf("daemonset %s: CURRENT(%d) != DESIRED(%d)", name, current, desired)
	}

	klog.V(4).Infof("DaemonSet %s: DESIRED=%d, CURRENT=%d", name, desired, current)
	return nil
}

// checkDeploymentStatus checks if a deployment is fully available and free of image errors
func (s *Service) checkDeploymentStatus(ctx context.Context, name, namespace string) error {
	// 1. Check Deployment status
	deploy, err := s.clientSet.AppsV1().Deployments(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("failed to get deployment %s: %v", name, err)
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

func (s *Service) UpdateRequestStatus(ctx context.Context, reqId int64, status, failureReason string) error {
	req, err := s.dbClient.GetDeploymentRequest(ctx, reqId)
	if err != nil {
		klog.ErrorS(err, "Failed to get request for update", "id", reqId)
		return err
	}

	req.Status = status
	if failureReason != "" {
		req.FailureReason = dbutils.NullString(failureReason)
	}

	return s.dbClient.UpdateDeploymentRequest(ctx, req)
}

// RecoverDeployingRequests checks for requests stuck in "deploying" status after apiserver restart
// and recovers them based on the corresponding Job status
func (s *Service) RecoverDeployingRequests(ctx context.Context) error {
	klog.Info("Checking for stuck deploying requests after restart...")

	// Query all requests with "deploying" status
	dbTags := dbclient.GetDeploymentRequestFieldTags()
	query := sqrl.Eq{dbclient.GetFieldTag(dbTags, "Status"): StatusDeploying}
	requests, err := s.dbClient.ListDeploymentRequests(ctx, query, nil, 100, 0)
	if err != nil {
		klog.ErrorS(err, "Failed to list deploying requests")
		return err
	}

	if len(requests) == 0 {
		klog.Info("No stuck deploying requests found")
		return nil
	}

	klog.Infof("Found %d stuck deploying requests, recovering...", len(requests))

	for _, req := range requests {
		// Extract job name from description (format: "xxx | Job: cd-upgrade-123-xxxx")
		jobName := s.extractJobNameFromDescription(req.Description.String)
		if jobName == "" {
			klog.Warningf("Request %d has no job name in description, marking as failed", req.Id)
			failReason := "Recovery failed: no job name found"
			s.UpdateRequestStatus(ctx, req.Id, StatusFailed, failReason)
			s.notifyDeploymentFailure(ctx, req, failReason)
			continue
		}

		klog.Infof("Recovering request %d with job %s", req.Id, jobName)

		// Check job status
		job, err := s.clientSet.BatchV1().Jobs(JobNamespace).Get(ctx, jobName, metav1.GetOptions{})
		if err != nil {
			// Job not found - probably deleted or never created
			klog.Warningf("Job %s not found for request %d, marking as failed", jobName, req.Id)
			failReason := "Recovery failed: job not found (may have been deleted)"
			s.UpdateRequestStatus(ctx, req.Id, StatusFailed, failReason)
			s.notifyDeploymentFailure(ctx, req, failReason)
			continue
		}

		// Check job completion status
		if job.Status.Succeeded > 0 {
			// Job succeeded - need to check if there's a remote job and then finalize
			klog.Infof("Job %s succeeded, checking for remote job for request %d", jobName, req.Id)
			// Start background monitoring to handle potential remote job and finalization
			go s.finalizeRecoveredDeployment(ctx, req)
			// Delete the completed local job
			s.DeleteJob(ctx, jobName, JobNamespace)
		} else if job.Status.Failed > 0 && job.Status.Failed >= *job.Spec.BackoffLimit+1 {
			// Job failed
			klog.Infof("Job %s failed, updating request %d to failed", jobName, req.Id)
			failReason := "Job execution failed (recovered after restart)"
			s.UpdateRequestStatus(ctx, req.Id, StatusFailed, failReason)
			s.notifyDeploymentFailure(ctx, req, failReason)
			// Delete the failed job
			s.DeleteJob(ctx, jobName, JobNamespace)
		} else {
			// Job still running - start monitoring in background
			klog.Infof("Job %s still running, resuming monitoring for request %d", jobName, req.Id)
			go s.resumeDeploymentMonitoring(ctx, req, jobName)
		}
	}

	return nil
}

// extractJobNameFromDescription extracts job name from description string
// Format: "xxx | Job: cd-upgrade-123-xxxx"
func (s *Service) extractJobNameFromDescription(description string) string {
	if description == "" {
		return ""
	}
	// Look for "Job: " prefix
	prefix := "Job: "
	idx := strings.Index(description, prefix)
	if idx == -1 {
		return ""
	}
	return strings.TrimSpace(description[idx+len(prefix):])
}

// resumeDeploymentMonitoring resumes monitoring a deployment after apiserver restart
func (s *Service) resumeDeploymentMonitoring(ctx context.Context, req *dbclient.DeploymentRequest, jobName string) {
	klog.Infof("Resuming monitoring for request %d, job %s", req.Id, jobName)

	// Wait for local job completion
	if err := s.WaitForJobCompletion(ctx, jobName, JobNamespace); err != nil {
		klog.ErrorS(err, "Job execution failed during recovery", "job", jobName)
		failReason := fmt.Sprintf("Job failed during recovery: %v", err)
		s.UpdateRequestStatus(ctx, req.Id, StatusFailed, failReason)
		s.notifyDeploymentFailure(ctx, req, failReason)
		return
	}

	// Check if there's a remote cluster job (for node-agent or CICD updates)
	// Parse config to check if remote updates were needed
	var config DeploymentConfig
	if err := json.Unmarshal([]byte(req.EnvConfig), &config); err == nil {
		hasNodeAgent := config.ImageVersions["node_agent"] != ""
		hasCICD := config.ImageVersions["cicd_runner"] != "" || config.ImageVersions["cicd_unified_job"] != ""

		if hasNodeAgent || hasCICD {
			// Check if remote job exists and wait for it
			remoteJobPrefix := fmt.Sprintf("cd-remote-%d-", req.Id)
			remoteJobName := s.findJobByPrefix(ctx, remoteJobPrefix, JobNamespace)

			if remoteJobName != "" {
				klog.Infof("Found remote job %s for request %d, waiting for completion", remoteJobName, req.Id)
				if err := s.WaitForJobCompletion(ctx, remoteJobName, JobNamespace); err != nil {
					klog.ErrorS(err, "Remote job execution failed during recovery", "job", remoteJobName)
					failReason := fmt.Sprintf("Remote cluster job failed during recovery: %v", err)
					s.UpdateRequestStatus(ctx, req.Id, StatusFailed, failReason)
					s.notifyDeploymentFailure(ctx, req, failReason)
					return
				}
			}
		}
	}

	// Verify deployment rollout
	if err := s.VerifyDeploymentRollout(ctx, req.EnvConfig); err != nil {
		klog.ErrorS(err, "Deployment verification failed during recovery", "id", req.Id)
		failReason := fmt.Sprintf("Rollout verification failed during recovery: %v", err)
		s.UpdateRequestStatus(ctx, req.Id, StatusFailed, failReason)
		s.notifyDeploymentFailure(ctx, req, failReason)
		return
	}

	// Success
	s.UpdateRequestStatus(ctx, req.Id, StatusDeployed, "")
	if err := s.CreateSnapshot(ctx, req.Id, req.EnvConfig); err != nil {
		klog.ErrorS(err, "Failed to create snapshot during recovery", "id", req.Id)
	}
	klog.Infof("Successfully recovered request %d", req.Id)
}

// findJobByPrefix finds a job by name prefix
func (s *Service) findJobByPrefix(ctx context.Context, prefix, namespace string) string {
	jobs, err := s.clientSet.BatchV1().Jobs(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		klog.ErrorS(err, "Failed to list jobs", "namespace", namespace)
		return ""
	}

	for _, job := range jobs.Items {
		if strings.HasPrefix(job.Name, prefix) {
			return job.Name
		}
	}
	return ""
}

// finalizeRecoveredDeployment handles the case where local job succeeded but we need to check remote job
func (s *Service) finalizeRecoveredDeployment(ctx context.Context, req *dbclient.DeploymentRequest) {
	klog.Infof("Finalizing recovered deployment for request %d", req.Id)

	// Check if there's a remote cluster job (for node-agent or CICD updates)
	var config DeploymentConfig
	if err := json.Unmarshal([]byte(req.EnvConfig), &config); err == nil {
		hasNodeAgent := config.ImageVersions["node_agent"] != ""
		hasCICD := config.ImageVersions["cicd_runner"] != "" || config.ImageVersions["cicd_unified_job"] != ""

		if hasNodeAgent || hasCICD {
			// Check if remote job exists and wait for it
			remoteJobPrefix := fmt.Sprintf("cd-remote-%d-", req.Id)
			remoteJobName := s.findJobByPrefix(ctx, remoteJobPrefix, JobNamespace)

			if remoteJobName != "" {
				klog.Infof("Found remote job %s for request %d, waiting for completion", remoteJobName, req.Id)
				if err := s.WaitForJobCompletion(ctx, remoteJobName, JobNamespace); err != nil {
					klog.ErrorS(err, "Remote job execution failed during recovery", "job", remoteJobName)
					failReason := fmt.Sprintf("Remote cluster job failed during recovery: %v", err)
					s.UpdateRequestStatus(ctx, req.Id, StatusFailed, failReason)
					s.notifyDeploymentFailure(ctx, req, failReason)
					return
				}
			}
		}
	}

	// Verify deployment rollout
	if err := s.VerifyDeploymentRollout(ctx, req.EnvConfig); err != nil {
		klog.ErrorS(err, "Deployment verification failed during recovery", "id", req.Id)
		failReason := fmt.Sprintf("Rollout verification failed during recovery: %v", err)
		s.UpdateRequestStatus(ctx, req.Id, StatusFailed, failReason)
		s.notifyDeploymentFailure(ctx, req, failReason)
		return
	}

	// Success
	s.UpdateRequestStatus(ctx, req.Id, StatusDeployed, "")
	if err := s.CreateSnapshot(ctx, req.Id, req.EnvConfig); err != nil {
		klog.ErrorS(err, "Failed to create snapshot during recovery", "id", req.Id)
	}
	klog.Infof("Successfully finalized recovered deployment for request %d", req.Id)
}

// Rollback creates a new request based on a previous snapshot
func (s *Service) Rollback(ctx context.Context, reqId int64, username string) (int64, error) {
	// 1. Validate target request exists and is in valid state
	targetReq, err := s.dbClient.GetDeploymentRequest(ctx, reqId)
	if err != nil {
		return 0, err
	}

	if targetReq.Status != StatusDeployed {
		return 0, fmt.Errorf("cannot rollback to a request with status %s (must be %s)",
			targetReq.Status, StatusDeployed)
	}

	// 2. Get the full config from snapshot (not from request, because request may contain partial config)
	var envConfig string
	snapshot, err := s.dbClient.GetEnvironmentSnapshotByRequestId(ctx, reqId)
	if err != nil {
		// Snapshot not found, fallback to request's EnvConfig (for backward compatibility)
		klog.Warningf("Snapshot not found for request %d, falling back to request EnvConfig", reqId)
		envConfig = targetReq.EnvConfig
	} else {
		// Use snapshot's full config
		envConfig = snapshot.EnvConfig
	}

	// 3. Create a new request that applies the old config
	newReq := &dbclient.DeploymentRequest{
		DeployName:     username,
		Status:         StatusPendingApproval,
		EnvConfig:      envConfig,
		Description:    dbutils.NullString(fmt.Sprintf("Rollback to version from request %d", reqId)),
		RollbackFromId: sql.NullInt64{Int64: reqId, Valid: true},
	}

	return s.dbClient.CreateDeploymentRequest(ctx, newReq)
}

// mergeWithLatestSnapshot merges current request config with the latest snapshot
// This ensures all historical image versions are preserved, and only the specified ones are updated
func (s *Service) mergeWithLatestSnapshot(ctx context.Context, currentConfig DeploymentConfig) (DeploymentConfig, error) {
	// Get the latest snapshot
	snapshots, err := s.dbClient.ListEnvironmentSnapshots(ctx, nil, []string{"created_at DESC"}, 1, 0)
	if err != nil {
		return currentConfig, fmt.Errorf("failed to get latest snapshot: %v", err)
	}

	if len(snapshots) == 0 {
		klog.Infof("No previous snapshot found, using current config only")
		return currentConfig, nil
	}

	// Parse the snapshot config
	var snapshotConfig DeploymentConfig
	if err := json.Unmarshal([]byte(snapshots[0].EnvConfig), &snapshotConfig); err != nil {
		return currentConfig, fmt.Errorf("failed to parse snapshot config: %v", err)
	}

	// Merge image versions: start with snapshot, then override with current request
	mergedImageVersions := make(map[string]string)

	// First, copy all image versions from snapshot
	for k, v := range snapshotConfig.ImageVersions {
		mergedImageVersions[k] = v
	}

	// Then, override with current request's image versions
	for k, v := range currentConfig.ImageVersions {
		mergedImageVersions[k] = v
		klog.Infof("Updating component %s: %s -> %s", k, snapshotConfig.ImageVersions[k], v)
	}

	// Merge env_file_config: use current if provided, otherwise use snapshot
	mergedEnvFileConfig := currentConfig.EnvFileConfig
	if mergedEnvFileConfig == "" {
		mergedEnvFileConfig = snapshotConfig.EnvFileConfig
		klog.Infof("Using env_file_config from latest snapshot")
	}

	return DeploymentConfig{
		ImageVersions: mergedImageVersions,
		EnvFileConfig: mergedEnvFileConfig,
	}, nil
}

// GetCurrentEnvConfig reads the current .env file content
// It tries to read from the actual file first, and falls back to the latest snapshot if file read fails
func (s *Service) GetCurrentEnvConfig(ctx context.Context) (content string, err error) {
	// Get from the latest successful deployment snapshot
	snapshots, err := s.dbClient.ListEnvironmentSnapshots(ctx, nil, []string{"created_at DESC"}, 1, 0)
	if err != nil {
		return "", fmt.Errorf("failed to read last .env record and no snapshot available: db_error=%v", err)
	}

	if len(snapshots) == 0 {
		return "", fmt.Errorf("failed to read last .env record and no snapshot available: %v", err)
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
		Id:              req.Id,
		DeployName:      req.DeployName,
		Status:          req.Status,
		ApproverName:    dbutils.ParseNullString(req.ApproverName),
		ApprovalResult:  dbutils.ParseNullString(req.ApprovalResult),
		Description:     dbutils.ParseNullString(req.Description),
		RejectionReason: dbutils.ParseNullString(req.RejectionReason),
		FailureReason:   dbutils.ParseNullString(req.FailureReason),
		RollbackFromId:  req.RollbackFromId.Int64,
		CreatedAt:       dbutils.ParseNullTimeToString(req.CreatedAt),
		UpdatedAt:       dbutils.ParseNullTimeToString(req.UpdatedAt),
		ApprovedAt:      dbutils.ParseNullTimeToString(req.ApprovedAt),
	}
}
