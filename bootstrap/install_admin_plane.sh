#!/bin/bash

#
# Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#

helm install primus-pgo -n primus-safe oci://registry-1.docker.io/primussafe/primus-pgo --version 5.8.2  --create-namespace

helm install primus-safe -n primus-safe oci://registry-1.docker.io/primussafe/primus-safe --version 0.2.0

helm install primus-safe-cr -n primus-safe oci://registry-1.docker.io/primussafe/primus-safe-cr --version 0.2.0