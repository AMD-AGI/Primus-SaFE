#!/usr/bin/env bash

#
# Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#

WORK_PATH=/opt/xcs/job-manager

. /etc/bashrc

cd ${WORK_PATH}
find ${WORK_PATH} -type f -name "*.sh" -exec chmod 700 {} +

./job_manager --config=${WORK_PATH}/config/config.yaml --log_file_path=${WORK_PATH}/logs/manager.log
