if [ -d "/opt/rocBLAS" ]; then
  exit 0
fi

REPO_URL="https://github.com/ROCm/rocBLAS.git"
cd /opt
git clone --branch rocm-7.1.1 --depth 1 "$REPO_URL"
if [ $? -ne 0 ]; then
  exit 1
fi

cd "./rocBLAS" || exit 1
chmod +x ./install.sh && ./install.sh --clients-only --clients_no_fortran --library-path /opt/rocm >/dev/null
if [ $? -ne 0 ]; then
  exit 1
fi