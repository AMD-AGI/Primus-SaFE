#!/bin/bash
# GPU Usage Analysis Agent 启动脚本

set -e

echo "=========================================="
echo "启动 GPU Usage Analysis Agent"
echo "=========================================="

# 检查环境变量
if [ -z "$LLM_API_KEY" ]; then
    echo "警告: LLM_API_KEY 未设置"
    echo "请设置环境变量或在 .env 文件中配置"
fi


echo "配置信息:"
echo "  - Lens API URL: $LENS_API_URL"
echo "  - LLM Provider: $LLM_PROVIDER"
echo "  - LLM Model: $LLM_MODEL"
echo ""

# 检查 Python 版本
python_version=$(python --version 2>&1 | awk '{print $2}')
echo "Python 版本: $python_version"

# 安装依赖（如果需要）
if [ ! -d "venv" ]; then
    echo "创建虚拟环境..."
    python -m venv venv
fi

echo "激活虚拟环境..."
source venv/bin/activate

echo "安装依赖..."
pip install -q -r requirements.txt

echo ""
echo "=========================================="
echo "启动服务..."
echo "=========================================="
echo ""

# 启动服务（从根目录运行，不要 cd 到 api）
python -m api.main

