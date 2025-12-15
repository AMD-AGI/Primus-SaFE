if [ -d "/opt/TransferBench" ]; then
  exit 0
fi

REPO_URL="https://github.com/ROCm/TransferBench.git"
cd /opt
git clone "$REPO_URL" >/dev/null
if [ $? -ne 0 ]; then
  exit 1
fi

cd "./TransferBench" || exit 1
# current supports mi300x or mi325x
CC=hipcc make GPU_TARGETS=gfx950 > /dev/null
if [ $? -ne 0 ]; then
  exit 1
fi