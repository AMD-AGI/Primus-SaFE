FROM rocm/megatron-lm:v25.5_py310

RUN apt-get update && apt-get install -y \
    curl \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /opt/workload-exporter

COPY . /opt/workload-exporter

RUN python3 -m venv /opt/venv \
    && /opt/venv/bin/pip install --upgrade pip \
    && /opt/venv/bin/pip install -e .

ENV PATH="/opt/venv/bin:$PATH"

CMD ["bash", "run.sh"]
