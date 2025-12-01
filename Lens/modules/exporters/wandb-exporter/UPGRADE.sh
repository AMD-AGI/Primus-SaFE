#!/bin/bash
################################################################################
# Primus Lens WandB Exporter - Quick Upgrade Script
# 
# This script forces upgrade to the latest version from PyPI
################################################################################

set -e

# Color definitions
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

print_success() {
    echo -e "${GREEN}✓${NC} $1"
}

print_error() {
    echo -e "${RED}✗${NC} $1"
}

print_info() {
    echo -e "${BLUE}ℹ${NC} $1"
}

echo "========================================="
echo "  Primus Lens WandB Exporter Upgrade"
echo "========================================="
echo

# Detect Python
if command -v python3 &> /dev/null; then
    PYTHON_CMD="python3"
elif command -v python &> /dev/null; then
    PYTHON_CMD="python"
else
    print_error "Python not found"
    exit 1
fi

# Detect pip
if command -v pip3 &> /dev/null; then
    PIP_CMD="pip3"
elif command -v pip &> /dev/null; then
    PIP_CMD="pip"
else
    print_error "pip not found"
    exit 1
fi

# Check current version
if $PYTHON_CMD -c "import primus_lens_wandb_exporter" 2>/dev/null; then
    CURRENT_VERSION=$($PYTHON_CMD -c "import primus_lens_wandb_exporter; print(primus_lens_wandb_exporter.__version__)" 2>/dev/null)
    print_info "Current version: $CURRENT_VERSION"
else
    print_info "Package not installed yet"
fi

echo
print_info "Upgrading to latest version from PyPI..."
print_info "Using: $PIP_CMD install --upgrade --no-cache-dir --force-reinstall primus-lens-wandb-exporter"
echo

# Force upgrade with cache clearing
if $PIP_CMD install --upgrade --no-cache-dir --force-reinstall primus-lens-wandb-exporter; then
    echo
    NEW_VERSION=$($PYTHON_CMD -c "import primus_lens_wandb_exporter; print(primus_lens_wandb_exporter.__version__)" 2>/dev/null)
    print_success "Upgraded successfully to v$NEW_VERSION"
    
    if [ "$CURRENT_VERSION" = "$NEW_VERSION" ]; then
        print_info "Version unchanged (already at latest)"
    else
        print_success "Version changed: $CURRENT_VERSION → $NEW_VERSION"
    fi
else
    print_error "Upgrade failed"
    exit 1
fi

echo
echo "========================================="
print_success "Upgrade completed!"
echo "========================================="

