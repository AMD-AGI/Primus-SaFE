@echo off
REM Primus Lens WandB Exporter 快速开始脚本 (Windows 版本)

echo ============================================================
echo      Primus Lens WandB Exporter - Quick Start
echo ============================================================
echo.

REM 步骤 1: 安装包
echo 步骤 1/4: 安装 Primus Lens WandB Exporter...
echo ----------------------------------------
pip install -e .
if errorlevel 1 (
    echo ❌ 安装失败！可能需要管理员权限。
    echo 请以管理员身份运行此脚本。
    pause
    exit /b 1
)
echo ✅ 安装成功！
echo.

REM 步骤 2: 验证安装
echo 步骤 2/4: 验证 .pth 文件安装...
echo ----------------------------------------
python install_hook.py check
echo.

REM 步骤 3: 运行测试
echo 步骤 3/4: 运行测试...
echo ----------------------------------------
python test_wandb_hook.py
echo.

REM 步骤 4: 环境变量说明
echo 步骤 4/4: 环境变量说明...
echo ----------------------------------------
echo 你可以设置以下环境变量来控制行为：
echo.
echo   set PRIMUS_LENS_WANDB_HOOK=true
echo   set PRIMUS_LENS_WANDB_ENHANCE_METRICS=true
echo   set PRIMUS_LENS_WANDB_SAVE_LOCAL=true
echo   set PRIMUS_LENS_WANDB_OUTPUT_PATH=C:\temp\metrics
echo.

REM 完成
echo ============================================================
echo                  🎉 安装完成！
echo ============================================================
echo.
echo 接下来你可以：
echo.
echo 1. 运行示例代码:
echo    python example_usage.py
echo.
echo 2. 在你的训练脚本中正常使用 wandb（无需修改代码）:
echo    python your_training_script.py
echo.
echo 3. 查看文档:
echo    - README.md      - 英文用户指南
echo    - 使用指南.md    - 中文使用指南
echo    - INSTALL.md     - 安装详解
echo.
echo 如需卸载 hook:
echo    python install_hook.py uninstall
echo.

pause

