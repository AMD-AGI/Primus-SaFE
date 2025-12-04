#Requires -Version 5.1
<#
.SYNOPSIS
    Primus Lens WandB Exporter - PyPI Publishing Script (PowerShell)

.DESCRIPTION
    Automated publishing workflow to PyPI or TestPyPI

.PARAMETER Test
    Upload to TestPyPI instead of production PyPI

.PARAMETER SkipTests
    Skip the testing phase

.PARAMETER SkipBuild
    Skip the build phase (reuse existing dist\)

.EXAMPLE
    $env:PYPI_TOKEN = "pypi-AgEI..."
    .\publish.ps1

.EXAMPLE
    $env:TESTPYPI_TOKEN = "pypi-AgEI..."
    .\publish.ps1 -Test

.EXAMPLE
    .\publish.ps1 -SkipTests
#>

[CmdletBinding()]
param(
    [switch]$Test,
    [switch]$SkipTests,
    [switch]$SkipBuild,
    [switch]$Help
)

# Error handling
$ErrorActionPreference = "Stop"
$ProgressPreference = "SilentlyContinue"

# Color functions
function Write-Info($message) {
    Write-Host "[INFO] " -ForegroundColor Blue -NoNewline
    Write-Host $message
}

function Write-Success($message) {
    Write-Host "[SUCCESS] " -ForegroundColor Green -NoNewline
    Write-Host $message
}

function Write-Warn($message) {
    Write-Host "[WARNING] " -ForegroundColor Yellow -NoNewline
    Write-Host $message
}

function Write-Err($message) {
    Write-Host "[ERROR] " -ForegroundColor Red -NoNewline
    Write-Host $message
}

# Show help
function Show-Help {
    Write-Host @"

Primus Lens WandB Exporter - PyPI Publishing Script

Usage:
    .\publish.ps1 [-Test] [-SkipTests] [-SkipBuild]

Environment Variables:
    `$env:PYPI_TOKEN          PyPI API Token (required)
    `$env:TESTPYPI_TOKEN      TestPyPI API Token (required with -Test)

Parameters:
    -Test              Upload to TestPyPI for testing
    -SkipTests         Skip the testing phase
    -SkipBuild         Skip the build phase (reuse dist\)
    -Help              Show this help message

Examples:
    # Publish to production PyPI
    `$env:PYPI_TOKEN = "pypi-AgEIcHlwaS5vcmcC..."
    .\publish.ps1

    # Test publish to TestPyPI first
    `$env:TESTPYPI_TOKEN = "pypi-AgEI..."
    .\publish.ps1 -Test

    # Skip tests and publish directly
    .\publish.ps1 -SkipTests

Get PyPI Token:
    1. Visit https://pypi.org/manage/account/token/
    2. Create a new API token
    3. Copy the token and set as environment variable

"@
}

if ($Help) {
    Show-Help
    exit 0
}

# Check environment variables
if ($Test) {
    if (-not $env:TESTPYPI_TOKEN) {
        Write-Err "TESTPYPI_TOKEN environment variable not set"
        Write-Host 'Please run: $env:TESTPYPI_TOKEN = "your-token-here"'
        exit 1
    }
    $PyPIToken = $env:TESTPYPI_TOKEN
    $Repository = "testpypi"
    $RepositoryUrl = "https://test.pypi.org/legacy/"
} else {
    if (-not $env:PYPI_TOKEN) {
        Write-Err "PYPI_TOKEN environment variable not set"
        Write-Host 'Please run: $env:PYPI_TOKEN = "your-token-here"'
        exit 1
    }
    $PyPIToken = $env:PYPI_TOKEN
    $Repository = "pypi"
    $RepositoryUrl = "https://upload.pypi.org/legacy/"
}

# Get script directory
$ScriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
Set-Location $ScriptDir

Write-Host ""
Write-Host "================================================================"
Write-Host "    Primus Lens WandB Exporter - PyPI Publishing Tool          "
Write-Host "================================================================"
Write-Host ""

Write-Info "Working directory: $ScriptDir"
Write-Info "Target repository: $Repository"
Write-Host ""

# Step 1: Check required tools
Write-Info "Step 1/6: Checking required tools..."

# Check Python
$PythonCmd = $null
$PythonVersions = @("python", "python3", "py")
foreach ($cmd in $PythonVersions) {
    try {
        $version = & $cmd --version 2>&1
        if ($version -match "Python (\d+\.\d+)") {
            $PythonCmd = $cmd
            Write-Success "Python: $version"
            break
        }
    } catch {
        continue
    }
}

if (-not $PythonCmd) {
    Write-Err "Python not installed or not in PATH"
    exit 1
}

# Check or create virtual environment
if (-not (Test-Path ".venv")) {
    Write-Warn "Virtual environment does not exist, creating..."
    & $PythonCmd -m venv .venv
}

# Activate virtual environment
$VenvActivate = Join-Path $ScriptDir ".venv\Scripts\Activate.ps1"
if (Test-Path $VenvActivate) {
    Write-Info "Activating virtual environment..."
    & $VenvActivate
} else {
    Write-Err "Virtual environment activation script not found: $VenvActivate"
    exit 1
}

# Install build tools
Write-Info "Installing build tools..."
& python -m pip install --upgrade pip --quiet
& pip install --upgrade build twine --quiet

Write-Success "Tools check completed"
Write-Host ""

# Step 2: Run tests
if (-not $SkipTests) {
    Write-Info "Step 2/6: Running test suite..."
    
    # Set test environment variables
    $env:PRIMUS_LENS_WANDB_HOOK = "true"
    $env:WANDB_MODE = "offline"
    $env:WANDB_SILENT = "true"
    
    try {
        & python test_real_scenario.py --scenario basic
        if ($LASTEXITCODE -eq 0) {
            Write-Success "Basic tests passed"
        } else {
            Write-Err "Tests failed"
            Write-Host ""
            $continue = Read-Host "Continue publishing? (y/N)"
            if ($continue -ne "y" -and $continue -ne "Y") {
                Write-Info "Publishing cancelled"
                exit 1
            }
        }
    } catch {
        Write-Err "Test execution failed: $_"
        $continue = Read-Host "Continue publishing? (y/N)"
        if ($continue -ne "y" -and $continue -ne "Y") {
            Write-Info "Publishing cancelled"
            exit 1
        }
    }
    Write-Host ""
} else {
    Write-Warn "Step 2/6: Skipping tests"
    Write-Host ""
}

# Step 3: Clean old build files
if (-not $SkipBuild) {
    Write-Info "Step 3/6: Cleaning old build files..."
    
    $CleanupDirs = @("build", "dist")
    foreach ($dir in $CleanupDirs) {
        if (Test-Path $dir) {
            Remove-Item -Path $dir -Recurse -Force
        }
    }
    
    Get-ChildItem -Path . -Filter "*.egg-info" -Recurse -Directory | Remove-Item -Recurse -Force
    Get-ChildItem -Path "src" -Filter "*.egg-info" -Recurse -Directory -ErrorAction SilentlyContinue | Remove-Item -Recurse -Force
    
    Write-Success "Cleanup completed"
    Write-Host ""
} else {
    Write-Warn "Step 3/6: Skipping cleanup (keeping existing build)"
    Write-Host ""
}

# Step 4: Build package
if (-not $SkipBuild) {
    Write-Info "Step 4/6: Building package..."
    
    try {
        & python -m build
        if ($LASTEXITCODE -eq 0) {
            Write-Success "Package built successfully"
            Write-Host ""
            Write-Info "Build artifacts:"
            Get-ChildItem dist\ | Select-Object Name, Length, LastWriteTime | Format-Table -AutoSize
        } else {
            Write-Err "Package build failed"
            exit 1
        }
    } catch {
        Write-Err "Package build failed: $_"
        exit 1
    }
    Write-Host ""
} else {
    Write-Warn "Step 4/6: Skipping build"
    Write-Host ""
}

# Step 5: Check package
Write-Info "Step 5/6: Checking package integrity..."

try {
    & twine check dist\*
    if ($LASTEXITCODE -eq 0) {
        Write-Success "Package check passed"
    } else {
        Write-Err "Package check failed"
        exit 1
    }
} catch {
    Write-Err "Package check failed: $_"
    exit 1
}
Write-Host ""

# Step 6: Upload to PyPI
Write-Info "Step 6/6: Uploading to $Repository..."
Write-Host ""

if ($Test) {
    Write-Warn "This is a TEST upload to TestPyPI"
    Write-Warn "Install test package: pip install --index-url https://test.pypi.org/simple/ primus-lens-wandb-exporter"
} else {
    Write-Warn "This is a PRODUCTION upload to PyPI. Please confirm!"
}
Write-Host ""

$confirm = Read-Host "Confirm upload? (y/N)"
if ($confirm -ne "y" -and $confirm -ne "Y") {
    Write-Info "Upload cancelled"
    exit 0
}

# Set twine environment variables
$env:TWINE_USERNAME = "__token__"
$env:TWINE_PASSWORD = $PyPIToken

try {
    if ($Test) {
        & twine upload --repository-url $RepositoryUrl dist\*
    } else {
        & twine upload dist\*
    }
    
    if ($LASTEXITCODE -eq 0) {
        Write-Host ""
        Write-Success "Upload successful!"
        Write-Host ""
        Write-Host "================================================================"
        Write-Host "                    Publishing Successful!                      "
        Write-Host "================================================================"
        Write-Host ""
        
        if ($Test) {
            Write-Host "Test installation command:"
            Write-Host "  pip install --index-url https://test.pypi.org/simple/ primus-lens-wandb-exporter"
        } else {
            Write-Host "Installation command:"
            Write-Host "  pip install primus-lens-wandb-exporter"
            Write-Host ""
            Write-Host "Package page:"
            Write-Host "  https://pypi.org/project/primus-lens-wandb-exporter/"
        }
        Write-Host ""
    } else {
        Write-Err "Upload failed"
        exit 1
    }
} catch {
    Write-Err "Upload failed: $_"
    exit 1
} finally {
    # Clean up environment variables
    Remove-Item Env:\TWINE_USERNAME -ErrorAction SilentlyContinue
    Remove-Item Env:\TWINE_PASSWORD -ErrorAction SilentlyContinue
}

Write-Info "Release process completed"
