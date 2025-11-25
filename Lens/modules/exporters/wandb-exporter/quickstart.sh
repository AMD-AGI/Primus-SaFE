#!/bin/bash
# Primus Lens WandB Exporter 快速开始脚本

set -e

echo "╔══════════════════════════════════════════════════════════╗"
echo "║     Primus Lens WandB Exporter - Quick Start            ║"
echo "╚══════════════════════════════════════════════════════════╝"
echo ""

# 检测操作系统
OS="$(uname -s)"
case "${OS}" in
    Linux*)     MACHINE=Linux;;
    Darwin*)    MACHINE=Mac;;
    *)          MACHINE="UNKNOWN:${OS}"
esac

echo "检测到操作系统: $MACHINE"
echo ""

# 步骤 1: 安装包
echo "步骤 1/4: 安装 Primus Lens WandB Exporter..."
echo "----------------------------------------"
pip install -e . || {
    echo "❌ 安装失败！可能需要管理员权限。"
    echo "请尝试: sudo pip install -e ."
    exit 1
}
echo "✅ 安装成功！"
echo ""

# 步骤 2: 验证安装
echo "步骤 2/4: 验证 .pth 文件安装..."
echo "----------------------------------------"
python install_hook.py check
echo ""

# 步骤 3: 运行测试
echo "步骤 3/4: 运行测试..."
echo "----------------------------------------"
python test_wandb_hook.py || {
    echo "⚠️  部分测试失败，但这可能是正常的（如果 wandb 未安装）"
}
echo ""

# 步骤 4: 设置环境变量
echo "步骤 4/4: 环境变量说明..."
echo "----------------------------------------"
echo "你可以设置以下环境变量来控制行为："
echo ""
echo "  export PRIMUS_LENS_WANDB_HOOK=true"
echo "  export PRIMUS_LENS_WANDB_ENHANCE_METRICS=true"
echo "  export PRIMUS_LENS_WANDB_SAVE_LOCAL=true"
echo "  export PRIMUS_LENS_WANDB_OUTPUT_PATH=/tmp/metrics"
echo ""

# 完成
echo "╔══════════════════════════════════════════════════════════╗"
echo "║                  🎉 安装完成！                          ║"
echo "╚══════════════════════════════════════════════════════════╝"
echo ""
echo "接下来你可以："
echo ""
echo "1. 运行示例代码:"
echo "   python example_usage.py"
echo ""
echo "2. 在你的训练脚本中正常使用 wandb（无需修改代码）:"
echo "   python your_training_script.py"
echo ""
echo "3. 查看文档:"
echo "   - README.md      - 英文用户指南"
echo "   - 使用指南.md    - 中文使用指南"
echo "   - INSTALL.md     - 安装详解"
echo ""
echo "如需卸载 hook:"
echo "   python install_hook.py uninstall"
echo ""

