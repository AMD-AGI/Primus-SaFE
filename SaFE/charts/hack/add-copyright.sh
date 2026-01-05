#!/bin/bash

#
# Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#

COPYRIGHT_HEADER="#
# Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#
"

CONFIG_DIRS=(
    "primus-safe/crds"
)

for dir in "${CONFIG_DIRS[@]}"; do
    if [ -d "$dir" ]; then
        find "$dir" -name '*.yaml' -o -name '*.yml' | while read file; do
            tmpfile=$(mktemp)
            echo -e "$COPYRIGHT_HEADER\n$(cat "$file")" > "$tmpfile"
            mv "$tmpfile" "$file"
        done
    fi
done

WEBHOOK_FILE="primus-safe/templates/webhooks/manifests.yaml"
if [ -f "$WEBHOOK_FILE" ]; then
    tmpfile=$(mktemp)
    echo -e "$COPYRIGHT_HEADER\n$(cat "$WEBHOOK_FILE")" > "$tmpfile"
    mv "$tmpfile" "$WEBHOOK_FILE"
else
    echo "⚠️ Warning: $WEBHOOK_FILE not found"
fi

ROLE_FILE="primus-safe/templates/rbac/role.yaml"
if [ -f "$ROLE_FILE" ]; then
    tmpfile=$(mktemp)
    echo -e "$COPYRIGHT_HEADER\n$(cat "$ROLE_FILE")" > "$tmpfile"
    mv "$tmpfile" "$ROLE_FILE"
else
    echo "⚠️ Warning: $ROLE_FILE not found"
fi