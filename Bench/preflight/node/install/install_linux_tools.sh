linux_tools="linux-tools-$(uname -r)"
dpkg -l | grep -q "$linux_tools"
if [ $? -ne 0 ]; then
  apt-get update >/dev/null && apt install -y "$linux_tools"  linux-tools-common linux-tools-6.8.0-60-generic linux-cloud-tools-6.8.0-60-generic >/dev/null
  if [ $? -ne 0 ]; then
    exit 1
  fi
fi
