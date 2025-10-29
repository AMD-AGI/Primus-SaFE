#!/bin/bash

#
# Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#

helm uninstall primus-safe -n primus-safe

helm uninstall primus-safe-cr -n primus-safe

helm uninstall grafana-operator -n primus-safe

helm uninstall primus-pgo -n primus-safe

helm uninstall node-agent -n primus-safe