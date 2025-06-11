#!/bin/sh

cd /shared-data || {
    echo "Failed to cd to /shared-data"
    exit 1
}

# check if openssh-server is already installed
dpkg -l | grep -q 'openssh-server'
ret=$?

if [ $ret -ne 0 ]; then
    echo "Installing openssh-server..."

    apt-get update > /dev/null 2>&1
    timeout 180 apt-get install -y openssh-server > /dev/null 2>&1
    ret=$?

    if [ $ret -ne 0 ]; then
        echo "First install failed, trying --fix-broken..."
        timeout 180 apt-get install -y --fix-broken > /dev/null 2>&1
        timeout 180 apt-get install -y openssh-server > /dev/null 2>&1
        ret=$?
    fi

    if [ $ret -ne 0 ]; then
        echo "openssh-server cannot be installed correctly, error code: $ret. Please disable the SSH switch."
        exit $ret
    fi
fi

# set root password
echo "root:root" | chpasswd
ret=$?
if [ $ret -ne 0 ]; then
    echo "Failed to change root password! Error code: $ret"
    exit $ret
fi

SSHD_CONFIG="/etc/ssh/sshd_config"

sed -i '/^Port /d' "$SSHD_CONFIG"
sed -i '/^PermitRootLogin /d' "$SSHD_CONFIG"
echo "Port ${SSH_PORT}" >> "$SSHD_CONFIG"
echo "PermitRootLogin yes" >> "$SSHD_CONFIG"
sed -i 's/^AllowUsers/#AllowUsers/' "$SSHD_CONFIG"
# enable PAM
sed -i 's/#UsePAM yes/UsePAM yes/' "$SSHD_CONFIG"

# start ssh server
service ssh restart > /dev/null 2>&1

# check if ssh is running
service ssh status | grep -q 'sshd is running'
ret=$?
if [ $ret -ne 0 ]; then
    echo "sshd is not running! Error code: $ret"
    exit $ret
fi

ssh_dir="$HOME/.ssh"
authorized_keys_file="$ssh_dir/authorized_keys"

if [ ! -d "$ssh_dir" ]; then
    mkdir -p "$ssh_dir"
fi

if [ ! -f "$authorized_keys_file" ]; then
    touch "$authorized_keys_file"
fi


# Set correct permissions for .ssh and authorized_keys
chmod 700 "$ssh_dir"
if [ "$(ls -ld "$ssh_dir" | awk '{print $1}')" != "drwx------" ]; then
    chmod 700 "$ssh_dir"
fi

if [ -f "$authorized_keys_file" ]; then
    if [ "$(ls -l "$authorized_keys_file" | awk '{print $1}')" != "-rw-------" ]; then
        chmod 600 "$authorized_keys_file"
    fi
fi

echo "run sshd with port $SSH_PORT successfully"
exit 0