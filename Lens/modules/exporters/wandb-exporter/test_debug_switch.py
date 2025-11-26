"""
测试 Debug 日志开关功能

该脚本演示如何使用 PRIMUS_LENS_WANDB_DEBUG 环境变量来控制日志输出。

使用方法：
    # 启用 debug 日志（打印所有 debug 信息）
    export PRIMUS_LENS_WANDB_DEBUG=true
    python test_debug_switch.py

    # 禁用 debug 日志（默认，不打印日志）
    export PRIMUS_LENS_WANDB_DEBUG=false
    python test_debug_switch.py
    
    # 或不设置（默认为 false）
    python test_debug_switch.py
"""
import os
import sys

# 在导入 wandb 之前设置环境变量（用于演示）
# 实际使用时应该在启动脚本之前设置环境变量
if len(sys.argv) > 1:
    debug_mode = sys.argv[1].lower() in ('true', '1', 'yes')
    os.environ['PRIMUS_LENS_WANDB_DEBUG'] = str(debug_mode).lower()

print("=" * 80)
print(f"测试 Debug 日志开关")
print(f"当前设置: PRIMUS_LENS_WANDB_DEBUG={os.environ.get('PRIMUS_LENS_WANDB_DEBUG', 'false')}")
print("=" * 80)

# 先测试 logger 模块本身
print("\n[1] 测试 logger 模块")
print("-" * 80)
from primus_lens_wandb_exporter.logger import debug_log, is_debug_enabled

print(f"Debug 模式是否启用: {is_debug_enabled()}")
print("调用 debug_log():")
debug_log("[Test] 这是一条 debug 日志")
if is_debug_enabled():
    print("  ✓ Debug 日志已打印")
else:
    print("  ✓ Debug 日志被抑制（符合预期）")

# 测试是否能正确劫持 wandb
print("\n[2] 测试 WandB 劫持")
print("-" * 80)

try:
    import wandb
    print(f"WandB 是否被 patch: {hasattr(wandb, '_primus_lens_patched')}")
    
    if hasattr(wandb, '_primus_lens_patched'):
        print("  ✓ WandB 成功被劫持")
    else:
        print("  ⚠ WandB 未被劫持（可能是因为 import hook 问题）")
except ImportError:
    print("  ⚠ WandB 未安装，跳过测试")

# 测试完整的 WandB 流程（仅在 wandb 可用时）
print("\n[3] 测试完整 WandB 流程（如果 wandb 可用）")
print("-" * 80)

try:
    import wandb
    
    # 设置为离线模式，避免实际上报
    os.environ['WANDB_MODE'] = 'offline'
    os.environ['WANDB_SILENT'] = 'true'
    
    print("初始化 WandB run...")
    run = wandb.init(project="test-debug-switch", name="test-run", reinit=True)
    
    print("记录一些指标...")
    wandb.log({"loss": 0.5, "accuracy": 0.9}, step=1)
    wandb.log({"loss": 0.3, "accuracy": 0.92}, step=2)
    
    print("完成 WandB run...")
    wandb.finish()
    
    print("  ✓ WandB 流程测试完成")
    
except ImportError:
    print("  ⚠ WandB 未安装，跳过完整流程测试")
except Exception as e:
    print(f"  ✗ 测试失败: {e}")
    import traceback
    traceback.print_exc()

# 总结
print("\n" + "=" * 80)
print("测试完成")
print("=" * 80)

if is_debug_enabled():
    print("\n💡 提示：当前启用了 debug 日志，你应该看到很多 [Primus Lens] 开头的消息")
    print("   如果想禁用这些日志，请设置：export PRIMUS_LENS_WANDB_DEBUG=false")
else:
    print("\n💡 提示：当前禁用了 debug 日志，你不应该看到 [Primus Lens] 开头的消息")
    print("   如果想查看详细日志，请设置：export PRIMUS_LENS_WANDB_DEBUG=true")

print("\n使用示例：")
print("  # 启用 debug 日志")
print("  export PRIMUS_LENS_WANDB_DEBUG=true")
print("  python test_debug_switch.py")
print("")
print("  # 禁用 debug 日志")
print("  export PRIMUS_LENS_WANDB_DEBUG=false")
print("  python test_debug_switch.py")
print("")
print("  # 或者直接通过参数测试")
print("  python test_debug_switch.py true   # 启用")
print("  python test_debug_switch.py false  # 禁用")

