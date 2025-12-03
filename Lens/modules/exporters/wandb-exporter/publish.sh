#!/bin/bash
#
# Primus Lens WandB Exporter - PyPI Publishing Script
#
# Usage:
#   1. Set environment variables:
#      export PYPI_TOKEN="pypi-AgEIcHlwaS5vcmcC..."
#      export TESTPYPI_TOKEN="pypi-AgEI..." (optional, for testing)
#
#   2. Run the script:
#      ./publish.sh [--test] [--skip-tests] [--skip-build]
#
# Arguments:
#   --test          Upload to TestPyPI instead of official PyPI
#   --skip-tests    Skip test phase
#   --skip-build    Skip build phase (reuse existing dist/)
#   --help          Show help message
#

set -e  # Exit immediately on error

# Color output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Logging functions
log_info() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

log_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

log_warning() {
    echo -e "${YELLOW}[WARNING]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# Show help message
show_help() {
    cat << EOF
Primus Lens WandB Exporter - PyPI Publishing Script

Usage:
    ./publish.sh [options]

Environment Variables:
    PYPI_TOKEN          PyPI API Token (required)
    TESTPYPI_TOKEN      TestPyPI API Token (required when using --test)

Options:
    --test              Upload to TestPyPI for testing
    --skip-tests        Skip test phase
    --skip-build        Skip build phase (reuse existing dist/)
    --help              Show this help message

Examples:
    # Publish to official PyPI
    export PYPI_TOKEN="pypi-AgEIcHlwaS5vcmcC..."
    ./publish.sh

    # Test publish to TestPyPI first
    export TESTPYPI_TOKEN="pypi-AgEI..."
    ./publish.sh --test

    # Skip tests and publish directly
    ./publish.sh --skip-tests

Get PyPI Token:
    1. Visit https://pypi.org/manage/account/token/
    2. Create a new API token
    3. Copy token and set as environment variable

EOF
}

# Parse command line arguments
USE_TESTPYPI=false
SKIP_TESTS=false
SKIP_BUILD=false

while [[ $# -gt 0 ]]; do
    case $1 in
        --test)
            USE_TESTPYPI=true
            shift
            ;;
        --skip-tests)
            SKIP_TESTS=true
            shift
            ;;
        --skip-build)
            SKIP_BUILD=true
            shift
            ;;
        --help)
            show_help
            exit 0
            ;;
        *)
            log_error "Unknown argument: $1"
            show_help
            exit 1
            ;;
    esac
done

# Check environment variables
if [ "$USE_TESTPYPI" = true ]; then
    if [ -z "$TESTPYPI_TOKEN" ]; then
        log_error "TESTPYPI_TOKEN environment variable not set"
        echo "Please run: export TESTPYPI_TOKEN=\"your-token-here\""
        exit 1
    fi
    PYPI_TOKEN="$TESTPYPI_TOKEN"
    REPOSITORY="testpypi"
    REPOSITORY_URL="https://test.pypi.org/legacy/"
else
    if [ -z "$PYPI_TOKEN" ]; then
        log_error "PYPI_TOKEN environment variable not set"
        echo "Please run: export PYPI_TOKEN=\"your-token-here\""
        exit 1
    fi
    REPOSITORY="pypi"
    REPOSITORY_URL="https://upload.pypi.org/legacy/"
fi

# Get script directory
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$SCRIPT_DIR"

echo ""
echo "╔════════════════════════════════════════════════════════════════╗"
echo "║    Primus Lens WandB Exporter - PyPI Publishing Tool          ║"
echo "╚════════════════════════════════════════════════════════════════╝"
echo ""

log_info "Working directory: $SCRIPT_DIR"
log_info "Target repository: $REPOSITORY"
echo ""

# Step 1: Check required tools
log_info "Step 1/6: Checking required tools..."

if ! command -v python3 &> /dev/null; then
    log_error "Python3 not installed"
    exit 1
fi

PYTHON_VERSION=$(python3 --version)
log_success "Python: $PYTHON_VERSION"

# Check virtual environment
if [ ! -d ".venv" ]; then
    log_warning "Virtual environment does not exist, creating..."
    python3 -m venv .venv
fi

# Activate virtual environment
source .venv/bin/activate

# Install necessary build tools
log_info "Installing build tools..."
pip install --upgrade pip build twine > /dev/null 2>&1

log_success "Tool check completed"
echo ""

# Step 2: Run tests
if [ "$SKIP_TESTS" = false ]; then
    log_info "Step 2/6: Running test suite..."
    
    # Set test environment variables
    export PRIMUS_LENS_WANDB_HOOK=true
    export WANDB_MODE=offline
    export WANDB_SILENT=true
    
    if python3 test_real_scenario.py --scenario basic; then
        log_success "Basic tests passed"
    else
        log_error "Tests failed"
        echo ""
        read -p "Continue with publishing? (y/N): " -n 1 -r
        echo ""
        if [[ ! $REPLY =~ ^[Yy]$ ]]; then
            log_info "Publishing cancelled"
            exit 1
        fi
    fi
    echo ""
else
    log_warning "Step 2/6: Skipping tests"
    echo ""
fi

# Step 3: Clean old build files
if [ "$SKIP_BUILD" = false ]; then
    log_info "Step 3/6: Cleaning old build files..."
    
    rm -rf build/ dist/ *.egg-info src/*.egg-info
    
    log_success "Cleanup completed"
    echo ""
else
    log_warning "Step 3/6: Skipping cleanup (keeping existing build)"
    echo ""
fi

# Step 4: Build package
if [ "$SKIP_BUILD" = false ]; then
    log_info "Step 4/6: Building package..."
    
    python3 -m build
    
    if [ $? -eq 0 ]; then
        log_success "Package built successfully"
        echo ""
        log_info "Build artifacts:"
        ls -lh dist/
    else
        log_error "Package build failed"
        exit 1
    fi
    echo ""
else
    log_warning "Step 4/6: Skipping build"
    echo ""
fi

# Step 5: Check package
log_info "Step 5/6: Checking package integrity..."

twine check dist/*

if [ $? -eq 0 ]; then
    log_success "Package check passed"
else
    log_error "Package check failed"
    exit 1
fi
echo ""

# Step 6: Upload to PyPI
log_info "Step 6/6: Uploading to $REPOSITORY..."
echo ""

if [ "$USE_TESTPYPI" = true ]; then
    log_warning "This is a test upload to TestPyPI"
    log_warning "Install test package: pip install --index-url https://test.pypi.org/simple/ primus-lens-wandb-exporter"
else
    log_warning "This is an official upload to PyPI, please confirm!"
fi
echo ""

read -p "Confirm upload? (y/N): " -n 1 -r
echo ""

if [[ ! $REPLY =~ ^[Yy]$ ]]; then
    log_info "Upload cancelled"
    exit 0
fi

# Use twine to upload, passing token via environment variables
export TWINE_USERNAME="__token__"
export TWINE_PASSWORD="$PYPI_TOKEN"

if [ "$USE_TESTPYPI" = true ]; then
    twine upload --repository-url "$REPOSITORY_URL" dist/*
else
    twine upload dist/*
fi

if [ $? -eq 0 ]; then
    echo ""
    log_success "Upload successful!"
    echo ""
    echo "╔════════════════════════════════════════════════════════════════╗"
    echo "║                    🎉 Publishing Successful!                   ║"
    echo "╚════════════════════════════════════════════════════════════════╝"
    echo ""
    
    if [ "$USE_TESTPYPI" = true ]; then
        echo "Test installation command:"
        echo "  pip install --index-url https://test.pypi.org/simple/ primus-lens-wandb-exporter"
    else
        echo "Installation command:"
        echo "  pip install primus-lens-wandb-exporter"
        echo ""
        echo "Package page:"
        echo "  https://pypi.org/project/primus-lens-wandb-exporter/"
    fi
    echo ""
else
    log_error "Upload failed"
    exit 1
fi

# Clean up environment variables
unset TWINE_USERNAME
unset TWINE_PASSWORD

log_info "Publishing process completed"
