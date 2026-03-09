#!/bin/bash

echo "fs.inotify.max_user_instances=1024" > /etc/sysctl.d/99-inotify.conf
echo "fs.inotify.max_user_watches=1048576" >> /etc/sysctl.d/99-inotify.conf
echo "fs.inotify.max_queued_events=16384" >> /etc/sysctl.d/99-inotify.conf
echo "vm.max_map_count=262144" > /etc/sysctl.d/99-max-map-count.conf
sysctl --system >/dev/null 2>&1