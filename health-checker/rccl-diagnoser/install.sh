#
# Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#

cd depends

pip3 install -r requirements.txt > /dev/null
if [ $? -ne 0 ]; then
  echo "failed to install python package"
  exit 1
fi

for script in *.sh; do
  bash "$script"
  if [ $? -ne 0 ]; then
    echo "failed to run $script"
    exit 1
  fi
done