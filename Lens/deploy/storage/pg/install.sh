#!/bin/bash


git clone https://github.com/CrunchyData/postgres-operator-examples.git

helm install -n primus-lens -f values.yaml pg-operator postgres-operator-examples/helm/install

rm -rf postgres-operator-examples
