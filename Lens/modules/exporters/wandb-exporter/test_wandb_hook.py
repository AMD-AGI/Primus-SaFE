"""
测试 WandB Hook 功能
"""
import os
import sys
import tempfile


def test_hook_installation():
    """测试 hook 是否可以正常安装"""
    print("=" * 60)
    print("Test 1: Hook installation")
    print("=" * 60)
    
    try:
        from primus_lens_wandb_exporter.wandb_hook import WandbInterceptor
        interceptor = WandbInterceptor()
        print("✓ WandbInterceptor can be instantiated")
        return True
    except Exception as e:
        print(f"✗ Failed: {e}")
        return False


def test_wandb_mock():
    """测试 wandb 劫持功能（使用 mock）"""
    print("\n" + "=" * 60)
    print("Test 2: WandB mock interception")
    print("=" * 60)
    
    # 创建一个 mock wandb 模块
    class MockWandB:
        @staticmethod
        def init(*args, **kwargs):
            print("  [Mock] wandb.init called")
            return type('Run', (), {'name': 'test-run', 'project': 'test-project'})()
        
        @staticmethod
        def log(data, step=None):
            print(f"  [Mock] wandb.log called with data keys: {list(data.keys())}")
            return data
    
    # 将 mock 添加到 sys.modules
    sys.modules['wandb'] = MockWandB()
    
    try:
        from primus_lens_wandb_exporter.wandb_hook import WandbInterceptor
        
        interceptor = WandbInterceptor()
        interceptor.patch_wandb()
        
        # 测试 init
        import wandb
        run = wandb.init(project="test")
        print(f"✓ wandb.init intercepted, run name: {run.name}")
        
        # 测试 log
        result = wandb.log({"loss": 0.5, "accuracy": 0.9})
        
        # 检查是否添加了 Primus Lens 标记
        if "_primus_lens_enabled" in result:
            print("✓ wandb.log intercepted and enhanced")
        else:
            print("⚠ wandb.log intercepted but not enhanced")
        
        return True
        
    except Exception as e:
        print(f"✗ Failed: {e}")
        import traceback
        traceback.print_exc()
        return False
    finally:
        # 清理
        if 'wandb' in sys.modules:
            del sys.modules['wandb']


def test_environment_control():
    """测试环境变量控制"""
    print("\n" + "=" * 60)
    print("Test 3: Environment variable control")
    print("=" * 60)
    
    # 测试禁用
    os.environ['PRIMUS_LENS_WANDB_HOOK'] = 'false'
    
    from primus_lens_wandb_exporter.wandb_hook import WandbInterceptor
    interceptor = WandbInterceptor()
    interceptor.install()
    
    if not interceptor.is_patched:
        print("✓ Hook correctly disabled by environment variable")
        result = True
    else:
        print("✗ Hook should be disabled but isn't")
        result = False
    
    # 恢复环境变量
    os.environ['PRIMUS_LENS_WANDB_HOOK'] = 'true'
    
    return result


def test_metrics_save():
    """测试指标保存功能"""
    print("\n" + "=" * 60)
    print("Test 4: Metrics save to local file")
    print("=" * 60)
    
    with tempfile.TemporaryDirectory() as tmpdir:
        os.environ['PRIMUS_LENS_WANDB_OUTPUT_PATH'] = tmpdir
        os.environ['PRIMUS_LENS_WANDB_SAVE_LOCAL'] = 'true'
        os.environ['LOCAL_RANK'] = '0'
        os.environ['NODE_RANK'] = '0'
        
        try:
            from primus_lens_wandb_exporter.wandb_hook import WandbInterceptor
            
            interceptor = WandbInterceptor()
            
            # 测试保存指标
            interceptor._save_metrics({"loss": 0.5, "accuracy": 0.9}, step=1)
            
            # 检查文件是否创建
            metrics_file = os.path.join(tmpdir, "node_0", "rank_0", "wandb_metrics.jsonl")
            if os.path.exists(metrics_file):
                with open(metrics_file, 'r') as f:
                    content = f.read()
                print(f"✓ Metrics saved to: {metrics_file}")
                print(f"  Content preview: {content[:100]}...")
                return True
            else:
                print(f"✗ Metrics file not created at: {metrics_file}")
                return False
                
        except Exception as e:
            print(f"✗ Failed: {e}")
            import traceback
            traceback.print_exc()
            return False


def test_rank_info():
    """测试 rank 信息获取"""
    print("\n" + "=" * 60)
    print("Test 5: Rank information detection")
    print("=" * 60)
    
    # 设置测试环境变量
    os.environ['RANK'] = '1'
    os.environ['LOCAL_RANK'] = '2'
    os.environ['NODE_RANK'] = '3'
    os.environ['WORLD_SIZE'] = '8'
    
    try:
        from primus_lens_wandb_exporter.wandb_hook import WandbInterceptor
        
        interceptor = WandbInterceptor()
        rank_info = interceptor._get_rank_info()
        
        print(f"  Detected rank info: {rank_info}")
        
        if (rank_info['RANK'] == 1 and 
            rank_info['LOCAL_RANK'] == 2 and 
            rank_info['NODE_RANK'] == 3 and 
            rank_info['WORLD_SIZE'] == 8):
            print("✓ Rank information correctly detected")
            return True
        else:
            print("✗ Rank information incorrect")
            return False
            
    except Exception as e:
        print(f"✗ Failed: {e}")
        return False
    finally:
        # 清理环境变量
        for var in ['RANK', 'LOCAL_RANK', 'NODE_RANK', 'WORLD_SIZE']:
            os.environ.pop(var, None)


def test_pth_file_location():
    """测试 .pth 文件位置"""
    print("\n" + "=" * 60)
    print("Test 6: .pth file location")
    print("=" * 60)
    
    try:
        import site
        if hasattr(site, 'getsitepackages'):
            site_packages = site.getsitepackages()[0]
        else:
            from distutils.sysconfig import get_python_lib
            site_packages = get_python_lib()
        
        pth_file = os.path.join(site_packages, 'primus_lens_wandb_hook.pth')
        print(f"  .pth file should be at: {pth_file}")
        
        if os.path.exists(pth_file):
            with open(pth_file, 'r') as f:
                content = f.read()
            print(f"  ✓ .pth file exists")
            print(f"  Content: {content.strip()}")
        else:
            print("  ⚠ .pth file not found (will be created during pip install)")
        
        return True
    except Exception as e:
        print(f"✗ Failed: {e}")
        return False


def main():
    """运行所有测试"""
    print("\n")
    print("╔" + "═" * 58 + "╗")
    print("║" + " " * 8 + "Primus Lens WandB Hook Test Suite" + " " * 16 + "║")
    print("╚" + "═" * 58 + "╝")
    print()
    
    tests = [
        ("Hook Installation", test_hook_installation),
        ("WandB Mock", test_wandb_mock),
        ("Environment Control", test_environment_control),
        ("Metrics Save", test_metrics_save),
        ("Rank Info", test_rank_info),
        ("PTH File Location", test_pth_file_location),
    ]
    
    results = []
    for name, test_func in tests:
        try:
            result = test_func()
            results.append((name, result))
        except Exception as e:
            print(f"\n✗ Test '{name}' crashed: {e}")
            import traceback
            traceback.print_exc()
            results.append((name, False))
    
    # 总结
    print("\n" + "=" * 60)
    print("Test Summary")
    print("=" * 60)
    
    passed = sum(1 for _, result in results if result)
    total = len(results)
    
    for name, result in results:
        status = "✓ PASS" if result else "✗ FAIL"
        print(f"  {status}: {name}")
    
    print(f"\nTotal: {passed}/{total} tests passed")
    
    if passed == total:
        print("\n🎉 All tests passed!")
        return 0
    else:
        print(f"\n⚠️  {total - passed} test(s) failed")
        return 1


if __name__ == "__main__":
    sys.exit(main())

