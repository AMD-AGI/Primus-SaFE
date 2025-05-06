#!/usr/bin/env bash

#
# Copyright © AMD. 2025-2026. All rights reserved.
#

WORK_PATH=/opt/safe/node-agent

. /etc/bashrc

cd ${WORK_PATH}
find ${WORK_PATH} -type f -name "*.sh" -exec chmod 700 {} +

cd ${WORK_PATH} && \
  ./node_agent --node-name=${NODE_NAME} --log_file_path=${WORK_PATH}/logs/node-agent.log \
  --configmap_path=${NODE_CONFIGMAP_PATH}
