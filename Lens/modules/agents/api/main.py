"""FastAPI Application for GPU Usage Analysis Agent."""

import os
import logging
from typing import Optional, List
from datetime import datetime

from fastapi import FastAPI, HTTPException, Depends
from fastapi.middleware.cors import CORSMiddleware
from fastapi.responses import StreamingResponse
from pydantic import BaseModel, Field
import httpx
import json
import asyncio

from langchain_openai import ChatOpenAI
from langchain_anthropic import ChatAnthropic

from gpu_usage_agent import GPUUsageAnalysisAgent
from config import load_config, get_config

# Import cache and storage modules
from cache.factory import create_cache
from storage.factory import create_storage

# Configure logging
logging.basicConfig(
    level=logging.INFO,
    format='%(asctime)s - %(name)s - %(levelname)s - %(message)s'
)
logger = logging.getLogger(__name__)

# Set DEBUG level logging for OpenAI client to capture detailed errors
openai_logger = logging.getLogger("openai")
openai_logger.setLevel(logging.DEBUG)

# Set detailed logging for httpx
httpx_logger = logging.getLogger("httpx")
httpx_logger.setLevel(logging.WARNING)  # Only show warnings and errors

# ============================================================================
# Configuration
# ============================================================================

# Load configuration file (supports environment variable override)
config = load_config()
logger.info("Configuration loaded")

# API Configuration
API_HOST = get_config("api.host", "0.0.0.0")
API_PORT = get_config("api.port", 8001)
API_TITLE = get_config("api.title", "GPU Usage Analysis Agent API")
API_VERSION = get_config("api.version", "1.0.0")
API_DESCRIPTION = "LangGraph-based GPU utilization analysis conversational agent"

# Lens API Configuration
LENS_API_URL = get_config("lens.api_url", "http://localhost:30182")
CLUSTER_NAME = get_config("lens.cluster_name", None)
LENS_TIMEOUT = get_config("lens.timeout", 30)

# LLM Configuration
LLM_PROVIDER = get_config("llm.provider", "openai")
LLM_MODEL = get_config("llm.model", "gpt-4")
LLM_API_KEY = get_config("llm.api_key", "")
LLM_BASE_URL = get_config("llm.base_url", None)
LLM_TEMPERATURE = get_config("llm.temperature", 0)
LLM_MAX_TOKENS = get_config("llm.max_tokens", 2000)
LLM_VERIFY_SSL = get_config("llm.verify_ssl", True)  # Whether to verify SSL certificate

# Agent Configuration
AGENT_MAX_ITERATIONS = get_config("agent.max_iterations", 10)
AGENT_TIMEOUT = get_config("agent.timeout", 120)

# Cache Configuration
CACHE_ENABLED = get_config("cache.enabled", True)
CACHE_BACKEND = get_config("cache.backend", "disk")

# Storage Configuration
STORAGE_ENABLED = get_config("storage.enabled", True)
STORAGE_BACKEND = get_config("storage.backend", "file")
STORAGE_RETENTION_DAYS = get_config("storage.retention_days", 30)

# Print configuration information
logger.info(f"API Config: {API_HOST}:{API_PORT}")
logger.info(f"Lens API: {LENS_API_URL}")
logger.info(f"Cluster: {CLUSTER_NAME or 'None (must specify in request)'}")
logger.info(f"LLM: {LLM_PROVIDER} - {LLM_MODEL}")
if LLM_BASE_URL:
    logger.info(f"LLM Base URL: {LLM_BASE_URL}")
if not LLM_VERIFY_SSL:
    logger.warning("Warning: SSL certificate verification is disabled!")
logger.info(f"Cache: {'Enabled' if CACHE_ENABLED else 'Disabled'} - Backend: {CACHE_BACKEND}")
logger.info(f"Storage: {'Enabled' if STORAGE_ENABLED else 'Disabled'} - Backend: {STORAGE_BACKEND}")

# ============================================================================
# FastAPI App
# ============================================================================

app = FastAPI(
    title=API_TITLE,
    version=API_VERSION,
    description=API_DESCRIPTION
)

# CORS Configuration
app.add_middleware(
    CORSMiddleware,
    allow_origins=["*"],
    allow_credentials=True,
    allow_methods=["*"],
    allow_headers=["*"],
)


# ============================================================================
# Request/Response Models
# ============================================================================

class ChatRequest(BaseModel):
    """Chat request"""
    query: str = Field(..., description="User query")
    conversation_history: Optional[List[dict]] = Field(
        default=None,
        description="Conversation history"
    )
    cluster_name: Optional[str] = Field(
        default=None,
        description="Cluster name (optional)"
    )
    session_id: Optional[str] = Field(
        default=None,
        description="Session ID (for persistent storage)"
    )
    save_history: bool = Field(
        default=True,
        description="Whether to save conversation history"
    )


class ChatResponse(BaseModel):
    """Chat response"""
    answer: str = Field(..., description="Answer")
    insights: List[str] = Field(default=[], description="Insights")
    data_collected: List[dict] = Field(default=[], description="Collected data")
    conversation_history: List[dict] = Field(default=[], description="Conversation history")
    debug_info: Optional[dict] = Field(default=None, description="Debug info")
    timestamp: datetime = Field(default_factory=datetime.now, description="Timestamp")


class HealthResponse(BaseModel):
    """Health check response"""
    status: str
    version: str
    llm_provider: str
    lens_api_url: str


# ============================================================================
# Global Instances
# ============================================================================

# Global cache instance
_cache_instance = None

# Global storage instance
_storage_instance = None


def get_cache():
    """Get cache instance (singleton)"""
    global _cache_instance
    
    if not CACHE_ENABLED:
        return None
    
    if _cache_instance is None:
        try:
            # Get configuration based on backend type
            backend_config = get_config(f"cache.{CACHE_BACKEND}", {})
            
            _cache_instance = create_cache(
                backend=CACHE_BACKEND,
                **backend_config
            )
            logger.info(f"Cache instance created successfully: {CACHE_BACKEND}")
        except Exception as e:
            logger.error(f"Failed to create cache instance: {e}")
            return None
    
    return _cache_instance


def get_storage():
    """Get storage instance (singleton)"""
    global _storage_instance
    
    if not STORAGE_ENABLED:
        return None
    
    if _storage_instance is None:
        try:
            # Get configuration based on backend type
            backend_config = get_config(f"storage.{STORAGE_BACKEND}", {})
            
            _storage_instance = create_storage(
                backend=STORAGE_BACKEND,
                **backend_config
            )
            logger.info(f"Storage instance created successfully: {STORAGE_BACKEND}")
        except Exception as e:
            logger.error(f"Failed to create storage instance: {e}")
            return None
    
    return _storage_instance


# ============================================================================
# Dependencies
# ============================================================================

def get_llm():
    """Get language model instance"""
    if LLM_PROVIDER == "openai":
        # Create custom HTTP client to support SSL configuration
        http_client = None
        if not LLM_VERIFY_SSL:
            # Disable SSL certificate verification
            http_client = httpx.Client(verify=False)
            logger.warning("LLM API SSL certificate verification disabled")
        
        return ChatOpenAI(
            model=LLM_MODEL,
            api_key=LLM_API_KEY,
            base_url=LLM_BASE_URL,
            temperature=LLM_TEMPERATURE,
            max_tokens=LLM_MAX_TOKENS,
            http_client=http_client
        )
    elif LLM_PROVIDER == "anthropic":
        return ChatAnthropic(
            model=LLM_MODEL,
            api_key=LLM_API_KEY,
            temperature=LLM_TEMPERATURE,
            max_tokens=LLM_MAX_TOKENS
        )
    else:
        raise ValueError(f"Unsupported LLM provider: {LLM_PROVIDER}")


def get_agent(llm = Depends(get_llm)) -> GPUUsageAnalysisAgent:
    """Get Agent instance"""
    # Get cache instance
    cache = get_cache()
    
    return GPUUsageAnalysisAgent(
        llm=llm,
        api_base_url=LENS_API_URL,
        cluster_name=CLUSTER_NAME,
        cache=cache,
        cache_enabled=CACHE_ENABLED
    )


# ============================================================================
# API Endpoints
# ============================================================================

@app.get("/", response_model=HealthResponse)
async def root():
    """Root path, return API information"""
    return HealthResponse(
        status="healthy",
        version=API_VERSION,
        llm_provider=LLM_PROVIDER,
        lens_api_url=LENS_API_URL
    )


@app.get("/health", response_model=HealthResponse)
async def health_check():
    """Health check"""
    return HealthResponse(
        status="healthy",
        version=API_VERSION,
        llm_provider=LLM_PROVIDER,
        lens_api_url=LENS_API_URL
    )


@app.post("/api/gpu-analysis/chat", response_model=ChatResponse)
async def chat(
    request: ChatRequest,
    agent: GPUUsageAnalysisAgent = Depends(get_agent)
):
    """
    GPU utilization analysis chat interface (non-streaming)
    
    Process user's natural language query and return complete analysis results
    """
    try:
        # Get storage instance
        storage = get_storage()
        
        # If session_id is provided, try to load conversation history
        if request.session_id and storage and not request.conversation_history:
            try:
                saved_conversation = storage.load_conversation(request.session_id)
                if saved_conversation:
                    # Try to get messages
                    messages = saved_conversation.get("messages", [])
                    # Compatible with old format
                    if not messages and "conversation" in saved_conversation:
                        messages = saved_conversation["conversation"].get("messages", [])
                        if not messages:
                            messages = saved_conversation["conversation"].get("history", [])
                    
                    if messages:
                        request.conversation_history = messages
                        logger.info(f"Loaded conversation history for session {request.session_id} ({len(request.conversation_history)} messages)")
            except Exception as e:
                logger.warning(f"Failed to load conversation history: {e}")
        
        # If cluster name is specified in request, update agent's cluster name
        if request.cluster_name:
            agent.cluster_name = request.cluster_name
        
        # Generate or use provided session_id (generate early so it can be saved even on error)
        import uuid
        session_id = request.session_id or str(uuid.uuid4())
        
        # Call agent (agent internally converts Message objects to dictionaries)
        result = agent.chat(
            user_query=request.query,
            conversation_history=request.conversation_history
        )
        
        # Return session_id in response's debug_info
        if result.get("debug_info"):
            result["debug_info"]["session_id"] = session_id
        else:
            result["debug_info"] = {"session_id": session_id}
        
        # If storage is enabled and history should be saved, save conversation record
        if storage and request.save_history:
            try:
                # Load existing conversation history (if exists)
                existing_messages = []
                try:
                    saved_conversation = storage.load_conversation(session_id)
                    if saved_conversation and "messages" in saved_conversation:
                        existing_messages = saved_conversation["messages"]
                except Exception as e:
                    logger.warning(f"Failed to load existing conversation history: {e}")
                
                # Append new user question
                existing_messages.append({
                    "role": "user",
                    "content": request.query,
                    "timestamp": datetime.now().isoformat()
                })
                
                # Append assistant reply (including data and other additional info)
                assistant_message = {
                    "role": "assistant",
                    "content": result["answer"],
                    "timestamp": datetime.now().isoformat()
                }
                
                # Save data field (if exists and not empty)
                if result.get("data"):
                    assistant_message["data"] = result["data"]
                
                # Save insights field (if exists and not empty)
                if result.get("insights"):
                    assistant_message["insights"] = result["insights"]
                
                existing_messages.append(assistant_message)
                
                # Save conversation history
                conversation_data = {
                    "messages": existing_messages
                }
                
                metadata = {
                    "cluster_name": request.cluster_name or agent.cluster_name,
                    "timestamp": datetime.now().isoformat()
                }
                
                storage.save_conversation(
                    session_id=session_id,
                    conversation_data=conversation_data,
                    metadata=metadata
                )
                logger.info(f"Saved conversation history for session {session_id} ({len(existing_messages)} messages)")
                
            except Exception as e:
                logger.warning(f"Failed to save conversation history: {e}")
        
        return ChatResponse(
            answer=result["answer"],
            insights=result.get("insights", []),
            data_collected=result.get("data_collected", []),
            conversation_history=result.get("conversation_history", []),
            debug_info=result.get("debug_info")
        )
    
    except Exception as e:
        # Log detailed error information
        import traceback
        import uuid
        error_type = type(e).__name__
        error_msg = str(e)
        error_traceback = traceback.format_exc()
        
        logger.error("="*60)
        logger.error(f"Error occurred while processing query")
        logger.error(f"Error type: {error_type}")
        logger.error(f"Error message: {error_msg}")
        logger.error(f"Full stack trace:\n{error_traceback}")
        logger.error("="*60)
        
        # Save session even on error for debugging
        if storage and request.save_history:
            try:
                session_id = request.session_id or str(uuid.uuid4())
                
                # Load existing conversation history (if exists)
                existing_messages = []
                try:
                    saved_conversation = storage.load_conversation(session_id)
                    if saved_conversation and "messages" in saved_conversation:
                        existing_messages = saved_conversation["messages"]
                except:
                    pass
                
                # Append user question and error reply
                existing_messages.append({
                    "role": "user",
                    "content": request.query,
                    "timestamp": datetime.now().isoformat()
                })
                existing_messages.append({
                    "role": "assistant",
                    "content": f"Error occurred while processing query: {error_msg}",
                    "timestamp": datetime.now().isoformat(),
                    "error": True
                })
                
                conversation_data = {
                    "messages": existing_messages
                }
                metadata = {
                    "cluster_name": request.cluster_name or agent.cluster_name,
                    "timestamp": datetime.now().isoformat(),
                    "status": "error"
                }
                storage.save_conversation(
                    session_id=session_id,
                    conversation_data=conversation_data,
                    metadata=metadata
                )
                logger.info(f"Saved error session {session_id}")
            except Exception as save_error:
                logger.warning(f"Failed to save error session: {save_error}")
        
        # Check if it's a connection error
        if "Connection" in error_type or "connection" in error_msg.lower():
            detail = f"LLM API connection error: {error_msg}. Please check if LLM API address ({LLM_BASE_URL}) is accessible."
        else:
            detail = f"Error occurred while processing query ({error_type}): {error_msg}"
        
        raise HTTPException(
            status_code=500,
            detail=detail
        )


@app.post("/api/gpu-analysis/chat/stream")
async def chat_stream(
    request: ChatRequest,
    agent: GPUUsageAnalysisAgent = Depends(get_agent)
):
    """
    GPU utilization analysis chat interface (SSE streaming)
    
    Process user's natural language query and return analysis results in Server-Sent Events format
    Avoid frontend timeout issues
    """
    
    async def generate_sse():
        """Generate SSE event stream"""
        try:
            # Get storage instance
            storage = get_storage()
            
            # If session_id is provided, try to load conversation history
            if request.session_id and storage and not request.conversation_history:
                try:
                    saved_conversation = storage.load_conversation(request.session_id)
                    if saved_conversation:
                        # Try to get messages
                        messages = saved_conversation.get("messages", [])
                        # Compatible with old format
                        if not messages and "conversation" in saved_conversation:
                            messages = saved_conversation["conversation"].get("messages", [])
                            if not messages:
                                messages = saved_conversation["conversation"].get("history", [])
                        
                        if messages:
                            request.conversation_history = messages
                            logger.info(f"Loaded conversation history for session {request.session_id} ({len(request.conversation_history)} messages)")
                except Exception as e:
                    logger.warning(f"Failed to load conversation history: {e}")
            
            # If cluster name is specified in request, update agent's cluster name
            if request.cluster_name:
                agent.cluster_name = request.cluster_name
            
            # Generate or use provided session_id
            import uuid
            session_id = request.session_id or str(uuid.uuid4())
            
            # Send session_id
            yield f"data: {json.dumps({'type': 'session', 'session_id': session_id}, ensure_ascii=False)}\n\n"
            
            # Store final result for saving conversation history
            final_result = None
            
            # Call streaming agent
            async for chunk in agent.stream_chat(
                user_query=request.query,
                conversation_history=request.conversation_history
            ):
                # Send SSE event
                yield f"data: {json.dumps(chunk, ensure_ascii=False)}\n\n"
                
                # If it's the final result, save for later storage
                if chunk.get("type") == "final":
                    final_result = chunk
                
                # Add small delay to ensure client can receive properly
                await asyncio.sleep(0.01)
            
            # If storage is enabled and history should be saved, save conversation record
            if storage and request.save_history and final_result:
                try:
                    # Load existing conversation history (if exists)
                    existing_messages = []
                    try:
                        saved_conversation = storage.load_conversation(session_id)
                        if saved_conversation:
                            # Try to get messages
                            existing_messages = saved_conversation.get("messages", [])
                            # Compatible with old format
                            if not existing_messages and "conversation" in saved_conversation:
                                existing_messages = saved_conversation["conversation"].get("messages", [])
                                if not existing_messages:
                                    existing_messages = saved_conversation["conversation"].get("history", [])
                    except Exception as e:
                        logger.warning(f"Failed to load existing conversation history: {e}")
                    
                    # Append new user question
                    existing_messages.append({
                        "role": "user",
                        "content": request.query,
                        "timestamp": datetime.now().isoformat()
                    })
                    
                    # Append assistant reply (including data and other additional info)
                    assistant_message = {
                        "role": "assistant",
                        "content": final_result.get("answer", ""),
                        "timestamp": datetime.now().isoformat()
                    }
                    
                    # Save data field (if exists and not empty)
                    if final_result.get("data"):
                        assistant_message["data"] = final_result["data"]
                    
                    existing_messages.append(assistant_message)
                    
                    # Save conversation history
                    conversation_data = {
                        "messages": existing_messages
                    }
                    
                    metadata = {
                        "cluster_name": request.cluster_name or agent.cluster_name,
                        "timestamp": datetime.now().isoformat()
                    }
                    
                    storage.save_conversation(
                        session_id=session_id,
                        conversation_data=conversation_data,
                        metadata=metadata
                    )
                    logger.info(f"Saved conversation history for session {session_id} ({len(existing_messages)} messages)")
                    
                except Exception as e:
                    logger.warning(f"Failed to save conversation history: {e}")
            
            # Send end signal
            yield f"data: {json.dumps({'type': 'done'}, ensure_ascii=False)}\n\n"
            
        except Exception as e:
            # Log detailed error information
            import traceback
            error_type = type(e).__name__
            error_msg = str(e)
            error_traceback = traceback.format_exc()
            
            logger.error("="*60)
            logger.error(f"Error occurred while streaming query processing")
            logger.error(f"Error type: {error_type}")
            logger.error(f"Error message: {error_msg}")
            logger.error(f"Full stack trace:\n{error_traceback}")
            logger.error("="*60)
            
            # Send error event
            error_data = {
                "type": "error",
                "error_type": error_type,
                "error_message": error_msg,
                "traceback": error_traceback
            }
            yield f"data: {json.dumps(error_data, ensure_ascii=False)}\n\n"
    
    return StreamingResponse(
        generate_sse(),
        media_type="text/event-stream",
        headers={
            "Cache-Control": "no-cache",
            "Connection": "keep-alive",
            "X-Accel-Buffering": "no"  # Disable nginx buffering
        }
    )


@app.get("/api/gpu-analysis/capabilities")
async def get_capabilities():
    """
    Get Agent capabilities list
    
    Return query types and features supported by Agent
    """
    return {
        "capabilities": [
            {
                "type": "trend",
                "name": "Trend Analysis",
                "description": "Query GPU utilization trends across different dimensions and time granularities",
                "examples": [
                    "What's the utilization trend over the past few days?",
                    "What's the GPU utilization trend for the cluster this month?"
                ]
            },
            {
                "type": "compare",
                "name": "Comparison Analysis",
                "description": "Compare usage across different time periods and entities",
                "examples": [
                    "Compare this week's and last week's utilization",
                    "Compare GPU usage between ml-team and cv-team"
                ]
            },
            {
                "type": "realtime",
                "name": "Real-time Status",
                "description": "Query current GPU allocation and usage",
                "examples": [
                    "What's the current GPU usage in the cluster?",
                    "How many GPUs are currently in use?"
                ]
            }
        ],
        "supported_dimensions": [
            "cluster",
            "namespace",
            "label",
            "workload"
        ],
        "supported_metrics": [
            "utilization",
            "allocation_rate"
        ]
    }


# ============================================================================
# Cache & Storage Management Endpoints
# ============================================================================

@app.get("/api/gpu-analysis/cache/stats")
async def get_cache_stats():
    """Get cache statistics"""
    cache = get_cache()
    if not cache:
        return {"enabled": False, "message": "Cache not enabled"}
    
    try:
        stats = cache.get_stats()
        return {
            "enabled": True,
            "backend": CACHE_BACKEND,
            "stats": stats
        }
    except Exception as e:
        logger.error(f"Failed to get cache statistics: {e}")
        return {"enabled": True, "backend": CACHE_BACKEND, "error": str(e)}


@app.post("/api/gpu-analysis/cache/clear")
async def clear_cache():
    """Clear cache"""
    cache = get_cache()
    if not cache:
        return {"success": False, "message": "Cache not enabled"}
    
    try:
        cache.clear()
        logger.info("Cache cleared")
        return {"success": True, "message": "Cache cleared"}
    except Exception as e:
        logger.error(f"Failed to clear cache: {e}")
        return {"success": False, "error": str(e)}


@app.post("/api/gpu-analysis/cache/cleanup")
async def cleanup_cache():
    """Clean up expired cache entries"""
    cache = get_cache()
    if not cache:
        return {"success": False, "message": "Cache not enabled"}
    
    try:
        if hasattr(cache, 'cleanup_expired'):
            removed = cache.cleanup_expired()
            logger.info(f"Cleaned up {removed} expired cache entries")
            return {"success": True, "removed_count": removed}
        else:
            return {"success": False, "message": "Current cache backend does not support cleanup operation"}
    except Exception as e:
        logger.error(f"Failed to cleanup cache: {e}")
        return {"success": False, "error": str(e)}


@app.get("/api/gpu-analysis/storage/stats")
async def get_storage_stats():
    """Get storage statistics"""
    storage = get_storage()
    if not storage:
        return {"enabled": False, "message": "Storage not enabled"}
    
    try:
        stats = storage.get_stats()
        return {
            "enabled": True,
            "backend": STORAGE_BACKEND,
            "retention_days": STORAGE_RETENTION_DAYS,
            "stats": stats
        }
    except Exception as e:
        logger.error(f"Failed to get storage statistics: {e}")
        return {"enabled": True, "backend": STORAGE_BACKEND, "error": str(e)}


@app.get("/api/gpu-analysis/storage/conversations")
async def list_conversations(
    limit: int = 20,
    offset: int = 0
):
    """List conversation records"""
    storage = get_storage()
    if not storage:
        raise HTTPException(status_code=400, detail="Storage not enabled")
    
    try:
        conversations = storage.list_conversations(limit=limit, offset=offset)
        
        # Process each conversation record
        processed_conversations = []
        for conv in conversations:
            # Remove file_path field
            conv_copy = {k: v for k, v in conv.items() if k != "file_path"}
            
            # Load full conversation data to get first user question
            session_id = conv.get("session_id")
            if session_id:
                full_conv = storage.load_conversation(session_id)
                if full_conv:
                    # Try to get first user message from new format
                    messages = full_conv.get("messages", [])
                    
                    # Compatible with old format (if conversation field exists)
                    if not messages and "conversation" in full_conv:
                        # First try to get from conversation.messages
                        messages = full_conv["conversation"].get("messages", [])
                        # If still empty, try from conversation.history
                        if not messages:
                            messages = full_conv["conversation"].get("history", [])
                    
                    if messages:
                        # Find first user message
                        first_user_msg = next((msg for msg in messages if msg.get("role") == "user"), None)
                        if first_user_msg:
                            conv_copy["name"] = first_user_msg.get("content", "Untitled conversation")
                        else:
                            conv_copy["name"] = "Untitled conversation"
                    # Compatible with old format
                    elif "conversation" in full_conv:
                        query = full_conv["conversation"].get("query", "")
                        conv_copy["name"] = query if query else "Untitled conversation"
                    else:
                        conv_copy["name"] = "Untitled conversation"
                else:
                    conv_copy["name"] = "Untitled conversation"
            else:
                conv_copy["name"] = "Untitled conversation"
            
            processed_conversations.append(conv_copy)
        
        return {
            "conversations": processed_conversations,
            "limit": limit,
            "offset": offset,
            "count": len(processed_conversations)
        }
    except Exception as e:
        logger.error(f"Failed to list conversations: {e}")
        raise HTTPException(status_code=500, detail=str(e))


@app.get("/api/gpu-analysis/storage/conversations/{session_id}")
async def get_conversation(session_id: str):
    """Get specified conversation record"""
    storage = get_storage()
    if not storage:
        raise HTTPException(status_code=400, detail="Storage not enabled")
    
    try:
        conversation = storage.load_conversation(session_id)
        if not conversation:
            raise HTTPException(status_code=404, detail="Conversation record not found")
        
        # Get messages array
        messages = conversation.get("messages", [])
        
        # Compatible with old format (if conversation field exists)
        if not messages and "conversation" in conversation:
            # First try to get from conversation.messages
            messages = conversation["conversation"].get("messages", [])
            # If still empty, try from conversation.history
            if not messages:
                messages = conversation["conversation"].get("history", [])
        
        # Extract first user message as name
        name = "Untitled conversation"
        if messages:
            first_user_msg = next((msg for msg in messages if msg.get("role") == "user"), None)
            if first_user_msg:
                name = first_user_msg.get("content", "Untitled conversation")
        # Compatible with old format
        elif "conversation" in conversation:
            name = conversation["conversation"].get("query", "Untitled conversation")
        
        return {
            "session_id": session_id,
            "name": name,
            "messages": messages,
            "created_at": conversation.get("created_at"),
            "updated_at": conversation.get("updated_at"),
            "metadata": conversation.get("metadata", {})
        }
    except HTTPException:
        raise
    except Exception as e:
        logger.error(f"Failed to get conversation record: {e}")
        raise HTTPException(status_code=500, detail=str(e))


@app.delete("/api/gpu-analysis/storage/conversations/{session_id}")
async def delete_conversation(session_id: str):
    """Delete specified conversation record"""
    storage = get_storage()
    if not storage:
        raise HTTPException(status_code=400, detail="Storage not enabled")
    
    try:
        success = storage.delete_conversation(session_id)
        if success:
            logger.info(f"Deleted session {session_id}")
            return {"success": True, "message": f"Deleted session {session_id}"}
        else:
            raise HTTPException(status_code=404, detail="Conversation record not found")
    except HTTPException:
        raise
    except Exception as e:
        logger.error(f"Failed to delete conversation record: {e}")
        raise HTTPException(status_code=500, detail=str(e))


@app.post("/api/gpu-analysis/storage/cleanup")
async def cleanup_old_conversations(days: Optional[int] = None):
    """Clean up old conversation records"""
    storage = get_storage()
    if not storage:
        raise HTTPException(status_code=400, detail="Storage not enabled")
    
    retention_days = days or STORAGE_RETENTION_DAYS
    
    try:
        removed = storage.cleanup_old_conversations(days=retention_days)
        logger.info(f"Cleaned up {removed} old conversation records (retained {retention_days} days)")
        return {
            "success": True,
            "removed_count": removed,
            "retention_days": retention_days
        }
    except Exception as e:
        logger.error(f"Failed to cleanup conversation records: {e}")
        raise HTTPException(status_code=500, detail=str(e))


@app.get("/api/gpu-analysis/storage/search")
async def search_conversations(
    query: str,
    limit: int = 10
):
    """Search conversation records"""
    storage = get_storage()
    if not storage:
        raise HTTPException(status_code=400, detail="Storage not enabled")
    
    try:
        results = storage.search_conversations(query=query, limit=limit)
        return {
            "results": results,
            "query": query,
            "count": len(results)
        }
    except Exception as e:
        logger.error(f"Failed to search conversation records: {e}")
        raise HTTPException(status_code=500, detail=str(e))


# ============================================================================
# Main
# ============================================================================

if __name__ == "__main__":
    import uvicorn
    
    logger.info("="*60)
    logger.info("Starting GPU Usage Analysis Agent API")
    logger.info(f"Access URL: http://{API_HOST}:{API_PORT}")
    logger.info(f"Documentation: http://{API_HOST}:{API_PORT}/docs")
    logger.info("="*60)
    
    uvicorn.run(
        "api.main:app",
        host=API_HOST,
        port=API_PORT,
        reload=True
    )
