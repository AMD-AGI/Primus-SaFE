#!/bin/bash

#
# Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#

pip3 install -r requirements.txt
if [ $? -ne 0 ]; then
  echo "failed to install python package"
  exit 1
fi
bash install_rccl_test.sh
if [ $? -ne 0 ]; then
  echo "failed to install rccl-tests"
  exit 1
fi

rm -rf /root/.ssh
mkdir /root/.ssh
touch /root/.ssh/authorized_keys
touch /root/.ssh/config
ssh-keygen -t rsa -b 4096 -N "" -f  /root/.ssh/id_rsa
if [ $? -ne 0 ]; then
  echo "failed to execute ssh-keygen"
  exit 1
fi

cd /root
python3 -m torch.distributed.launch --nnodes $WORLD_SIZE --node_rank $RANK \
  --master_addr $MASTER_ADDR --master_port $MASTER_PORT \
  ./sync_ssh.py  --distributed-timeout-minutes 30 --interface $GLOO_SOCKET_IFNAME
if [ $? -ne 0 ]; then
  echo "failed to execute sync_ssh.py"
  exit 1
fi

cat /root/.ssh/config  | grep "Host " | awk '{print $2}' > hosts

if [[ "$RANK" == "0" ]]; then
  debug=""
  if [[ "$NCCL_DEBUG" == "INFO" ]] || [[ "$NCCL_DEBUG" == "info" ]] ; then
    debug="--debug"
  fi
  sleep 3000
  python3 rccl_diagnose.py --socket-ifname "$NCCL_SOCKET_IFNAME" --ib-hca "$NCCL_IB_HCA" $debug
  if [ $? -ne 0 ]; then
      echo "The exec binary_search_run_nccl_test.py command failed."
      exit 1
  fi
fi

python3 -m torch.distributed.launch --nnodes $WORLD_SIZE --node_rank $RANK \
  --master_addr $MASTER_ADDR --master_port $MASTER_PORT \
  ./sync_ssh.py  --distributed-timeout-minutes 30 --no-data-sync 1
if [ $? -ne 0 ]; then
  echo "failed to execute sync_ssh.py"
  exit 1
fi