if [ -d "/opt/rccl-tests" ]; then
  exit 0
fi

REPO_URL="https://github.com/ROCm/rccl-tests.git"
cd /opt && git clone "$REPO_URL" >/dev/null
if [ $? -ne 0 ]; then
  exit 1
fi

cd "./rccl-tests" || exit 1
make MPI=1 MPI_HOME=/usr/lib/x86_64-linux-gnu/openmpi NCCL_HOME=/opt/rocm/ >/dev/null
if [ $? -ne 0 ]; then
  exit 1
fi