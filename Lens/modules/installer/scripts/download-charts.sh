#!/bin/bash
# Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#
# Script to package Helm charts from local source for offline installation.
# This script downloads dependencies and packages charts into .tgz files.
#
# Usage:
#   ./download-charts.sh [OPTIONS]
#
# Options:
#   -s, --source-dir    Source directory containing charts (default: ../../charts relative to script)
#   -o, --output-dir    Output directory for packaged charts (default: ./packaged-charts)
#   -h, --help          Show this help message

set -e

# Get script directory
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# Default values
SOURCE_DIR="${SCRIPT_DIR}/../../charts"
OUTPUT_DIR="./packaged-charts"

# Chart names that need to be packaged
CHARTS=(
    "primus-lens-operators"
    "primus-lens-infrastructure"
    "primus-lens-init"
    "primus-lens-apps-dataplane"
)

# Charts that have external dependencies
CHARTS_WITH_DEPS=(
    "primus-lens-operators"
)

# Parse arguments
while [[ $# -gt 0 ]]; do
    case $1 in
        -s|--source-dir)
            SOURCE_DIR="$2"
            shift 2
            ;;
        -o|--output-dir)
            OUTPUT_DIR="$2"
            shift 2
            ;;
        -h|--help)
            head -20 "$0" | tail -n +2 | sed 's/^# //' | sed 's/^#//'
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

# Resolve source directory to absolute path
SOURCE_DIR="$(cd "$SOURCE_DIR" && pwd)"

echo "Source directory: $SOURCE_DIR"
echo "Output directory: $OUTPUT_DIR"
echo ""

# Create output directory
mkdir -p "$OUTPUT_DIR"

# Function to check if chart has dependencies
has_dependencies() {
    local chart="$1"
    for dep_chart in "${CHARTS_WITH_DEPS[@]}"; do
        if [[ "$chart" == "$dep_chart" ]]; then
            return 0
        fi
    done
    return 1
}

# Package each chart
for chart in "${CHARTS[@]}"; do
    chart_path="$SOURCE_DIR/$chart"
    
    if [[ ! -d "$chart_path" ]]; then
        echo "Warning: Chart directory not found: $chart_path"
        continue
    fi
    
    echo "Packaging $chart..."
    
    # Update dependencies if needed
    if has_dependencies "$chart"; then
        echo "  Downloading dependencies..."
        (cd "$chart_path" && helm dependency update)
    fi
    
    # Package the chart
    helm package "$chart_path" -d "$OUTPUT_DIR"
    echo "  Packaged: $chart"
done

echo ""
echo "Charts packaged successfully:"
ls -la "$OUTPUT_DIR"
