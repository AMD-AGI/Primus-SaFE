ARG REGISTRY=docker.io

FROM ${REGISTRY}/library/ubuntu:22.04

ARG APP_HOME=/cicd/runner-proxy
RUN mkdir -p ${APP_HOME} && mkdir -p ${APP_HOME} && chmod -R  750 ${APP_HOME}

RUN apt update && \
    apt install -y --no-install-recommends \
        python3 python3-pip python3-distutils python3-setuptools python3-wheel && \
    apt clean && rm -rf /var/lib/apt/lists/*

COPY ./SaFE/docker/cicd/runner-proxy/requirements.txt ${APP_HOME}

# Installed here rather than at startup: a runner behind a proxy that does not
# serve the package index cannot reach it, and the container exits before it
# ever registers.
RUN pip install --no-cache-dir -r ${APP_HOME}/requirements.txt

COPY ./SaFE/docker/cicd/runner-proxy/*.sh ${APP_HOME}
COPY ./SaFE/docker/cicd/runner-proxy/*.py ${APP_HOME}

RUN chmod +x ${APP_HOME}/*.sh
RUN chmod +x ${APP_HOME}/*.py

WORKDIR ${APP_HOME}
CMD ["/bin/bash", "run.sh"]
