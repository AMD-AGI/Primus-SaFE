#
# Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#

echo "begin to install openmpi"
wget https://download.open-mpi.org/release/open-mpi/v4.1/openmpi-4.1.8.tar.gz > /dev/null 2>&1
if [ $? -ne 0 ]; then
  exit 1
fi
tar -xzf openmpi-4.1.8.tar.gz
cd openmpi-4.1.8

./configure \
  --prefix=/opt/openmpi-4.1.8 \
  --enable-mpi1-compatibility \
  --with-platform=optimized \
  --enable-ipv6 \
  --with-libevent=internal > /dev/null 2>&1
if [ $? -ne 0 ]; then
  exit 1
fi

make install > /dev/null
if [ $? -ne 0 ]; then
  exit 1
fi