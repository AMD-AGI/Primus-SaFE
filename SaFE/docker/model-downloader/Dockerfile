# Use lightweight Python base image
FROM python:3.9-slim

# Set non-interactive mode
ENV DEBIAN_FRONTEND=noninteractive

# Install system dependencies (git is often needed for hf cli)
RUN apt-get update && apt-get install -y --no-install-recommends \
    git \
    ca-certificates \
    && rm -rf /var/lib/apt/lists/*

# Install huggingface_hub[cli] and awscli
# --no-cache-dir to keep image size small
RUN pip install --no-cache-dir \
    "huggingface_hub[cli]" \
    awscli

# Verify installation
RUN huggingface-cli --version && aws --version

# Set workdir
WORKDIR /workspace

# Default command (can be overridden by K8s Job)
CMD ["/bin/bash"]

#docker build -t primus-safe/model-downloader:latest .

