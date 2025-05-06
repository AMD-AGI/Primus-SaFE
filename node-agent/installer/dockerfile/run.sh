#!/usr/bin/env bash

#
# /*
#  * Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
#  * See LICENSE for license information.
#  */
#

WORK_PATH=/opt/primus-safe/node-agent

. /etc/bashrc

cd ${WORK_PATH}
find ${WORK_PATH} -type f -name "*.sh" -exec chmod 700 {} +

cd ${WORK_PATH} && \
  ./node_agent --node-name=${NODE_NAME} --log_file_path=${WORK_PATH}/logs/node-agent.log \
  --configmap_path=${NODE_CONFIGMAP_PATH}
