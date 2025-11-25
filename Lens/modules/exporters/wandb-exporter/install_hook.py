"""
安装/卸载 Primus Lens WandB Hook 的辅助脚本
"""
import os
import sys
import site
import argparse


def get_pth_file_path():
    """获取 .pth 文件路径"""
    if hasattr(site, 'getsitepackages'):
        site_packages = site.getsitepackages()[0]
    else:
        from distutils.sysconfig import get_python_lib
        site_packages = get_python_lib()
    
    return os.path.join(site_packages, 'primus_lens_wandb_hook.pth')


def install_pth_file():
    """安装 .pth 文件"""
    pth_file = get_pth_file_path()
    
    # .pth 文件内容
    pth_content = 'import primus_lens_wandb_exporter.wandb_hook\n'
    
    try:
        # 检查目录是否可写
        site_packages = os.path.dirname(pth_file)
        if not os.access(site_packages, os.W_OK):
            print(f"✗ Error: No write permission to {site_packages}")
            print("  Please run with sudo or as administrator:")
            if sys.platform == "win32":
                print(f"  (Run PowerShell/CMD as administrator)")
            else:
                print(f"  sudo python {sys.argv[0]} install")
            return False
        
        # 写入 .pth 文件
        with open(pth_file, 'w') as f:
            f.write(pth_content)
        
        print("✓ Successfully installed Primus Lens WandB Hook!")
        print(f"  .pth file created at: {pth_file}")
        print()
        print("The hook will automatically intercept wandb when Python starts.")
        print("No code changes needed in your training scripts!")
        print()
        print("Environment variables:")
        print("  PRIMUS_LENS_WANDB_HOOK=true/false          - Enable/disable the hook")
        print("  PRIMUS_LENS_WANDB_ENHANCE_METRICS=true/false  - Add system metrics")
        print("  PRIMUS_LENS_WANDB_OUTPUT_PATH=<path>       - Output path for metrics")
        print("  PRIMUS_LENS_WANDB_SAVE_LOCAL=true/false    - Save metrics locally")
        
        return True
        
    except Exception as e:
        print(f"✗ Failed to install .pth file: {e}")
        return False


def uninstall_pth_file():
    """卸载 .pth 文件"""
    pth_file = get_pth_file_path()
    
    try:
        if os.path.exists(pth_file):
            os.remove(pth_file)
            print("✓ Successfully uninstalled Primus Lens WandB Hook!")
            print(f"  Removed: {pth_file}")
            print()
            print("WandB will no longer be intercepted.")
        else:
            print("⚠ .pth file not found, nothing to uninstall.")
            print(f"  Expected location: {pth_file}")
        
        return True
        
    except Exception as e:
        print(f"✗ Failed to uninstall .pth file: {e}")
        return False


def check_installation():
    """检查安装状态"""
    pth_file = get_pth_file_path()
    
    print("Checking Primus Lens WandB Hook installation...")
    print(f"  .pth file location: {pth_file}")
    print()
    
    if os.path.exists(pth_file):
        print(f"  Status: ✓ INSTALLED")
        
        # 读取内容
        with open(pth_file, 'r') as f:
            content = f.read()
        print(f"  Content: {content.strip()}")
        print()
        
        # 检查包是否可导入
        try:
            import primus_lens_wandb_exporter.wandb_hook
            print(f"  Package: ✓ Available")
        except ImportError as e:
            print(f"  Package: ✗ Not available ({e})")
        
        print()
        
        # 检查环境变量
        print("Environment variables:")
        env_vars = [
            'PRIMUS_LENS_WANDB_HOOK',
            'PRIMUS_LENS_WANDB_ENHANCE_METRICS',
            'PRIMUS_LENS_WANDB_SAVE_LOCAL',
            'PRIMUS_LENS_WANDB_OUTPUT_PATH',
        ]
        for var in env_vars:
            value = os.environ.get(var, '<not set>')
            print(f"  {var}: {value}")
        
    else:
        print(f"  Status: ✗ NOT INSTALLED")
        print()
        print("To install, run:")
        print(f"  python {sys.argv[0]} install")
        print()
        print("Or use pip:")
        print(f"  pip install -e .")
    
    return True


def main():
    """主函数"""
    parser = argparse.ArgumentParser(
        description='Install/uninstall Primus Lens WandB Hook',
        formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog="""
Examples:
  # Install the hook
  python install_hook.py install
  
  # Check installation status
  python install_hook.py check
  
  # Uninstall the hook
  python install_hook.py uninstall
        """
    )
    
    parser.add_argument(
        'action',
        choices=['install', 'uninstall', 'check'],
        help='Action to perform'
    )
    
    args = parser.parse_args()
    
    print()
    print("╔" + "═" * 58 + "╗")
    print("║" + " " * 8 + "Primus Lens WandB Exporter Manager" + " " * 16 + "║")
    print("╚" + "═" * 58 + "╝")
    print()
    
    if args.action == 'install':
        success = install_pth_file()
    elif args.action == 'uninstall':
        success = uninstall_pth_file()
    elif args.action == 'check':
        success = check_installation()
    else:
        print(f"Unknown action: {args.action}")
        success = False
    
    print()
    
    return 0 if success else 1


if __name__ == "__main__":
    sys.exit(main())

