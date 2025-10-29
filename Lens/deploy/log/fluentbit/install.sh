#!/bin/bash

helm repo add fluent https://fluent.github.io/helm-charts

helm install fluent-operator fluent/fluent-operator -n primus-lens -f values.yaml