#!/bin/sh

#
# Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#

. /shared-data/utils.sh
install_if_not_exists openssh-server
if [ $? -eq 0 ]; then
  echo "openssh-server installation succeeded"
else
  echo "openssh-server installation failed"
fi
