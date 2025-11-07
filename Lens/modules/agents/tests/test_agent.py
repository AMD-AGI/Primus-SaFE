"""Tests for GPU Usage Analysis Agent."""

import pytest
from unittest.mock import Mock, patch
from langchain_core.messages import HumanMessage

from gpu_usage_agent import GPUUsageAnalysisAgent
from gpu_usage_agent.state import GPUAnalysisState


@pytest.fixture
def mock_llm():
    """创建模拟的 LLM"""
    llm = Mock()
    return llm


@pytest.fixture
def agent(mock_llm):
    """创建 Agent 实例"""
    return GPUUsageAnalysisAgent(
        llm=mock_llm,
        api_base_url="http://localhost:8080",
        cluster_name="test-cluster"
    )


def test_agent_initialization(agent):
    """测试 Agent 初始化"""
    assert agent.api_base_url == "http://localhost:8080"
    assert agent.cluster_name == "test-cluster"
    assert agent.max_iterations == 10
    assert len(agent.tools) > 0


def test_understand_query_trend(agent, mock_llm):
    """测试理解查询 - 趋势分析"""
    # 模拟 LLM 返回
    mock_response = Mock()
    mock_response.content = '''
    {
        "intent": ["trend"],
        "entities": {
            "time_range": {"type": "relative", "value": "7d"},
            "dimension": "cluster",
            "metric": "utilization"
        },
        "needs_clarification": false,
        "understanding": "用户想查看最近7天集群GPU使用率趋势"
    }
    '''
    mock_llm.invoke.return_value = mock_response
    
    state: GPUAnalysisState = {
        "user_query": "最近7天的GPU使用率趋势如何？",
        "conversation_history": [],
        "intent": [],
        "entities": {},
        "current_step": 0,
        "analysis_plan": [],
        "tool_calls": [],
        "data_collected": [],
        "insights": [],
        "answer": "",
        "needs_clarification": False,
        "clarification_question": None,
        "should_continue": True,
        "cluster_name": "test-cluster",
        "start_time": None,
        "iterations": 0,
        "max_iterations": 10,
        "error_message": None
    }
    
    result = agent._understand_query(state)
    
    assert result["intent"] == ["trend"]
    assert result["entities"]["dimension"] == "cluster"
    assert not result["needs_clarification"]


def test_chat_basic_query(agent, mock_llm):
    """测试基本对话"""
    # 模拟 LLM 返回
    mock_response = Mock()
    mock_response.content = "这是一个测试回答"
    mock_llm.invoke.return_value = mock_response
    
    with patch.object(agent, 'graph') as mock_graph:
        mock_graph.invoke.return_value = {
            "answer": "最近7天集群GPU使用率整体呈上升趋势",
            "insights": ["使用率从55%上升到68%"],
            "data_collected": [],
            "conversation_history": [],
            "intent": ["trend"],
            "entities": {},
            "analysis_plan": [],
            "tool_calls": [],
            "iterations": 1
        }
        
        result = agent.chat("最近7天的GPU使用率趋势如何？")
        
        assert "answer" in result
        assert "insights" in result
        assert "debug_info" in result


@pytest.mark.asyncio
async def test_achat(agent):
    """测试异步对话"""
    with patch.object(agent, 'chat') as mock_chat:
        mock_chat.return_value = {
            "answer": "测试回答",
            "insights": [],
            "data_collected": [],
            "conversation_history": [],
            "debug_info": {}
        }
        
        result = await agent.achat("测试查询")
        
        assert result["answer"] == "测试回答"


def test_should_continue_after_understand_clarify(agent):
    """测试需要澄清的情况"""
    state: GPUAnalysisState = {
        "user_query": "查询",
        "conversation_history": [],
        "intent": [],
        "entities": {},
        "current_step": 0,
        "analysis_plan": [],
        "tool_calls": [],
        "data_collected": [],
        "insights": [],
        "answer": "",
        "needs_clarification": True,
        "clarification_question": "请提供更多信息",
        "should_continue": True,
        "cluster_name": "",
        "start_time": None,
        "iterations": 0,
        "max_iterations": 10,
        "error_message": None
    }
    
    result = agent._should_continue_after_understand(state)
    
    assert result == "clarify"
    assert state["answer"] == "请提供更多信息"


def test_should_call_tool_continue(agent):
    """测试应该继续调用工具"""
    state: GPUAnalysisState = {
        "user_query": "",
        "conversation_history": [],
        "intent": [],
        "entities": {},
        "current_step": 0,
        "analysis_plan": [],
        "tool_calls": [],
        "data_collected": [],
        "insights": [],
        "answer": "",
        "needs_clarification": False,
        "clarification_question": None,
        "should_continue": True,
        "cluster_name": "",
        "start_time": None,
        "iterations": 0,
        "max_iterations": 10,
        "error_message": None
    }
    
    result = agent._should_call_tool(state)
    
    assert result == "tools"
    assert state["iterations"] == 1


def test_should_call_tool_max_iterations(agent):
    """测试达到最大迭代次数"""
    state: GPUAnalysisState = {
        "user_query": "",
        "conversation_history": [],
        "intent": [],
        "entities": {},
        "current_step": 0,
        "analysis_plan": [],
        "tool_calls": [],
        "data_collected": [],
        "insights": [],
        "answer": "",
        "needs_clarification": False,
        "clarification_question": None,
        "should_continue": True,
        "cluster_name": "",
        "start_time": None,
        "iterations": 10,
        "max_iterations": 10,
        "error_message": None
    }
    
    result = agent._should_call_tool(state)
    
    assert result == "synthesize"

