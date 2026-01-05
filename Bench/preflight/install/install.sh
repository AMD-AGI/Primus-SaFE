#
# Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#

echo "============== begin to install preflight components =============="
set -e
set -o pipefail

# Change to script directory
cd "$(dirname "${BASH_SOURCE[0]}")" || exit 1

# Check if ROCM_VERSION is set
if [ -z "${ROCM_VERSION}" ]; then
  echo "Error: ROCM_VERSION environment variable is not set"
  exit 1
fi
export ROCM_VERSION=${ROCM_VERSION}
export GPU_ARCHS=${GPU_ARCHS}
echo "ROCM_VERSION: ${ROCM_VERSION}"
echo "GPU_ARCHS: ${GPU_ARCHS}"

# Function to run a script and check result
run_script() {
  local script=$1
  echo "----------------------------------------"
  echo "Executing $script..."
  if ! bash "$script"; then
    echo "Error: Failed to run $script"
    exit 1
  fi
  echo "$script completed successfully"
}

# Common scripts to run
COMMON_SCRIPTS=(
    "install_linux_tools.sh"
    "install_ucx.sh"
    "install_open_mpi.sh"
    "install_mpich.sh"
    "install_roc_blas.sh"
    "install_rocm_validation.sh"
    "install_transfer_bench.sh"
    "install_rccl_test.sh"
    "install_rdma_test.sh"
)

for script in "${COMMON_SCRIPTS[@]}"; do
  run_script "$script"
done

# Driver installation based on AINIC_BUNDLE_PATH
if [ -z "${AINIC_BUNDLE_PATH}" ]; then
  echo "AINIC_BUNDLE_PATH not set, installing bnxt driver..."
  run_script "install_bnxt_driver.sh"
else
  echo "AINIC_BUNDLE_PATH set, installing AINIC driver and AMD ANP..."
  AINIC_SCRIPTS=(
    "install_ainic_driver.sh"
    "install_amd_anp.sh"
  )
  for script in "${AINIC_SCRIPTS[@]}"; do
    run_script "$script"
  done
fi

echo "============== install preflight components successfully =============="