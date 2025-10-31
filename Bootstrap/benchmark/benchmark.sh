#!/usr/bin/env bash
wget https://github.com/kube-burner/kube-burner/releases/download/v1.16.3/kube-burner-V1.16.3-linux-x86_64.tar.gz
tar -xvzf kube-burner-V1.16.3-linux-x86_64.tar.gz
rm kube-burner-V1.16.3-linux-x86_64.tar.gz

./kube-burner init -c api-intensive.yml

bash parse_pod_latency.sh podLatencyMeasurement-api-intensive.json
bash parse_pod_latency.sh podLatencyMeasurement-ensure-pods-removal.json
bash parse_pod_quantiles.sh podLatencyQuantilesMeasurement-api-intensive.json
bash parse_pod_quantiles.sh podLatencyQuantilesMeasurement-ensure-pods-removal.json

bash parse_node_latency.sh nodeLatencyMeasurement-api-intensive.json
bash parse_node_latency.sh nodeLatencyMeasurement-ensure-pods-removal.json
bash parse_node_quantiles.sh nodeLatencyQuantilesMeasurement-api-intensive.json
bash parse_node_quantiles.sh nodeLatencyQuantilesMeasurement-ensure-pods-removal.json