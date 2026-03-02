#!/bin/bash

systemd_path="/etc/systemd/system"
systemd_script="$systemd_path/enable-cpu-performance.sh"
systemd_service_name="enable-cpu-performance.service"
systemd_service_path="$systemd_path/$systemd_service_name"

if [ ! -f "$systemd_script" ]; then
  echo "#!/bin/sh
echo performance | tee /sys/devices/system/cpu/cpu*/cpufreq/scaling_governor" > $systemd_script
  chmod +x "$systemd_script"
fi
if [ -f "$systemd_service_path" ]; then
  exit 0
fi

cat > "$systemd_service_path" << 'EOF'
[Unit]
Description=Performance Tuning: CPU Governor
After=network.target

[Service]
Type=oneshot
ExecStart=/bin/sh /etc/systemd/system/enable-cpu-performance.sh
RemainAfterExit=yes

[Install]
WantedBy=multi-user.target
EOF

systemctl daemon-reload && systemctl enable "$systemd_service_name" && systemctl start "$systemd_service_name"
exit $?