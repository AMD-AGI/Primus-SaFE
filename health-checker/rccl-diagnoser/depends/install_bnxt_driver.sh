#
# Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#

echo "==============  begin to install bnxt_re-231.0.162.0 =============="

bnxt_package="libbnxt_re-231.0.162.0.tar.gz"
if [ ! -f "$bnxt_package" ]; then
  exit 1
fi

apt -y install linux-headers-"$(uname -r)"
if [ $? -ne 0 ]; then
  exit 1
fi

tar xzf "${bnxt_package}" -C /tmp/ && \
mv /usr/lib/x86_64-linux-gnu/libibverbs/libbnxt_re-rdmav34.so /usr/lib/x86_64-linux-gnu/libibverbs/libbnxt_re-rdmav34.so.inbox && \
cd /tmp/libbnxt_re-231.0.162.0/ && sh ./autogen.sh && ./configure && \
make clean && make all && make install && \
echo '/usr/local/lib' > /etc/ld.so.conf.d/libbnxt_re.conf && \
ldconfig && \
cp -f /tmp/libbnxt_re-231.0.162.0/bnxt_re.driver /etc/libibverbs.d/

if [ $? -ne 0 ]; then
  exit 1
fi

echo "============== install bnxt_re-231.0.162.0 successfully =============="