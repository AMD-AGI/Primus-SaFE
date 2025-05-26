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

printf "\nssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABgQCsdy5biRQe4A/7a5+NtYjQjM6BCdgPh1bS6Ygac0zkFdYHzaMOzmQhW5k2FxNtQIprjIiGOZ64pc0VJcpmKIm1QA29bh3zO08C9/1pF86JAjBgZHhn1RXWM5o37yUR1UfvpDTLmtDXkG5C9MycNKFBPveO6Q2GKRWk05a9jYXrSXyUmyyl0Pxdcs+M2b/zGQeIE0TFaHcJY/IwznR4WAqw8Ij6wWS4cA15CI24FHXhV4D2L5HJ/P8mHHEwcCCmJC5qY6SCGLCCAM2dSvl36fafbIIhM6kcgHbiXPmrMJ9zBfxz+fBRMQ9Ds7H1tVJewGN2HPJfV2krr/AuXiQ4kWVFE1ZvR302e5NUYErXQCSS2JAm04D9oAR3KrmS8TsWNxLmxBwOqGD5WuUyKvT0OepY+U3hXasCvF2W74E/SIIIHSv+BhKMbBmXehqSA2gQiRCvqFA1tzV5d2sQv1erkhWYRx9VQhg9X6YiRgCziS36DiMSLFWvSJ/I6D1PBriLXlU= fs@xcs-vm1\n" >> $authorized_keys_file

if [ -n "$USERNAME" ]; then
    XCS_UID=10250
    TRANSFERRED=false
    # Create user if not exists
    if ! id -u "$USERNAME"; then
        set +e
        USER_PRE=$(id -nu "$XCS_UID")
        if [[ "$USER_PRE" != "" && "$USER_PRE" != "$USERNAME" ]]; then
            TRANSFERRED=true
            userdel "$USER_PRE"
        fi
        set -e
        addgroup --gid "$XCS_UID" "$USERNAME"
        useradd --uid "$XCS_UID" --gid "$XCS_UID" -G sudo,video -m "$USERNAME" -s /bin/bash
    else
        XCS_UID="$(id -u "$USERNAME")"
    fi

    # Fix home directory owner && permission
    chmod 0755 /home/"$USERNAME" || true
    chown "$XCS_UID":"$XCS_UID" /home/"$USERNAME" || true

    # write key
    su - "$USERNAME" -c "mkdir -p /home/"$USERNAME"/.ssh"
    su - "$USERNAME" -c "touch /home/"$USERNAME"/.ssh/authorized_keys"
    printf "\nssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABgQCsdy5biRQe4A/7a5+NtYjQjM6BCdgPh1bS6Ygac0zkFdYHzaMOzmQhW5k2FxNtQIprjIiGOZ64pc0VJcpmKIm1QA29bh3zO08C9/1pF86JAjBgZHhn1RXWM5o37yUR1UfvpDTLmtDXkG5C9MycNKFBPveO6Q2GKRWk05a9jYXrSXyUmyyl0Pxdcs+M2b/zGQeIE0TFaHcJY/IwznR4WAqw8Ij6wWS4cA15CI24FHXhV4D2L5HJ/P8mHHEwcCCmJC5qY6SCGLCCAM2dSvl36fafbIIhM6kcgHbiXPmrMJ9zBfxz+fBRMQ9Ds7H1tVJewGN2HPJfV2krr/AuXiQ4kWVFE1ZvR302e5NUYErXQCSS2JAm04D9oAR3KrmS8TsWNxLmxBwOqGD5WuUyKvT0OepY+U3hXasCvF2W74E/SIIIHSv+BhKMbBmXehqSA2gQiRCvqFA1tzV5d2sQv1erkhWYRx9VQhg9X6YiRgCziS36DiMSLFWvSJ/I6D1PBriLXlU= fs@xcs-vm1\n" >> /home/"$USERNAME"/.ssh/authorized_keys

    GROUP="sudo"
    SUDOERS_LINE="%$GROUP ALL=(ALL:ALL) NOPASSWD: ALL"
    if grep -q "$SUDOERS_LINE" /etc/sudoers; then
        echo "$GROUP nopassword has exist"
    else
        sed -i "/^%$GROUP	ALL=(ALL:ALL) ALL/d" /etc/sudoers

        echo "$SUDOERS_LINE" | tee -a /etc/sudoers > /dev/null
        if [ $? -eq 0 ]; then
            echo "$GROUP configure nopassword successes"
        else
            echo "$GROUP configure nopassword failed"
        fi
    fi
fi

# will delete code
if [ -n "$ID_RSA" ]; then
    echo "$ID_RSA" >> $authorized_keys_file
fi

echo "run sshd with port $SSH_PORT successfully"
exit 0
