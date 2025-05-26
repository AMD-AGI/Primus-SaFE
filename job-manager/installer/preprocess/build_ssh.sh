#!/bin/bash

cd /shared-data

dpkg -l | grep openssh-server
if [ $? -ne 0 ]; then
    apt-get update > /dev/null 2>&1
    timeout 180s install -y openssh-server > /dev/null 2>&1
    ret=$?
    if [ $ret != 0 ]; then
       timeout 180s apt -y --fix-broken install > /dev/null 2>&1
       timeout 180s apt-get install -y openssh-server > /dev/null 2>&1
       ret=$?
    fi
    if [ $ret != 0 ]; then
        echo "openssh-server cannot be installed correctly, $ret. pls close the switch for ssh"
        exit $ret
    fi
fi

echo root:root | chpasswd
ret=$?
if [ $ret != 0 ]; then
    echo "fail to change password! $ret"
    exit $ret
fi

# sed -i "s/^#\(Port\) .*/Port $SSH_PORT/" /etc/ssh/sshd_config
# sed -i "s/^#PermitRootLogin .*/PermitRootLogin yes/" /etc/ssh/sshd_config
sed -i '/Port /d' /etc/ssh/sshd_config
sed -i '/PermitRootLogin yes/d' /etc/ssh/sshd_config
echo "Port $SSH_PORT" >> /etc/ssh/sshd_config
echo "PermitRootLogin yes" >> /etc/ssh/sshd_config
sed -i "s/^AllowUsers/#AllowUsers/" /etc/ssh/sshd_config
sed -i 's/# UsePAM yes/UsePAM yes/' /etc/ssh/sshd_config

service ssh restart > /dev/null 2>&1
service ssh status | grep "sshd is running"
ret=$?
if [ $ret != 0 ]; then
    echo "sshd is not running! $ret"
    exit $ret
fi

ssh_dir="$HOME/.ssh"
authorized_keys_file="$ssh_dir/authorized_keys"
if [ ! -d "$ssh_dir" ]; then
    mkdir -p "$ssh_dir"
fi

if [ ! -e "$authorized_keys_file" ]; then
    touch "$authorized_keys_file"
fi

permissions1=$(stat -c '%a' "$ssh_dir")
if [ "$permissions1" -ne 700 ]; then
    chmod 700 "$ssh_dir"
fi

permissions2=$(stat -c '%a' "$authorized_keys_file")
if [ "$permissions2" -ne 600 ]; then
    chmod 600 "$authorized_keys_file"
fi

echo "run sshd with port $SSH_PORT successfully"
exit 0
