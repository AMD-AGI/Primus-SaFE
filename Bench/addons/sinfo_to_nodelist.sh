#!/bin/bash
#
# Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#
# Parse sinfo output and expand NODELIST to one host per line.
# Handles bracket notation: [021,042,079-080] -> 021, 042, 079, 080
#
# Usage:
#   ./sinfo_to_nodelist.sh [output_file]
#   sinfo | ./sinfo_to_nodelist.sh [output_file]
#
# If output_file is omitted, prints to stdout.

set -euo pipefail

# Expand a single bracket expression: prefix-[a,b,c-d,e]
# Returns one host per line. Handles ranges like 079-080 (preserves zero-padding).
expand_nodelist() {
    local input="$1"
    if [[ ! "$input" =~ \[.*\] ]]; then
        echo "$input"
        return
    fi

    local prefix="${input%%\[*}"
    local suffix="${input##*\]}"
    local inner="${input#*\[}"
    inner="${inner%\]*}"

    IFS=',' read -ra parts <<< "$inner"
    for part in "${parts[@]}"; do
        part="${part// /}"
        if [[ "$part" =~ ^([0-9]+)-([0-9]+)$ ]]; then
            local start="${BASH_REMATCH[1]}"
            local end="${BASH_REMATCH[2]}"
            local width=${#start}
            for ((i=10#$start; i<=10#$end; i++)); do
                printf "%s%0${width}d%s\n" "$prefix" "$i" "$suffix"
            done
        else
            echo "${prefix}${part}${suffix}"
        fi
    done
}

# Parse sinfo output: skip header, extract NODELIST (last column)
parse_sinfo() {
    while IFS= read -r line; do
        [[ -z "${line// /}" ]] && continue
        [[ "$line" =~ ^PARTITION ]] && continue
        local nodelist="${line##* }"
        [[ "$nodelist" =~ \[.*\] ]] || continue
        expand_nodelist "$nodelist"
    done
}

# Main: read from stdin or run sinfo
if [ -t 0 ]; then
    input=$(sinfo 2>/dev/null) || { echo "Error: sinfo failed or not found" >&2; exit 1; }
else
    input=$(cat)
fi

output=$(echo "$input" | parse_sinfo | sort -u)

if [ $# -ge 1 ]; then
    echo "$output" > "$1"
    echo "Wrote $(echo "$output" | wc -l) nodes to $1"
else
    echo "$output"
fi
