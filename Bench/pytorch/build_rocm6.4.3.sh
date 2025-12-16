docker build -f pytorch/Dockerfile \
  --build-arg ROCM_VERSION=6.4.3 \
  -t primussafe/pytorch:rocm6.4.3_ubuntu22.04_py3.10
