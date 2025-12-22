#
# Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#

package="libbnxt_re-234.0.154.0"
echo "==============  begin to install $package =============="

full_package="$package.tar.gz"
if [ ! -f "${full_package}" ]; then
  echo "Error: ${full_package} not found in current directory"
  exit 1
fi

apt update > /dev/null

# Check if we're running in a container environment
if [ -f /.dockerenv ] || [ -n "${DOCKER_CONTAINER:-}" ]; then
  echo "Running in container environment, kernel headers may not be needed"
  SKIP_KERNEL_HEADERS=true
else
  SKIP_KERNEL_HEADERS=false
fi

if [ "$SKIP_KERNEL_HEADERS" = "false" ]; then
  # Try to install kernel headers (may be needed for some driver features)
  KERNEL_VERSION=$(uname -r)
  echo "Trying to install linux-headers for kernel: $KERNEL_VERSION"
  
  if apt-cache show linux-headers-"$KERNEL_VERSION" > /dev/null 2>&1; then
    echo "Installing exact kernel headers: linux-headers-$KERNEL_VERSION"
    apt -y install linux-headers-"$KERNEL_VERSION"
  else
    echo "Exact kernel headers not found, trying generic headers..."
    # Try to install generic headers as fallback
    apt -y install linux-headers-generic || {
      echo "Warning: Could not install linux-headers."
      echo "Note: BNXT userspace library should still work without kernel headers."
    }
  fi
else
  echo "Skipping kernel headers installation in container environment"
fi

tar xzf "${full_package}" -C /tmp/ && \
mv /usr/lib/x86_64-linux-gnu/libibverbs/libbnxt_re-rdmav34.so /usr/lib/x86_64-linux-gnu/libibverbs/libbnxt_re-rdmav34.so.inbox && \
cd /tmp/$package/ && sh ./autogen.sh && ./configure && \
make clean && make all && make install && \
echo '/usr/local/lib' > /etc/ld.so.conf.d/libbnxt_re.conf && \
ldconfig && \
cp -f /tmp/$package/bnxt_re.driver /etc/libibverbs.d/

if [ $? -ne 0 ]; then
  exit 1
fi
rm -rf /tmp/$package

echo "============== install $package successfully =============="