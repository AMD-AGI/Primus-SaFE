#
# Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#

helm install training-operator -n kubeflow  oci://registry-1.docker.io/primussafe/training-operator --version 1.9.2 --create-namespace

helm install node-agent -n primus-safe oci://registry-1.docker.io/primussafe/node-agent --version 0.1.1  --create-namespace
