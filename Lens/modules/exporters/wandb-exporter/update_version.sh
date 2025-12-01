#!/bin/bash
################################################################################
# Version Updater
# 一次性更新所有文件中的版本号
################################################################################

set -e

# Color definitions
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
NC='\033[0m' # No Color

print_success() {
    echo -e "${GREEN}✓${NC} $1"
}

print_error() {
    echo -e "${RED}✗${NC} $1"
}

print_info() {
    echo -e "${BLUE}ℹ${NC} $1"
}

print_warning() {
    echo -e "${YELLOW}⚠${NC} $1"
}

print_header() {
    echo -e "${CYAN}$1${NC}"
}

# 显示使用说明
show_usage() {
    echo "Usage: $0 <new_version>"
    echo
    echo "Example:"
    echo "  $0 0.3.0"
    echo "  $0 1.0.0-beta.1"
    echo
    echo "This script will update version numbers in:"
    echo "  1. src/primus_lens_wandb_exporter/__init__.py"
    echo "  2. setup.py"
    echo "  3. pyproject.toml"
    echo
}

# 验证版本号格式（语义化版本）
validate_version() {
    local version=$1
    # 支持格式：X.Y.Z 或 X.Y.Z-suffix（如 1.0.0-beta.1）
    if [[ ! $version =~ ^[0-9]+\.[0-9]+\.[0-9]+(-[a-zA-Z0-9.]+)?$ ]]; then
        print_error "Invalid version format: $version"
        echo "Version should follow semantic versioning (e.g., 1.0.0 or 1.0.0-beta.1)"
        return 1
    fi
    return 0
}

# 获取当前版本号
get_current_version() {
    if [ -f "src/primus_lens_wandb_exporter/__init__.py" ]; then
        grep "__version__" src/primus_lens_wandb_exporter/__init__.py | cut -d'"' -f2
    else
        echo "unknown"
    fi
}

# 备份文件
backup_file() {
    local file=$1
    cp "$file" "${file}.bak"
    print_info "Backed up: ${file}.bak"
}

# 恢复所有备份
restore_backups() {
    print_warning "Restoring backups..."
    [ -f "src/primus_lens_wandb_exporter/__init__.py.bak" ] && mv src/primus_lens_wandb_exporter/__init__.py.bak src/primus_lens_wandb_exporter/__init__.py
    [ -f "setup.py.bak" ] && mv setup.py.bak setup.py
    [ -f "pyproject.toml.bak" ] && mv pyproject.toml.bak pyproject.toml
    print_info "Backups restored"
}

# 删除所有备份
remove_backups() {
    rm -f src/primus_lens_wandb_exporter/__init__.py.bak
    rm -f setup.py.bak
    rm -f pyproject.toml.bak
}

# 更新 __init__.py
update_init_py() {
    local new_version=$1
    local file="src/primus_lens_wandb_exporter/__init__.py"
    
    if [ ! -f "$file" ]; then
        print_error "$file not found"
        return 1
    fi
    
    backup_file "$file"
    
    # 使用 sed 替换版本号
    if [[ "$OSTYPE" == "darwin"* ]]; then
        # macOS
        sed -i '' "s/__version__ = \".*\"/__version__ = \"$new_version\"/" "$file"
    else
        # Linux
        sed -i "s/__version__ = \".*\"/__version__ = \"$new_version\"/" "$file"
    fi
    
    # 验证修改
    local updated_version=$(grep "__version__" "$file" | cut -d'"' -f2)
    if [ "$updated_version" = "$new_version" ]; then
        print_success "Updated $file"
        return 0
    else
        print_error "Failed to update $file"
        return 1
    fi
}

# 更新 setup.py
update_setup_py() {
    local new_version=$1
    local file="setup.py"
    
    if [ ! -f "$file" ]; then
        print_error "$file not found"
        return 1
    fi
    
    backup_file "$file"
    
    # 使用 sed 替换版本号
    if [[ "$OSTYPE" == "darwin"* ]]; then
        # macOS
        sed -i '' "s/version='[^']*'/version='$new_version'/" "$file"
    else
        # Linux
        sed -i "s/version='[^']*'/version='$new_version'/" "$file"
    fi
    
    # 验证修改
    local updated_version=$(grep "version=" "$file" | head -1 | sed "s/.*version='\([^']*\)'.*/\1/")
    if [ "$updated_version" = "$new_version" ]; then
        print_success "Updated $file"
        return 0
    else
        print_error "Failed to update $file"
        return 1
    fi
}

# 更新 pyproject.toml
update_pyproject_toml() {
    local new_version=$1
    local file="pyproject.toml"
    
    if [ ! -f "$file" ]; then
        print_error "$file not found"
        return 1
    fi
    
    backup_file "$file"
    
    # 使用 sed 替换版本号（只替换第一个 version = ）
    if [[ "$OSTYPE" == "darwin"* ]]; then
        # macOS
        sed -i '' "0,/^version = \".*\"/s//version = \"$new_version\"/" "$file"
    else
        # Linux
        sed -i "0,/^version = \".*\"/s//version = \"$new_version\"/" "$file"
    fi
    
    # 验证修改
    local updated_version=$(grep "^version" "$file" | head -1 | cut -d'"' -f2)
    if [ "$updated_version" = "$new_version" ]; then
        print_success "Updated $file"
        return 0
    else
        print_error "Failed to update $file"
        return 1
    fi
}

# 主程序
main() {
    print_header "========================================="
    print_header "  Version Updater"
    print_header "========================================="
    echo
    
    # 检查参数
    if [ $# -ne 1 ]; then
        show_usage
        exit 1
    fi
    
    NEW_VERSION=$1
    
    # 验证版本号格式
    if ! validate_version "$NEW_VERSION"; then
        exit 1
    fi
    
    # 获取当前版本
    CURRENT_VERSION=$(get_current_version)
    
    echo "Current version: $CURRENT_VERSION"
    echo "New version:     $NEW_VERSION"
    echo
    
    # 确认操作
    read -p "Do you want to update the version? [y/N] " -n 1 -r
    echo
    if [[ ! $REPLY =~ ^[Yy]$ ]]; then
        print_warning "Operation cancelled"
        exit 0
    fi
    
    echo
    print_info "Updating version numbers..."
    echo
    
    # 更新所有文件
    SUCCESS=true
    
    if ! update_init_py "$NEW_VERSION"; then
        SUCCESS=false
    fi
    
    if ! update_setup_py "$NEW_VERSION"; then
        SUCCESS=false
    fi
    
    if ! update_pyproject_toml "$NEW_VERSION"; then
        SUCCESS=false
    fi
    
    echo
    
    # 检查结果
    if [ "$SUCCESS" = true ]; then
        print_success "All files updated successfully!"
        echo
        print_info "Running version consistency check..."
        echo
        
        # 运行检查脚本
        if [ -f "check_version.sh" ]; then
            bash check_version.sh
            CHECK_RESULT=$?
            
            if [ $CHECK_RESULT -eq 0 ]; then
                echo
                print_success "Version update completed successfully!"
                print_info "Removing backup files..."
                remove_backups
            else
                print_error "Version check failed! Rolling back..."
                restore_backups
                exit 1
            fi
        else
            print_warning "check_version.sh not found, skipping verification"
            print_info "Removing backup files..."
            remove_backups
        fi
    else
        print_error "Some files failed to update!"
        echo
        read -p "Do you want to restore backups? [Y/n] " -n 1 -r
        echo
        if [[ ! $REPLY =~ ^[Nn]$ ]]; then
            restore_backups
        fi
        exit 1
    fi
    
    echo
    print_success "Done! Version updated from $CURRENT_VERSION to $NEW_VERSION"
    echo
    print_info "Next steps:"
    echo "  1. Review the changes: git diff"
    echo "  2. Commit the changes: git add . && git commit -m 'chore: bump version to $NEW_VERSION'"
    echo "  3. Create a git tag: git tag v$NEW_VERSION"
    echo
}

# 运行主程序
main "$@"

