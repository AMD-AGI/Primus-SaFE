"""
Setup script for primus-lens-wandb-exporter
支持通过 .pth 文件自动劫持 wandb
"""
import os
import sys
from setuptools import setup, find_packages
from setuptools.command.install import install


class PostInstallCommand(install):
    """自定义安装命令 - 安装 .pth 文件来实现自动劫持"""
    
    def run(self):
        install.run(self)
        
        # 创建 .pth 文件
        self.create_pth_file()
    
    def create_pth_file(self):
        """创建 .pth 文件到 site-packages"""
        try:
            import site
            
            # 获取 site-packages 目录
            if hasattr(site, 'getsitepackages'):
                site_packages = site.getsitepackages()[0]
            else:
                # 某些环境下可能没有 getsitepackages
                from distutils.sysconfig import get_python_lib
                site_packages = get_python_lib()
            
            pth_file = os.path.join(site_packages, 'primus_lens_wandb_hook.pth')
            
            # .pth 文件内容 - 导入 wandb_hook 模块
            pth_content = 'import primus_lens_wandb_exporter.wandb_hook\n'
            
            print(f"Installing .pth file to: {pth_file}")
            with open(pth_file, 'w') as f:
                f.write(pth_content)
            
            print("[Primus Lens WandB] Hook installed successfully!")
            print("The hook will automatically intercept wandb when Python starts.")
            
        except Exception as e:
            print(f"Warning: Failed to create .pth file: {e}")
            print("You can manually create it later using: python install_hook.py install")


setup(
    name='primus-lens-wandb-exporter',
    version='0.1.2',
    description='Primus Lens WandB Exporter - 自动劫持 wandb 上报，无需代码修改',
    long_description=open('README.md', encoding='utf-8').read() if os.path.exists('README.md') else '',
    long_description_content_type='text/markdown',
    author='Primus Team',
    packages=find_packages(where='src'),
    package_dir={'': 'src'},
    python_requires='>=3.7',
    install_requires=[
        'psutil>=5.8.0',  # 用于系统指标收集
    ],
    extras_require={
        'dev': [
            'pytest>=7.0.0',
            'pytest-cov>=3.0.0',
        ],
        'gpu': [
            'nvidia-ml-py3>=7.352.0',  # 用于 GPU 指标收集
        ],
    },
    cmdclass={
        'install': PostInstallCommand,
    },
    classifiers=[
        'Development Status :: 4 - Beta',
        'Intended Audience :: Developers',
        'Programming Language :: Python :: 3',
        'Programming Language :: Python :: 3.7',
        'Programming Language :: Python :: 3.8',
        'Programming Language :: Python :: 3.9',
        'Programming Language :: Python :: 3.10',
        'Programming Language :: Python :: 3.11',
    ],
)

