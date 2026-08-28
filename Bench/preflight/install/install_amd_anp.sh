#
# Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#

set -e

ANP_REPO="https://github.com/rocm/amd-anp.git"
ANP_DIR="amd-anp"
WORKDIR="/opt"
RCCL_HOME="${RCCL_HOME:-/opt/rccl}"
RCCL_BUILD="${RCCL_BUILD:-${RCCL_HOME}/build/release}"
# Neither ANP_VERSION nor LIBIONIC_VERSION is a constant here:
#   ANP_VERSION     is chosen from the RCCL headers -- see "Select the ANP release".
#   LIBIONIC_VERSION defaults to whatever the AINIC repo for this bundle ships.

# Get AINIC_DRIVER_VERSION from environment or extract from AINIC_BUNDLE_PATH
if [ -z "${AINIC_DRIVER_VERSION}" ] && [ -n "${AINIC_BUNDLE_PATH}" ]; then
  # Extract version from filename like ainic_bundle_1.117.5-a-56.tar.gz -> 1.117.5-a-56
  AINIC_BUNDLE_FILENAME=$(basename "${AINIC_BUNDLE_PATH}")
  AINIC_DRIVER_VERSION=$(echo "${AINIC_BUNDLE_FILENAME}" | sed -n 's/ainic_bundle_\(.*\)\.tar\.gz/\1/p')
fi

if [ -z "${AINIC_DRIVER_VERSION}" ]; then
  echo "Error: AINIC_DRIVER_VERSION not specified and could not be extracted from AINIC_BUNDLE_PATH"
  exit 1
fi

# ---------------------------------------------------------------------------
# Select the ANP release that matches the RCCL this image actually built.
#
# The plugin compiles against RCCL's *internal* headers, so a mismatch is a hard
# compile error in src/net_ib.cc, not a runtime surprise. RCCL changed
#     int          ncclFindInterfaces(ifNames, ifAddrs, ifNameMaxSize, maxIfs)
# to
#     ncclResult_t ncclFindInterfaces(ifNames, ifAddrs, ifNameMaxSize, maxIfs, nIfs)
# in rocm-7.1.0, and ANP v1.3.0 is v1.2.0 plus exactly that adaptation and
# nothing else (upstream diff: 3 lines, one file). So the two tags are
# interchangeable in features; only the RCCL they compile against differs:
#     rccl rocm-6.4.3 / 7.0.1 / 7.0.2 -> 4-arg -> ANP v1.2.0
#     rccl rocm-7.1.0 / 7.2.0         -> 5-arg -> ANP v1.3.0
#
# Detect this from the header rather than re-deriving it from ROCM_VERSION: the
# ROCM_VERSION -> RCCL tag mapping lives in pytorch/install_rccl.sh (7.0.3
# deliberately builds rocm-7.1.0, for instance), and a second copy here would
# silently drift out of sync the next time that file gains a version.
RCCL_SOCKET_H=""
for h in "${RCCL_BUILD}/hipify/src/include/socket.h" \
         "${RCCL_HOME}/src/include/socket.h"; do
  if [ -f "$h" ]; then RCCL_SOCKET_H="$h"; break; fi
done
if [ -z "${RCCL_SOCKET_H}" ]; then
  echo "Error: cannot locate RCCL's socket.h under ${RCCL_BUILD} or ${RCCL_HOME}." >&2
  echo "       RCCL must be built before the ANP plugin." >&2
  exit 1
fi
if grep -qE '^[[:space:]]*ncclResult_t[[:space:]]+ncclFindInterfaces' "${RCCL_SOCKET_H}"; then
  ANP_VERSION="v1.3.0"
else
  ANP_VERSION="v1.2.0"
fi
echo "RCCL headers: ${RCCL_SOCKET_H}"
echo "  -> ncclFindInterfaces is the $([ "${ANP_VERSION}" = "v1.3.0" ] && echo 5-arg || echo 4-arg) form, selecting ANP ${ANP_VERSION}"

echo "============== begin to install AMD AINIC Network Plugin (amd-anp) ${ANP_VERSION} =============="
echo "AINIC Driver Version: ${AINIC_DRIVER_VERSION}"

cd ${WORKDIR}

# Install dependencies - libionic-dev, needed to build ANP.
#
# install_ainic_driver.sh runs before this script and hands the AINIC bundle's
# own install.sh the job, which already installs libionic1/libionic-dev: it
# picks the .deb for this image's Ubuntu suite out of the bundle, serves it from
# a local repo at /opt/amd/ainic/deb-repo, and verifies the installed version
# afterwards. When that has happened the matching libionic is on the system by
# construction, and this script must leave it alone.
#
# Reinstalling it from the pensando repo is not merely redundant, it is wrong.
# That repo serves the *same file* for its jammy and noble suites -- both are
# libionic-dev_54.0-149.g3304be71_amd64.deb, md5 8e659561..., the jammy build --
# while the bundle ships a genuinely different noble build, md5 58cf9765...,
# under an identical version string. On a jammy base the two are byte-identical,
# so apt saw nothing to do and this step was a silent no-op; that is the only
# reason it ever worked. On a noble base apt finds the same version from a
# higher-priority source with different contents and stops:
#
#   The following packages will be DOWNGRADED: libionic-dev libionic1
#   E: Packages were downgraded and -y was used without --allow-downgrades.
#
# Forcing that through would overwrite the bundle's noble libionic with the
# jammy build -- exactly the pairing this script is supposed to protect. So the
# remote repo is a fallback for builds that carry no bundle, not the norm.
LIBIONIC_INSTALLED=$(dpkg-query -W -f='${Status}|${Version}' libionic-dev 2>/dev/null \
    | awk -F'|' '$1 == "install ok installed" { print $2 }' || true)

if [ -n "${LIBIONIC_INSTALLED}" ] && [ -z "${LIBIONIC_VERSION:-}" ]; then
  echo "libionic-dev ${LIBIONIC_INSTALLED} is already installed (AINIC bundle); \
not adding the pensando repo."
  SKIP_PENSANDO_REPO=true
else
  SKIP_PENSANDO_REPO=false
fi

if [ "${SKIP_PENSANDO_REPO}" != "true" ]; then

echo "Adding AMD AINIC pensando repository for driver version ${AINIC_DRIVER_VERSION}..."

# The suite must be this image's own Ubuntu release, not a fixed one. libionic
# has to pair with the ionic driver the AINIC bundle installed, and that bundle
# picks its .deb by ${UBUNTU_VERSION} read from /etc/os-release -- so a
# hardcoded suite here only agrees with the bundle by coincidence. This line
# said "jammy" unconditionally, which was right on a 22.04 base and silently
# wrong on 24.04: noble ionic packages from the bundle, jammy libionic from the
# repo. That mismatch surfaces far downstream as
#   "Driver ionic does not support the kernel ABI of N ... IB device ionic_0 not found".
UBUNTU_SUITE="${UBUNTU_SUITE:-$( . /etc/os-release 2>/dev/null && echo "${VERSION_CODENAME}" )}"
case "${UBUNTU_SUITE}" in
  jammy|noble) ;;
  *)
    echo "Error: unsupported Ubuntu suite '${UBUNTU_SUITE}' for the AINIC pensando repo." >&2
    echo "  The AINIC bundle ships packages for jammy and noble only." >&2
    echo "  Override with UBUNTU_SUITE=<jammy|noble> if this is intentional." >&2
    exit 1
    ;;
esac
echo "AINIC pensando repo suite: ${UBUNTU_SUITE}"

# Add repository with trusted=yes to bypass GPG signature verification
# This is consistent with using --allow-unauthenticated for apt-get install
echo "deb [arch=amd64 trusted=yes] https://repo.radeon.com/amdainic/pensando/ubuntu/${AINIC_DRIVER_VERSION} ${UBUNTU_SUITE} main" \
    > /etc/apt/sources.list.d/amdainic-pensando.list

# NOTE on error handling below: `set -e` is in effect, so a plain
# `cmd; if [ $? -ne 0 ]` never reaches the if -- the shell has already exited.
# Both handlers here used to be written that way and were dead code, which is
# why an apt failure in this step produced no message at all. Use `|| { ... }`
# so the handler actually runs, and do not swallow apt's own output: it is the
# only description of what went wrong.
apt-get update || echo "Warning: apt-get update had issues, continuing anyway..."

# libionic must pair with the *host* ionic kernel driver, not merely be recent:
# libibverbs refuses a provider whose declared ABI range excludes the kernel's
# uverbs abi_version, and the failure surfaces far downstream as
#   "Driver ionic does not support the kernel ABI of N ... IB device ionic_0 not found".
#
# Each AINIC release directory in the pensando repo ships exactly one libionic,
# so the bundle version already determines it. LIBIONIC_VERSION used to be a
# separate hardcoded constant, which meant swapping the bundle silently left the
# two disagreeing. Default to the repo's own version; keep the env override for
# the case where a specific build has to be forced.
LIBIONIC_PIN=""
if [ -n "${LIBIONIC_VERSION}" ]; then
  LIBIONIC_PIN="=${LIBIONIC_VERSION}"
  echo "Installing libionic-dev${LIBIONIC_PIN} (pinned via LIBIONIC_VERSION)..."
else
  echo "Installing libionic-dev from AINIC ${AINIC_DRIVER_VERSION}: $(apt-cache policy libionic-dev 2>/dev/null | awk '/Candidate:/{print $2}')"
fi
apt-get install -y --allow-unauthenticated libionic-dev${LIBIONIC_PIN} || {
  echo "Error: Failed to install libionic-dev${LIBIONIC_PIN}." >&2
  echo "--- apt-cache policy (what apt actually sees) ---" >&2
  apt-cache policy libionic-dev libionic1 libibverbs1 >&2 || true
  exit 1
}

fi  # SKIP_PENSANDO_REPO

echo "libionic installed: $(dpkg-query -W -f='${Version}' libionic1 2>/dev/null || echo unknown)"

# ANP builds against libionic's headers, so their absence must stop the build
# here rather than surface as a compile error a hundred lines down.
if ! dpkg-query -W -f='${Status}' libionic-dev 2>/dev/null | grep -q "install ok installed"; then
  echo "Error: libionic-dev is not installed; ANP cannot be built." >&2
  apt-cache policy libionic-dev libionic1 libibverbs1 >&2 || true
  exit 1
fi

# Clone AMD ANP repository (retry on transient network errors)
echo "Cloning AMD ANP repository..."
rm -rf ${ANP_DIR}
git config --global http.postBuffer 524288000
for i in 1 2 3 4 5; do
  if git clone ${ANP_REPO} && cd ${ANP_DIR} && git checkout -q tags/${ANP_VERSION}; then
    break
  fi
  echo "Attempt $i failed, retrying in 15s..." >&2
  cd ${WORKDIR}
  rm -rf ${ANP_DIR}
  sleep 15
done
if [ ! -d "${WORKDIR}/${ANP_DIR}" ]; then
  echo "Error: Failed to clone AMD ANP repository after 5 attempts" >&2
  exit 1
fi

cd ${WORKDIR}/${ANP_DIR}

# Modify Makefile for GPU architecture support
if [ -z "${GPU_ARCHS}" ]; then
  echo "Warning: GPU_ARCHS not set, defaulting to gfx950"
  GPU_ARCHS="gfx950"
fi

echo "Modifying Makefile for GPU architectures: ${GPU_ARCHS}..."
# Build CFLAGS line with all specified architectures
ARCH_FLAGS=""
for arch in ${GPU_ARCHS}; do
  ARCH_FLAGS="${ARCH_FLAGS} --offload-arch=${arch}"
done
sed -i "5a CFLAGS +=${ARCH_FLAGS}" ./Makefile
if [ $? -ne 0 ]; then
  echo "Error: Failed to modify Makefile"
  exit 1
fi

# Build
echo "Building AMD ANP driver..."
export RCCL_HOME
# RCCL_BUILD points to where RCCL is installed (with lib/ and include/ subdirectories)
if ! make -j 16 MPI_INCLUDE=/opt/mpich/include/ \
           MPI_LIB_PATH=/opt/mpich/lib/ \
           ROCM_PATH=/opt/rocm \
           RCCL_HOME=/opt/rccl; then
  echo "Error: Failed to build AMD ANP driver."
  exit 1
fi

# Name the plugin every way RCCL might ask for it.
#
# With NCCL_NET_PLUGIN=<name> set -- config.sh sets it to "anp" whenever
# ENABLE_AINIC=true -- RCCL does not look for librccl-net.so at all. It
# interpolates the name and opens librccl-net-<name>.so. This block used to
# create librccl-anp.so, librccl-net.so and libnccl-net.so, none of which is
# that filename, so the plugin was never loaded:
#
#   NET/Plugin: Could not find: librccl-net-anp.so. Using internal net plugin.
#   Using network IB
#
# and every AINIC run silently benchmarked RCCL's built-in IB transport
# instead, with all the ANP tuning in config.sh inert.
ANP_BUILD_DIR="${WORKDIR}/${ANP_DIR}/build"
cd "${ANP_BUILD_DIR}"

# Whichever of these the build produced is the real library; the rest become
# symlinks to it.
ANP_LIB=""
for candidate in librccl-net.so librccl-anp.so libnccl-net.so; do
  if [ -f "${candidate}" ] && [ ! -L "${candidate}" ]; then
    ANP_LIB="${candidate}"
    break
  fi
done
if [ -z "${ANP_LIB}" ]; then
  echo "Error: the ANP build produced no plugin library in ${ANP_BUILD_DIR}." >&2
  ls -la "${ANP_BUILD_DIR}" >&2 || true
  exit 1
fi
echo "ANP plugin library: ${ANP_LIB}"

# librccl-net-anp.so / libnccl-net-anp.so are what NCCL_NET_PLUGIN=anp resolves
# to; the unsuffixed names are what an unset NCCL_NET_PLUGIN falls back to.
for alias in librccl-net.so librccl-anp.so libnccl-net.so \
             librccl-net-anp.so libnccl-net-anp.so; do
  if [ "${alias}" = "${ANP_LIB}" ]; then
    continue
  fi
  if [ -e "${alias}" ]; then
    continue
  fi
  echo "Creating symlink: ${alias} -> ${ANP_LIB}"
  ln -sf "${ANP_LIB}" "${alias}"
done

# The plugin loading is only observable at runtime, so make the artefacts
# visible at build time instead of discovering the gap in a benchmark result.
echo "ANP plugin libraries in ${ANP_BUILD_DIR}:"
ls -la librccl-*.so libnccl-*.so 2>/dev/null || true

echo "============== install  AMD AINIC Network Plugin (amd-anp) ${ANP_VERSION} successfully =============="
