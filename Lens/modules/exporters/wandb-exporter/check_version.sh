#!/bin/bash
################################################################################
# Version Consistency Checker
# Check if version numbers in three files are consistent
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

print_warning() {
    echo -e "${YELLOW}⚠${NC} $1"
}

echo "========================================="
echo "  Version Consistency Check"
echo "========================================="
echo

# Extract version numbers from three files
if [ -f "src/primus_lens_wandb_exporter/__init__.py" ]; then
    INIT_VERSION=$(grep "__version__" src/primus_lens_wandb_exporter/__init__.py | cut -d'"' -f2)
else
    print_error "__init__.py not found"
    exit 1
fi

if [ -f "setup.py" ]; then
    SETUP_VERSION=$(grep "version=" setup.py | head -1 | sed "s/.*version='\([^']*\)'.*/\1/")
else
    print_error "setup.py not found"
    exit 1
fi

if [ -f "pyproject.toml" ]; then
    TOML_VERSION=$(grep "^version" pyproject.toml | cut -d'"' -f2)
else
    print_error "pyproject.toml not found"
    exit 1
fi

# Display version numbers
print_info "Version in __init__.py:     $INIT_VERSION"
print_info "Version in setup.py:        $SETUP_VERSION"
print_info "Version in pyproject.toml:  $TOML_VERSION"
echo

# Check consistency
if [ "$INIT_VERSION" = "$SETUP_VERSION" ] && [ "$SETUP_VERSION" = "$TOML_VERSION" ]; then
    print_success "All versions match: $INIT_VERSION"
    echo
    exit 0
else
    print_error "Version mismatch detected!"
    echo
    
    if [ "$INIT_VERSION" != "$SETUP_VERSION" ]; then
        print_warning "__init__.py ($INIT_VERSION) != setup.py ($SETUP_VERSION)"
    fi
    
    if [ "$SETUP_VERSION" != "$TOML_VERSION" ]; then
        print_warning "setup.py ($SETUP_VERSION) != pyproject.toml ($TOML_VERSION)"
    fi
    
    if [ "$INIT_VERSION" != "$TOML_VERSION" ]; then
        print_warning "__init__.py ($INIT_VERSION) != pyproject.toml ($TOML_VERSION)"
    fi
    
    echo
    print_error "Please update all version numbers to be consistent!"
    echo
    echo "Files to update:"
    echo "  1. src/primus_lens_wandb_exporter/__init__.py"
    echo "  2. setup.py"
    echo "  3. pyproject.toml"
    echo
    exit 1
fi
