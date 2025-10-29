#!/usr/bin/env bash

#
# Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#
ulimit -n 65535

WORK_PATH=/opt/primus-safe/webhooks

. /etc/bashrc

cd ${WORK_PATH}
find ${WORK_PATH} -type f -name "*.sh" -exec chmod 700 {} +

./webhooks --config=${WORK_PATH}/config/config.yaml \
--log_file_path=${WORK_PATH}/logs/webhooks.log --cert_dir=/opt/primus-safe/webhooks/cert
