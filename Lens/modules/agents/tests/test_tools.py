"""Tests for GPU Analysis Tools."""

import pytest
import json
from unittest.mock import Mock, patch
from datetime import datetime, timedelta

from gpu_usage_agent.tools import GPUAnalysisTools


@pytest.fixture
def tools():
    """创建工具实例"""
    return GPUAnalysisTools(
        api_base_url="http://localhost:8080",
        cluster_name="test-cluster"
    )


@pytest.fixture
def mock_response():
    """创建模拟的 API 响应"""
    return {
        "code": 0,
        "message": "success",
        "data": [
            {
                "id": 1,
                "cluster_name": "test-cluster",
                "stat_hour": "2025-11-05T14:00:00Z",
                "total_gpu_capacity": 128,
                "allocated_gpu_count": 96.5,
                "allocation_rate": 0.7539,
                "avg_utilization": 0.6823,
                "max_utilization": 0.9850,
                "min_utilization": 0.1234,
                "p50_utilization": 0.6750,
                "p95_utilization": 0.9200
            }
        ]
    }


def test_tools_initialization(tools):
    """测试工具初始化"""
    assert tools.api_base_url == "http://localhost:8080"
    assert tools.cluster_name == "test-cluster"


def test_make_request(tools, mock_response):
    """测试 API 请求"""
    with patch('requests.get') as mock_get:
        mock_get.return_value.json.return_value = mock_response
        mock_get.return_value.raise_for_status = Mock()
        
        result = tools._make_request("/test", {"param": "value"})
        
        assert result["code"] == 0
        mock_get.assert_called_once()


def test_query_gpu_usage_trend_cluster(tools, mock_response):
    """测试查询集群使用率趋势"""
    with patch.object(tools, '_make_request', return_value=mock_response):
        result_str = tools.query_gpu_usage_trend(
            dimension="cluster",
            granularity="day",
            time_range_days=7,
            metric_type="utilization"
        )
        
        result = json.loads(result_str)
        
        assert "data_points" in result
        assert "statistics" in result
        assert result["statistics"]["average"] > 0


def test_query_gpu_usage_trend_namespace(tools, mock_response):
    """测试查询 namespace 使用率趋势"""
    with patch.object(tools, '_make_request', return_value=mock_response):
        result_str = tools.query_gpu_usage_trend(
            dimension="namespace",
            granularity="day",
            time_range_days=7,
            dimension_value="ml-training",
            metric_type="utilization"
        )
        
        result = json.loads(result_str)
        
        assert "data_points" in result
        assert "statistics" in result


def test_query_gpu_usage_trend_label(tools, mock_response):
    """测试查询 label 使用率趋势"""
    with patch.object(tools, '_make_request', return_value=mock_response):
        result_str = tools.query_gpu_usage_trend(
            dimension="label",
            granularity="day",
            time_range_days=7,
            dimension_value="team:research",
            metric_type="utilization"
        )
        
        result = json.loads(result_str)
        
        assert "data_points" in result


def test_analyze_workload_history(tools):
    """测试分析 workload 历史"""
    mock_data = {
        "code": 0,
        "message": "success",
        "data": {
            "data": [
                {
                    "uid": "workload-1",
                    "name": "training-job-1",
                    "namespace": "ml-training",
                    "gpuAllocated": 8
                }
            ],
            "total": 1
        }
    }
    
    with patch.object(tools, '_make_request', return_value=mock_data):
        result_str = tools.analyze_workload_history(
            time_range_days=7,
            namespace="ml-training",
            limit=20
        )
        
        result = json.loads(result_str)
        
        assert "workloads" in result
        assert "aggregated_stats" in result
        assert result["total_count"] == 1


def test_get_latest_snapshot(tools):
    """测试获取最新快照"""
    mock_data = {
        "code": 0,
        "message": "success",
        "data": {
            "cluster_name": "test-cluster",
            "snapshot_time": "2025-11-05T14:30:00Z",
            "total_gpu_capacity": 128,
            "allocated_gpu_count": 96
        }
    }
    
    with patch.object(tools, '_make_request', return_value=mock_data):
        result_str = tools.get_latest_snapshot()
        result = json.loads(result_str)
        
        assert result["cluster_name"] == "test-cluster"
        assert result["total_gpu_capacity"] == 128


def test_get_workload_metadata(tools):
    """测试获取 workload 元数据"""
    mock_data = {
        "code": 0,
        "message": "success",
        "data": {
            "namespaces": ["ml-training", "ml-inference"],
            "kinds": ["Job", "Deployment"]
        }
    }
    
    with patch.object(tools, '_make_request', return_value=mock_data):
        result_str = tools.get_workload_metadata()
        result = json.loads(result_str)
        
        assert "namespaces" in result
        assert "kinds" in result
        assert len(result["namespaces"]) == 2


def test_get_tools(tools):
    """测试获取工具列表"""
    tool_list = tools.get_tools()
    
    assert len(tool_list) >= 7  # 包含新增的元数据工具
    assert all(callable(tool) for tool in tool_list)


def test_query_gpu_usage_trend_no_data(tools):
    """测试查询无数据的情况"""
    mock_empty = {
        "code": 0,
        "message": "success",
        "data": []
    }
    
    with patch.object(tools, '_make_request', return_value=mock_empty):
        result_str = tools.query_gpu_usage_trend(
            dimension="cluster",
            granularity="day",
            time_range_days=7
        )
        
        result = json.loads(result_str)
        
        assert result["data_points"] == []
        assert result["statistics"]["trend"] == "no_data"


def test_api_error_handling(tools):
    """测试 API 错误处理"""
    mock_error = {
        "code": 500,
        "message": "Internal server error",
        "data": None
    }
    
    with patch.object(tools, '_make_request', return_value=mock_error):
        result_str = tools.query_gpu_usage_trend(
            dimension="cluster",
            granularity="day",
            time_range_days=7
        )
        
        result = json.loads(result_str)
        
        assert "error" in result
        assert result["data"] is None

