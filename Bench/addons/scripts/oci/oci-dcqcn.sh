#!/bin/bash

#
# Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#

TOKEN_BUCKET_SIZE=800000
AI_RATE=160
ALPHA_UPDATE_INTERVAL=1
ALPHA_UPDATE_G=512
INITIAL_ALPHA_VALUE=64
RATE_INCREASE_BYTE_COUNT=431068
HAI_RATE=300
RATE_REDUCE_MONITOR_PERIOD=1
RATE_INCREASE_THRESHOLD=1
RATE_INCREASE_INTERVAL=1

ibdevs=$(ibdev2netdev | grep -v ens | awk '{print $1}')
profile_id=1
for ibdev in ${ibdevs};
do
    nicctl update dcqcn -r ${ibdev} --profile-id ${profile_id} \
    --rate-reduce-monitor-period ${RATE_REDUCE_MONITOR_PERIOD} \
    --alpha-update-interval ${ALPHA_UPDATE_INTERVAL} \
    --rate-increase-threshold ${RATE_INCREASE_THRESHOLD} \
    --rate-increase-byte-count ${RATE_INCREASE_BYTE_COUNT} \
    --ai-rate ${AI_RATE} \
    --alpha-update-g ${ALPHA_UPDATE_G} \
    --token-bucket-size ${TOKEN_BUCKET_SIZE} \
    --rate-increase-interval ${RATE_INCREASE_INTERVAL} \
    --hai-rate ${HAI_RATE} \
    --initial-alpha-value ${INITIAL_ALPHA_VALUE}  \
    --cnp-dscp 46
done
