#!/bin/bash

cp /etc/default/grub /etc/default/grub.bak
sed -i 's/^GRUB_CMDLINE_LINUX=.*/GRUB_CMDLINE_LINUX="pci=realloc=off pci=bfsort iommu=pt numa_balancing=disable modprobe.blacklist=amdgpu"/' /etc/default/grub
grep -q '^GRUB_CMDLINE_LINUX=' /etc/default/grub || echo 'GRUB_CMDLINE_LINUX="pci=realloc=off pci=bfsort iommu=pt numa_balancing=disable modprobe.blacklist=amdgpu"' | sudo tee -a /etc/default/grub
update-grub