#!/bin/bash

#
# Copyright © AMD. 2025-2026. All rights reserved.
#

if [ "$#" -lt 1 ]; then
  echo 'Error: Missing parameter node-info. example: {"expectedGpuCount": 8, "observedGpuCount": 8}'
  exit 2
fi

expectedCount=`echo "$1" |jq '.expectedGpuCount'`
observedCount=`echo "$1" |jq '.observedGpuCount'`

if [ -z "$expectedCount" ] || [ "$expectedCount" == "null" ] || [ $expectedCount -le 0 ]; then
  echo "Error: failed to get expectedGpuCount from input: $1"
  exit 2
fi

if [ -z "$observedCount" ] || [ "$observedCount" == "null" ]; then
  echo "Error: failed to get observedGpuCount from input: $1"
  exit 2
fi

if  [ $observedCount -ne $expectedCount ]; then
  echo 'Error: The gpu cards reported by the device plugin is' $observedCount, 'but the expected value is' $expectedCount
  exit 1
fi