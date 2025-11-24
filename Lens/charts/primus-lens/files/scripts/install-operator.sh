#!/bin/bash
# 通用的 Operator 安装脚本

set -ex

OPERATOR_NAME="$1"
REPO_URL="$2"
CHART_NAME="$3"
NAMESPACE="$4"
VALUES_FILE="$5"

echo "Installing $OPERATOR_NAME Operator..."

# 添加 Helm Repo
helm repo add "$OPERATOR_NAME" "$REPO_URL" || true
helm repo update

# 检查是否已安装
if helm list -n "$NAMESPACE" | grep -q "$OPERATOR_NAME"; then
  echo "$OPERATOR_NAME Operator already installed, checking for updates..."
  helm upgrade "$OPERATOR_NAME" "$CHART_NAME" \
    --namespace "$NAMESPACE" \
    --reuse-values \
    --wait \
    --timeout 5m
else
  echo "Installing $OPERATOR_NAME Operator..."
  helm install "$OPERATOR_NAME" "$CHART_NAME" \
    --namespace "$NAMESPACE" \
    --create-namespace \
    --wait \
    --timeout 5m \
    ${VALUES_FILE:+-f $VALUES_FILE}
fi

echo "✅ $OPERATOR_NAME Operator installed successfully"

