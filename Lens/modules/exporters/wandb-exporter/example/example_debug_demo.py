"""
Debug 日志开关演示

这个脚本演示了 PRIMUS_LENS_WANDB_DEBUG 环境变量的效果。

运行方式：
    # 启用 debug 日志
    PRIMUS_LENS_WANDB_DEBUG=true python example_debug_demo.py

    # 禁用 debug 日志（默认）
    PRIMUS_LENS_WANDB_DEBUG=false python example_debug_demo.py
    # 或
    python example_debug_demo.py
"""

import os
import time

print("\n" + "=" * 80)
print("Debug 日志开关演示")
print("=" * 80)

# 显示当前配置
debug_env = os.environ.get("PRIMUS_LENS_WANDB_DEBUG", "false")
print(f"\n当前环境变量设置: PRIMUS_LENS_WANDB_DEBUG={debug_env}")

if debug_env.lower() in ("true", "1", "yes"):
    print("✅ Debug 日志已启用 - 你将看到详细的 [Primus Lens] 日志消息")
else:
    print("✅ Debug 日志已禁用 - 你将只看到必要的输出（推荐用于生产训练）")

print("\n" + "-" * 80)
print("开始模拟训练...")
print("-" * 80 + "\n")

try:
    # 设置 WandB 为离线模式
    os.environ['WANDB_MODE'] = 'offline'
    os.environ['WANDB_SILENT'] = 'true'
    
    import wandb
    
    # 初始化 WandB
    print(">>> wandb.init(project='debug-demo', name='test-run')")
    run = wandb.init(
        project="debug-demo",
        name="test-run",
        config={
            "learning_rate": 0.001,
            "epochs": 10,
        },
        reinit=True
    )
    
    # 模拟训练循环
    print("\n>>> 开始训练循环...")
    for epoch in range(3):
        print(f"\nEpoch {epoch + 1}/3")
        
        # 模拟训练指标
        loss = 1.0 / (epoch + 1)
        accuracy = 0.5 + (epoch * 0.15)
        
        print(f"  loss: {loss:.4f}, accuracy: {accuracy:.4f}")
        
        # 记录指标
        wandb.log({
            "epoch": epoch + 1,
            "loss": loss,
            "accuracy": accuracy,
        }, step=epoch + 1)
        
        time.sleep(0.1)  # 模拟训练时间
    
    print("\n>>> wandb.finish()")
    wandb.finish()
    
    print("\n" + "-" * 80)
    print("训练完成！")
    print("-" * 80)
    
except ImportError:
    print("❌ WandB 未安装")
    print("\n安装方法：")
    print("  pip install wandb")
    exit(1)

except Exception as e:
    print(f"❌ 发生错误: {e}")
    import traceback
    traceback.print_exc()
    exit(1)

# 总结
print("\n" + "=" * 80)
print("演示完成")
print("=" * 80)

if debug_env.lower() in ("true", "1", "yes"):
    print("""
💡 观察要点（debug 模式）：
   - 你应该看到很多 [Primus Lens WandB] 开头的日志
   - 包括劫持成功、初始化信息、每次 log 的详细信息
   - 这些信息对调试很有帮助，但在生产环境可能显得冗余

🔄 试试禁用 debug 日志：
   export PRIMUS_LENS_WANDB_DEBUG=false
   python example_debug_demo.py
""")
else:
    print("""
💡 观察要点（正常模式）：
   - 你不应该看到 [Primus Lens] 开头的日志
   - 输出干净清爽，只有训练本身的信息
   - 这是推荐的生产环境配置

🔍 如果需要调试，可以启用 debug 日志：
   export PRIMUS_LENS_WANDB_DEBUG=true
   python example_debug_demo.py
""")

print("\n📚 更多信息请参考: DEBUG_LOGGING.md")
print("=" * 80 + "\n")

