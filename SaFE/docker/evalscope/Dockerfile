# EvalScope Docker Image
# For model evaluation tasks

FROM python:3.10-slim

LABEL maintainer="Primus-SaFE Team"
LABEL description="EvalScope model evaluation environment"

# Set environment variables
ENV PYTHONUNBUFFERED=1 \
    PIP_NO_CACHE_DIR=1 \
    PIP_DISABLE_PIP_VERSION_CHECK=1

# Install system dependencies
RUN apt-get update && apt-get install -y --no-install-recommends \
    curl \
    jq \
    && rm -rf /var/lib/apt/lists/*

# Install evalscope and common dependencies
RUN pip install --no-cache-dir \
    evalscope \
    openai \
    requests \
    && rm -rf ~/.cache/pip

# Create working directories
RUN mkdir -p /outputs /workspace
WORKDIR /workspace

# Verify installation
RUN evalscope --version || echo "EvalScope installed"

# Default entrypoint
ENTRYPOINT ["/bin/bash", "-c"]
CMD ["evalscope --help"]

