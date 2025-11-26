#!/usr/bin/env python3
"""
测试 WandB Exporter 的 API 上报功能
"""

import os
import sys
import time
import unittest
from unittest.mock import Mock, patch, MagicMock

# 添加 src 到路径
sys.path.insert(0, os.path.join(os.path.dirname(__file__), 'src'))

from primus_lens_wandb_exporter.api_reporter import AsyncAPIReporter
from primus_lens_wandb_exporter.data_collector import DataCollector


class TestAsyncAPIReporter(unittest.TestCase):
    """测试异步 API 上报器"""
    
    def setUp(self):
        """设置测试环境"""
        self.reporter = AsyncAPIReporter(
            api_base_url="http://test-api:8080/api/v1",
            batch_size=5,
            flush_interval=0.5,
        )
    
    def tearDown(self):
        """清理测试环境"""
        if self.reporter:
            self.reporter.stop()
    
    def test_reporter_initialization(self):
        """测试上报器初始化"""
        self.assertIsNotNone(self.reporter)
        self.assertEqual(self.reporter.api_base_url, "http://test-api:8080/api/v1")
        self.assertEqual(self.reporter.batch_size, 5)
        self.assertFalse(self.reporter.running)
    
    def test_reporter_start_stop(self):
        """测试启动和停止"""
        self.reporter.start()
        self.assertTrue(self.reporter.running)
        self.assertIsNotNone(self.reporter.worker_thread)
        
        self.reporter.stop()
        self.assertFalse(self.reporter.running)
    
    def test_report_detection(self):
        """测试框架检测数据上报"""
        detection_data = {
            "workload_uid": "test-workload",
            "framework": "primus",
        }
        
        self.reporter.report_detection(detection_data)
        self.assertEqual(self.reporter.detection_queue.qsize(), 1)
    
    def test_report_metrics(self):
        """测试指标数据上报"""
        metrics_data = {
            "workload_uid": "test-workload",
            "metrics": [{"name": "loss", "value": 2.5}],
        }
        
        self.reporter.report_metrics(metrics_data)
        self.assertEqual(self.reporter.metrics_queue.qsize(), 1)
    
    def test_queue_overflow(self):
        """测试队列溢出处理"""
        # 填满队列
        for i in range(150):
            self.reporter.report_detection({"id": i})
        
        # 队列应该只保留最大容量
        self.assertLessEqual(self.reporter.detection_queue.qsize(), 100)


class TestDataCollector(unittest.TestCase):
    """测试数据采集器"""
    
    def setUp(self):
        """设置测试环境"""
        self.collector = DataCollector()
        
        # 设置测试环境变量
        os.environ["WORKLOAD_UID"] = "test-workload-123"
        os.environ["POD_NAME"] = "test-pod-456"
        os.environ["PRIMUS_CONFIG"] = "/test/config.yaml"
        os.environ["PRIMUS_VERSION"] = "1.2.3"
    
    def tearDown(self):
        """清理环境变量"""
        env_vars = ["WORKLOAD_UID", "POD_NAME", "PRIMUS_CONFIG", "PRIMUS_VERSION"]
        for var in env_vars:
            if var in os.environ:
                del os.environ[var]
    
    def test_collector_initialization(self):
        """测试采集器初始化"""
        self.assertIsNotNone(self.collector)
        self.assertEqual(self.collector.collector_version, "1.0.0")
    
    def test_extract_environment_vars(self):
        """测试环境变量提取"""
        env_vars = self.collector._extract_environment_vars()
        
        self.assertIn("PRIMUS_CONFIG", env_vars)
        self.assertEqual(env_vars["PRIMUS_CONFIG"], "/test/config.yaml")
        self.assertEqual(env_vars["PRIMUS_VERSION"], "1.2.3")
    
    def test_get_framework_hints(self):
        """测试 hints 生成"""
        evidence = {
            "environment": {
                "PRIMUS_CONFIG": "/config.yaml",
                "PRIMUS_VERSION": "1.2.3",
            },
            "wandb": {
                "config": {"framework": "primus"},
            },
            "pytorch": {"available": False},
        }
        
        hints = self.collector._get_framework_hints(evidence)
        
        self.assertIn("primus", hints["possible_frameworks"])
        self.assertEqual(hints["confidence"], "high")
        self.assertGreater(len(hints["primary_indicators"]), 0)
    
    def test_collect_env_hints(self):
        """测试环境变量 hints 采集"""
        hints = {
            "possible_frameworks": [],
            "primary_indicators": [],
        }
        
        env = {
            "PRIMUS_CONFIG": "/config.yaml",
            "DEEPSPEED_CONFIG": "/ds_config.json",
        }
        
        self.collector._collect_env_hints(env, hints)
        
        self.assertIn("primus", hints["possible_frameworks"])
        self.assertIn("deepspeed", hints["possible_frameworks"])
    
    def test_evaluate_confidence(self):
        """测试置信度评估"""
        # High confidence
        indicators = ["PRIMUS env vars", "DEEPSPEED env vars"]
        self.assertEqual(self.collector._evaluate_confidence(indicators), "high")
        
        # Medium confidence
        indicators = ["PRIMUS env vars"]
        self.assertEqual(self.collector._evaluate_confidence(indicators), "medium")
        
        # Low confidence
        indicators = ["project_name=test"]
        self.assertEqual(self.collector._evaluate_confidence(indicators), "low")


class TestIntegration(unittest.TestCase):
    """集成测试"""
    
    def setUp(self):
        """设置测试环境"""
        os.environ["WORKLOAD_UID"] = "integration-test-workload"
        os.environ["POD_NAME"] = "integration-test-pod"
        os.environ["PRIMUS_CONFIG"] = "/config/primus.yaml"
    
    def tearDown(self):
        """清理环境"""
        env_vars = ["WORKLOAD_UID", "POD_NAME", "PRIMUS_CONFIG"]
        for var in env_vars:
            if var in os.environ:
                del os.environ[var]
    
    @patch('primus_lens_wandb_exporter.api_reporter.urlopen')
    def test_end_to_end_detection(self, mock_urlopen):
        """测试端到端的检测数据采集和上报"""
        # Mock HTTP 响应
        mock_response = MagicMock()
        mock_response.status = 200
        mock_response.__enter__ = Mock(return_value=mock_response)
        mock_response.__exit__ = Mock(return_value=False)
        mock_urlopen.return_value = mock_response
        
        # 创建组件
        collector = DataCollector()
        reporter = AsyncAPIReporter(
            api_base_url="http://test:8080/api/v1",
            flush_interval=0.1,
        )
        reporter.start()
        
        # 模拟 wandb run
        mock_run = Mock()
        mock_run.project = "test-project"
        mock_run.name = "test-run"
        mock_run.id = "test-run-123"
        mock_run.config = {"framework": "primus"}
        
        # 采集数据
        detection_data = collector.collect_detection_data(mock_run)
        
        # 验证数据结构
        self.assertEqual(detection_data["source"], "wandb")
        self.assertEqual(detection_data["workload_uid"], "integration-test-workload")
        self.assertIn("primus", detection_data["hints"]["possible_frameworks"])
        
        # 上报数据
        reporter.report_detection(detection_data)
        
        # 等待处理
        time.sleep(0.5)
        
        # 停止上报器
        reporter.stop()
        
        # 验证统计
        self.assertGreater(reporter.stats["detection_sent"], 0)


def run_tests():
    """运行所有测试"""
    print("=" * 60)
    print("WandB Exporter API Reporting - Test Suite")
    print("=" * 60)
    print()
    
    # 创建测试套件
    loader = unittest.TestLoader()
    suite = unittest.TestSuite()
    
    # 添加测试
    suite.addTests(loader.loadTestsFromTestCase(TestAsyncAPIReporter))
    suite.addTests(loader.loadTestsFromTestCase(TestDataCollector))
    suite.addTests(loader.loadTestsFromTestCase(TestIntegration))
    
    # 运行测试
    runner = unittest.TextTestRunner(verbosity=2)
    result = runner.run(suite)
    
    # 输出总结
    print()
    print("=" * 60)
    print("Test Summary")
    print("=" * 60)
    print(f"Tests run: {result.testsRun}")
    print(f"Successes: {result.testsRun - len(result.failures) - len(result.errors)}")
    print(f"Failures: {len(result.failures)}")
    print(f"Errors: {len(result.errors)}")
    print()
    
    return 0 if result.wasSuccessful() else 1


if __name__ == "__main__":
    sys.exit(run_tests())

