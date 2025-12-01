#!/bin/bash
#
# Primus Lens WandB Exporter - PyPI 发布脚本
#
# 使用方法：
#   1. 设置环境变量：
#      export PYPI_TOKEN="pypi-AgEIcHlwaS5vcmcC..."
#      export TESTPYPI_TOKEN="pypi-AgEI..." (可选，用于测试)
#
#   2. 运行脚本：
#      ./publish.sh [--test] [--skip-tests] [--skip-build]
#
# 参数：
#   --test          上传到 TestPyPI 而不是正式 PyPI
#   --skip-tests    跳过测试阶段
#   --skip-build    跳过构建阶段（重用已有的 dist/）
#   --help          显示帮助信息
#

set -e  # 遇到错误立即退出

# 颜色输出
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# 日志函数
log_info() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

log_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

log_warning() {
    echo -e "${YELLOW}[WARNING]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# 显示帮助信息
show_help() {
    cat << EOF
Primus Lens WandB Exporter - PyPI 发布脚本

使用方法:
    ./publish.sh [选项]

环境变量:
    PYPI_TOKEN          PyPI API Token (必需)
    TESTPYPI_TOKEN      TestPyPI API Token (使用 --test 时必需)

选项:
    --test              上传到 TestPyPI 进行测试
    --skip-tests        跳过测试阶段
    --skip-build        跳过构建阶段（重用已有的 dist/）
    --help              显示此帮助信息

示例:
    # 发布到正式 PyPI
    export PYPI_TOKEN="pypi-AgEIcHlwaS5vcmcC..."
    ./publish.sh

    # 先测试发布到 TestPyPI
    export TESTPYPI_TOKEN="pypi-AgEI..."
    ./publish.sh --test

    # 跳过测试直接发布
    ./publish.sh --skip-tests

获取 PyPI Token:
    1. 访问 https://pypi.org/manage/account/token/
    2. 创建新的 API token
    3. 复制 token 并设置为环境变量

EOF
}

# 解析命令行参数
USE_TESTPYPI=false
SKIP_TESTS=false
SKIP_BUILD=false

while [[ $# -gt 0 ]]; do
    case $1 in
        --test)
            USE_TESTPYPI=true
            shift
            ;;
        --skip-tests)
            SKIP_TESTS=true
            shift
            ;;
        --skip-build)
            SKIP_BUILD=true
            shift
            ;;
        --help)
            show_help
            exit 0
            ;;
        *)
            log_error "未知参数: $1"
            show_help
            exit 1
            ;;
    esac
done

# 检查环境变量
if [ "$USE_TESTPYPI" = true ]; then
    if [ -z "$TESTPYPI_TOKEN" ]; then
        log_error "TESTPYPI_TOKEN 环境变量未设置"
        echo "请运行: export TESTPYPI_TOKEN=\"your-token-here\""
        exit 1
    fi
    PYPI_TOKEN="$TESTPYPI_TOKEN"
    REPOSITORY="testpypi"
    REPOSITORY_URL="https://test.pypi.org/legacy/"
else
    if [ -z "$PYPI_TOKEN" ]; then
        log_error "PYPI_TOKEN 环境变量未设置"
        echo "请运行: export PYPI_TOKEN=\"your-token-here\""
        exit 1
    fi
    REPOSITORY="pypi"
    REPOSITORY_URL="https://upload.pypi.org/legacy/"
fi

# 获取脚本所在目录
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$SCRIPT_DIR"

echo ""
echo "╔════════════════════════════════════════════════════════════════╗"
echo "║    Primus Lens WandB Exporter - PyPI 发布工具                 ║"
echo "╚════════════════════════════════════════════════════════════════╝"
echo ""

log_info "工作目录: $SCRIPT_DIR"
log_info "目标仓库: $REPOSITORY"
echo ""

# 步骤 1: 检查必要的工具
log_info "步骤 1/6: 检查必要的工具..."

if ! command -v python3 &> /dev/null; then
    log_error "Python3 未安装"
    exit 1
fi

PYTHON_VERSION=$(python3 --version)
log_success "Python: $PYTHON_VERSION"

# 检查虚拟环境
if [ ! -d ".venv" ]; then
    log_warning "虚拟环境不存在，正在创建..."
    python3 -m venv .venv
fi

# 激活虚拟环境
source .venv/bin/activate

# 安装必要的构建工具
log_info "安装构建工具..."
pip install --upgrade pip build twine > /dev/null 2>&1

log_success "工具检查完成"
echo ""

# 步骤 2: 运行测试
if [ "$SKIP_TESTS" = false ]; then
    log_info "步骤 2/6: 运行测试套件..."
    
    # 设置测试环境变量
    export PRIMUS_LENS_WANDB_HOOK=true
    export WANDB_MODE=offline
    export WANDB_SILENT=true
    
    if python3 test_real_scenario.py --scenario basic; then
        log_success "基础测试通过"
    else
        log_error "测试失败"
        echo ""
        read -p "是否继续发布？(y/N): " -n 1 -r
        echo ""
        if [[ ! $REPLY =~ ^[Yy]$ ]]; then
            log_info "发布已取消"
            exit 1
        fi
    fi
    echo ""
else
    log_warning "步骤 2/6: 跳过测试"
    echo ""
fi

# 步骤 3: 清理旧的构建文件
if [ "$SKIP_BUILD" = false ]; then
    log_info "步骤 3/6: 清理旧的构建文件..."
    
    rm -rf build/ dist/ *.egg-info src/*.egg-info
    
    log_success "清理完成"
    echo ""
else
    log_warning "步骤 3/6: 跳过清理（保留现有构建）"
    echo ""
fi

# 步骤 4: 构建包
if [ "$SKIP_BUILD" = false ]; then
    log_info "步骤 4/6: 构建包..."
    
    python3 -m build
    
    if [ $? -eq 0 ]; then
        log_success "包构建成功"
        echo ""
        log_info "构建产物:"
        ls -lh dist/
    else
        log_error "包构建失败"
        exit 1
    fi
    echo ""
else
    log_warning "步骤 4/6: 跳过构建"
    echo ""
fi

# 步骤 5: 检查包
log_info "步骤 5/6: 检查包完整性..."

twine check dist/*

if [ $? -eq 0 ]; then
    log_success "包检查通过"
else
    log_error "包检查失败"
    exit 1
fi
echo ""

# 步骤 6: 上传到 PyPI
log_info "步骤 6/6: 上传到 $REPOSITORY..."
echo ""

if [ "$USE_TESTPYPI" = true ]; then
    log_warning "这是测试上传到 TestPyPI"
    log_warning "安装测试包: pip install --index-url https://test.pypi.org/simple/ primus-lens-wandb-exporter"
else
    log_warning "这是正式上传到 PyPI，请确认！"
fi
echo ""

read -p "确认上传？(y/N): " -n 1 -r
echo ""

if [[ ! $REPLY =~ ^[Yy]$ ]]; then
    log_info "上传已取消"
    exit 0
fi

# 使用 twine 上传，通过环境变量传递 token
export TWINE_USERNAME="__token__"
export TWINE_PASSWORD="$PYPI_TOKEN"

if [ "$USE_TESTPYPI" = true ]; then
    twine upload --repository-url "$REPOSITORY_URL" dist/*
else
    twine upload dist/*
fi

if [ $? -eq 0 ]; then
    echo ""
    log_success "上传成功！"
    echo ""
    echo "╔════════════════════════════════════════════════════════════════╗"
    echo "║                    🎉 发布成功！                                ║"
    echo "╚════════════════════════════════════════════════════════════════╝"
    echo ""
    
    if [ "$USE_TESTPYPI" = true ]; then
        echo "测试安装命令:"
        echo "  pip install --index-url https://test.pypi.org/simple/ primus-lens-wandb-exporter"
    else
        echo "安装命令:"
        echo "  pip install primus-lens-wandb-exporter"
        echo ""
        echo "包页面:"
        echo "  https://pypi.org/project/primus-lens-wandb-exporter/"
    fi
    echo ""
else
    log_error "上传失败"
    exit 1
fi

# 清理环境变量
unset TWINE_USERNAME
unset TWINE_PASSWORD

log_info "发布流程完成"

