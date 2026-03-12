#
# Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REQUIREMENTS_FILE="$SCRIPT_DIR/../node/model_check/requirements.txt"

echo "============== begin to install model_check dependencies =============="

if ! python3 -c "import datasets" 2>/dev/null; then
  echo "Installing required packages (this may take a few minutes)..."
  if [ "${OS_VERSION}" = "24.04" ]; then
    pip3 install -r "$REQUIREMENTS_FILE" --break-system-packages || {
      echo "Error: Failed to install model_check dependencies" >&2
      exit 1
    }
  else
    pip3 install -r "$REQUIREMENTS_FILE" || {
      echo "Error: Failed to install model_check dependencies" >&2
      exit 1
    }
  fi
  echo "Dependencies installed successfully!"
else
  echo "model_check dependencies already installed, skipping."
fi

echo "============== install model_check dependencies successfully =============="