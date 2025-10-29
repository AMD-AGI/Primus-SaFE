#!/bin/bash
helm repo add opensearch-operator https://opensearch-project.github.io/opensearch-k8s-operator/

helm install -n primus-lens opensearch-operator  opensearch-operator/opensearch-operator -f opensearch-operators.yaml