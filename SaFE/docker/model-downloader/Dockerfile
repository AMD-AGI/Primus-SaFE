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

# Install huggingface_hub[cli] and awscli
# --no-cache-dir to keep image size small
RUN pip install --no-cache-dir "huggingface_hub[cli]" awscli

# Create a stable symlink to the installed console script if it lives in a non-standard location
# (some environments install console scripts into /root/.local/bin)
RUN HFC_PATH="$(python -c 'import shutil; import sys; print(shutil.which(\"huggingface-cli\") or shutil.which(\"huggingface\") or \"\")')" \
    && if [ -n "$HFC_PATH" ]; then ln -sf "$HFC_PATH" /usr/local/bin/huggingface-cli || true; fi \
    && echo "huggingface-cli resolved at: $HFC_PATH" || true

# Verify installation in a robust way:
#  - check python import (required)
#  - if CLI exists, print its version (optional)
#  - check aws
RUN python -c "import huggingface_hub as hf; print('huggingface_hub', getattr(hf, '__version__', 'unknown'))" \
  && ( command -v huggingface-cli >/dev/null 2>&1 && huggingface-cli --version || ( command -v huggingface >/dev/null 2>&1 && huggingface --version ) || echo 'warning: huggingface-cli not found, but python package is installed' ) \
  && aws --version

# Set workdir
WORKDIR /workspace

# Default command (can be overridden by K8s Job)
CMD ["/bin/bash"]

# Build:
# docker build -t harbor.tas.primus-safe.amd.com/proxy/primussafe/model-downloader:latest .
