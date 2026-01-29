#!/bin/bash
# Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#
# Script to download Helm charts for offline installation.
# This script can be used in CI/CD pipelines to pre-download charts
# before building the installer image.
#
# Usage:
#   ./download-charts.sh [OPTIONS]
#
# Options:
#   -o, --output-dir    Output directory for downloaded charts (default: ./charts)
#   -r, --registry      OCI registry URL (default: oci://docker.io/primussafe)
#   -v, --version       Chart version to download (default: latest)
#   -u, --username      Registry username (optional)
#   -p, --password      Registry password (optional)
#   -h, --help          Show this help message

set -e

# Default values
OUTPUT_DIR="./charts"
REGISTRY="oci://docker.io/primussafe"
VERSION=""
USERNAME=""
PASSWORD=""

# Chart names
CHARTS=(
    "primus-lens-operators"
    "primus-lens-infrastructure"
    "primus-lens-init"
    "primus-lens-apps-dataplane"
)

# Parse arguments
while [[ $# -gt 0 ]]; do
    case $1 in
        -o|--output-dir)
            OUTPUT_DIR="$2"
            shift 2
            ;;
        -r|--registry)
            REGISTRY="$2"
            shift 2
            ;;
        -v|--version)
            VERSION="$2"
            shift 2
            ;;
        -u|--username)
            USERNAME="$2"
            shift 2
            ;;
        -p|--password)
            PASSWORD="$2"
            shift 2
            ;;
        -h|--help)
            head -25 "$0" | tail -n +2 | sed 's/^# //' | sed 's/^#//'
            exit 0
            ;;
        *)
            echo "Unknown option: $1"
            exit 1
            ;;
    esac
done

# Check if helm is installed
if ! command -v helm &> /dev/null; then
    echo "Error: helm is not installed"
    exit 1
fi

# Create output directory
mkdir -p "$OUTPUT_DIR"

# Login to registry if credentials provided
if [[ -n "$USERNAME" ]] && [[ -n "$PASSWORD" ]]; then
    echo "Logging into registry..."
    # Extract host from OCI URL (e.g., oci://docker.io/primussafe -> docker.io)
    REGISTRY_HOST=$(echo "$REGISTRY" | sed 's|oci://||' | cut -d'/' -f1)
    echo "$PASSWORD" | helm registry login "$REGISTRY_HOST" -u "$USERNAME" --password-stdin
fi

echo "Downloading charts to: $OUTPUT_DIR"
echo "Registry: $REGISTRY"
echo "Version: ${VERSION:-latest}"
echo ""

# Download each chart
for chart in "${CHARTS[@]}"; do
    echo "Downloading $chart..."
    
    if [[ -n "$VERSION" ]]; then
        helm pull "$REGISTRY/$chart" --version "$VERSION" --destination "$OUTPUT_DIR" || {
            echo "Warning: Failed to download $chart version $VERSION, trying without version..."
            helm pull "$REGISTRY/$chart" --destination "$OUTPUT_DIR"
        }
    else
        helm pull "$REGISTRY/$chart" --destination "$OUTPUT_DIR"
    fi
    
    echo "  Downloaded: $chart"
done

echo ""
echo "Charts downloaded successfully:"
ls -la "$OUTPUT_DIR"
