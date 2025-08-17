FROM primussafe/pytorch:rocm6.4.3_ubuntu22.04_py3.10
ENV DEBIAN_FRONTEND=noninteractive

RUN apt update && \
    apt install -y --no-install-recommends openssh-server && \
    apt clean

ARG APP_HOME=/opt/primus-safe/diagnoser
RUN mkdir -p ${APP_HOME}
COPY ./health-checker/rccl-diagnoser/*.py ${APP_HOME}
COPY ./health-checker/rccl-diagnoser/*.sh ${APP_HOME}

WORKDIR ${APP_HOME}
