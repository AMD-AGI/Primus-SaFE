#!/usr/bin/env bash

#
# Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#

WORK_PATH=/opt/primus-safe/node-agent

. /etc/bashrc

cd ${WORK_PATH}
find ${WORK_PATH} -type f -name "*.sh" -exec chmod 700 {} +

./node_agent --node_name=${NODE_NAME} --log_file_path=${WORK_PATH}/logs/node-agent.log \
  --configmap_path=/opt/primus-safe/node-agent/config --script_path=/opt/primus-safe/node-agent/scripts
