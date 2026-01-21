FROM docker.io/primussafe/buildah-runner:v2.329.0-3

USER root

ARG RUNNER_VERSION
ARG TARGETARCH=amd64

WORKDIR /home/runner/actions-runner

# Remove old runner files (keep externals if needed)
RUN rm -rf bin *.json *.sh run-helper.sh.template || true

# Download and install the new runner version
# If RUNNER_VERSION is not provided, fetch the latest version from GitHub API
RUN if [ -z "$RUNNER_VERSION" ]; then \
        RUNNER_VERSION=$(curl -s https://api.github.com/repos/actions/runner/releases/latest | grep '"tag_name":' | sed -E 's/.*"v([^"]+)".*/\1/'); \
    fi && \
    echo "Installing GitHub Actions Runner version: ${RUNNER_VERSION}" && \
    curl -fsSL -o runner.tar.gz "https://github.com/actions/runner/releases/download/v${RUNNER_VERSION}/actions-runner-linux-x64-${RUNNER_VERSION}.tar.gz" && \
    tar xzf runner.tar.gz && \
    rm -f runner.tar.gz && \
    ./bin/installdependencies.sh || true

# Ensure runner user exists and fix permissions
RUN id runner 2>/dev/null || useradd -m runner && \
    mkdir -p /home/runner/.config /home/runner/.local/share/containers && \
    chown -R runner:runner /home/runner

USER runner

WORKDIR /home/runner/actions-runner
