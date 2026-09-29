#!/bin/sh

#
# Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#

# Writes passwd and group files that name the uid an external provider runs the main
# container as. The provider assigns that uid at admission and records it in pod
# annotations, which the dispatcher renders into IDENTITY_DIR. The main container
# mounts the files over /etc/passwd and /etc/group. The files are always written, from
# this image's own entries, so the mounts have something to bind when no uid is given.

IDENTITY_DIR="${EXTERNAL_IDENTITY_DIR:-/etc/external-identity}"
OUT_DIR="${EXTERNAL_IDENTITY_OUT:-/shared-data/etc}"
HOME_DIR="${EXTERNAL_IDENTITY_HOME:-/tmp}"

read_value() {
  [ -f "$IDENTITY_DIR/$1" ] && tr -d ' \t\r\n' < "$IDENTITY_DIR/$1"
}

is_number() {
  case "$1" in
    ''|*[!0-9]*) return 1 ;;
    *) return 0 ;;
  esac
}

# has_id reports whether the file holds an entry with the given numeric id in field 3.
has_id() {
  awk -F: -v id="$2" '$3 == id { found = 1 } END { exit !found }' "$1"
}

# has_name reports whether the file holds an entry with the given name in field 1.
has_name() {
  awk -F: -v n="$2" '$1 == n { found = 1 } END { exit !found }' "$1"
}

mkdir -p "$OUT_DIR"
cp /etc/passwd "$OUT_DIR/passwd"
cp /etc/group "$OUT_DIR/group"

uid=$(read_value uid)
gid=$(read_value gid)
groups=$(read_value groups)
account=$(read_value account)

if is_number "$uid"; then
  is_number "$gid" || gid="$uid"
  case "$account" in
    [a-z_]*) name=$(printf '%s' "$account" | tr -c 'a-z0-9_.-' '_' | cut -c1-32) ;;
    *) name="" ;;
  esac
  if [ -z "$name" ] || has_name "$OUT_DIR/passwd" "$name"; then
    name="user$uid"
  fi
  if ! has_id "$OUT_DIR/passwd" "$uid"; then
    echo "$name:x:$uid:$gid::$HOME_DIR:/bin/bash" >> "$OUT_DIR/passwd"
  fi
  if ! has_id "$OUT_DIR/group" "$gid"; then
    group_name="$name"
    has_name "$OUT_DIR/group" "$group_name" && group_name="group$gid"
    echo "$group_name:x:$gid:" >> "$OUT_DIR/group"
  fi
  for g in $(printf '%s' "$groups" | tr ',' ' '); do
    is_number "$g" || continue
    if ! has_id "$OUT_DIR/group" "$g"; then
      echo "group$g:x:$g:$name" >> "$OUT_DIR/group"
    fi
  done
  echo "INFO: external identity $name uid=$uid gid=$gid written to $OUT_DIR"
fi

chmod 0644 "$OUT_DIR/passwd" "$OUT_DIR/group"
