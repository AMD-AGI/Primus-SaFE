#!/bin/bash
#
# Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#
# Lens CD Deployment Script (Simplified)
# Deploys Control Plane to local cluster and Data Plane to remote clusters

set -e

# Configuration
MOUNT_DIR="${MOUNT_DIR:-/home/primus-safe-cd}"
REPO_DIR="${REPO_DIR:-$MOUNT_DIR/Primus-SaFE}"
NAMESPACE="primus-lens"
CP_CHART="$REPO_DIR/Lens/charts/primus-lens-apps-control-plane"
DP_CHART="$REPO_DIR/Lens/charts/primus-lens-apps-dataplane"
CONTROL_PLANE_LABEL="primus-safe.cluster.control-plane"

echo "=== Lens CD Deployment ==="
echo "ConfigMap: $LENS_CONFIGMAP_NAME"

# Check ConfigMap
if [ -z "$LENS_CONFIGMAP_NAME" ]; then
    echo "Error: LENS_CONFIGMAP_NAME not set"
    exit 1
fi

# Read ConfigMap
CM_JSON=$(kubectl get configmap "$LENS_CONFIGMAP_NAME" -n default -o json 2>/dev/null)
if [ -z "$CM_JSON" ]; then
    echo "Error: ConfigMap $LENS_CONFIGMAP_NAME not found"
    exit 1
fi

CP_CONTENT=$(echo "$CM_JSON" | jq -r '.data["cp-values.yaml"] // empty')
DP_CONTENT=$(echo "$CM_JSON" | jq -r '.data["dp-values.yaml"] // empty')

if [ -z "$CP_CONTENT" ] && [ -z "$DP_CONTENT" ]; then
    echo "Error: No configuration found in ConfigMap"
    exit 1
fi

# Deploy Control Plane
if [ -n "$CP_CONTENT" ]; then
    echo ""
    echo "=== Deploying Control Plane ==="
    echo "$CP_CONTENT" > /tmp/cp-values.yaml
    helm upgrade -i primus-lens-apps-control-plane "$CP_CHART" \
        -n $NAMESPACE --create-namespace \
        -f /tmp/cp-values.yaml
    echo "✓ Control Plane deployed"
fi

# Deploy Data Plane to remote clusters
if [ -n "$DP_CONTENT" ]; then
    echo ""
    echo "=== Deploying Data Plane ==="
    echo "$DP_CONTENT" > /tmp/dp-values.yaml
    
    # Get data plane clusters (without control-plane label)
    CLUSTERS=$(kubectl get cluster -o json 2>/dev/null | jq -r '
        .items[] | 
        select(.metadata.labels["'$CONTROL_PLANE_LABEL'"] == null) | 
        .metadata.name
    ')
    
    if [ -z "$CLUSTERS" ]; then
        echo "No data plane clusters found"
    else
        echo "Found clusters: $(echo $CLUSTERS | tr '\n' ' ')"
        
        for CLUSTER_ID in $CLUSTERS; do
            [ -z "$CLUSTER_ID" ] && continue
            echo ""
            echo "--- Processing: $CLUSTER_ID ---"
            
            # Get cluster info
            CLUSTER_JSON=$(kubectl get cluster "$CLUSTER_ID" -o json 2>/dev/null || echo "")
            if [ -z "$CLUSTER_JSON" ]; then
                echo "⚠ Cluster not found, skipping"
                continue
            fi
            
            # Check phase
            PHASE=$(echo "$CLUSTER_JSON" | jq -r '.status.controlPlaneStatus.phase // empty')
            if [ "$PHASE" != "Ready" ]; then
                echo "⚠ Cluster not Ready (phase: $PHASE), skipping"
                continue
            fi
            
            # Generate kubeconfig
            CA=$(echo "$CLUSTER_JSON" | jq -r '.status.controlPlaneStatus.CAData // empty')
            CERT=$(echo "$CLUSTER_JSON" | jq -r '.status.controlPlaneStatus.certData // empty')
            KEY=$(echo "$CLUSTER_JSON" | jq -r '.status.controlPlaneStatus.keyData // empty')
            ENDPOINT=$(echo "$CLUSTER_JSON" | jq -r '.status.controlPlaneStatus.endpoints[0] // empty' | sed 's|^\(https\?://[^:/]*\).*|\1:6443|')
            
            if [ -z "$CA" ] || [ -z "$CERT" ] || [ -z "$KEY" ] || [ -z "$ENDPOINT" ]; then
                echo "⚠ Missing kubeconfig data, skipping"
                continue
            fi
            
            cat > /tmp/kubeconfig-$CLUSTER_ID << EOF
apiVersion: v1
kind: Config
clusters:
- cluster:
    certificate-authority-data: $CA
    server: $ENDPOINT
  name: $CLUSTER_ID
contexts:
- context:
    cluster: $CLUSTER_ID
    user: admin
  name: $CLUSTER_ID
current-context: $CLUSTER_ID
users:
- name: admin
  user:
    client-certificate-data: $CERT
    client-key-data: $KEY
EOF
            
            # Test connection
            if ! kubectl --kubeconfig=/tmp/kubeconfig-$CLUSTER_ID get nodes > /dev/null 2>&1; then
                echo "⚠ Cannot connect, skipping"
                rm -f /tmp/kubeconfig-$CLUSTER_ID
                continue
            fi
            
            # Deploy
            helm --kubeconfig=/tmp/kubeconfig-$CLUSTER_ID upgrade -i primus-lens-apps-dataplane "$DP_CHART" \
                -n $NAMESPACE --create-namespace \
                -f /tmp/dp-values.yaml \
                || echo "⚠ Helm upgrade failed for $CLUSTER_ID"
            
            echo "✓ Deployed to $CLUSTER_ID"
            rm -f /tmp/kubeconfig-$CLUSTER_ID
        done
    fi
    echo ""
    echo "✓ Data Plane deployment completed"
fi

# Cleanup
rm -f /tmp/cp-values.yaml /tmp/dp-values.yaml

echo ""
echo "=== Lens CD Deployment Complete ==="
