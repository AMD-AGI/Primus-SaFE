#!/bin/bash
# Primus-Lens Helm Chart 测试命令清单

set -e

CHART_DIR="charts/primus-lens"

echo "========================================"
echo "Primus-Lens Helm Chart 测试"
echo "========================================"
echo ""

# 1. Lint 测试
echo "1️⃣  Running Helm Lint..."
helm lint "$CHART_DIR"
echo "✅ Lint passed"
echo ""

# 2. 渲染所有模板
echo "2️⃣  Rendering all templates (all-in-one mode)..."
helm template primus-lens "$CHART_DIR" \
  -f "$CHART_DIR/examples/values-all-in-one.yaml" \
  > /tmp/primus-lens-all-in-one.yaml
echo "✅ Generated $(grep -c '^kind:' /tmp/primus-lens-all-in-one.yaml) resources"
echo ""

# 3. 测试 Management 模式
echo "3️⃣  Testing Management mode..."
helm template primus-lens "$CHART_DIR" \
  -f "$CHART_DIR/examples/values-management.yaml" \
  > /tmp/primus-lens-management.yaml
MGMT_RESOURCES=$(grep -c '^kind:' /tmp/primus-lens-management.yaml)
echo "✅ Management mode: $MGMT_RESOURCES resources"
echo ""

# 4. 测试 Data 模式
echo "4️⃣  Testing Data mode..."
helm template primus-lens "$CHART_DIR" \
  -f "$CHART_DIR/examples/values-data.yaml" \
  > /tmp/primus-lens-data.yaml
DATA_RESOURCES=$(grep -c '^kind:' /tmp/primus-lens-data.yaml)
echo "✅ Data mode: $DATA_RESOURCES resources"
echo ""

# 5. 验证条件渲染
echo "5️⃣  Verifying conditional rendering..."

# 检查 Management 组件
MGMT_API=$(grep -c "primus-lens-api" /tmp/primus-lens-management.yaml || echo 0)
DATA_API=$(grep -c "primus-lens-api" /tmp/primus-lens-data.yaml || echo 0)

if [ "$MGMT_API" -gt 0 ] && [ "$DATA_API" -eq 0 ]; then
    echo "✅ API only in management mode: correct"
else
    echo "❌ API rendering issue"
fi

# 检查 Data 组件
MGMT_NODE_EXPORTER=$(grep -c "node-exporter" /tmp/primus-lens-management.yaml || echo 0)
DATA_NODE_EXPORTER=$(grep -c "node-exporter" /tmp/primus-lens-data.yaml || echo 0)

if [ "$DATA_NODE_EXPORTER" -gt 0 ] && [ "$MGMT_NODE_EXPORTER" -eq 0 ]; then
    echo "✅ Node Exporter only in data mode: correct"
else
    echo "❌ Node Exporter rendering issue"
fi
echo ""

# 6. 检查 Hook 权重
echo "6️⃣  Checking Hook weights..."
echo "System Tuner hooks:"
grep -A 2 "helm.sh/hook-weight" /tmp/primus-lens-all-in-one.yaml | grep -A 1 "system-tuner" | head -3

echo ""
echo "Operator installation hooks:"
grep -B 2 "helm.sh/hook-weight.*\"[0-9][0-9]\"" /tmp/primus-lens-all-in-one.yaml | grep "name:" | head -5
echo ""

# 7. 检查 System Tuner
echo "7️⃣  Verifying System Tuner..."
if grep -q "kind: DaemonSet" /tmp/primus-lens-all-in-one.yaml && \
   grep -q "system-tuner" /tmp/primus-lens-all-in-one.yaml; then
    echo "✅ System Tuner DaemonSet found"
else
    echo "❌ System Tuner not found"
fi
echo ""

# 8. 检查中间件 Operators
echo "8️⃣  Checking Middleware Operators..."
OPERATORS=(
    "install-pg-operator"
    "install-opensearch-operator"
    "install-vm-operator"
    "install-fluentbit-operator"
    "install-grafana-operator"
)

for op in "${OPERATORS[@]}"; do
    if grep -q "$op" /tmp/primus-lens-all-in-one.yaml; then
        echo "  ✅ $op"
    else
        echo "  ❌ $op not found"
    fi
done
echo ""

# 9. 检查中间件实例
echo "9️⃣  Checking Middleware Instances..."
INSTANCES=(
    "PostgresCluster"
    "OpenSearchCluster"
    "VMCluster"
    "otel-collector"
)

for inst in "${INSTANCES[@]}"; do
    if grep -q "$inst" /tmp/primus-lens-all-in-one.yaml; then
        echo "  ✅ $inst"
    else
        echo "  ❌ $inst not found"
    fi
done
echo ""

# 10. 统计信息
echo "🔟  Statistics:"
echo "----------------------------------------"
echo "Total Resources (all-in-one): $ALL_RESOURCES"
echo "Management Mode Resources: $MGMT_RESOURCES"
echo "Data Mode Resources: $DATA_RESOURCES"
echo ""
echo "Resource Types:"
grep "^kind:" /tmp/primus-lens-all-in-one.yaml | sort | uniq -c | sort -rn
echo ""

# 11. 生成的文件
echo "📁 Generated files:"
echo "  - /tmp/primus-lens-all-in-one.yaml"
echo "  - /tmp/primus-lens-management.yaml"
echo "  - /tmp/primus-lens-data.yaml"
echo ""

echo "========================================"
echo "✅ All tests completed!"
echo "========================================"
echo ""
echo "Next steps:"
echo "1. Review generated YAML files in /tmp/"
echo "2. Check REMAINING_WORK.md for completion tasks"
echo "3. Build Docker images for components"
echo "4. Run dry-run: helm install primus-lens $CHART_DIR --dry-run --debug"
echo ""

