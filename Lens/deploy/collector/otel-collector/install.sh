#!/bin/bash

helm repo add open-telemetry https://open-telemetry.github.io/opentelemetry-helm-charts

helm repo update

helm install opentelemetry-operator open-telemetry/opentelemetry-operator -n primus-lens -f values.yaml