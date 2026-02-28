#!/bin/bash

systemctl stop apt-daily.service apt-daily-upgrade.service || true
systemctl disable apt-daily.service apt-daily-upgrade.service || true
systemctl mask apt-daily.service apt-daily-upgrade.service || true
systemctl stop apt-daily.timer apt-daily-upgrade.timer || true
systemctl disable apt-daily.timer apt-daily-upgrade.timer || true
systemctl mask apt-daily.timer apt-daily-upgrade.timer || true