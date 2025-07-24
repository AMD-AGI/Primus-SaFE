#
# Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#

helm uninstall training-operator -n kubeflow

helm uninstall node-agent -n primus-safe