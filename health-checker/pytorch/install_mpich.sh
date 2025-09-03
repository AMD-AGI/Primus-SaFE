#
# Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#

echo "============== begin to install mpich-4.3.1 =============="
set -e
cd /tmp && wget https://www.mpich.org/static/downloads/4.3.1/mpich-4.3.1.tar.gz > /dev/null
if [ $? -ne 0 ]; then
  exit 1
fi
mkdir -p mpich && tar -zxf mpich-4.3.1.tar.gz -C mpich --strip-components=1 && cd mpich
if [ $? -ne 0 ]; then
  exit 1
fi

if [ ! -d "build" ]; then
  mkdir build
fi
cd build
../configure --prefix=/opt/mpich --disable-fortran --with-ucx=embedded > /dev/null
if [ $? -ne 0 ]; then
  echo "failed to configure mpich-4.3.1"
  exit 1
fi

make -j 16 > /dev/null
if [ $? -ne 0 ]; then
  echo "failed to make mpich-4.3.1"
  exit 1
fi

make install > /dev/null
if [ $? -ne 0 ]; then
  echo "failed to install mpich-4.3.1"
  exit 1
fi

rm -rf /tmp/mpich
rm -f /tmp/mpich-4.3.1.tar.gz
echo "============== install mpich-4.3.1 successfully =============="