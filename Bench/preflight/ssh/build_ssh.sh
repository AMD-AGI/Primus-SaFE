#!/bin/bash

#
# Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#

echo root:root | chpasswd
ret=$?
if [ $ret != 0 ]; then
  echo "failed to change password! $ret"
  exit $ret
fi

rm -rf /root/.ssh
mkdir /root/.ssh
chmod 700 "/root/.ssh"
touch /root/.ssh/authorized_keys
chmod 600 /root/.ssh/authorized_keys
touch /root/.ssh/config
ssh-keygen -t rsa -b 4096 -N "" -f  /root/.ssh/id_rsa
if [ $? -ne 0 ]; then
  echo "failed to execute ssh-keygen"
  exit 1
fi

sed -i '/Port /d' /etc/ssh/sshd_config
sed -i '/PermitRootLogin yes/d' /etc/ssh/sshd_config
sed -i 's/^#PubkeyAuthentication yes/PubkeyAuthentication yes/' /etc/ssh/sshd_config
sed -i 's/^#AuthorizedKeysFile/AuthorizedKeysFile/' /etc/ssh/sshd_config
sed -i "s/^AllowUsers/#AllowUsers/" /etc/ssh/sshd_config
sed -i 's/# UsePAM yes/UsePAM yes/' /etc/ssh/sshd_config
echo "Port $SSH_PORT" >> /etc/ssh/sshd_config
echo "PermitRootLogin yes" >> /etc/ssh/sshd_config

service ssh restart > /dev/null
service ssh status | grep "sshd is running"
ret=$?
if [ $ret != 0 ]; then
  echo "sshd is not running! $ret"
  exit $ret
fi

echo "run sshd with port $SSH_PORT successfully"
