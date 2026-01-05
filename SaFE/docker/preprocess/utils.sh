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
    echo "Installing missing packages:$missing_packages"

    # Detect OS type and use appropriate package manager
    if command -v apt >/dev/null 2>&1; then
      # Ubuntu/Debian system
      apt update >/dev/null
      apt-get install -y $missing_packages >/dev/null
    elif command -v yum >/dev/null 2>&1; then
      # CentOS/RHEL system
      yum install -y $missing_packages >/dev/null
    else
      echo "Unsupported package manager. Neither apt nor yum found."
      exit 1
    fi
  fi
}
