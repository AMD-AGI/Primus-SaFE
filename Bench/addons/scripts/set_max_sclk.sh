#!/bin/bash

systemd_path="/etc/systemd/system"
systemd_script="$systemd_path/set-max-sclk.sh"
systemd_service_name="set-max-sclk.service"
systemd_service_path="$systemd_path/$systemd_service_name"

if [ ! -f "$systemd_script" ]; then
  echo "#!/bin/sh
if [ -f /usr/bin/rocm-smi ]; then
  yes | /usr/bin/rocm-smi --setextremum max sclk 1800
fi" > $systemd_script
  chmod +x "$systemd_script"
fi
if [ -f "$systemd_service_path" ]; then
  exit 0
fi

cat > "$systemd_service_path" << 'EOF'
[Unit]
Description=Set Max SCLK
After=network.target

[Service]
Type=oneshot
ExecStart=/bin/sh /etc/systemd/system/set-max-sclk.sh
RemainAfterExit=yes

[Install]
WantedBy=multi-user.target
EOF

systemctl daemon-reload && systemctl enable "$systemd_service_name" && systemctl start "$systemd_service_name"
exit $?