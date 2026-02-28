#!/bin/bash

echo "fs.inotify.max_user_instances=1024" > /etc/sysctl.d/99-inotify.conf
echo "fs.inotify.max_user_watches=1048576" >> /etc/sysctl.d/99-inotify.conf
echo "fs.inotify.max_queued_events=16384" >> /etc/sysctl.d/99-inotify.conf
sysctl --system