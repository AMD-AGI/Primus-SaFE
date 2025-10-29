#!/bin/bash

helm repo add vm https://victoriametrics.github.io/helm-charts/

helm repo update

helm install -n primus-lens primus-lens-vm vm/victoria-metrics-operator -f values.yaml