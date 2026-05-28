#!/bin/sh

#
# Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#

# ---------------------------------------------------------------------------
# Inputs: at least one of the two env vars below must be supplied.
#   AINIC_DRIVER_VERSION       e.g. 1.117.5-a-56 (check via `ibv_devinfo` on host).
#                              When set alone, the tarball is auto-discovered
#                              under /shared-data/drivers/.
#   PATH_TO_AINIC_TAR_PACKAGE  Absolute path to an AINIC bundle tarball.
#                              When set alone, AINIC_DRIVER_VERSION is derived
#                              from the filename (fixed layout
#                              `ainic_bundle_<version>.tar.gz`).
# When both are supplied the explicit AINIC_DRIVER_VERSION wins; the filename
# is not re-parsed (caller is trusted).
# When neither is supplied the install step is skipped (exit 0).
# ---------------------------------------------------------------------------

if [ -n "${PATH_TO_AINIC_TAR_PACKAGE}" ]; then
  if [ ! -f "${PATH_TO_AINIC_TAR_PACKAGE}" ]; then
    echo "Error: PATH_TO_AINIC_TAR_PACKAGE=${PATH_TO_AINIC_TAR_PACKAGE} is set but file does not exist."
    exit 1
  fi
  if [ -z "${AINIC_DRIVER_VERSION}" ]; then
    # Derive version from the tarball filename. Rule: strip the `.tar.gz`
    # suffix, then take everything starting from the first `-`, `_` or `.`
    # that is immediately followed by a digit -- that digit marks the start
    # of the version string. Tolerates any prefix layout, e.g.:
    #   ainic_bundle_1.117.5-a-56.tar.gz  -> 1.117.5-a-56
    #   ainic-release-2.0.5_b_3.tar.gz    -> 2.0.5_b_3
    #   foo.bar.1.0.tar.gz                -> 1.0
    _ainic_basename=$(basename "${PATH_TO_AINIC_TAR_PACKAGE}")
    case "${_ainic_basename}" in
      *.tar.gz) _ainic_stem="${_ainic_basename%.tar.gz}" ;;
      *)
        echo "Error: tarball filename '${_ainic_basename}' does not end with .tar.gz."
        exit 1
        ;;
    esac
    AINIC_DRIVER_VERSION=$(printf '%s\n' "${_ainic_stem}" \
      | awk 'match($0, /[-_.][0-9]/) { print substr($0, RSTART + 1) }')
    if [ -z "${AINIC_DRIVER_VERSION}" ]; then
      echo "Error: cannot derive AINIC_DRIVER_VERSION from filename '${_ainic_basename}'."
      echo "       Expected one of '-', '_' or '.' followed by a digit somewhere in the name,"
      echo "       e.g. 'ainic_bundle_1.2.3.tar.gz'. Or set AINIC_DRIVER_VERSION explicitly."
      exit 1
    fi
    echo "Derived AINIC_DRIVER_VERSION=${AINIC_DRIVER_VERSION} from tarball filename '${_ainic_basename}'."
    unset _ainic_basename _ainic_stem
  fi
fi

echo "============== begin to install AMD AINIC (version: ${AINIC_DRIVER_VERSION}) =============="
set -e

# ---------------------------------------------------------------------------
# Version mapping: AINIC_DRIVER_VERSION -> (AMD_ANP_VERSION, LIBIONIC_VERSION)
# ROCM_VERSION is read from /opt/rocm/.info/version
# ---------------------------------------------------------------------------
set_versions_from_driver() {
  _driver_version="$1"
  # Patterns are shell globs, matched as prefix: any driver version whose
  # string starts with the listed prefix is mapped to the same ANP / libionic
  # pair. Loose matching is intentional so vendor-suffixed builds (e.g.
  # `1.117.5-a-56`, `1.117.5-b-1`) share one mapping; tighten only if a
  # specific suffix needs a different pair.
  case "${_driver_version}" in
    1.117.5*)
      AMD_ANP_VERSION="v1.3.0"
      LIBIONIC_VERSION="54.0-184"
      ;;
    *)
      echo "Error: Unknown AINIC driver version ${_driver_version}."
      echo "Please add version mapping in build_ainic.sh"
      exit 1
      ;;
  esac
  unset _driver_version
}

set_versions_from_driver "${AINIC_DRIVER_VERSION}"

# Get ROCm version from container
ROCM_VERSION=$(cat /opt/rocm/.info/version 2>/dev/null | tr -d '[:space:]' | cut -d'-' -f1)
if [ -z "${ROCM_VERSION}" ]; then
  echo "Error: ROCm not found. Cannot read /opt/rocm/.info/version"
  exit 1
fi

echo "Mapped AINIC driver version ${AINIC_DRIVER_VERSION} -> ANP: ${AMD_ANP_VERSION}, ROCM: ${ROCM_VERSION}, LIBIONIC: ${LIBIONIC_VERSION}"

# When PATH_TO_AINIC_TAR_PACKAGE was not pre-set above, auto-discover a
# matching tarball under /shared-data/drivers/ using AINIC_DRIVER_VERSION.
if [ -z "${PATH_TO_AINIC_TAR_PACKAGE}" ]; then
  DRIVERS_DIR="/shared-data/drivers"
  if [ ! -d "${DRIVERS_DIR}" ]; then
    echo "Error: Drivers directory ${DRIVERS_DIR} does not exist."
    exit 1
  fi

  PATH_TO_AINIC_TAR_PACKAGE=$(ls ${DRIVERS_DIR}/*${AINIC_DRIVER_VERSION}*.tar.gz 2>/dev/null | head -n 1)
  if [ -z "${PATH_TO_AINIC_TAR_PACKAGE}" ] || [ ! -f "${PATH_TO_AINIC_TAR_PACKAGE}" ]; then
    echo "Error: No AINIC driver tarball found matching version ${AINIC_DRIVER_VERSION} in ${DRIVERS_DIR}"
    echo "Available files:"
    ls -la ${DRIVERS_DIR}/ 2>/dev/null || echo "  (directory empty or not accessible)"
    exit 1
  fi
  echo "Found AINIC driver tarball: ${PATH_TO_AINIC_TAR_PACKAGE}"
else
  echo "Using AINIC driver tarball from PATH_TO_AINIC_TAR_PACKAGE env: ${PATH_TO_AINIC_TAR_PACKAGE}"
fi

. /shared-data/utils.sh
_start=$(date +%s)
echo "Installing dependencies ..."
install_if_not_exists dpkg-dev kmod xz-utils libfmt-dev libboost-all-dev libibverbs-dev ibverbs-utils infiniband-diags jq initramfs-tools
_end=$(date +%s)
echo "Dependencies installed in $((_end - _start)) seconds"

# Call build_anp.sh with required parameters
export AMD_ANP_VERSION=${AMD_ANP_VERSION}
export ROCM_VERSION=${ROCM_VERSION}
export AINIC_DRIVER_VERSION=${AINIC_DRIVER_VERSION}
export LIBIONIC_VERSION=${LIBIONIC_VERSION}
_start=$(date +%s)
/bin/sh /shared-data/build_anp.sh
_end=$(date +%s)
echo "ANP install.sh completed in $((_end - _start)) seconds"

WORKDIR="/opt"
cd ${WORKDIR}

# Extract tarball name and directory name from full path
AINIC_TARBALL=$(basename "${PATH_TO_AINIC_TAR_PACKAGE}")
AINIC_DIR="${AINIC_TARBALL%.tar.gz}"

cp ${PATH_TO_AINIC_TAR_PACKAGE} ${WORKDIR}/
if [ $? -ne 0 ]; then
  echo "Error: Failed to copy AINIC bundle"
  exit 1
fi

# Extract AINIC bundle
tar zxf ${AINIC_TARBALL}
if [ $? -ne 0 ]; then
  echo "Error: Failed to extract ${AINIC_TARBALL}"
  exit 1
fi
rm -f ${AINIC_TARBALL}

# Extract host software package
cd ${AINIC_DIR}
if [ ! -f "host_sw_pkg.tar.gz" ]; then
  echo "Error: host_sw_pkg.tar.gz not found in ${AINIC_DIR}"
  exit 1
fi
tar zxf host_sw_pkg.tar.gz
if [ $? -ne 0 ]; then
  echo "Error: Failed to extract host_sw_pkg.tar.gz"
  exit 1
fi

# Run installation script
cd host_sw_pkg
if [ ! -f "./install.sh" ]; then
  echo "Error: install.sh not found in host_sw_pkg"
  exit 1
fi
_start=$(date +%s)
./install.sh --domain=user -y
_exit=$?
_end=$(date +%s)
echo "AINIC driver install.sh completed in $((_end - _start)) seconds"
if [ $_exit -ne 0 ]; then
  echo "Error: Failed to install AINIC driver."
  exit 1
fi

# Cleanup
echo "Cleaning up temporary files..."
cd ${WORKDIR}
rm -rf ${AINIC_DIR}

echo "============== install AMD AINIC ${AINIC_DRIVER_VERSION} successfully =============="
