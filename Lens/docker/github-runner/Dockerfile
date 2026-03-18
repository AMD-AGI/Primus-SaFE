# GitHub Actions Runner with Docker support
# Based on Ubuntu 22.04 with Docker and common build tools

ARG REGISTRY=""
FROM ${REGISTRY}ubuntu:22.04

USER root

ARG RUNNER_VERSION
ARG TARGETARCH=amd64

ENV DEBIAN_FRONTEND=noninteractive

# Install essential packages and Docker
RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates \
    curl \
    gnupg \
    lsb-release \
    git \
    jq \
    wget \
    unzip \
    zip \
    sudo \
    software-properties-common \
    build-essential \
    python3 \
    python3-pip \
    nodejs \
    npm \
    && rm -rf /var/lib/apt/lists/*

# Install Docker
RUN install -m 0755 -d /etc/apt/keyrings && \
    curl -fsSL https://download.docker.com/linux/ubuntu/gpg -o /etc/apt/keyrings/docker.asc && \
    chmod a+r /etc/apt/keyrings/docker.asc && \
    echo "deb [arch=$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/docker.asc] https://download.docker.com/linux/ubuntu \
    $(. /etc/os-release && echo "$VERSION_CODENAME") stable" | tee /etc/apt/sources.list.d/docker.list > /dev/null && \
    apt-get update && \
    apt-get install -y --no-install-recommends \
    docker-ce \
    docker-ce-cli \
    containerd.io \
    docker-buildx-plugin \
    docker-compose-plugin \
    && rm -rf /var/lib/apt/lists/*

# Create runner user
RUN useradd -m -s /bin/bash runner && \
    usermod -aG docker runner && \
    echo "runner ALL=(ALL) NOPASSWD:ALL" >> /etc/sudoers

# Setup actions-runner directory
WORKDIR /home/runner/actions-runner

# Download and install GitHub Actions Runner
# If RUNNER_VERSION is not provided, fetch the latest version from GitHub API
RUN if [ -z "$RUNNER_VERSION" ]; then \
        RUNNER_VERSION=$(curl -s https://api.github.com/repos/actions/runner/releases/latest | grep '"tag_name":' | sed -E 's/.*"v([^"]+)".*/\1/'); \
    fi && \
    echo "Installing GitHub Actions Runner version: ${RUNNER_VERSION}" && \
    curl -fsSL -o runner.tar.gz "https://github.com/actions/runner/releases/download/v${RUNNER_VERSION}/actions-runner-linux-x64-${RUNNER_VERSION}.tar.gz" && \
    tar xzf runner.tar.gz && \
    rm -f runner.tar.gz && \
    ./bin/installdependencies.sh || true

# Fix permissions
RUN mkdir -p /home/runner/.config /home/runner/.local/share/containers && \
    chown -R runner:runner /home/runner

USER runner

WORKDIR /home/runner/actions-runner
