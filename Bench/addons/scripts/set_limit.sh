#!/bin/bash

grep -q ^"* hard nofile 65535" "/etc/security/limits.conf"
if [ $? -eq 0 ]; then
  exit 0
fi
echo "* soft nofile 65535" >> /etc/security/limits.conf
echo "* hard nofile 65535" >> /etc/security/limits.conf