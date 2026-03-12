#!/bin/bash

#
# Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#

set -o pipefail
export PATH="/usr/bin:/bin:${PATH:-}"

# Check if resolv.conf files contain the specified search domain.
# If not present, add the domain to the search line (append only, no modification of existing).
# Two files: /run/systemd/resolve/resolv.conf and /etc/resolv.conf

if [ "$#" -lt 1 ]; then
  echo "Usage: $0 <search_domain>"
  echo "Example: $0 amd.com"
  exit 2
fi

SEARCH_DOMAIN="$1"
if [ -z "$SEARCH_DOMAIN" ]; then
  echo "Error: search_domain cannot be empty"
  exit 2
fi

NSENTER="nsenter --target 1 --mount --uts --ipc --net --pid --"
FILES=(
  "/run/systemd/resolve/resolv.conf"
  "/etc/resolv.conf"
)

# Check if domain exists in search line (as a whole word)
# Returns 0 if exists, 1 if not
domain_in_search_line() {
  local search_line="$1"
  local domain="$2"
  # Extract each domain from "search d1 d2 d3" and check for exact match
  echo "$search_line" | awk '/^search / {for(i=2;i<=NF;i++) print $i}' | grep -qxF "$domain"
}

# Ensure domain exists in file: add to search line if not present (append only)
# $1=file to modify, $2=domain, $3=display name for messages
# Returns 0 on success, 1 on failure
ensure_search_domain() {
  local file="$1"
  local domain="$2"
  local display="${3:-$file}"
  local content
  local search_line
  local new_search_line

  content=$(${NSENTER} cat "$file" 2>/dev/null)
  if [ $? -ne 0 ]; then
    echo "Error: failed to read $display"
    return 1
  fi

  search_line=$(echo "$content" | grep -E "^search ")
  if [ -z "$search_line" ]; then
    # No search line: add "search domain" before first nameserver
    new_search_line="search $domain"
    if echo "$content" | grep -qE "^nameserver "; then
      new_content=$(echo "$content" | awk -v line="$new_search_line" '
        /^nameserver/ && !done { print line; done=1 }
        { print }
      ')
    else
      new_content="${content}${content:+$'\n'}$new_search_line"
    fi
  else
    if domain_in_search_line "$search_line" "$domain"; then
      return 0
    fi
    # Domain not in list: append to search line (do not modify existing domains)
    new_content=$(echo "$content" | awk -v domain="$domain" '
      /^search / { $0 = $0 " " domain; print; next }
      { print }
    ')
  fi

  # Remove immutable attribute before writing (if set)
  ${NSENTER} chattr -i "$file" 2>/dev/null

  # Write back
  if echo "$new_content" | ${NSENTER} tee "$file" >/dev/null 2>&1; then
    ${NSENTER} chattr +i "$file" 2>/dev/null
    echo "Added search domain $domain to $display"
    return 0
  fi

  # Try alternative: use a temp file
  local tmp_file
  tmp_file=$(${NSENTER} mktemp 2>/dev/null)
  if [ -z "$tmp_file" ]; then
    ${NSENTER} chattr +i "$file" 2>/dev/null
    echo "Error: failed to add search domain $domain to $display"
    return 1
  fi
  echo "$new_content" | ${NSENTER} tee "$tmp_file" >/dev/null 2>&1
  if [ $? -ne 0 ]; then
    ${NSENTER} rm -f "$tmp_file" 2>/dev/null
    ${NSENTER} chattr +i "$file" 2>/dev/null
    echo "Error: failed to add search domain $domain to $display"
    return 1
  fi
  ${NSENTER} mv "$tmp_file" "$file" 2>/dev/null
  if [ $? -ne 0 ]; then
    ${NSENTER} rm -f "$tmp_file" 2>/dev/null
    ${NSENTER} chattr +i "$file" 2>/dev/null
    echo "Error: failed to add search domain $domain to $display"
    return 1
  fi
  ${NSENTER} chattr +i "$file" 2>/dev/null
  echo "Added search domain $domain to $display"
  return 0
}

exit_code=0
for file in "${FILES[@]}"; do
  if ! ${NSENTER} test -e "$file" 2>/dev/null; then
    echo "Error: $file does not exist"
    exit_code=1
    continue
  fi

  target="$file"
  if [ "$file" = "/etc/resolv.conf" ]; then
    if ${NSENTER} test -L "$file" 2>/dev/null; then
      target=$(${NSENTER} readlink -f "$file" 2>/dev/null)
      if [ -z "$target" ]; then
        echo "Error: failed to resolve symlink $file"
        exit_code=1
        continue
      fi
    fi
  fi

  if ! ensure_search_domain "$target" "$SEARCH_DOMAIN" "$file"; then
    exit_code=1
  fi
done

exit $exit_code
