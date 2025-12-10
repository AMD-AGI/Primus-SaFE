#
# Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#

set -euo pipefail
package="libbnxt_re-234.0.154.0"
if [ -f "/usr/local/lib/$package.so" ]; then
  exit 0
fi

echo "==============  begin to install $package =============="

full_package="$package.tar.gz"
if [ ! -f "${full_package}" ]; then
  exit 1
fi

apt update > /dev/null

# Try to install exact kernel version headers first
KERNEL_VERSION=$(uname -r)
echo "Trying to install linux-headers for kernel: $KERNEL_VERSION"

if apt-cache show linux-headers-"$KERNEL_VERSION" > /dev/null 2>&1; then
  echo "Installing exact kernel headers: linux-headers-$KERNEL_VERSION"
  apt -y install linux-headers-"$KERNEL_VERSION"
else
  echo "Exact kernel headers not found, trying generic headers..."
  # Try to install generic headers as fallback
  apt -y install linux-headers-generic || {
    echo "Warning: Could not install linux-headers. Some features may not work."
    echo "Continuing without kernel headers..."
    # Don't exit with error since this might not be critical in container
  }
fi

tar xzf "${full_package}" -C /tmp/
if [ -f /usr/lib/x86_64-linux-gnu/libibverbs/libbnxt_re-rdmav34.so ]; then
  mv /usr/lib/x86_64-linux-gnu/libibverbs/libbnxt_re-rdmav34.so /usr/lib/x86_64-linux-gnu/libibverbs/libbnxt_re-rdmav34.so.inbox
fi

cd /tmp/$package/ && sh ./autogen.sh && ./configure && \
make clean && make all && make install && \
echo '/usr/local/lib' > /etc/ld.so.conf.d/libbnxt_re.conf && \
ldconfig && cp -f /tmp/$package/bnxt_re.driver /etc/libibverbs.d/

if [ $? -ne 0 ]; then
  exit 1
fi
rm -rf /tmp/$package

echo "============== install $package successfully =============="