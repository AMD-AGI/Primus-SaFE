#!/bin/bash
################################################################################
# Primus Lens WandB Exporter - One-Click Installation Script
# 
# Usage:
#   Method 1: curl -fsSL https://raw.githubusercontent.com/your-repo/main/install.sh | bash
#   Method 2: wget -qO- https://raw.githubusercontent.com/your-repo/main/install.sh | bash
#   Method 3: bash install.sh
#   Method 4: bash install.sh --install-system-python  (also install to system Python)
#
# This script will:
#   1. Detect Python environment
#   2. Install primus-lens-wandb-exporter package
#   3. Automatically create .pth file
#   4. Verify installation success
#   5. Detect multi-interpreter environments (venv vs system Python)
################################################################################

set -e

# Parse arguments
INSTALL_SYSTEM_PYTHON=false
for arg in "$@"; do
    case $arg in
        --install-system-python)
            INSTALL_SYSTEM_PYTHON=true
            shift
            ;;
    esac
done

# Color definitions
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Print functions
print_header() {
    echo -e "${BLUE}========================================${NC}"
    echo -e "${BLUE}  Primus Lens WandB Exporter${NC}"
    echo -e "${BLUE}  One-Click Installation${NC}"
    echo -e "${BLUE}========================================${NC}"
    echo
}

print_success() {
    echo -e "${GREEN}✓${NC} $1"
}

print_error() {
    echo -e "${RED}✗${NC} $1"
}

print_warning() {
    echo -e "${YELLOW}⚠${NC} $1"
}

print_info() {
    echo -e "${BLUE}ℹ${NC} $1"
}

# Detect Python command
detect_python() {
    if command -v python3 &> /dev/null; then
        PYTHON_CMD="python3"
    elif command -v python &> /dev/null; then
        PYTHON_CMD="python"
    else
        print_error "Python not found, please install Python 3.7+ first"
        exit 1
    fi
    
    # Check Python version
    PYTHON_VERSION=$($PYTHON_CMD --version 2>&1 | awk '{print $2}')
    print_success "Detected Python: $PYTHON_CMD ($PYTHON_VERSION)"
}

# Detect pip command
detect_pip() {
    if command -v pip3 &> /dev/null; then
        PIP_CMD="pip3"
    elif command -v pip &> /dev/null; then
        PIP_CMD="pip"
    else
        print_error "pip not found, please install pip first"
        exit 1
    fi
    
    PIP_VERSION=$($PIP_CMD --version 2>&1 | awk '{print $2}')
    print_success "Detected pip: $PIP_CMD ($PIP_VERSION)"
}

# Install package
install_package() {
    echo
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo "Step 1/4: Install Package"
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    
    # Check if already installed
    if $PYTHON_CMD -c "import primus_lens_wandb_exporter" 2>/dev/null; then
        INSTALLED_VERSION=$($PYTHON_CMD -c "import primus_lens_wandb_exporter; print(primus_lens_wandb_exporter.__version__)" 2>/dev/null)
        print_info "Already installed version: $INSTALLED_VERSION"
        
        # Check latest version from PyPI
        print_info "Checking latest version from PyPI..."
        LATEST_VERSION=$($PIP_CMD index versions primus-lens-wandb-exporter 2>/dev/null | grep "primus-lens-wandb-exporter" | head -1 | awk '{print $2}' | tr -d '()')
        
        if [ -n "$LATEST_VERSION" ]; then
            print_info "Latest version on PyPI: $LATEST_VERSION"
            
            if [ "$INSTALLED_VERSION" = "$LATEST_VERSION" ]; then
                print_success "Already at latest version"
                read -p "Reinstall anyway? [y/N] " -n 1 -r
                echo
                if [[ ! $REPLY =~ ^[Yy]$ ]]; then
                    print_info "Skipping package installation"
                    return 0
                fi
            else
                print_warning "Newer version available: $LATEST_VERSION"
                read -p "Upgrade to latest version? [Y/n] " -n 1 -r
                echo
                if [[ $REPLY =~ ^[Nn]$ ]]; then
                    print_info "Skipping package upgrade"
                    return 0
                fi
            fi
        else
            read -p "Reinstall? [y/N] " -n 1 -r
            echo
            if [[ ! $REPLY =~ ^[Yy]$ ]]; then
                print_info "Skipping package installation"
                return 0
            fi
        fi
    fi
    
    print_info "Installing primus-lens-wandb-exporter (with --upgrade --no-cache-dir)..."
    # Use --upgrade to force update, --no-cache-dir to clear cache, ensure getting latest version
    if $PIP_CMD install --upgrade --no-cache-dir primus-lens-wandb-exporter; then
        VERSION=$($PYTHON_CMD -c "import primus_lens_wandb_exporter; print(primus_lens_wandb_exporter.__version__)" 2>/dev/null)
        print_success "Package installed successfully (v$VERSION)"
        return 0
    else
        print_error "Package installation failed"
        return 1
    fi
}

# Create .pth file
create_pth_file() {
    echo
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo "Step 2/4: Configure Hook (.pth file)"
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    
    # Create .pth file using Python
    PTH_RESULT=$($PYTHON_CMD << 'PYTHON_SCRIPT'
import site
import os
import sys

try:
    # Get site-packages path
    if hasattr(site, 'getsitepackages'):
        site_packages = site.getsitepackages()[0]
    else:
        from distutils.sysconfig import get_python_lib
        site_packages = get_python_lib()
    
    pth_file = os.path.join(site_packages, 'primus_lens_wandb_hook.pth')
    pth_content = 'import primus_lens_wandb_exporter.wandb_hook\n'
    
    # Check if already exists
    if os.path.exists(pth_file):
        with open(pth_file, 'r') as f:
            existing_content = f.read()
        if existing_content.strip() == pth_content.strip():
            print(f"EXISTS|{pth_file}")
            sys.exit(0)
    
    # Try to create
    with open(pth_file, 'w') as f:
        f.write(pth_content)
    
    print(f"SUCCESS|{pth_file}")
    sys.exit(0)

except PermissionError:
    print(f"PERMISSION_ERROR|{pth_file}")
    sys.exit(1)
except Exception as e:
    print(f"ERROR|{e}")
    sys.exit(1)
PYTHON_SCRIPT
)
    
    PTH_STATUS=$(echo "$PTH_RESULT" | cut -d'|' -f1)
    PTH_PATH=$(echo "$PTH_RESULT" | cut -d'|' -f2)
    
    if [ "$PTH_STATUS" = "SUCCESS" ]; then
        print_success ".pth file created successfully"
        print_info "Location: $PTH_PATH"
        return 0
    elif [ "$PTH_STATUS" = "EXISTS" ]; then
        print_success ".pth file already exists"
        print_info "Location: $PTH_PATH"
        return 0
    elif [ "$PTH_STATUS" = "PERMISSION_ERROR" ]; then
        print_warning "Insufficient permissions, sudo required"
        print_info "Location: $PTH_PATH"
        echo
        
        # Try using sudo
        if command -v sudo &> /dev/null; then
            read -p "Create using sudo? [Y/n] " -n 1 -r
            echo
            if [[ ! $REPLY =~ ^[Nn]$ ]]; then
                if sudo $PYTHON_CMD -c "open('$PTH_PATH', 'w').write('import primus_lens_wandb_exporter.wandb_hook\n')"; then
                    print_success ".pth file created successfully (with sudo)"
                    return 0
                else
                    print_error ".pth file creation failed"
                    return 1
                fi
            fi
        fi
        
        # Provide manual installation command
        echo
        print_warning "Please run the following command manually:"
        echo
        echo "  sudo $PYTHON_CMD -c \"open('$PTH_PATH', 'w').write('import primus_lens_wandb_exporter.wandb_hook\\\\n')\""
        echo
        return 1
    else
        print_error ".pth file creation failed: $PTH_PATH"
        return 1
    fi
}

# Verify installation
verify_installation() {
    echo
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo "Step 3/4: Verify Installation"
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    
    VERIFY_RESULT=$($PYTHON_CMD << 'PYTHON_SCRIPT'
import sys
import os

# Check package
try:
    import primus_lens_wandb_exporter
    version = primus_lens_wandb_exporter.__version__
    print(f"PACKAGE|OK|{version}")
except ImportError as e:
    print(f"PACKAGE|FAIL|{e}")
    sys.exit(1)

# Check .pth file
try:
    import site
    if hasattr(site, 'getsitepackages'):
        site_packages = site.getsitepackages()[0]
    else:
        from distutils.sysconfig import get_python_lib
        site_packages = get_python_lib()
    
    pth_file = os.path.join(site_packages, 'primus_lens_wandb_hook.pth')
    
    if os.path.exists(pth_file):
        print(f"PTH|OK|{pth_file}")
    else:
        print(f"PTH|FAIL|{pth_file}")
except Exception as e:
    print(f"PTH|ERROR|{e}")

# Check if wandb_hook is loaded (via .pth)
if 'primus_lens_wandb_exporter.wandb_hook' in sys.modules:
    print("HOOK|LOADED|via .pth")
else:
    print("HOOK|NOT_LOADED|needs restart")
PYTHON_SCRIPT
)
    
    # Parse results
    PACKAGE_STATUS=$(echo "$VERIFY_RESULT" | grep "^PACKAGE" | cut -d'|' -f2)
    PACKAGE_INFO=$(echo "$VERIFY_RESULT" | grep "^PACKAGE" | cut -d'|' -f3)
    PTH_STATUS=$(echo "$VERIFY_RESULT" | grep "^PTH" | cut -d'|' -f2)
    PTH_INFO=$(echo "$VERIFY_RESULT" | grep "^PTH" | cut -d'|' -f3)
    HOOK_STATUS=$(echo "$VERIFY_RESULT" | grep "^HOOK" | cut -d'|' -f2)
    HOOK_INFO=$(echo "$VERIFY_RESULT" | grep "^HOOK" | cut -d'|' -f3)
    
    # Display results
    if [ "$PACKAGE_STATUS" = "OK" ]; then
        print_success "Package installed: v$PACKAGE_INFO"
    else
        print_error "Package check failed: $PACKAGE_INFO"
        return 1
    fi
    
    if [ "$PTH_STATUS" = "OK" ]; then
        print_success ".pth file exists: $PTH_INFO"
    else
        print_error ".pth file check failed: $PTH_INFO"
        return 1
    fi
    
    if [ "$HOOK_STATUS" = "LOADED" ]; then
        print_success "Hook loaded: $HOOK_INFO"
    elif [ "$HOOK_STATUS" = "NOT_LOADED" ]; then
        print_info "Hook not loaded: $HOOK_INFO"
        print_info "This is normal, the hook will be loaded automatically on next Python startup"
    fi
    
    return 0
}

# Detect multi-Python environments (venv vs system Python)
detect_multi_python() {
    echo
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo "Step 4/4: Multi-Python Environment Check"
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"

    MULTI_PYTHON_RESULT=$($PYTHON_CMD << 'PYTHON_SCRIPT'
import sys
import os
import site

venv_prefix = getattr(sys, 'prefix', '')
base_prefix = getattr(sys, 'base_prefix', getattr(sys, 'real_prefix', ''))
in_venv = venv_prefix != base_prefix

if not in_venv:
    print("NO_VENV")
    sys.exit(0)

venv_exe = sys.executable
real_exe = os.path.realpath(venv_exe)
venv_sp = site.getsitepackages()[0] if hasattr(site, 'getsitepackages') else None

# Try to find the system python's site-packages
# The real executable typically lives in /usr/bin/ or similar
sys_python = None
for candidate in [real_exe, '/usr/bin/python3', '/usr/bin/python3.10', '/usr/bin/python3.11', '/usr/bin/python3.12']:
    if os.path.exists(candidate) and candidate != venv_exe:
        sys_python = candidate
        break

if not sys_python:
    print("NO_SYS_PYTHON")
    sys.exit(0)

# Check if system python has the .pth file in its site-packages
import subprocess
try:
    result = subprocess.run(
        [sys_python, '-c', 
         'import site,os; sp=site.getsitepackages(); '
         'found=any(os.path.exists(os.path.join(p,"primus_lens_wandb_hook.pth")) for p in sp); '
         'print("YES" if found else "NO"); '
         '[print(p) for p in sp]'],
        capture_output=True, text=True, timeout=5
    )
    lines = result.stdout.strip().split('\n')
    has_pth = lines[0] == 'YES' if lines else False
    sys_sp = lines[1] if len(lines) > 1 else 'unknown'
except Exception:
    has_pth = False
    sys_sp = 'unknown'

if has_pth:
    print(f"ALREADY_INSTALLED|{sys_python}|{sys_sp}")
else:
    print(f"MISSING|{sys_python}|{sys_sp}|{venv_sp}")
PYTHON_SCRIPT
)

    if [ "$MULTI_PYTHON_RESULT" = "NO_VENV" ]; then
        print_success "Not running in a virtualenv, no multi-Python concerns"
        return 0
    fi

    if [ "$MULTI_PYTHON_RESULT" = "NO_SYS_PYTHON" ]; then
        print_success "No separate system Python detected"
        return 0
    fi

    MP_STATUS=$(echo "$MULTI_PYTHON_RESULT" | cut -d'|' -f1)
    MP_SYS_PYTHON=$(echo "$MULTI_PYTHON_RESULT" | cut -d'|' -f2)
    MP_SYS_SP=$(echo "$MULTI_PYTHON_RESULT" | cut -d'|' -f3)

    if [ "$MP_STATUS" = "ALREADY_INSTALLED" ]; then
        print_success "System Python ($MP_SYS_PYTHON) already has the .pth hook"
        return 0
    fi

    if [ "$MP_STATUS" = "MISSING" ]; then
        MP_VENV_SP=$(echo "$MULTI_PYTHON_RESULT" | cut -d'|' -f4)
        echo
        print_warning "Detected virtualenv environment"
        print_warning "The .pth hook is installed in:     $MP_VENV_SP"
        print_warning "But system Python ($MP_SYS_PYTHON) uses: $MP_SYS_SP"
        echo
        print_warning "If your training framework spawns subprocesses using the system"
        print_warning "Python (e.g., Monarch, torchrun, DeepSpeed), the WandB hook will"
        print_warning "NOT activate in those subprocesses."
        echo

        if [ "$INSTALL_SYSTEM_PYTHON" = true ]; then
            print_info "Installing to system Python (--install-system-python)..."
            if $MP_SYS_PYTHON -m pip install --no-cache-dir primus-lens-wandb-exporter 2>/dev/null; then
                print_success "Installed to system Python: $MP_SYS_PYTHON"
                return 0
            else
                print_error "Failed to install to system Python (permission denied?)"
                print_info "Try: sudo $MP_SYS_PYTHON -m pip install --no-cache-dir primus-lens-wandb-exporter"
                return 1
            fi
        else
            print_info "To fix this, run ONE of the following:"
            echo
            echo "  Option 1: Re-run this installer with the flag:"
            echo "     bash install.sh --install-system-python"
            echo
            echo "  Option 2: Manually install to system Python:"
            echo "     $MP_SYS_PYTHON -m pip install --no-cache-dir primus-lens-wandb-exporter"
            echo
            return 0
        fi
    fi

    return 0
}

# Show usage instructions
show_usage() {
    echo
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo "Installation Successful!"
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo
    print_success "Primus Lens WandB Exporter has been installed and configured"
    echo
    echo "📋 Next Steps:"
    echo
    echo "  1. Set environment variables (optional but recommended):"
    echo
    echo "     export PRIMUS_LENS_WANDB_DEBUG=true"
    echo "     export PRIMUS_LENS_WANDB_API_REPORTING=true"
    echo "     export PRIMUS_LENS_API_BASE_URL=http://your-api-endpoint"
    echo
    echo "  2. Run your training script:"
    echo
    echo "     python3 train.py"
    echo
    echo "  3. Check logs, you should see:"
    echo
    echo "     [Primus Lens WandB] Installing WandB hook..."
    echo "     [Primus Lens WandB] WandB successfully patched!"
    echo "     [Primus Lens WandB] Intercepted wandb.init()"
    echo
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo
    echo "📚 More Information:"
    echo "   GitHub: https://github.com/AMD-AIG-AIMA/Primus"
    echo "   Docs: https://github.com/AMD-AIG-AIMA/Primus/tree/main/Lens"
    echo
}

# Main function
main() {
    print_header
    
    # Detect environment
    detect_python
    detect_pip
    
    # Install package
    if ! install_package; then
        print_error "Installation failed"
        exit 1
    fi
    
    # Create .pth file
    PTH_SUCCESS=0
    if ! create_pth_file; then
        PTH_SUCCESS=1
    fi
    
    # Verify installation
    if ! verify_installation; then
        print_error "Verification failed, but package may already be installed"
        exit 1
    fi

    # Detect multi-Python environments
    detect_multi_python
    
    show_usage
    
    if [ $PTH_SUCCESS -eq 1 ]; then
        exit 1
    else
        exit 0
    fi
}

# Run main function
main


