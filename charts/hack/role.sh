#!/usr/bin/env bash
#
# Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#

if [ -f "primus-safe/templates/rbac/role.yaml" ]; then
  cat config/role_patch.txt >> primus-safe/templates/rbac/role.yaml
fi

