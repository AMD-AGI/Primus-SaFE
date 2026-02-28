#!/bin/bash

target='GRUB_CMDLINE_LINUX="pci=realloc=off pci=bfsort iommu=pt numa_balancing=disable modprobe.blacklist=amdgpu"'
if grep -qF 'pci=realloc=off pci=bfsort iommu=pt numa_balancing=disable modprobe.blacklist=amdgpu' /etc/default/grub; then
  exit 0
fi
cp /etc/default/grub /etc/default/grub.bak
if grep -q '^GRUB_CMDLINE_LINUX=' /etc/default/grub; then
  sed -i 's/^GRUB_CMDLINE_LINUX=.*/'"$target"'/' /etc/default/grub
else
  echo "$target" >> /etc/default/grub
fi
update-grub