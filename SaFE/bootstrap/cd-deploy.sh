#!/bin/bash
#
# Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#
# CD Job - Unified Deployment Script
# This script is executed by CD Job workloads to deploy Primus-SaFE components.
# All configuration is passed via environment variables.
#

set -e

# ==========================================
# Environment Variables (set by CD Job controller)
# ==========================================
# MOUNT_DIR           - Mount directory for workspace (default: /home/primus-safe-cd)
# REPO_DIR            - Repository directory (set by entrypoint after git clone)
# DEPLOY_BRANCH       - Git branch to deploy
# COMPONENT_TAGS      - Component tags in format: "key1=value1;key2=value2"
# NODE_AGENT_TAGS     - Node agent tags in format: "key1=value1;key2=value2"
# ENV_FILE_CONFIG     - Base64 encoded .env file content
# HAS_NODE_AGENT      - Whether to update node-agent (true/false)
# HAS_CICD            - Whether to update CICD runners (true/false)
# NODE_AGENT_IMAGE    - Node agent image to deploy
# CICD_RUNNER_IMAGE   - CICD runner image to deploy
# CICD_UNIFIED_IMAGE  - CICD unified job image to deploy

# ==========================================
# Configuration
# ==========================================
MOUNT_DIR="${MOUNT_DIR:-/home/primus-safe-cd}"
REPO_DIR="${REPO_DIR:-$MOUNT_DIR/Primus-SaFE}"
NAMESPACE="primus-safe"

PRIMUS_VALUES="$REPO_DIR/SaFE/charts/primus-safe/values.yaml"
NODE_AGENT_VALUES="$REPO_DIR/SaFE/node-agent/charts/node-agent/values.yaml"
NODE_AGENT_CHART="$REPO_DIR/SaFE/node-agent/charts/node-agent"
ENV_FILE="$REPO_DIR/SaFE/bootstrap/.env"

# Convert string to boolean
HAS_NODE_AGENT="${HAS_NODE_AGENT:-false}"
HAS_CICD="${HAS_CICD:-false}"

echo "=========================================="
echo "CD Job - Starting Deployment"
echo "=========================================="
echo "Branch: ${DEPLOY_BRANCH:-default}"
echo "Node Agent Update: $HAS_NODE_AGENT (image: $NODE_AGENT_IMAGE)"
echo "CICD Update: $HAS_CICD (runner: $CICD_RUNNER_IMAGE)"
echo "=========================================="

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

# ==========================================
# Step 1: Update Configuration Files
# ==========================================
echo ""
echo "=========================================="
echo "Step 1: Updating configuration files..."
echo "=========================================="

# Create .env file from user request config
if [ -n "$ENV_FILE_CONFIG" ]; then
    echo "Creating .env file..."
    echo "$ENV_FILE_CONFIG" | base64 -d > "$ENV_FILE"
    echo "✓ .env file created"
fi

# Update components
if [ -n "$COMPONENT_TAGS" ]; then
    IFS=';' read -ra COMPS <<< "$COMPONENT_TAGS"
    for comp in "${COMPS[@]}"; do
        if [ -n "$comp" ]; then
            KEY=$(echo $comp | cut -d= -f1)
            VAL=$(echo $comp | cut -d= -f2)
            update_yaml "$KEY" "$VAL" "$PRIMUS_VALUES"
            echo "✓ Updated $KEY in primus-safe/values.yaml"
        fi
    done
fi

# Update node-agent values
if [ -n "$NODE_AGENT_TAGS" ]; then
    IFS=';' read -ra AGENTS <<< "$NODE_AGENT_TAGS"
    for agent in "${AGENTS[@]}"; do
        if [ -n "$agent" ]; then
            KEY=$(echo $agent | cut -d= -f1)
            VAL=$(echo $agent | cut -d= -f2)
            update_yaml "node_agent.$KEY" "$VAL" "$NODE_AGENT_VALUES"
            echo "✓ Updated node_agent.$KEY in node-agent/values.yaml"
        fi
    done
fi

# ==========================================
# Step 2: Run Local Upgrade Script
# ==========================================
echo ""
echo "=========================================="
echo "Step 2: Running local upgrade script..."
echo "=========================================="
cd "$REPO_DIR/SaFE/bootstrap/"
/bin/bash ./upgrade.sh

# ==========================================
# Step 3: Verify Local Deployments
# ==========================================
echo ""
echo "=========================================="
echo "Step 3: Verifying local deployments..."
echo "=========================================="

# Function to wait for Deployment to be ready
# Checks readyReplicas, updatedReplicas, and Pod error states
wait_deployment_ready() {
    local name=$1
    local ns=$2
    local kubeconfig_opt=$3
    local max_retries=30
    local retry_interval=10
    
    echo "Verifying deployment/$name..."
    for i in $(seq 1 $max_retries); do
        # Get deployment status
        READY=$(kubectl $kubeconfig_opt get deployment/$name -n $ns -o jsonpath='{.status.readyReplicas}' 2>/dev/null || echo "0")
        UPDATED=$(kubectl $kubeconfig_opt get deployment/$name -n $ns -o jsonpath='{.status.updatedReplicas}' 2>/dev/null || echo "0")
        DESIRED=$(kubectl $kubeconfig_opt get deployment/$name -n $ns -o jsonpath='{.spec.replicas}' 2>/dev/null || echo "0")
        READY=${READY:-0}
        UPDATED=${UPDATED:-0}
        
        # Check for Pods with image pull errors (filter by deployment name prefix)
        POD_STATUS=$(kubectl $kubeconfig_opt get pods -n $ns --no-headers 2>/dev/null | grep "^$name-" || echo "")
        if echo "$POD_STATUS" | grep -qE "ErrImagePull|ImagePullBackOff|CrashLoopBackOff"; then
            echo "✗ deployment/$name has Pod errors!"
            echo "$POD_STATUS" | grep -E "ErrImagePull|ImagePullBackOff|CrashLoopBackOff" | head -3
            return 1
        fi
        
        # Check if all replicas are updated AND ready
        if [ "$READY" = "$DESIRED" ] && [ "$UPDATED" = "$DESIRED" ] && [ "$DESIRED" != "0" ]; then
            echo "✓ deployment/$name ready: $READY/$DESIRED replicas (updated: $UPDATED)"
            return 0
        fi
        echo "  Waiting for deployment/$name... (ready=$READY updated=$UPDATED desired=$DESIRED) [$i/$max_retries]"
        sleep $retry_interval
    done
    echo "⚠ deployment/$name not ready after $((max_retries * retry_interval))s"
    return 1
}

# Function to wait for DaemonSet to be ready
# Checks if desiredNumberScheduled == currentNumberScheduled == numberReady
wait_daemonset_ready() {
    local name=$1
    local ns=$2
    local kubeconfig_opt=$3
    local max_retries=30
    local retry_interval=10
    
    echo "Verifying daemonset/$name..."
    
    # First check if DaemonSet exists
    if ! kubectl $kubeconfig_opt get daemonset/$name -n $ns > /dev/null 2>&1; then
        echo "⚠ daemonset/$name not found, skipping..."
        return 0
    fi
    
    for i in $(seq 1 $max_retries); do
        DESIRED=$(kubectl $kubeconfig_opt get daemonset/$name -n $ns -o jsonpath='{.status.desiredNumberScheduled}' 2>/dev/null || echo "0")
        CURRENT=$(kubectl $kubeconfig_opt get daemonset/$name -n $ns -o jsonpath='{.status.currentNumberScheduled}' 2>/dev/null || echo "0")
        READY=$(kubectl $kubeconfig_opt get daemonset/$name -n $ns -o jsonpath='{.status.numberReady}' 2>/dev/null || echo "0")
        DESIRED=${DESIRED:-0}
        CURRENT=${CURRENT:-0}
        READY=${READY:-0}

        if [ "$DESIRED" = "$CURRENT" ] && [ "$DESIRED" != "0" ]; then
            echo "✓ daemonset/$name scheduled: desired=$DESIRED current=$CURRENT (ready=$READY)"
            return 0
        fi
        echo "  Waiting for daemonset/$name... (desired=$DESIRED current=$CURRENT) [$i/$max_retries]"
        sleep $retry_interval
    done
    echo "⚠ daemonset/$name not scheduled after $((max_retries * retry_interval))s: desired=$DESIRED current=$CURRENT"
    return 1
}

echo "Checking Deployment status..."
wait_deployment_ready "primus-safe-apiserver" "$NAMESPACE" ""
wait_deployment_ready "primus-safe-resource-manager" "$NAMESPACE" ""
wait_deployment_ready "primus-safe-job-manager" "$NAMESPACE" ""
wait_deployment_ready "primus-safe-webhooks" "$NAMESPACE" ""
wait_deployment_ready "primus-safe-web" "$NAMESPACE" ""

echo ""
echo "Checking DaemonSet status..."
wait_daemonset_ready "primus-safe-node-agent" "$NAMESPACE" ""

echo ""
echo "✓ Local deployment verification completed"

# ==========================================
# Step 4: Remote Cluster Updates (if needed)
# ==========================================
if [ "$HAS_NODE_AGENT" = "true" ] || [ "$HAS_CICD" = "true" ]; then
    echo ""
    echo "=========================================="
    echo "Step 4: Remote cluster updates..."
    echo "=========================================="
    
    # Get all unique cluster IDs from Cluster CRDs
    ALL_CLUSTER_IDS=$(kubectl get cluster -o jsonpath='{.items[*].metadata.name}' 2>/dev/null || echo "")
    echo "Found clusters: $ALL_CLUSTER_IDS"
    
    ALL_CLUSTER_IDS=$(echo "$ALL_CLUSTER_IDS" | tr ' ' '\n' | sort -u | tr '\n' ' ')
    
    for CLUSTER_ID in $ALL_CLUSTER_IDS; do
        [ -z "$CLUSTER_ID" ] && continue
        
        echo ""
        echo "----------------------------------------"
        echo "Processing cluster: $CLUSTER_ID"
        echo "----------------------------------------"
        
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
            echo "⚠ Kubeconfig data not available for $CLUSTER_ID, skipping..."
            continue
        fi
        
        if [ "$PHASE" != "Ready" ]; then
            echo "⚠ Cluster $CLUSTER_ID not in Ready phase (phase: $PHASE), skipping..."
            continue
        fi
        
        # Generate kubeconfig for this cluster
        ENDPOINT_HOST=$(echo "$ENDPOINT_RAW" | sed 's|^\(https\?://[^:/]*\).*|\1|')
        ENDPOINTS="${ENDPOINT_HOST}:6443"
        
        KUBECONFIG_FILE="$MOUNT_DIR/kubeconfig-$CLUSTER_ID"
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
        
        # Test connection, skip if failed
        if ! kubectl --kubeconfig="$KUBECONFIG_FILE" get nodes > /dev/null 2>&1; then
            echo "⚠ Cannot connect to cluster $CLUSTER_ID, skipping..."
            rm -f "$KUBECONFIG_FILE"
            continue
        fi
        
        echo "✓ Connected to cluster $CLUSTER_ID"
        KUBECONFIG_OPT="--kubeconfig=$KUBECONFIG_FILE"
        
        # Update node-agent if needed
        if [ "$HAS_NODE_AGENT" = "true" ]; then
            echo "Updating node-agent on $CLUSTER_ID..."
            
            NODE_AGENT_TMP_VALUES="$NODE_AGENT_CHART/.values.yaml"
            cp "$NODE_AGENT_CHART/values.yaml" "$NODE_AGENT_TMP_VALUES"
            sed -i "s|image: \".*\"|image: \"$NODE_AGENT_IMAGE\"|" "$NODE_AGENT_TMP_VALUES"
            
            helm $KUBECONFIG_OPT upgrade -i node-agent "$NODE_AGENT_CHART" \
                -n $NAMESPACE --create-namespace \
                -f "$NODE_AGENT_TMP_VALUES" \
                || echo "⚠ helm upgrade failed for $CLUSTER_ID, continuing..."
            
            rm -f "$NODE_AGENT_TMP_VALUES"
            
            # Verify node-agent DaemonSet using precise check
            wait_daemonset_ready "primus-safe-node-agent" "$NAMESPACE" "$KUBECONFIG_OPT"
            
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
    echo "✓ Remote cluster updates completed"
fi

# ==========================================
# Step 5: Final Summary
# ==========================================
echo ""
echo "=========================================="
echo "✓ CD Deployment Completed Successfully!"
echo "=========================================="

