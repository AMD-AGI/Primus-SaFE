#!/usr/bin/env bash

hostname=$(hostname)

ETCDCTL_API=3 etcdctl \
  --endpoints=https://127.0.0.1:2379 \
  --cacert=/etc/ssl/etcd/ssl/ca.pem \
  --cert=/etc/ssl/etcd/ssl/node-${hostname}.pem \
  --key=/etc/ssl/etcd/ssl/node-${hostname}-key.pem check perf \