#!/bin/bash

cat /etc/apt/apt.conf.d/20auto-upgrades |grep Unattended-Upgrade|awk "{print substr(\$2, 2, 1)}"|grep -q 0
if [ $? -eq 0 ]; then
  exit 0
fi
apt-mark hold unattended-upgrades && apt-get update && sed -i -e "s/Unattended-Upgrade \"1\"/Unattended-Upgrade \"0\"/g" /etc/apt/apt.conf.d/20auto-upgrades