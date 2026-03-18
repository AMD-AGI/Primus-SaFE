#!/bin/bash

[ "$(id -u)" -eq 0 ] || { echo "Error: must run as root"; exit 1; }

GRUB_FILE="/etc/default/grub"
target='GRUB_CMDLINE_LINUX="pci=realloc=off pci=bfsort iommu=pt numa_balancing=disable modprobe.blacklist=amdgpu"'
if grep -qF 'pci=realloc=off pci=bfsort iommu=pt numa_balancing=disable modprobe.blacklist=amdgpu' "$GRUB_FILE"; then
  exit 0
fi

cp "$GRUB_FILE" "${GRUB_FILE}.bak" || { echo "Error: failed to backup $GRUB_FILE"; exit 1; }
if grep -q '^GRUB_CMDLINE_LINUX=' "$GRUB_FILE"; then
  sed -i 's/^GRUB_CMDLINE_LINUX=.*/'"$target"'/' "$GRUB_FILE" || { cp "${GRUB_FILE}.bak" "$GRUB_FILE"; echo "Error: failed to update grub"; exit 1; }
else
  echo "$target" >> "$GRUB_FILE" || { cp "${GRUB_FILE}.bak" "$GRUB_FILE"; echo "Error: failed to append to grub"; exit 1; }
fi
update-grub
exit $?