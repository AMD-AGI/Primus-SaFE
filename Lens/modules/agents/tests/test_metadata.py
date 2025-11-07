"""Tests for Metadata Tools."""

import pytest
import json
from unittest.mock import Mock, patch

from gpu_usage_agent.tools import GPUAnalysisTools


@pytest.fixture
def tools():
    """创建工具实例"""
    return GPUAnalysisTools(
        api_base_url="http://localhost:8080",
        cluster_name="test-cluster"
    )


def test_get_available_clusters(tools):
    """测试获取集群列表"""
    mock_response = {
        "code": 0,
        "message": "success",
        "data": ["gpu-cluster-01", "gpu-cluster-02", "gpu-cluster-03"]
    }
    
    with patch.object(tools, '_make_request', return_value=mock_response):
        result_str = tools.get_available_clusters()
        result = json.loads(result_str)
        
        assert "clusters" in result
        assert len(result["clusters"]) == 3
        assert result["count"] == 3
        assert "gpu-cluster-01" in result["clusters"]


def test_get_available_namespaces(tools):
    """测试获取 namespace 列表"""
    mock_response = {
        "code": 0,
        "message": "success",
        "data": ["ml-training", "ml-inference", "data-processing"]
    }
    
    with patch.object(tools, '_make_request', return_value=mock_response):
        result_str = tools.get_available_namespaces(time_range_days=7)
        result = json.loads(result_str)
        
        assert "namespaces" in result
        assert len(result["namespaces"]) == 3
        assert result["count"] == 3
        assert result["time_range_days"] == 7


def test_get_available_namespaces_with_cluster(tools):
    """测试获取指定集群的 namespace 列表"""
    mock_response = {
        "code": 0,
        "message": "success",
        "data": ["ml-training", "ml-inference"]
    }
    
    with patch.object(tools, '_make_request', return_value=mock_response):
        result_str = tools.get_available_namespaces(
            time_range_days=7,
            cluster="gpu-cluster-01"
        )
        result = json.loads(result_str)
        
        assert "namespaces" in result
        assert len(result["namespaces"]) == 2


def test_get_available_dimension_keys_label(tools):
    """测试获取 label keys"""
    mock_response = {
        "code": 0,
        "message": "success",
        "data": ["team", "project", "environment", "priority"]
    }
    
    with patch.object(tools, '_make_request', return_value=mock_response):
        result_str = tools.get_available_dimension_keys("label", 7)
        result = json.loads(result_str)
        
        assert result["dimension_type"] == "label"
        assert "dimension_keys" in result
        assert len(result["dimension_keys"]) == 4
        assert "team" in result["dimension_keys"]


def test_get_available_dimension_keys_annotation(tools):
    """测试获取 annotation keys"""
    mock_response = {
        "code": 0,
        "message": "success",
        "data": ["cost-center", "owner", "project-id"]
    }
    
    with patch.object(tools, '_make_request', return_value=mock_response):
        result_str = tools.get_available_dimension_keys("annotation", 7)
        result = json.loads(result_str)
        
        assert result["dimension_type"] == "annotation"
        assert len(result["dimension_keys"]) == 3


def test_get_available_dimension_keys_invalid_type(tools):
    """测试无效的维度类型"""
    result_str = tools.get_available_dimension_keys("invalid_type", 7)
    result = json.loads(result_str)
    
    assert "error" in result
    assert "Invalid dimension_type" in result["error"]


def test_get_available_clusters_error(tools):
    """测试获取集群列表失败"""
    mock_error = {
        "code": 500,
        "message": "Internal server error",
        "data": None
    }
    
    with patch.object(tools, '_make_request', return_value=mock_error):
        result_str = tools.get_available_clusters()
        result = json.loads(result_str)
        
        assert "error" in result
        assert result["clusters"] == []


def test_get_available_namespaces_empty(tools):
    """测试没有可用 namespace 的情况"""
    mock_response = {
        "code": 0,
        "message": "success",
        "data": []
    }
    
    with patch.object(tools, '_make_request', return_value=mock_response):
        result_str = tools.get_available_namespaces()
        result = json.loads(result_str)
        
        assert result["namespaces"] == []
        assert result["count"] == 0


def test_metadata_tools_in_tool_list(tools):
    """测试元数据工具是否在工具列表中"""
    tool_list = tools.get_tools()
    tool_names = [tool.__name__ for tool in tool_list]
    
    assert "get_available_clusters" in tool_names
    assert "get_available_namespaces" in tool_names
    assert "get_available_dimension_keys" in tool_names

