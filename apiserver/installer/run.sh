#!/usr/bin/env bash

WORK_PATH=/opt/primus-safe/apiserver

. /etc/bashrc

cd ${WORK_PATH}
find ${WORK_PATH} -type f -name "*.sh" -exec chmod 700 {} +

./apiserver --config=${WORK_PATH}/config/config.toml --log_file_path=${WORK_PATH}/logs/apiserver.log
