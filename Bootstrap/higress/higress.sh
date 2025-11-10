#!/usr/bin/env bash

################################################################################
# Higress Gateway Deployment Script
# Description: Automated deployment of Higress cloud-native gateway and Gateway API configuration
# Features:
#   1. Add Higress Helm repository
#   2. Install/Upgrade Higress gateway
#   3. Deploy Kubernetes Gateway API CRDs
#   4. Apply custom Gateway configuration
################################################################################

set -euo pipefail  # Strict mode: exit on error, error on undefined variables, propagate pipe errors

# Get the absolute path of the script directory
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# Color definitions
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m' # No Color

# Logging functions
log_info() {
    echo -e "${GREEN}[INFO]${NC} $1"
}

log_warn() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# Check required dependencies
check_dependencies() {
    log_info "Checking required dependencies..."
    
    if ! command -v helm &> /dev/null; then
        log_error "helm command not found, please install Helm first"
        exit 1
    fi
    
    if ! command -v kubectl &> /dev/null; then
        log_error "kubectl command not found, please install kubectl first"
        exit 1
    fi
    
    log_info "Dependency check passed"
}

# Add Higress Helm repository
add_helm_repo() {
    log_info "Adding Higress Helm repository..."
    
    if helm repo list | grep -q "higress.io"; then
        log_warn "Higress Helm repository already exists, updating repository..."
        helm repo update higress.io
    else
        helm repo add higress.io https://higress.io/helm-charts
        helm repo update
    fi
    
    log_info "Helm repository configuration completed"
}

# Install/Upgrade Higress gateway
install_higress() {
    log_info "Starting Higress gateway deployment..."
    
    # Check if values.yaml exists
    if [ ! -f "${SCRIPT_DIR}/values.yaml" ]; then
        log_error "Configuration file not found: ${SCRIPT_DIR}/values.yaml"
        exit 1
    fi
    
    # Install or upgrade Higress using Helm
    # --namespace: specify the installation namespace
    # --create-namespace: create namespace if it doesn't exist
    # -f: specify custom configuration file
    helm upgrade --install higress higress.io/higress \
        --namespace higress-system \
        --create-namespace \
        -f "${SCRIPT_DIR}/values.yaml" \
        --wait \
        --timeout 5m
    
    log_info "Higress gateway deployment completed"
}

# Deploy Gateway API CRDs
install_gateway_api() {
    log_info "Deploying Kubernetes Gateway API CRDs..."
    
    # Install Gateway API v1.0.0 experimental version
    # Includes GatewayClass, Gateway, HTTPRoute, TCPRoute and other CRDs
    kubectl apply -f https://github.com/kubernetes-sigs/gateway-api/releases/download/v1.0.0/experimental-install.yaml
    
    log_info "Gateway API CRDs deployment completed"
}

# Apply custom Gateway configuration
apply_gateway_config() {
    log_info "Applying custom Gateway configuration..."
    
    # Check if gateway.yaml exists
    if [ ! -f "${SCRIPT_DIR}/gateway.yaml" ]; then
        log_error "Configuration file not found: ${SCRIPT_DIR}/gateway.yaml"
        exit 1
    fi
    
    # Apply GatewayClass and Gateway configuration
    kubectl apply -f "${SCRIPT_DIR}/gateway.yaml"
    
    log_info "Gateway configuration applied successfully"
}

# Verify deployment status
verify_deployment() {
    log_info "Verifying Higress deployment status..."
    
    # Wait for pods to be ready
    kubectl wait --for=condition=ready pod \
        -l app.kubernetes.io/name=higress \
        -n higress-system \
        --timeout=300s || {
        log_error "Higress pods failed to become ready within the timeout period"
        return 1
    }
    
    # Display service status
    log_info "Higress service status:"
    kubectl get svc -n higress-system
    
    log_info "Gateway status:"
    kubectl get gateway -n higress-system
    
    log_info "Deployment verification completed"
}

# Main function
main() {
    log_info "Starting Higress deployment workflow..."
    
    # Execute deployment steps
    check_dependencies
    add_helm_repo
    install_higress
    install_gateway_api
    apply_gateway_config
    verify_deployment
    
    log_info "======================================"
    log_info "Higress deployment workflow completed!"
    log_info "======================================"
}

# Execute main function
main