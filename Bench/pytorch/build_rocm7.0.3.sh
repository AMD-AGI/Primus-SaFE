docker build -f pytorch/Dockerfile \
  --build-arg ROCM_VERSION=7.0.3 \
  -t primussafe/pytorch:rocm7.0.3_ubuntu22.04_py3.10
