#!/bin/bash
# 运行测试脚本

set -e

echo "=========================================="
echo "运行测试"
echo "=========================================="

# 激活虚拟环境（如果存在）
if [ -d "venv" ]; then
    source venv/bin/activate
fi

# 安装测试依赖
pip install -q pytest pytest-asyncio pytest-cov

# 运行测试
echo ""
echo "运行单元测试..."
pytest tests/ -v

echo ""
echo "运行测试并生成覆盖率报告..."
pytest --cov=gpu_usage_agent --cov-report=html --cov-report=term tests/

echo ""
echo "=========================================="
echo "测试完成"
echo "覆盖率报告: htmlcov/index.html"
echo "=========================================="

