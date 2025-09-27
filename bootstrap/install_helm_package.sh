#!/bin/bash

#
# Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#

set -euo pipefail

helm install primus-pgo -n primus-safe oci://registry-1.docker.io/primussafe/primus-pgo --version 5.8.2  --create-namespace

sleep 10

helm install primus-safe -n primus-safe oci://registry-1.docker.io/primussafe/primus-safe --version 0.2.2

helm install primus-safe-cr -n primus-safe oci://registry-1.docker.io/primussafe/primus-safe-cr --version 0.2.2

helm install node-agent -n primus-safe oci://registry-1.docker.io/primussafe/node-agent --version 0.1.3  --create-namespace
