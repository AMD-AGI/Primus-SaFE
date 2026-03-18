#!/bin/bash

systemd_path="/etc/systemd/system"
systemd_script="$systemd_path/disable-huge-pages.sh"
systemd_service_name="disable-huge-pages.service"
systemd_service_path="$systemd_path/$systemd_service_name"

echo "#!/bin/sh
echo never > /sys/kernel/mm/transparent_hugepage/enabled
echo never > /sys/kernel/mm/transparent_hugepage/defrag" > $systemd_script
chmod +x "$systemd_script"

cat > "$systemd_service_path" << 'EOF'
[Unit]
Description=Disable THP
After=network.target

[Service]
Type=oneshot
ExecStart=/bin/sh /etc/systemd/system/disable-huge-pages.sh
RemainAfterExit=yes

[Install]
WantedBy=multi-user.target
EOF

systemctl disable --now enable-huge-pages.service 2>/dev/null || true
rm -f $systemd_path/enable-huge-pages.service
rm -f $systemd_path/enable-huge-pages.sh

systemctl daemon-reload && systemctl enable "$systemd_service_name" && systemctl start "$systemd_service_name"
exit $?