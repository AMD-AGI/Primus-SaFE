#
# Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#

package="libbnxt_re-234.0.154.0"
echo "==============  begin to install $package =============="

full_package="$package.tar.gz"
if [ ! -f "${full_package}" ]; then
  exit 1
fi

apt update > /dev/null
apt -y install linux-headers-"$(uname -r)"
if [ $? -ne 0 ]; then
  exit 1
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