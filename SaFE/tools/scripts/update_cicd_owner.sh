#!/bin/bash
#
# Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#

# ==============================================================================
# Update CICD Owner Script
#
# This script updates the owner (user.id and user.name) of workload, the secret associated
# with a workload, and also updates the AutoscalingRunnerSet.
#
# Usage: ./update_secret_owner.sh <workload_name> <user_id> <user_name>
# ==============================================================================

set -e

# Check arguments
if [ $# -ne 3 ]; then
    echo "Usage: $0 <workload_name> <user_id> <user_name>"
    echo ""
    echo "Arguments:"
    echo "  workload_name  - The name of the workload (e.g., primus-safe-cicd-xfxr6)"
    echo "  user_id        - The new user ID to set"
    echo "  user_name      - The new user name to set"
    echo ""
    echo "Example:"
    echo "  $0 primus-safe-cicd-xfxr6 abc123def456 john.doe"
    exit 1
fi

WORKLOAD_NAME=$1
USER_ID=$2
USER_NAME=$3

echo "======================================================================"
echo "Update Workload, Secret and AutoscalingRunnerSet Owner"
echo "======================================================================"
echo "Workload: $WORKLOAD_NAME"
echo "New User ID: $USER_ID"
echo "New User Name: $USER_NAME"
echo ""

# Step 1: Get workload and extract secret ID and workspace from annotation
echo "[Step 1] Getting workload information..."

SECRET_ID=$(kubectl get workloads "$WORKLOAD_NAME" -o jsonpath='{.metadata.annotations.primus-safe\.github\.secret\.id}' 2>/dev/null)
WORKSPACE=$(kubectl get workloads "$WORKLOAD_NAME" -o jsonpath='{.spec.workspace}' 2>/dev/null)

if [ -z "$SECRET_ID" ]; then
    echo "❌ Error: Could not find annotation 'primus-safe.github.secret.id' in workload $WORKLOAD_NAME"
    exit 1
fi

if [ -z "$WORKSPACE" ]; then
    echo "❌ Error: Could not find 'spec.workspace' in workload $WORKLOAD_NAME"
    exit 1
fi

echo "✅ Found secret ID: $SECRET_ID"
echo "✅ Found workspace: $WORKSPACE"
echo ""

# ==============================================================================
# Part 1: Update Secret
# ==============================================================================
echo "======================================================================"
echo "Part 1: Update Secret"
echo "======================================================================"

# Step 2: Verify secret exists
echo "[Step 2] Verifying secret exists..."
if ! kubectl -n primus-safe get secret "$SECRET_ID" -o name > /dev/null 2>&1; then
    echo "❌ Error: Secret '$SECRET_ID' not found in namespace 'primus-safe'"
    exit 1
fi
echo "✅ Secret exists"
echo ""

# Step 3: Show current secret info
echo "[Step 3] Current secret information:"
echo "  Label primus-safe.user.id: $(kubectl -n primus-safe get secret "$SECRET_ID" -o jsonpath='{.metadata.labels.primus-safe\.user\.id}')"
echo "  Annotation primus-safe.user.name: $(kubectl -n primus-safe get secret "$SECRET_ID" -o jsonpath='{.metadata.annotations.primus-safe\.user\.name}')"
echo ""

# Step 4: Update secret label and annotation
echo "[Step 4] Updating secret..."

# Update label: primus-safe.user.id
kubectl -n primus-safe label secret "$SECRET_ID" \
    "primus-safe.user.id=$USER_ID" \
    --overwrite

# Update annotation: primus-safe.user.name
kubectl -n primus-safe annotate secret "$SECRET_ID" \
    "primus-safe.user.name=$USER_NAME" \
    --overwrite

echo "✅ Secret updated successfully!"
echo ""

# Step 5: Verify secret changes
echo "[Step 5] Verifying secret changes:"
echo "  Label primus-safe.user.id: $(kubectl -n primus-safe get secret "$SECRET_ID" -o jsonpath='{.metadata.labels.primus-safe\.user\.id}')"
echo "  Annotation primus-safe.user.name: $(kubectl -n primus-safe get secret "$SECRET_ID" -o jsonpath='{.metadata.annotations.primus-safe\.user\.name}')"
echo ""

# ==============================================================================
# Part 2: Update AutoscalingRunnerSet
# ==============================================================================
echo "======================================================================"
echo "Part 2: Update AutoscalingRunnerSet"
echo "======================================================================"

# Step 6: Verify AutoscalingRunnerSet exists
echo "[Step 6] Verifying AutoscalingRunnerSet exists..."
if ! kubectl -n "$WORKSPACE" get AutoscalingRunnerSet "$WORKLOAD_NAME" -o name > /dev/null 2>&1; then
    echo "⚠️  Warning: AutoscalingRunnerSet '$WORKLOAD_NAME' not found in namespace '$WORKSPACE'"
    echo "   Skipping AutoscalingRunnerSet update..."
else
    echo "✅ AutoscalingRunnerSet exists"
    echo ""

    # Step 7: Show current AutoscalingRunnerSet info
    echo "[Step 7] Current AutoscalingRunnerSet information:"
    echo "  metadata.annotations.primus-safe.user.name: $(kubectl -n "$WORKSPACE" get AutoscalingRunnerSet "$WORKLOAD_NAME" -o jsonpath='{.metadata.annotations.primus-safe\.user\.name}')"
    echo "  spec.template.metadata.annotations.primus-safe.user.name: $(kubectl -n "$WORKSPACE" get AutoscalingRunnerSet "$WORKLOAD_NAME" -o jsonpath='{.spec.template.metadata.annotations.primus-safe\.user\.name}')"
    echo ""

    # Step 8: Update AutoscalingRunnerSet annotations
    echo "[Step 8] Updating AutoscalingRunnerSet annotations..."

    # Update metadata.annotations.primus-safe.user.name
    kubectl -n "$WORKSPACE" annotate AutoscalingRunnerSet "$WORKLOAD_NAME" \
        "primus-safe.user.name=$USER_NAME" \
        --overwrite

    # Update spec.template.metadata.annotations.primus-safe.user.name using patch
    kubectl -n "$WORKSPACE" patch AutoscalingRunnerSet "$WORKLOAD_NAME" \
        --type='merge' \
        -p "{\"spec\":{\"template\":{\"metadata\":{\"annotations\":{\"primus-safe.user.name\":\"$USER_NAME\"}}}}}"

    echo "✅ Annotations updated!"
    echo ""

    # Step 9: Update USER_ID environment variable in all containers
    echo "[Step 9] Updating USER_ID env variable in containers..."

    # Get the number of containers
    CONTAINER_COUNT=$(kubectl -n "$WORKSPACE" get AutoscalingRunnerSet "$WORKLOAD_NAME" -o jsonpath='{.spec.template.spec.containers}' | jq 'length')

    echo "  Found $CONTAINER_COUNT container(s)"

    UPDATED_COUNT=0

    for ((i=0; i<CONTAINER_COUNT; i++)); do
        CONTAINER_NAME=$(kubectl -n "$WORKSPACE" get AutoscalingRunnerSet "$WORKLOAD_NAME" -o jsonpath="{.spec.template.spec.containers[$i].name}")
        echo "  [$((i+1))/$CONTAINER_COUNT] Processing container: $CONTAINER_NAME"

        # Find the index of USER_ID env var in this container
        ENV_COUNT=$(kubectl -n "$WORKSPACE" get AutoscalingRunnerSet "$WORKLOAD_NAME" -o jsonpath="{.spec.template.spec.containers[$i].env}" | jq 'length')

        FOUND_USER_ID=false
        for ((j=0; j<ENV_COUNT; j++)); do
            ENV_NAME=$(kubectl -n "$WORKSPACE" get AutoscalingRunnerSet "$WORKLOAD_NAME" -o jsonpath="{.spec.template.spec.containers[$i].env[$j].name}")
            if [ "$ENV_NAME" == "USER_ID" ]; then
                echo "    Found USER_ID at env index $j, updating..."

                # Use JSON patch to update the specific env var
                kubectl -n "$WORKSPACE" patch AutoscalingRunnerSet "$WORKLOAD_NAME" \
                    --type='json' \
                    -p "[{\"op\": \"replace\", \"path\": \"/spec/template/spec/containers/$i/env/$j/value\", \"value\": \"$USER_ID\"}]"

                echo "    ✅ Updated USER_ID in container '$CONTAINER_NAME'"
                UPDATED_COUNT=$((UPDATED_COUNT + 1))
                FOUND_USER_ID=true
                break
            fi
        done

        if [ "$FOUND_USER_ID" = false ]; then
            echo "    ⚠️  No USER_ID env var found in container '$CONTAINER_NAME'"
        fi
    done

    echo ""
    echo "✅ Updated USER_ID in $UPDATED_COUNT of $CONTAINER_COUNT container(s)"
    echo ""

    # Step 10: Verify AutoscalingRunnerSet changes
    echo "[Step 10] Verifying AutoscalingRunnerSet changes:"
    echo "  metadata.annotations.primus-safe.user.name: $(kubectl -n "$WORKSPACE" get AutoscalingRunnerSet "$WORKLOAD_NAME" -o jsonpath='{.metadata.annotations.primus-safe\.user\.name}')"
    echo "  spec.template.metadata.annotations.primus-safe.user.name: $(kubectl -n "$WORKSPACE" get AutoscalingRunnerSet "$WORKLOAD_NAME" -o jsonpath='{.spec.template.metadata.annotations.primus-safe\.user\.name}')"

    # Verify USER_ID in containers
    for ((i=0; i<CONTAINER_COUNT; i++)); do
        CONTAINER_NAME=$(kubectl -n "$WORKSPACE" get AutoscalingRunnerSet "$WORKLOAD_NAME" -o jsonpath="{.spec.template.spec.containers[$i].name}")
        USER_ID_VALUE=$(kubectl -n "$WORKSPACE" get AutoscalingRunnerSet "$WORKLOAD_NAME" -o jsonpath="{.spec.template.spec.containers[$i].env[?(@.name=='USER_ID')].value}")
        echo "  Container '$CONTAINER_NAME' USER_ID: $USER_ID_VALUE"
    done
fi


# Step 10: Update user.id and user.name variable in workload
echo "[Step 10] Updating user.id and user.name variable in workload..."

kubectl -n primus-safe label workload "$WORKLOAD_NAME" \
    "primus-safe.user.id=$USER_ID" \
    --overwrite

kubectl -n primus-safe label workload "$WORKLOAD_NAME" \
    "primus-safe.user.name.md5=$USER_ID" \
    --overwrite

kubectl -n primus-safe annotate workload "$WORKLOAD_NAME" \
    "primus-safe.user.name=$USER_NAME" \
    --overwrite

echo ""
echo "======================================================================"
echo "✅ All Done!"
echo "======================================================================"
