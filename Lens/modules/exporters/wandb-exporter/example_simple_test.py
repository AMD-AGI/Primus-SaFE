#!/usr/bin/env python3
"""
简单的 WandB Exporter 测试示例

这是一个最简单的测试示例，用于快速验证 wandb-exporter 是否正常工作。
如果需要更全面的测试，请使用 test_real_scenario.py。

运行方式：
    python example_simple_test.py
"""

import os
import sys
import tempfile
import time

# 设置环境变量
os.environ["PRIMUS_LENS_WANDB_HOOK"] = "true"
os.environ["PRIMUS_LENS_WANDB_ENHANCE_METRICS"] = "true"
os.environ["PRIMUS_LENS_WANDB_SAVE_LOCAL"] = "true"
os.environ["PRIMUS_LENS_WANDB_OUTPUT_PATH"] = tempfile.mkdtemp(prefix="wandb_simple_test_")
os.environ["PRIMUS_LENS_WANDB_API_REPORTING"] = "false"  # 禁用 API 上报（本地测试不需要）
os.environ["WANDB_MODE"] = "offline"  # 使用离线模式，不真实上报到 W&B
os.environ["WANDB_SILENT"] = "true"

print("="*60)
print("WandB Exporter 简单测试")
print("="*60)
print()

# 导入 wandb
print("1. 导入 wandb...")
try:
    import wandb
    print("   ✓ wandb 导入成功")
except ImportError:
    print("   ✗ wandb 未安装")
    print("   请运行: pip install wandb")
    sys.exit(1)

# 检查是否被劫持
print("\n2. 检查劫持状态...")
if hasattr(wandb, '_primus_lens_patched'):
    print("   ✓ WandB 已被 Primus Lens 成功劫持")
    print(f"   wandb.log 类型: {type(wandb.log)}")
    print(f"   wandb.log 名称: {wandb.log.__name__ if hasattr(wandb.log, '__name__') else 'N/A'}")
    # 尝试直接调用一次看看
    print("   测试直接调用 wandb.log:")
    try:
        wandb.log({"test": 123})
        print("   ✓ wandb.log() 可以调用")
    except Exception as e:
        print(f"   ! wandb.log() 调用失败: {e}")
else:
    print("   ✗ WandB 未被劫持")
    print("   请运行: python install_hook.py install")
    sys.exit(1)

# 初始化 wandb
print("\n3. 初始化 WandB run...")
try:
    run = wandb.init(
        project="simple-test",
        name="test-run",
        config={"test": True}
    )
    print(f"   ✓ Run 初始化成功: {run.name}")
except Exception as e:
    print(f"   ✗ 初始化失败: {e}")
    sys.exit(1)

# 记录一些指标
print("\n4. 记录训练指标...")
try:
    for step in range(5):
        print(f"   记录步骤 {step}...")
        wandb.log({
            "loss": 1.0 - (step * 0.1),
            "accuracy": 0.5 + (step * 0.08),
        }, step=step)
    print(f"   ✓ 成功记录 5 步指标")
except Exception as e:
    print(f"   ✗ 记录失败: {e}")
    import traceback
    traceback.print_exc()
    sys.exit(1)

# 完成 run
print("\n5. 完成 WandB run...")
wandb.finish()
print("   ✓ Run 已完成")

# 等待文件写入
time.sleep(0.5)

# 验证输出文件
print("\n6. 验证输出文件...")
output_path = os.environ["PRIMUS_LENS_WANDB_OUTPUT_PATH"]
print(f"   输出目录: {output_path}")

# 检查目录结构
if os.path.exists(output_path):
    print(f"   ✓ 输出目录存在")
    # 列出所有文件
    for root, dirs, files in os.walk(output_path):
        level = root.replace(output_path, '').count(os.sep)
        indent = ' ' * 2 * level
        print(f"   {indent}{os.path.basename(root)}/")
        subindent = ' ' * 2 * (level + 1)
        for file in files:
            print(f"   {subindent}{file}")
else:
    print(f"   ✗ 输出目录不存在")

# 在非分布式环境下，LOCAL_RANK 默认为 -1
metrics_file = os.path.join(output_path, "node_0", "rank_-1", "wandb_metrics.jsonl")
print(f"   期望文件: {metrics_file}")

if os.path.exists(metrics_file):
    with open(metrics_file, 'r') as f:
        lines = f.readlines()
    print(f"   ✓ 指标文件已生成: {metrics_file}")
    print(f"   ✓ 包含 {len(lines)} 条记录")
    
    # 显示第一条记录
    import json
    first_record = json.loads(lines[0])
    print(f"\n   第一条记录示例:")
    print(f"   - Timestamp: {first_record['timestamp']}")
    print(f"   - Step: {first_record['step']}")
    print(f"   - 指标数量: {len(first_record['data'])}")
    
    # 检查是否包含 Primus Lens 标记
    if "_primus_lens_enabled" in first_record['data']:
        print(f"   ✓ 包含 Primus Lens 标记")
    
    # 检查系统指标
    sys_metrics = [k for k in first_record['data'].keys() if k.startswith('_primus_sys_')]
    if sys_metrics:
        print(f"   ✓ 包含系统指标: {', '.join(sys_metrics)}")
else:
    print(f"   ✗ 指标文件未生成")
    sys.exit(1)

# 清理
print(f"\n7. 清理临时文件...")
import shutil
try:
    shutil.rmtree(output_path)
    print(f"   ✓ 已清理: {output_path}")
except:
    print(f"   ⚠ 清理失败（可手动删除）: {output_path}")

# 总结
print("\n" + "="*60)
print("🎉 测试成功！WandB Exporter 工作正常！")
print("="*60)
print()
print("接下来可以:")
print("  1. 运行完整测试: python test_real_scenario.py")
print("  2. 查看测试指南: cat TEST_GUIDE.md")
print("  3. 在你的训练脚本中使用（无需修改代码）")
print()

