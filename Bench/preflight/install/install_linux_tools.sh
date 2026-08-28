#!/bin/bash
#
# Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#

echo "============== begin to install linux-tools =============="

if [ "${OS_NAME}" = "oci" ]; then
  echo "Skipping linux-tools installation (OS_NAME=oci)"
  echo "============== linux-tools skipped (OCI) =============="
  exit 0
fi

# linux-tools is `perf` and friends: useful when diagnosing a node, but nothing
# in PrimusBench depends on it at run time. This step used to `exit 1` on
# failure, and install.sh turns a non-zero child into `exit 1` for the whole
# run, so a missing perf package aborted the entire image build.
#
# It fails for a reason that has nothing to do with the image, too: the package
# name is pinned to `uname -r`, which inside a build container is the *build
# host's* kernel, not the kernel of the nodes the image will run on. Build on a
# machine whose kernel has no linux-tools package in the archive and the build
# dies -- which is why OS_NAME=oci above exists as an escape hatch, and why the
# shipped image was tagged "oci" while being plain Ubuntu.
#
# So: warn and carry on. A genuinely required dependency belongs in a step that
# is allowed to fail the build; this one is not that.
apt-get update >/dev/null 2>&1 || true

KERNEL_VERSION=$(uname -r)
linux_tools="linux-tools-${KERNEL_VERSION} linux-tools-common"
echo "Trying to install $linux_tools (build host kernel: ${KERNEL_VERSION})..."

if apt-get install -y $linux_tools; then
  echo "============== $linux_tools installation completed =============="
else
  echo "Warning: could not install $linux_tools; continuing without perf." >&2
  echo "  This is a diagnostic tool only. The package name follows the build" >&2
  echo "  host's kernel, so it is expected to be unavailable when building on" >&2
  echo "  a machine whose kernel differs from the target nodes'." >&2
  echo "============== linux-tools skipped (not available) =============="
fi

exit 0
