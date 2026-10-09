#!/bin/sh

#
# Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#

# Function to check and install packages if not already installed
install_if_not_exists() {
  missing_packages=""

  # Check each package if it's installed
  for package in "$@"; do
    if ! dpkg -l | grep -q "^ii  $package "; then
      missing_packages="$missing_packages $package"
    fi
  done

  # Install only missing packages
  if [ -n "$missing_packages" ]; then
    echo "Installing missing packages:$missing_packages (uid=$(id -u) euid=$(id -u) user=$(id -un 2>/dev/null || true))"

    if command -v apt-get >/dev/null 2>&1; then
      echo "=== apt-get update ==="
      if ! apt-get update; then
        echo "Error: apt-get update failed"
        return 1
      fi
      echo "=== apt-get install -y$missing_packages ==="
      if ! apt-get install -y $missing_packages; then
        echo "Error: apt-get install failed for:$missing_packages"
        return 1
      fi
    elif command -v yum >/dev/null 2>&1; then
      echo "=== yum install -y$missing_packages ==="
      if ! yum install -y $missing_packages; then
        echo "Error: yum install failed for:$missing_packages"
        return 1
      fi
    else
      echo "Unsupported package manager. Neither apt-get nor yum found."
      return 1
    fi
  fi
}
