# Image of the pod that prewarms a model on a node without node-agent.
# The prewarm script needs bash, find, xargs, stat, sort and awk; the launcher needs base64.
FROM debian:bookworm-slim

ENV DEBIAN_FRONTEND=noninteractive

RUN apt-get update && apt-get install -y --no-install-recommends \
    bash \
    coreutils \
    findutils \
    gawk \
    && rm -rf /var/lib/apt/lists/*

CMD ["/bin/bash"]

# Build:
# docker build -t harbor.tas.primus-safe.amd.com/proxy/primussafe/model-prewarm:latest .
