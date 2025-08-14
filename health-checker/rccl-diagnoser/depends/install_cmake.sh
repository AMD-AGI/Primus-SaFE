#!/bin/bash

#
# Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#

set -e

CMAKE_VERSION="3.25.3"
CMAKE_URL_PREFIX="https://github.com/Kitware/CMake/releases/download/v${CMAKE_VERSION}"
CMAKE_SOURCE_FILE="cmake-${CMAKE_VERSION}.tar.gz"
CMAKE_SOURCE_DIR="cmake-${CMAKE_VERSION}"
CMAKE_INSTALL_PREFIX="/usr/local"

echo "begin to CMake ${CMAKE_VERSION}, install-prefix: ${CMAKE_INSTALL_PREFIX}"

apt-get update
apt-get install -y build-essential libssl-dev > /dev/null

if [ ! -f "${CMAKE_SOURCE_FILE}" ]; then
  wget "${CMAKE_URL_PREFIX}/${CMAKE_SOURCE_FILE}"
fi

if [ ! -d "${CMAKE_SOURCE_DIR}" ]; then
  tar -xzf "${CMAKE_SOURCE_FILE}" > /dev/null
fi

cd "${CMAKE_SOURCE_DIR}"
./configure --prefix="${CMAKE_INSTALL_PREFIX}" > /dev/null
make -j$(nproc) > /dev/null

make install > /dev/null

ldconfig
if command -v cmake &> /dev/null; then
  INSTALLED_VERSION=$(cmake --version | head -n 1)
  echo "CMake has been installed. version: ${INSTALLED_VERSION}, path: $(which cmake)"

  MIN_VERSION="3.2.5"
  if [[ "$(printf '%s\n%s' "$MIN_VERSION" "$(echo $INSTALLED_VERSION | awk '{print $3}')" | sort -V | head -n1)" == "$MIN_VERSION" ]]; then
    echo "install cmake successfully"
  else
    echo "[Warning]: the version($INSTALLED_VERSION) is less than($MIN_VERSION)"
    exit 1
  fi
else
  echo "failed to find cmake"
  exit 1
fi