# Use lightweight Python base image
FROM python:3.9-slim

# Set non-interactive mode
ENV DEBIAN_FRONTEND=noninteractive

# Ensure pip-installed scripts under root's local bin are in PATH
ENV PATH="/root/.local/bin:/usr/local/bin:${PATH}"

# Install system dependencies (git is often needed for hf cli) and ca-certificates
RUN apt-get update && apt-get install -y --no-install-recommends \
    git \
    ca-certificates \
    curl \
    && rm -rf /var/lib/apt/lists/*

# Upgrade pip first (optional but recommended)
RUN pip install --no-cache-dir --upgrade pip

# Install huggingface_hub[cli] with fixed version and awscli
# --no-cache-dir to keep image size small
RUN pip install --no-cache-dir "huggingface_hub[cli]==0.24.0" awscli

# Ensure huggingface-cli is in PATH
RUN ln -sf $(which huggingface-cli || echo /usr/local/bin/huggingface-cli) /usr/local/bin/huggingface-cli 2>/dev/null || true

# Verify installation
RUN python -c "import huggingface_hub as hf; print('huggingface_hub version:', hf.__version__)" \
  && command -v huggingface-cli \
  && aws --version

# Set workdir
WORKDIR /workspace

# Default command (can be overridden by K8s Job)
CMD ["/bin/bash"]

# Build:
# docker build -t harbor.tas.primus-safe.amd.com/proxy/primussafe/model-downloader:latest .
