#
# Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#

echo "begin to install openmpi"
wget -q https://download.open-mpi.org/release/open-mpi/v4.1/openmpi-4.1.8.tar.gz
if [ $? -ne 0 ]; then
  exit 1
fi
tar -xzf openmpi-4.1.8.tar.gz
cd openmpi-4.1.8

./configure \
  --prefix=/opt/openmpi-4.1.8 \
  --enable-mpi1-compatibility \
  --with-platform=optimized \
  --with-libevent=internal > /dev/null 2>&1
if [ $? -ne 0 ]; then
  echo "failed to configure openmpi-4.1.8"
  exit 1
fi

make -j 16 > /dev/null 2>&1
if [ $? -ne 0 ]; then
  echo "failed to make openmpi-4.1.8"
  exit 1
fi

make install > /dev/null 2>&1
if [ $? -ne 0 ]; then
  echo "failed to install openmpi-4.1.8"
  exit 1
fi
echo "install openmpi-4.1.8 successfully"