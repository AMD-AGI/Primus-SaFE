"""Tests for GPU Usage Analysis Agent."""

import pytest
from unittest.mock import Mock, patch
from langchain_core.messages import HumanMessage

from gpu_usage_agent import GPUUsageAnalysisAgent
from gpu_usage_agent.state import GPUAnalysisState


@pytest.fixture
def mock_llm():
    """Create a mock LLM"""
    llm = Mock()
    return llm


@pytest.fixture
def agent(mock_llm):
    """Create an Agent instance"""
    return GPUUsageAnalysisAgent(
        llm=mock_llm,
        api_base_url="http://localhost:8080",
        cluster_name="test-cluster"
    )


def test_agent_initialization(agent):
    """Test Agent initialization"""
    assert agent.api_base_url == "http://localhost:8080"
    assert agent.cluster_name == "test-cluster"
    assert agent.max_iterations == 10
    assert len(agent.tools) > 0


def test_understand_query_trend(agent, mock_llm):
    """Test query understanding - trend analysis"""
    # Mock LLM response
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
        "understanding": "User wants to view cluster GPU utilization trend for the past 7 days"
    }
    '''
    mock_llm.invoke.return_value = mock_response
    
    state: GPUAnalysisState = {
        "user_query": "What is the GPU utilization trend over the past 7 days?",
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
    """Test basic conversation"""
    # Mock LLM response
    mock_response = Mock()
    mock_response.content = "This is a test answer"
    mock_llm.invoke.return_value = mock_response
    
    with patch.object(agent, 'graph') as mock_graph:
        mock_graph.invoke.return_value = {
            "answer": "Cluster GPU utilization shows an overall upward trend over the past 7 days",
            "insights": ["Utilization increased from 55% to 68%"],
            "data_collected": [],
            "conversation_history": [],
            "intent": ["trend"],
            "entities": {},
            "analysis_plan": [],
            "tool_calls": [],
            "iterations": 1
        }
        
        result = agent.chat("What is the GPU utilization trend over the past 7 days?")
        
        assert "answer" in result
        assert "insights" in result
        assert "debug_info" in result


@pytest.mark.asyncio
async def test_achat(agent):
    """Test asynchronous conversation"""
    with patch.object(agent, 'chat') as mock_chat:
        mock_chat.return_value = {
            "answer": "Test answer",
            "insights": [],
            "data_collected": [],
            "conversation_history": [],
            "debug_info": {}
        }
        
        result = await agent.achat("Test query")
        
        assert result["answer"] == "Test answer"


def test_should_continue_after_understand_clarify(agent):
    """Test scenario requiring clarification"""
    state: GPUAnalysisState = {
        "user_query": "Query",
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
        "clarification_question": "Please provide more information",
        "should_continue": True,
        "cluster_name": "",
        "start_time": None,
        "iterations": 0,
        "max_iterations": 10,
        "error_message": None
    }
    
    result = agent._should_continue_after_understand(state)
    
    assert result == "clarify"
    assert state["answer"] == "Please provide more information"


def test_should_call_tool_continue(agent):
    """Test should continue calling tools"""
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
    """Test reaching maximum iteration count"""
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

