#!/bin/sh

cd /shared-data || exit 1

# Check if openssh-server is installed
if ! dpkg -l | grep -q 'openssh-server'; then
    apt-get update > /dev/null 2>&1
    timeout 180s apt-get install -y openssh-server > /dev/null 2>&1
    ret=$?
    if [ $ret != 0 ]; then
        timeout 180s apt-get -y --fix-broken install > /dev/null 2>&1
        timeout 180s apt-get install -y openssh-server > /dev/null 2>&1
        ret=$?
    fi
    if [ $ret != 0 ]; then
        echo "openssh-server cannot be installed correctly, $ret. pls close the switch for ssh"
        exit $ret
    fi
fi

# Set root password
echo root:root | chpasswd
ret=$?
if [ $ret != 0 ]; then
    echo "fail to change password! $ret"
    exit $ret
fi

# Configure SSHD config
sed -i '/Port /d' /etc/ssh/sshd_config
sed -i '/PermitRootLogin yes/d' /etc/ssh/sshd_config
echo "Port $SSH_PORT" >> /etc/ssh/sshd_config
echo "PermitRootLogin yes" >> /etc/ssh/sshd_config
sed -i 's/^AllowUsers/#AllowUsers/' /etc/ssh/sshd_config
sed -i 's/# UsePAM yes/UsePAM yes/' /etc/ssh/sshd_config

# Restart SSH service
if ! systemctl is-active --quiet ssh; then
    systemctl start ssh > /dev/null 2>&1
else
    systemctl restart ssh > /dev/null 2>&1
fi

# Verify SSH is running
if ! pgrep -x sshd > /dev/null; then
    echo "sshd is not running!"
    exit 1
fi

# Setup .ssh directory and authorized_keys
ssh_dir="$HOME/.ssh"
authorized_keys_file="$ssh_dir/authorized_keys"

if [ ! -d "$ssh_dir" ]; then
    mkdir -p "$ssh_dir"
fi

if [ ! -e "$authorized_keys_file" ]; then
    touch "$authorized_keys_file"
fi

# Set correct permissions
chmod 700 "$ssh_dir"
chmod 600 "$authorized_keys_file"

echo "run sshd with port $SSH_PORT successfully"
exit 0