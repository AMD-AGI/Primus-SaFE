"""
Setup script for primus-lens-wandb-exporter
Support automatic wandb interception via .pth file
"""
import os
import sys
from setuptools import setup, find_packages
from setuptools.command.install import install


class PostInstallCommand(install):
    """Custom install command - Install .pth file for automatic interception"""
    
    def run(self):
        install.run(self)
        
        # Create .pth file
        self.create_pth_file()
    
    def create_pth_file(self):
        """Create .pth file in site-packages"""
        try:
            import site
            
            # Get site-packages directory
            if hasattr(site, 'getsitepackages'):
                site_packages = site.getsitepackages()[0]
            else:
                # Some environments may not have getsitepackages
                from distutils.sysconfig import get_python_lib
                site_packages = get_python_lib()
            
            pth_file = os.path.join(site_packages, 'primus_lens_wandb_hook.pth')
            
            # .pth file content - import wandb_hook module
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
    version='0.1.4',
    description='Primus Lens WandB Exporter - Automatically intercept wandb reporting without code changes',
    long_description=open('README.md', encoding='utf-8').read() if os.path.exists('README.md') else '',
    long_description_content_type='text/markdown',
    author='Primus Team',
    packages=find_packages(where='src'),
    package_dir={'': 'src'},
    python_requires='>=3.7',
    install_requires=[
        'psutil>=5.8.0',  # For system metrics collection
    ],
    extras_require={
        'dev': [
            'pytest>=7.0.0',
            'pytest-cov>=3.0.0',
        ],
        'gpu': [
            'nvidia-ml-py3>=7.352.0',  # For GPU metrics collection
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

