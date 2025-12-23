/*
 * Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package ops_job

import (
	"context"
	"encoding/base64"
	"fmt"
	"sync"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
	"k8s.io/utils/pointer"
	ctrlruntime "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	commonjob "github.com/AMD-AIG-AIMA/SAFE/common/pkg/ops_job"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/backoff"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/timeutil"
)

const (
	// CD Job specific constants
	CDJobImage = "dtzar/helm-kubectl:latest" // Image with bash, git, helm, kubectl

	// Git repository URL for Primus-SaFE
	PrimusSaFERepoURL = "https://github.com/AMD-AGI/Primus-SaFE.git"

	// Container mount path for CD workspace
	ContainerMountPath = "/home/primus-safe-cd"

	// Host path on the node for persistent storage
	HostMountPath = "/mnt/primus-safe-cd"

	// CD deployment phases
	CDPhaseLocal  = "local"  // Local cluster deployment
	CDPhaseRemote = "remote" // Remote cluster updates
)

type CDJobReconciler struct {
	*OpsJobBaseReconciler
	sync.RWMutex
}

// SetupCDJobController initializes and registers the CDJobReconciler with the controller manager.
func SetupCDJobController(mgr manager.Manager) error {
	r := &CDJobReconciler{
		OpsJobBaseReconciler: &OpsJobBaseReconciler{
			Client: mgr.GetClient(),
		},
	}
	err := ctrlruntime.NewControllerManagedBy(mgr).
		For(&v1.OpsJob{}, builder.WithPredicates(predicate.Or(
			predicate.GenerationChangedPredicate{}, onFirstPhaseChangedPredicate()))).
		Watches(&v1.Workload{}, r.handleWorkloadEvent()).
		Complete(r)
	if err != nil {
		return err
	}
	klog.Infof("Setup CD Job Controller successfully")
	return nil
}

// handleWorkloadEvent creates an event handler that watches Workload resource events for CD jobs.
func (r *CDJobReconciler) handleWorkloadEvent() handler.EventHandler {
	return handler.Funcs{
		CreateFunc: func(ctx context.Context, evt event.CreateEvent, q v1.RequestWorkQueue) {
			workload, ok := evt.Object.(*v1.Workload)
			if !ok || !isCDWorkload(workload) {
				return
			}
			r.handleWorkloadEventImpl(ctx, workload)
		},
		UpdateFunc: func(ctx context.Context, evt event.UpdateEvent, q v1.RequestWorkQueue) {
			oldWorkload, ok1 := evt.ObjectOld.(*v1.Workload)
			newWorkload, ok2 := evt.ObjectNew.(*v1.Workload)
			if !ok1 || !ok2 || !isCDWorkload(newWorkload) {
				return
			}
			if (!oldWorkload.IsEnd() && newWorkload.IsEnd()) ||
				(!oldWorkload.IsRunning() && newWorkload.IsRunning()) {
				r.handleWorkloadEventImpl(ctx, newWorkload)
			}
		},
	}
}

// isCDWorkload checks if a workload is a CD job workload.
func isCDWorkload(workload *v1.Workload) bool {
	return v1.GetOpsJobId(workload) != "" &&
		v1.GetOpsJobType(workload) == string(v1.OpsJobCDType)
}

// handleWorkloadEventImpl handles workload events by updating the corresponding OpsJob status.
func (r *CDJobReconciler) handleWorkloadEventImpl(ctx context.Context, workload *v1.Workload) {
	var phase v1.OpsJobPhase
	completionMessage := ""

	switch {
	case workload.IsEnd():
		if workload.Status.Phase == v1.WorkloadSucceeded {
			phase = v1.OpsJobSucceeded
		} else {
			phase = v1.OpsJobFailed
		}
		completionMessage = getWorkloadCompletionMessage(workload)
		if completionMessage == "" {
			completionMessage = "CD deployment completed"
		}
	case workload.IsRunning():
		phase = v1.OpsJobRunning
	default:
		phase = v1.OpsJobPending
	}

	jobId := v1.GetOpsJobId(workload)
	err := backoff.Retry(func() error {
		job := &v1.OpsJob{}
		var err error
		if err = r.Get(ctx, client.ObjectKey{Name: jobId}, job); err != nil {
			return client.IgnoreNotFound(err)
		}
		switch phase {
		case v1.OpsJobPending, v1.OpsJobRunning:
			err = r.setJobPhase(ctx, job, phase)
		default:
			output := []v1.Parameter{
				{Name: "result", Value: completionMessage},
				{Name: v1.ParameterDeployPhase, Value: workload.GetEnv("DEPLOY_PHASE")},
			}
			err = r.setJobCompleted(ctx, job, phase, completionMessage, output)
		}
		if err != nil {
			return err
		}
		return nil
	}, 2*time.Second, 200*time.Millisecond)
	if err != nil {
		klog.ErrorS(err, "failed to update CD job status", "jobId", jobId)
	}
}

// Reconcile is the main control loop for CD Job resources.
func (r *CDJobReconciler) Reconcile(ctx context.Context, req ctrlruntime.Request) (ctrlruntime.Result, error) {
	clearFuncs := []ClearFunc{r.cleanupJobRelatedInfo}
	return r.OpsJobBaseReconciler.Reconcile(ctx, req, r, clearFuncs...)
}

// cleanupJobRelatedInfo cleans up job-related resources.
func (r *CDJobReconciler) cleanupJobRelatedInfo(ctx context.Context, job *v1.OpsJob) error {
	return commonjob.CleanupJobRelatedResource(ctx, r.Client, job.Name)
}

// observe the job status. Returns true if the expected state is met (no handling required), false otherwise.
func (r *CDJobReconciler) observe(_ context.Context, job *v1.OpsJob) (bool, error) {
	return job.IsEnd(), nil
}

// filter determines if the job should be processed by this CD job reconciler.
func (r *CDJobReconciler) filter(_ context.Context, job *v1.OpsJob) bool {
	return job.Spec.Type != v1.OpsJobCDType
}

// handle processes the CD job by creating a corresponding workload.
func (r *CDJobReconciler) handle(ctx context.Context, job *v1.OpsJob) (ctrlruntime.Result, error) {
	if job.Status.Phase == "" {
		originalJob := client.MergeFrom(job.DeepCopy())
		job.Status.Phase = v1.OpsJobPending
		if err := r.Status().Patch(ctx, job, originalJob); err != nil {
			return ctrlruntime.Result{}, err
		}
		// ensure that job will be reconciled when it is timeout
		return newRequeueAfterResult(job), nil
	}

	// Check if workload already exists
	workload := &v1.Workload{}
	if r.Get(ctx, client.ObjectKey{Name: job.Name}, workload) == nil {
		return ctrlruntime.Result{}, nil
	}

	// Generate CD workload
	var err error
	workload, err = r.generateCDWorkload(ctx, job)
	if err != nil {
		klog.ErrorS(err, "failed to generate CD workload", "job", job.Name)
		return ctrlruntime.Result{}, err
	}

	if err = r.Create(ctx, workload); err != nil {
		return ctrlruntime.Result{}, client.IgnoreAlreadyExists(err)
	}
	klog.Infof("Processing CD job %s for workload %s", job.Name, workload.Name)
	return ctrlruntime.Result{}, nil
}

// generateCDWorkload generates a CD workload based on the job specification.
func (r *CDJobReconciler) generateCDWorkload(ctx context.Context, job *v1.OpsJob) (*v1.Workload, error) {
	// Get deployment parameters from job inputs
	componentTags := getParameterValue(job, v1.ParameterComponentTags, "")
	nodeAgentTags := getParameterValue(job, v1.ParameterNodeAgentTags, "")
	envFileConfig := getParameterValue(job, v1.ParameterEnvFileConfig, "")
	deployBranch := getParameterValue(job, v1.ParameterDeployBranch, "")
	hasNodeAgent := getParameterValue(job, v1.ParameterHasNodeAgent, "false") == "true"
	hasCICD := getParameterValue(job, v1.ParameterHasCICD, "false") == "true"
	installNodeAgent := getParameterValue(job, v1.ParameterInstallNodeAgent, "false") == "true"
	nodeAgentImage := getParameterValue(job, v1.ParameterNodeAgentImage, "")
	cicdRunnerImage := getParameterValue(job, v1.ParameterCICDRunnerImage, "")
	cicdUnifiedImage := getParameterValue(job, v1.ParameterCICDUnifiedImage, "")

	// Build the complete deployment script (local + remote if needed)
	script := r.buildCompleteDeployScript(
		componentTags, nodeAgentTags, envFileConfig, deployBranch,
		hasNodeAgent, hasCICD, installNodeAgent,
		nodeAgentImage, cicdRunnerImage, cicdUnifiedImage,
	)

	// Base64 encode the script for entrypoint
	entryPoint := base64.StdEncoding.EncodeToString([]byte(script))

	// Create workload with minimal resource requirements (no GPU needed)
	workload := &v1.Workload{
		ObjectMeta: metav1.ObjectMeta{
			Name: job.Name,
			Labels: map[string]string{
				v1.ClusterIdLabel:   v1.GetClusterId(job),       // Get cluster ID from OpsJob
				v1.WorkspaceIdLabel: common.PrimusSafeNamespace, // Use primus-safe workspace
				v1.UserIdLabel:      v1.GetUserId(job),
				v1.OpsJobIdLabel:    job.Name,
				v1.OpsJobTypeLabel:  string(job.Spec.Type),
				v1.DisplayNameLabel: job.Name,
			},
			Annotations: map[string]string{
				v1.UserNameAnnotation: v1.GetUserName(job),
			},
		},
		Spec: v1.WorkloadSpec{
			Resource: v1.WorkloadResource{
				Replica: 1,
				CPU:     "2",
				Memory:  "4Gi",
				// No GPU required for CD jobs
			},
			EntryPoint: entryPoint,
			GroupVersionKind: v1.GroupVersionKind{
				Version: common.DefaultVersion,
				Kind:    common.PytorchJobKind, // Uses Job template
			},
			IsTolerateAll: true, // Can run on any node
			Priority:      common.HighPriorityInt,
			Workspace:     common.PrimusSafeNamespace, // Use primus-safe namespace
			Image:         CDJobImage,
			Env: map[string]string{
				"DEPLOYMENT_REQUEST_ID": getParameterValue(job, v1.ParameterDeploymentRequestId, ""),
				"HAS_NODE_AGENT":        fmt.Sprintf("%t", hasNodeAgent),
				"HAS_CICD":              fmt.Sprintf("%t", hasCICD),
			},
			Hostpath: []string{HostMountPath},
		},
	}

	// Dispatch the workload immediately, skipping the queue
	v1.SetAnnotation(workload, v1.WorkloadScheduledAnnotation, timeutil.FormatRFC3339(time.Now().UTC()))

	if job.Spec.TimeoutSecond > 0 {
		workload.Spec.Timeout = pointer.Int(job.Spec.TimeoutSecond)
	} else {
		// Default timeout of 30 minutes for CD jobs
		workload.Spec.Timeout = pointer.Int(1800)
	}

	if job.Spec.TTLSecondsAfterFinished > 0 {
		workload.Spec.TTLSecondsAfterFinished = pointer.Int(job.Spec.TTLSecondsAfterFinished)
	} else {
		// Default TTL of 1 hour
		workload.Spec.TTLSecondsAfterFinished = pointer.Int(3600)
	}

	return workload, nil
}

// buildCompleteDeployScript builds the complete deployment script including local and remote phases.
func (r *CDJobReconciler) buildCompleteDeployScript(
	componentTags, nodeAgentTags, envFileConfig, deployBranch string,
	hasNodeAgent, hasCICD, installNodeAgent bool,
	nodeAgentImage, cicdRunnerImage, cicdUnifiedImage string,
) string {
	// Build local deployment script
	localScript := r.buildLocalDeployScript(componentTags, nodeAgentTags, envFileConfig, deployBranch)

	// If no remote updates needed, return local script only
	if !hasNodeAgent && !hasCICD {
		return localScript
	}

	// Build remote deployment script
	remoteScript := r.buildRemoteClusterScript(
		hasNodeAgent, hasCICD, installNodeAgent,
		nodeAgentImage, cicdRunnerImage, cicdUnifiedImage, deployBranch,
	)

	// Combine local and remote scripts
	return fmt.Sprintf(`%s

echo ""
echo "=========================================="
echo "Local deployment completed, starting remote cluster updates..."
echo "=========================================="

%s
`, localScript, remoteScript)
}

// getParameterValue retrieves a parameter value from job inputs with a default fallback.
func getParameterValue(job *v1.OpsJob, name, defaultValue string) string {
	param := job.GetParameter(name)
	if param != nil && param.Value != "" {
		return param.Value
	}
	return defaultValue
}

// buildLocalDeployScript builds the shell script for local cluster deployment.
func (r *CDJobReconciler) buildLocalDeployScript(componentTags, nodeAgentTags, envFileConfig, deployBranch string) string {
	// Prepare .env file content (base64 encoded for safe passing)
	envFileBase64 := ""
	if envFileConfig != "" {
		envFileBase64 = base64.StdEncoding.EncodeToString([]byte(envFileConfig))
	}

	return fmt.Sprintf(`
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
echo "CD Job - Local Cluster Deployment"
echo "=========================================="

echo "Step 1: Preparing repository..."
mkdir -p "$MOUNT_DIR"

# Always do a fresh clone to ensure we have the latest code
if [ -d "$REPO_DIR" ]; then
    echo "Removing existing repository at $REPO_DIR..."
    rm -rf "$REPO_DIR"
fi

# Git branch for deployment (empty means use default branch)
DEPLOY_BRANCH="%s"

echo "Cloning repository from $REPO_URL..."
if [ -n "$DEPLOY_BRANCH" ]; then
    echo "Using branch: $DEPLOY_BRANCH"
    git clone --depth 1 -b "$DEPLOY_BRANCH" "$REPO_URL" "$REPO_DIR"
else
    echo "Using default branch"
    git clone --depth 1 "$REPO_URL" "$REPO_DIR"
fi
echo "✓ Repository cloned successfully"

# Helper function to update yaml using sed
update_yaml() {
    local key=$1
    local value=$2
    local file=$3
    local escaped_value=$(echo $value | sed 's/\//\\\//g')
    local parent=$(echo $key | cut -d. -f1)
    local child=$(echo $key | cut -d. -f2)
    
    if [ "$parent" != "$child" ]; then
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
        update_yaml "node_agent.$KEY" "$VAL" "$NODE_AGENT_VALUES"
        echo "✓ Updated node_agent.$KEY in node-agent/values.yaml"
    fi
done

echo "=========================================="
echo "Step 3: Starting upgrade script..."
echo "=========================================="
cd "$REPO_DIR/SaFE/bootstrap/"
/bin/bash ./upgrade.sh

echo "=========================================="
echo "✓ Local deployment completed!"
echo "=========================================="
`, ContainerMountPath, PrimusSaFERepoURL, deployBranch, envFileBase64, componentTags, nodeAgentTags)
}

// buildRemoteClusterScript builds the shell script for remote cluster updates.
// All clusters are updated without excluding any specific cluster.
func (r *CDJobReconciler) buildRemoteClusterScript(hasNodeAgent, hasCICD, installNodeAgent bool,
	nodeAgentImage, cicdRunnerImage, cicdUnifiedImage, deployBranch string) string {

	return fmt.Sprintf(`
set -e

echo "=========================================="
echo "CD Job - Remote Cluster Updates"
echo "=========================================="
echo "NodeAgent update: %t (image: %s)"
echo "CICD update: %t (runner: %s, unified: %s)"
echo "=========================================="

# Configuration
WORK_DIR="%s"
REPO_URL="%s"
REPO_DIR="$WORK_DIR/Primus-SaFE"
NODE_AGENT_CHART="$REPO_DIR/SaFE/node-agent/charts/node-agent"
HAS_NODE_AGENT=%t
HAS_CICD=%t
INSTALL_NODE_AGENT=%t
NODE_AGENT_IMAGE="%s"
CICD_RUNNER_IMAGE="%s"
CICD_UNIFIED_IMAGE="%s"
DEPLOY_BRANCH="%s"

mkdir -p "$WORK_DIR"

# Clone repo if node-agent update needed (for helm chart)
if [ "$HAS_NODE_AGENT" = "true" ] && [ "$INSTALL_NODE_AGENT" = "true" ]; then
    if [ -d "$REPO_DIR" ]; then
        rm -rf "$REPO_DIR"
    fi
    if [ -n "$DEPLOY_BRANCH" ]; then
        echo "Cloning repository for node-agent chart (branch: $DEPLOY_BRANCH)..."
        git clone --depth 1 -b "$DEPLOY_BRANCH" "$REPO_URL" "$REPO_DIR"
    else
        echo "Cloning repository for node-agent chart (default branch)..."
        git clone --depth 1 "$REPO_URL" "$REPO_DIR"
    fi
    echo "✓ Repository cloned"
fi

echo "=========================================="
echo "Step 1: Discover clusters"
echo "=========================================="

# Get all unique cluster IDs from Cluster CRDs
ALL_CLUSTER_IDS=$(kubectl get cluster -o jsonpath='{.items[*].metadata.name}' 2>/dev/null || echo "")
echo "Found clusters: $ALL_CLUSTER_IDS"

ALL_CLUSTER_IDS=$(echo "$ALL_CLUSTER_IDS" | tr ' ' '\n' | sort -u | tr '\n' ' ')

for CLUSTER_ID in $ALL_CLUSTER_IDS; do
    [ -z "$CLUSTER_ID" ] && continue
    
    echo ""
    echo "=========================================="
    echo "Processing cluster: $CLUSTER_ID"
    echo "=========================================="
    
    # Try to get kubeconfig data from Cluster CRD
    CLUSTER_JSON=$(kubectl get cluster "$CLUSTER_ID" -o json 2>/dev/null || echo "")
    
    if [ -z "$CLUSTER_JSON" ]; then
        echo "⚠ Cluster resource not found for $CLUSTER_ID, skipping..."
        continue
    fi
    
    CA_DATA=$(echo "$CLUSTER_JSON" | jq -r '.status.controlPlaneStatus.CAData // empty')
    CERT_DATA=$(echo "$CLUSTER_JSON" | jq -r '.status.controlPlaneStatus.certData // empty')
    KEY_DATA=$(echo "$CLUSTER_JSON" | jq -r '.status.controlPlaneStatus.keyData // empty')
    ENDPOINT_RAW=$(echo "$CLUSTER_JSON" | jq -r '.status.controlPlaneStatus.endpoints[0] // empty')
    PHASE=$(echo "$CLUSTER_JSON" | jq -r '.status.controlPlaneStatus.phase // empty')
    
    # Check if kubeconfig data is available
    if [ -z "$CA_DATA" ] || [ -z "$CERT_DATA" ] || [ -z "$KEY_DATA" ] || [ -z "$ENDPOINT_RAW" ]; then
        echo "Kubeconfig data not available, trying in-cluster config..."
        KUBECONFIG_OPT=""
        KUBECONFIG_FILE=""
    elif [ "$PHASE" != "Ready" ]; then
        echo "⚠ Cluster $CLUSTER_ID not in Ready phase (phase: $PHASE), skipping..."
        continue
    else
        # Generate kubeconfig for this cluster
        ENDPOINT_HOST=$(echo "$ENDPOINT_RAW" | sed 's|^\(https\?://[^:/]*\).*|\1|')
        ENDPOINTS="${ENDPOINT_HOST}:6443"
        
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
        
        if ! kubectl --kubeconfig="$KUBECONFIG_FILE" get nodes > /dev/null 2>&1; then
            echo "⚠ Cannot connect using kubeconfig, trying in-cluster config..."
            rm -f "$KUBECONFIG_FILE"
            KUBECONFIG_OPT=""
            KUBECONFIG_FILE=""
        else
            echo "✓ Connected to cluster $CLUSTER_ID"
            KUBECONFIG_OPT="--kubeconfig=$KUBECONFIG_FILE"
        fi
    fi
    
    # Update node-agent if needed (update ALL clusters including admin cluster)
    if [ "$HAS_NODE_AGENT" = "true" ] && [ "$INSTALL_NODE_AGENT" = "true" ]; then
        echo "Updating node-agent on $CLUSTER_ID..."
        
        NODE_AGENT_VALUES="$NODE_AGENT_CHART/.values.yaml"
        cp "$NODE_AGENT_CHART/values.yaml" "$NODE_AGENT_VALUES"
        sed -i "s|image: \".*\"|image: \"$NODE_AGENT_IMAGE\"|" "$NODE_AGENT_VALUES"
        
        helm $KUBECONFIG_OPT upgrade -i node-agent "$NODE_AGENT_CHART" \
            -n primus-safe --create-namespace \
            -f "$NODE_AGENT_VALUES" \
            || echo "⚠ helm upgrade failed for $CLUSTER_ID, continuing..."
        
        rm -f "$NODE_AGENT_VALUES"
        echo "✓ node-agent updated on $CLUSTER_ID"
    fi
    
    # Update CICD if needed
    if [ "$HAS_CICD" = "true" ]; then
        echo "Updating CICD on $CLUSTER_ID..."
        
        WORKLOADS=$(kubectl get workload -l "primus-safe.workload.kind=AutoscalingRunnerSet,primus-safe.cluster.id=$CLUSTER_ID" -o json 2>/dev/null || echo '{"items":[]}')
        WORKLOAD_COUNT=$(echo "$WORKLOADS" | jq '.items | length')
        
        echo "Found $WORKLOAD_COUNT AutoscalingRunnerSet workloads for cluster $CLUSTER_ID"
        
        for i in $(seq 0 $((WORKLOAD_COUNT - 1))); do
            WORKLOAD_NAME=$(echo "$WORKLOADS" | jq -r ".items[$i].metadata.name")
            WORKSPACE=$(echo "$WORKLOADS" | jq -r ".items[$i].spec.workspace")
            UNIFIED_JOB_ENABLE=$(echo "$WORKLOADS" | jq -r ".items[$i].spec.env.UNIFIED_JOB_ENABLE // \"false\"")
            
            echo "  Processing workload: $WORKLOAD_NAME (namespace: $WORKSPACE)"
            
            CURRENT_ARS=$(kubectl $KUBECONFIG_OPT get autoscalingrunnersets "$WORKLOAD_NAME" -n "$WORKSPACE" -o json 2>/dev/null || echo "")
            
            if [ -z "$CURRENT_ARS" ]; then
                echo "  ⚠ AutoscalingRunnerSet not found, skipping..."
                continue
            fi
            
            CURRENT_RUNNER_IMAGE=$(echo "$CURRENT_ARS" | jq -r '.spec.template.spec.containers[0].image')
            REGISTRY_PREFIX=$(echo "$CURRENT_RUNNER_IMAGE" | sed 's|/[^/]*$|/|')
            FULL_RUNNER_IMAGE="${REGISTRY_PREFIX}${CICD_RUNNER_IMAGE}"
            
            PATCH_RUNNER='[{"op": "replace", "path": "/spec/template/spec/containers/0/image", "value": "'"$FULL_RUNNER_IMAGE"'"}]'
            
            kubectl $KUBECONFIG_OPT patch autoscalingrunnersets "$WORKLOAD_NAME" \
                -n "$WORKSPACE" --type='json' -p="$PATCH_RUNNER" \
                || echo "  ⚠ Failed to patch runner for $WORKLOAD_NAME"
            
            echo "  ✓ Updated runner image"
            
            if [ "$UNIFIED_JOB_ENABLE" = "true" ] && [ -n "$CICD_UNIFIED_IMAGE" ]; then
                FULL_UNIFIED_IMAGE="${REGISTRY_PREFIX}${CICD_UNIFIED_IMAGE}"
                
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
    
    # Cleanup kubeconfig file if generated
    if [ -n "$KUBECONFIG_FILE" ]; then
        rm -f "$KUBECONFIG_FILE"
    fi
done

echo ""
echo "=========================================="
echo "✓ All cluster updates completed!"
echo "=========================================="
`,
		hasNodeAgent, nodeAgentImage,
		hasCICD, cicdRunnerImage, cicdUnifiedImage,
		ContainerMountPath,
		PrimusSaFERepoURL,
		hasNodeAgent,
		hasCICD,
		installNodeAgent,
		nodeAgentImage,
		cicdRunnerImage,
		cicdUnifiedImage,
		deployBranch,
	)
}
