#!/bin/bash

systemd_path="/etc/systemd/system"
systemd_script="$systemd_path/disable-numa-balancing.sh"
systemd_service_name="disable-numa-balancing.service"
systemd_service_path="$systemd_path/$systemd_service_name"

if [ ! -f "$systemd_script" ]; then
  echo "#!/bin/sh
sysctl -w kernel.numa_balancing=0" > $systemd_script
  chmod +x "$systemd_script"
fi
if [ -f "$systemd_service_path" ]; then
  exit 0
fi

cat > "$systemd_service_path" << 'EOF'
[Unit]
Description=Disable NUMA Balancing
After=network.target

[Service]
Type=oneshot
ExecStart=/bin/sh /etc/systemd/system/disable-numa-balancing.sh
RemainAfterExit=yes

[Install]
WantedBy=multi-user.target
EOF

systemctl daemon-reload && systemctl enable "$systemd_service_name" && systemctl start "$systemd_service_name"
exit $?