#
# Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#

echo "begin to install mpich-4.3.1"

wget https://www.mpich.org/static/downloads/4.3.1/mpich-4.3.1.tar.gz
if [ $? -ne 0 ]; then
  exit 1
fi
mkdir -p mpich && tar -zxf mpich-4.3.1.tar.gz -C mpich --strip-components=1 && cd mpich
if [ $? -ne 0 ]; then
  exit 1
fi
mkdir build && cd build
../configure --prefix=/opt/mpich --disable-fortran --with-ucx=embedded > /dev/null 2>&1
if [ $? -ne 0 ]; then
  echo "failed to configure mpich-4.3.1"
  exit 1
fi

make -j 16 > /dev/null 2>&1
if [ $? -ne 0 ]; then
  echo "failed to make mpich-4.3.1"
  exit 1
fi

make install > /dev/null 2>&1
if [ $? -ne 0 ]; then
  echo "failed to install mpich-4.3.1"
  exit 1
fi
echo "install mpich-4.3.1 successfully"