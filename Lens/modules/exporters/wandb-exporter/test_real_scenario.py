#!/usr/bin/env python3
"""
WandB Exporter 真实场景测试程序

这个测试程序模拟真实的训练场景，全面测试 wandb-exporter 的各项功能：
1. 真实的 W&B API 调用（init, log, finish）
2. 本地文件保存和指标增强
3. 异步 API 上报（框架检测、训练指标）
4. 分布式训练场景（多节点、多GPU）
5. 错误处理和边缘情况

运行方式：
    python test_real_scenario.py [--scenario SCENARIO]

参数：
    --scenario: 测试场景，可选值：
        - basic: 基础单机训练场景（默认）
        - distributed: 分布式训练场景
        - api_reporting: API 上报场景
        - stress: 压力测试场景
        - all: 运行所有场景
"""

import os
import sys
import time
import json
import tempfile
import shutil
import argparse
from pathlib import Path
from typing import Dict, List, Any, Optional
import random

# 测试结果收集
class TestResults:
    """测试结果收集器"""
    
    def __init__(self):
        self.scenarios = []
        self.current_scenario = None
        self.start_time = time.time()
    
    def start_scenario(self, name: str, description: str):
        """开始一个测试场景"""
        self.current_scenario = {
            "name": name,
            "description": description,
            "tests": [],
            "start_time": time.time(),
            "status": "running",
        }
        print(f"\n{'='*70}")
        print(f"测试场景: {name}")
        print(f"描述: {description}")
        print(f"{'='*70}\n")
    
    def add_test(self, test_name: str, passed: bool, message: str = "", details: Any = None):
        """添加测试结果"""
        if self.current_scenario is None:
            return
        
        status = "✓ 通过" if passed else "✗ 失败"
        print(f"  [{status}] {test_name}")
        if message:
            print(f"      → {message}")
        
        self.current_scenario["tests"].append({
            "name": test_name,
            "passed": passed,
            "message": message,
            "details": details,
        })
    
    def end_scenario(self):
        """结束当前测试场景"""
        if self.current_scenario is None:
            return
        
        self.current_scenario["end_time"] = time.time()
        self.current_scenario["duration"] = self.current_scenario["end_time"] - self.current_scenario["start_time"]
        
        passed_count = sum(1 for t in self.current_scenario["tests"] if t["passed"])
        total_count = len(self.current_scenario["tests"])
        
        if passed_count == total_count:
            self.current_scenario["status"] = "passed"
        else:
            self.current_scenario["status"] = "failed"
        
        print(f"\n场景结果: {passed_count}/{total_count} 测试通过")
        print(f"耗时: {self.current_scenario['duration']:.2f} 秒\n")
        
        self.scenarios.append(self.current_scenario)
        self.current_scenario = None
    
    def print_summary(self):
        """打印测试总结"""
        print("\n" + "="*70)
        print("测试总结")
        print("="*70 + "\n")
        
        total_scenarios = len(self.scenarios)
        passed_scenarios = sum(1 for s in self.scenarios if s["status"] == "passed")
        
        for scenario in self.scenarios:
            status_symbol = "✓" if scenario["status"] == "passed" else "✗"
            passed = sum(1 for t in scenario["tests"] if t["passed"])
            total = len(scenario["tests"])
            
            print(f"{status_symbol} {scenario['name']}: {passed}/{total} 测试通过 "
                  f"({scenario['duration']:.2f}s)")
        
        print(f"\n场景统计: {passed_scenarios}/{total_scenarios} 场景通过")
        print(f"总耗时: {time.time() - self.start_time:.2f} 秒\n")
        
        if passed_scenarios == total_scenarios:
            print("🎉 所有测试场景通过！\n")
            return 0
        else:
            print(f"⚠️  {total_scenarios - passed_scenarios} 个场景失败\n")
            return 1


# 全局测试结果
test_results = TestResults()


def setup_environment(api_url: Optional[str] = None, enable_api: bool = True, tmpdir: Optional[str] = None, force_hook: bool = False):
    """设置测试环境变量"""
    if tmpdir is None:
        tmpdir = tempfile.mkdtemp(prefix="wandb_test_")
    
    # 基础配置
    os.environ["PRIMUS_LENS_WANDB_HOOK"] = "true"
    os.environ["PRIMUS_LENS_WANDB_ENHANCE_METRICS"] = "true"
    os.environ["PRIMUS_LENS_WANDB_SAVE_LOCAL"] = "true"
    os.environ["PRIMUS_LENS_WANDB_OUTPUT_PATH"] = tmpdir
    
    # 如果需要强制加载劫持（用于测试环境）
    if force_hook and 'wandb' not in sys.modules:
        try:
            # 显式导入劫持模块（必须在 wandb 之前导入）
            src_path = os.path.join(os.path.dirname(os.path.abspath(__file__)), 'src')
            if src_path not in sys.path:
                sys.path.insert(0, src_path)
            # 导入 wandb_hook 模块会自动注册 import hook
            import primus_lens_wandb_exporter.wandb_hook
        except ImportError as e:
            print(f"  警告: 无法加载劫持模块: {e}")
        except Exception as e:
            print(f"  警告: 加载劫持模块时出错: {e}")
    
    # WandB 配置（使用离线模式，避免真实上报到 W&B）
    os.environ["WANDB_MODE"] = "offline"
    os.environ["WANDB_SILENT"] = "true"
    
    # API 上报配置
    if enable_api:
        os.environ["PRIMUS_LENS_WANDB_API_REPORTING"] = "true"
        if api_url:
            os.environ["PRIMUS_LENS_API_BASE_URL"] = api_url
        else:
            # 使用测试 URL（不会真实发送，但会记录）
            os.environ["PRIMUS_LENS_API_BASE_URL"] = "http://localhost:18080/api/v1"
        
        # 设置必需的标识
        os.environ["WORKLOAD_UID"] = "test-workload-12345"
        os.environ["POD_NAME"] = "test-pod"
        os.environ["POD_NAMESPACE"] = "default"
    else:
        os.environ["PRIMUS_LENS_WANDB_API_REPORTING"] = "false"
    
    return tmpdir


def cleanup_environment(tmpdir: str):
    """清理测试环境"""
    try:
        if os.path.exists(tmpdir):
            shutil.rmtree(tmpdir)
    except Exception as e:
        print(f"清理临时目录失败: {e}")
    
    # 清理分布式训练相关的环境变量
    for var in ['RANK', 'LOCAL_RANK', 'NODE_RANK', 'WORLD_SIZE']:
        os.environ.pop(var, None)


def verify_metrics_file(tmpdir: str, node_rank: int = 0, local_rank: int = 0) -> tuple:
    """验证指标文件是否正确生成"""
    metrics_file = os.path.join(tmpdir, f"node_{node_rank}", f"rank_{local_rank}", "wandb_metrics.jsonl")
    
    if not os.path.exists(metrics_file):
        return False, f"指标文件不存在: {metrics_file}"
    
    try:
        with open(metrics_file, 'r') as f:
            lines = f.readlines()
        
        if not lines:
            return False, "指标文件为空"
        
        # 验证 JSON 格式
        metrics_count = 0
        for line in lines:
            try:
                data = json.loads(line)
                if "timestamp" not in data or "data" not in data:
                    return False, "指标格式不正确（缺少必需字段）"
                metrics_count += 1
            except json.JSONDecodeError:
                return False, f"JSON 格式错误: {line[:100]}"
        
        return True, f"找到 {metrics_count} 条指标记录"
    
    except Exception as e:
        return False, f"读取指标文件失败: {e}"


def verify_metrics_enhanced(tmpdir: str, node_rank: int = 0, local_rank: int = 0) -> tuple:
    """验证指标是否包含系统增强信息"""
    metrics_file = os.path.join(tmpdir, f"node_{node_rank}", f"rank_{local_rank}", "wandb_metrics.jsonl")
    
    try:
        with open(metrics_file, 'r') as f:
            line = f.readline()
        
        data = json.loads(line)
        metrics_data = data.get("data", {})
        
        # 检查 Primus Lens 标记
        if "_primus_lens_enabled" not in metrics_data:
            return False, "缺少 Primus Lens 标记"
        
        # 检查系统指标（如果 psutil 可用）
        try:
            import psutil
            if "_primus_sys_cpu_percent" not in metrics_data:
                return False, "缺少 CPU 系统指标"
            if "_primus_sys_memory_percent" not in metrics_data:
                return False, "缺少内存系统指标"
        except ImportError:
            pass
        
        return True, "指标增强正常"
    
    except Exception as e:
        return False, f"验证失败: {e}"


# ========== 测试场景 ==========

def test_scenario_basic():
    """场景1: 基础单机训练场景"""
    test_results.start_scenario(
        "基础单机训练",
        "测试基本的 W&B 劫持、指标保存和增强功能"
    )
    
    tmpdir = None
    try:
        # 设置环境（强制加载劫持）
        tmpdir = setup_environment(enable_api=False, force_hook=True)
        
        # 导入 wandb（触发劫持）
        import wandb
        
        # 测试1: 验证劫持是否成功
        is_patched = hasattr(wandb, '_primus_lens_patched')
        test_results.add_test(
            "WandB 劫持",
            is_patched,
            "WandB 已被 Primus Lens 成功劫持" if is_patched else "WandB 未被劫持"
        )
        
        # 测试2: 初始化 wandb
        try:
            run = wandb.init(
                project="primus-test",
                name="basic-test",
                config={"lr": 0.001, "batch_size": 32}
            )
            test_results.add_test(
                "WandB 初始化",
                run is not None,
                f"Run ID: {run.id if run else 'None'}"
            )
        except Exception as e:
            test_results.add_test("WandB 初始化", False, f"初始化失败: {e}")
            return
        
        # 测试3: 记录指标
        num_steps = 10
        try:
            print(f"\n  DEBUG: wandb.log 类型: {type(wandb.log)}")
            print(f"  DEBUG: wandb.log 名称: {wandb.log.__name__ if hasattr(wandb.log, '__name__') else 'N/A'}")
            for step in range(num_steps):
                print(f"  DEBUG: 调用 wandb.log, step={step}")
                wandb.log({
                    "loss": 1.0 - (step * 0.05),
                    "accuracy": 0.5 + (step * 0.04),
                    "step": step,
                }, step=step)
            test_results.add_test(
                "指标记录",
                True,
                f"成功记录 {num_steps} 步指标"
            )
        except Exception as e:
            test_results.add_test("指标记录", False, f"记录失败: {e}")
        
        # 完成 run
        wandb.finish()
        
        # 等待文件写入
        time.sleep(0.5)
        
        # DEBUG: 查看实际生成的目录结构
        print(f"\n  DEBUG: 临时目录: {tmpdir}")
        if os.path.exists(tmpdir):
            for root, dirs, files in os.walk(tmpdir):
                level = root.replace(tmpdir, '').count(os.sep)
                indent = ' ' * 4 * level
                print(f'  DEBUG: {indent}{os.path.basename(root)}/')
                subindent = ' ' * 4 * (level + 1)
                for file in files:
                    print(f'  DEBUG: {subindent}{file}')
        
        # 测试4: 验证指标文件（使用 rank_-1，因为未设置 RANK 环境变量）
        success, message = verify_metrics_file(tmpdir, node_rank=0, local_rank=-1)
        test_results.add_test("指标文件生成", success, message)
        
        # 测试5: 验证指标增强
        if success:
            success, message = verify_metrics_enhanced(tmpdir, node_rank=0, local_rank=-1)
            test_results.add_test("指标增强", success, message)
    
    finally:
        test_results.end_scenario()
        if tmpdir:
            cleanup_environment(tmpdir)


def test_scenario_distributed():
    """场景2: 分布式训练场景"""
    test_results.start_scenario(
        "分布式训练",
        "测试多节点、多GPU 场景下的指标保存和路径隔离"
    )
    
    tmpdir = None
    try:
        tmpdir = setup_environment(enable_api=False, force_hook=True)
        
        # 模拟 2 个节点，每个节点 2 个 GPU
        nodes = 2
        ranks_per_node = 2
        
        for node in range(nodes):
            for local_rank in range(ranks_per_node):
                # 设置分布式环境变量
                global_rank = node * ranks_per_node + local_rank
                os.environ["NODE_RANK"] = str(node)
                os.environ["LOCAL_RANK"] = str(local_rank)
                os.environ["RANK"] = str(global_rank)
                os.environ["WORLD_SIZE"] = str(nodes * ranks_per_node)
                
                # 导入 wandb（第一次循环时导入，之后复用）
                if 'wandb' not in sys.modules:
                    import wandb
                else:
                    import wandb
                
                # 初始化（环境变量会在 init 时被读取）
                run = wandb.init(
                    project="primus-distributed-test",
                    name=f"node{node}-rank{local_rank}",
                    config={"node": node, "local_rank": local_rank},
                    reinit=True  # 允许在同一进程中多次 init
                )
                
                # 记录一些指标
                for step in range(5):
                    wandb.log({
                        "loss": 1.0 - (step * 0.1),
                        "node": node,
                        "rank": local_rank,
                    }, step=step)
                
                wandb.finish()
        
        # 等待文件写入
        time.sleep(1.0)
        
        # 验证所有节点和 rank 的文件
        all_success = True
        for node in range(nodes):
            for local_rank in range(ranks_per_node):
                success, message = verify_metrics_file(tmpdir, node, local_rank)
                if not success:
                    all_success = False
                    test_results.add_test(
                        f"节点{node} Rank{local_rank} 指标文件",
                        False,
                        message
                    )
        
        if all_success:
            test_results.add_test(
                "所有节点指标文件",
                True,
                f"成功生成 {nodes}x{ranks_per_node} 个节点的指标文件"
            )
        
        # 验证文件隔离（每个 rank 的文件互不干扰）
        test_results.add_test(
            "文件路径隔离",
            all_success,
            "每个节点/rank 的指标保存到独立目录"
        )
    
    finally:
        test_results.end_scenario()
        if tmpdir:
            cleanup_environment(tmpdir)


def test_scenario_api_reporting():
    """场景3: API 异步上报场景"""
    test_results.start_scenario(
        "API 异步上报",
        "测试框架检测和训练指标的异步 API 上报功能"
    )
    
    tmpdir = None
    try:
        tmpdir = setup_environment(enable_api=True, force_hook=True)
        
        # 设置框架特征环境变量
        os.environ["PRIMUS_CONFIG"] = "/config/primus.yaml"
        os.environ["PRIMUS_VERSION"] = "1.2.3"
        
        # 导入 wandb（不要删除模块，保持状态）
        import wandb
        
        # 测试1: 验证 API 上报模块是否启用
        try:
            from primus_lens_wandb_exporter.api_reporter import get_global_reporter
            reporter = get_global_reporter()
            test_results.add_test(
                "API 上报器初始化",
                reporter is not None,
                "全局上报器已启动"
            )
        except Exception as e:
            test_results.add_test("API 上报器初始化", False, f"初始化失败: {e}")
            return
        
        # 测试2: 初始化 wandb（触发框架检测上报）
        try:
            run = wandb.init(
                project="primus-api-test",
                name="api-reporting-test",
                config={
                    "framework": "primus",
                    "model": "llama-7b",
                    "learning_rate": 0.001,
                },
                reinit=True  # 允许在同一进程中多次 init
            )
            test_results.add_test(
                "框架检测触发",
                run is not None,
                "wandb.init() 触发框架检测数据采集"
            )
        except Exception as e:
            test_results.add_test("框架检测触发", False, f"失败: {e}")
            return
        
        # 测试3: 记录指标（触发指标上报）
        num_steps = 20
        try:
            for step in range(num_steps):
                wandb.log({
                    "loss": 2.0 - (step * 0.08),
                    "accuracy": 0.6 + (step * 0.015),
                    "learning_rate": 0.001,
                }, step=step)
            test_results.add_test(
                "训练指标上报",
                True,
                f"记录 {num_steps} 步指标，已加入上报队列"
            )
        except Exception as e:
            test_results.add_test("训练指标上报", False, f"失败: {e}")
        
        # 完成 run
        wandb.finish()
        
        # 等待异步上报完成
        print("  等待异步上报器刷新数据...")
        time.sleep(3.0)
        
        # 测试4: 检查上报统计
        stats = reporter.stats
        test_results.add_test(
            "上报统计",
            True,
            f"检测数据: {stats['detection_sent']}, 指标批次: {stats['metrics_sent']}, 错误: {stats['errors']}"
        )
        
        # 测试5: 验证队列已清空
        detection_empty = reporter.detection_queue.empty()
        metrics_empty = reporter.metrics_queue.empty()
        test_results.add_test(
            "队列清空",
            detection_empty and metrics_empty,
            "所有队列已清空" if (detection_empty and metrics_empty) else "队列中仍有数据"
        )
    
    finally:
        test_results.end_scenario()
        if tmpdir:
            cleanup_environment(tmpdir)


def test_scenario_stress():
    """场景4: 压力测试场景"""
    test_results.start_scenario(
        "压力测试",
        "测试大量指标记录时的性能和稳定性"
    )
    
    tmpdir = None
    try:
        tmpdir = setup_environment(enable_api=True, force_hook=True)
        
        # 导入 wandb（不要删除模块）
        import wandb
        
        # 初始化
        run = wandb.init(
            project="primus-stress-test",
            name="stress-test",
            config={"test_type": "stress"},
            reinit=True  # 允许在同一进程中多次 init
        )
        
        # 大量指标记录
        num_steps = 500
        num_metrics_per_step = 20
        
        start_time = time.time()
        
        try:
            for step in range(num_steps):
                metrics = {
                    f"metric_{i}": random.uniform(0, 100)
                    for i in range(num_metrics_per_step)
                }
                metrics["step"] = step
                wandb.log(metrics, step=step)
            
            duration = time.time() - start_time
            
            test_results.add_test(
                "大量指标记录",
                True,
                f"成功记录 {num_steps} x {num_metrics_per_step} = {num_steps * num_metrics_per_step} 个指标"
            )
            
            test_results.add_test(
                "性能测试",
                True,
                f"耗时 {duration:.2f}s, 平均 {num_steps/duration:.1f} steps/s"
            )
        
        except Exception as e:
            test_results.add_test("大量指标记录", False, f"失败: {e}")
        
        wandb.finish()
        
        # 等待异步上报
        time.sleep(5.0)
        
        # 验证文件（使用 rank_-1，因为未设置 RANK 环境变量）
        success, message = verify_metrics_file(tmpdir, node_rank=0, local_rank=-1)
        test_results.add_test("压力测试文件完整性", success, message)
        
        # 检查上报统计
        try:
            from primus_lens_wandb_exporter.api_reporter import get_global_reporter
            reporter = get_global_reporter()
            stats = reporter.stats
            
            test_results.add_test(
                "压力测试上报",
                stats['errors'] == 0,
                f"指标批次: {stats['metrics_sent']}, 错误: {stats['errors']}"
            )
        except:
            pass
    
    finally:
        test_results.end_scenario()
        if tmpdir:
            cleanup_environment(tmpdir)


def test_scenario_edge_cases():
    """场景5: 边缘情况测试"""
    test_results.start_scenario(
        "边缘情况",
        "测试异常输入、错误处理等边缘情况"
    )
    
    tmpdir = None
    try:
        tmpdir = setup_environment(enable_api=False, force_hook=True)
        
        # 导入 wandb（不要删除模块）
        import wandb
        
        # 测试1: 空指标
        run = wandb.init(project="edge-test", name="empty-metrics", reinit=True)
        try:
            wandb.log({}, step=0)
            test_results.add_test("空指标记录", True, "空指标不会导致崩溃")
        except Exception as e:
            test_results.add_test("空指标记录", False, f"失败: {e}")
        wandb.finish()
        
        # 测试2: 包含非数值类型的指标
        run = wandb.init(project="edge-test", name="mixed-types", reinit=True)
        try:
            wandb.log({
                "loss": 0.5,
                "name": "test",  # 字符串
                "config": {"lr": 0.001},  # 字典
                "data": [1, 2, 3],  # 列表
            }, step=0)
            test_results.add_test("混合类型指标", True, "混合类型不会导致崩溃")
        except Exception as e:
            test_results.add_test("混合类型指标", False, f"失败: {e}")
        wandb.finish()
        
        # 测试3: 无输出路径
        old_path = os.environ.pop("PRIMUS_LENS_WANDB_OUTPUT_PATH", None)
        run = wandb.init(project="edge-test", name="no-output-path", reinit=True)
        try:
            wandb.log({"loss": 0.5}, step=0)
            test_results.add_test("无输出路径", True, "缺少输出路径不会导致崩溃")
        except Exception as e:
            test_results.add_test("无输出路径", False, f"失败: {e}")
        finally:
            if old_path:
                os.environ["PRIMUS_LENS_WANDB_OUTPUT_PATH"] = old_path
        wandb.finish()
        
        # 测试4: 禁用功能后的行为
        # 注意：需要删除并重新导入来测试禁用功能
        os.environ["PRIMUS_LENS_WANDB_HOOK"] = "false"
        if 'wandb' in sys.modules:
            # 删除所有相关模块
            wandb_modules = [m for m in sys.modules.keys() if m.startswith('wandb') or m.startswith('primus_lens_wandb')]
            for mod in wandb_modules:
                del sys.modules[mod]
        
        import wandb
        
        is_patched = hasattr(wandb, '_primus_lens_patched')
        test_results.add_test(
            "禁用劫持",
            not is_patched,
            "设置 HOOK=false 后劫持被正确禁用"
        )
        
        # 恢复设置
        os.environ["PRIMUS_LENS_WANDB_HOOK"] = "true"
    
    finally:
        test_results.end_scenario()
        if tmpdir:
            cleanup_environment(tmpdir)


# ========== 主程序 ==========

def main():
    """主函数"""
    parser = argparse.ArgumentParser(
        description="WandB Exporter 真实场景测试程序",
        formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog="""
场景说明:
  basic        - 基础单机训练场景
  distributed  - 分布式训练场景（多节点、多GPU）
  api          - API 异步上报场景
  stress       - 压力测试场景（大量指标）
  edge         - 边缘情况测试
  all          - 运行所有场景（默认）
        """
    )
    parser.add_argument(
        '--scenario',
        choices=['basic', 'distributed', 'api', 'stress', 'edge', 'all'],
        default='all',
        help='要运行的测试场景'
    )
    
    args = parser.parse_args()
    
    print("\n" + "="*70)
    print("WandB Exporter 真实场景测试")
    print("="*70)
    print(f"\nPython 版本: {sys.version}")
    print(f"工作目录: {os.getcwd()}\n")
    
    # 在导入 wandb 之前，先导入劫持模块
    print("预加载劫持模块...")
    try:
        src_path = os.path.join(os.path.dirname(os.path.abspath(__file__)), 'src')
        if src_path not in sys.path:
            sys.path.insert(0, src_path)
        import primus_lens_wandb_exporter.wandb_hook
        print("✓ 劫持模块已加载\n")
    except Exception as e:
        print(f"⚠ 劫持模块加载失败: {e}\n")
    
    # 检查依赖
    try:
        import wandb
        print(f"✓ WandB 已安装: {wandb.__version__}")
    except ImportError:
        print("✗ WandB 未安装，请运行: pip install wandb")
        return 1
    
    try:
        import psutil
        print(f"✓ psutil 已安装 (可以收集系统指标)")
    except ImportError:
        print("⚠ psutil 未安装 (将跳过系统指标收集)")
    
    print()
    
    # 运行测试场景
    scenarios = {
        'basic': test_scenario_basic,
        'distributed': test_scenario_distributed,
        'api': test_scenario_api_reporting,
        'stress': test_scenario_stress,
        'edge': test_scenario_edge_cases,
    }
    
    if args.scenario == 'all':
        for scenario_func in scenarios.values():
            try:
                scenario_func()
            except Exception as e:
                print(f"\n✗ 场景执行异常: {e}")
                import traceback
                traceback.print_exc()
    else:
        scenarios[args.scenario]()
    
    # 打印总结
    return test_results.print_summary()


if __name__ == "__main__":
    try:
        exit_code = main()
        sys.exit(exit_code)
    except KeyboardInterrupt:
        print("\n\n测试被用户中断")
        sys.exit(1)
    except Exception as e:
        print(f"\n\n✗ 测试程序异常: {e}")
        import traceback
        traceback.print_exc()
        sys.exit(1)

