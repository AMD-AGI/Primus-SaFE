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

# 导入缓存和存储模块
from cache.factory import create_cache
from storage.factory import create_storage

# 配置日志
logging.basicConfig(
    level=logging.INFO,
    format='%(asctime)s - %(name)s - %(levelname)s - %(message)s'
)
logger = logging.getLogger(__name__)

# 为OpenAI客户端设置DEBUG级别日志以捕获详细错误
openai_logger = logging.getLogger("openai")
openai_logger.setLevel(logging.DEBUG)

# 为httpx设置详细日志
httpx_logger = logging.getLogger("httpx")
httpx_logger.setLevel(logging.WARNING)  # 只显示警告和错误

# ============================================================================
# Configuration
# ============================================================================

# 加载配置文件（支持环境变量覆盖）
config = load_config()
logger.info("配置加载完成")

# API 配置
API_HOST = get_config("api.host", "0.0.0.0")
API_PORT = get_config("api.port", 8001)
API_TITLE = get_config("api.title", "GPU Usage Analysis Agent API")
API_VERSION = get_config("api.version", "1.0.0")
API_DESCRIPTION = "基于 LangGraph 的 GPU 使用率分析对话 Agent"

# Lens API 配置
LENS_API_URL = get_config("lens.api_url", "http://localhost:30182")
CLUSTER_NAME = get_config("lens.cluster_name", None)
LENS_TIMEOUT = get_config("lens.timeout", 30)

# LLM 配置
LLM_PROVIDER = get_config("llm.provider", "openai")
LLM_MODEL = get_config("llm.model", "gpt-4")
LLM_API_KEY = get_config("llm.api_key", "")
LLM_BASE_URL = get_config("llm.base_url", None)
LLM_TEMPERATURE = get_config("llm.temperature", 0)
LLM_MAX_TOKENS = get_config("llm.max_tokens", 2000)
LLM_VERIFY_SSL = get_config("llm.verify_ssl", True)  # 是否验证 SSL 证书

# Agent 配置
AGENT_MAX_ITERATIONS = get_config("agent.max_iterations", 10)
AGENT_TIMEOUT = get_config("agent.timeout", 120)

# 缓存配置
CACHE_ENABLED = get_config("cache.enabled", True)
CACHE_BACKEND = get_config("cache.backend", "disk")

# 存储配置
STORAGE_ENABLED = get_config("storage.enabled", True)
STORAGE_BACKEND = get_config("storage.backend", "file")
STORAGE_RETENTION_DAYS = get_config("storage.retention_days", 30)

# 打印配置信息
logger.info(f"API 配置: {API_HOST}:{API_PORT}")
logger.info(f"Lens API: {LENS_API_URL}")
logger.info(f"Cluster: {CLUSTER_NAME or 'None (需要在请求中指定)'}")
logger.info(f"LLM: {LLM_PROVIDER} - {LLM_MODEL}")
if LLM_BASE_URL:
    logger.info(f"LLM Base URL: {LLM_BASE_URL}")
if not LLM_VERIFY_SSL:
    logger.warning("警告: SSL 证书验证已禁用!")
logger.info(f"缓存: {'启用' if CACHE_ENABLED else '禁用'} - 后端: {CACHE_BACKEND}")
logger.info(f"存储: {'启用' if STORAGE_ENABLED else '禁用'} - 后端: {STORAGE_BACKEND}")

# ============================================================================
# FastAPI App
# ============================================================================

app = FastAPI(
    title=API_TITLE,
    version=API_VERSION,
    description=API_DESCRIPTION
)

# CORS 配置
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
    """对话请求"""
    query: str = Field(..., description="用户查询")
    conversation_history: Optional[List[dict]] = Field(
        default=None,
        description="对话历史"
    )
    cluster_name: Optional[str] = Field(
        default=None,
        description="集群名称（可选）"
    )
    session_id: Optional[str] = Field(
        default=None,
        description="会话ID（用于持久化存储）"
    )
    save_history: bool = Field(
        default=True,
        description="是否保存对话历史"
    )


class ChatResponse(BaseModel):
    """对话响应"""
    answer: str = Field(..., description="答案")
    insights: List[str] = Field(default=[], description="洞察")
    data_collected: List[dict] = Field(default=[], description="收集的数据")
    conversation_history: List[dict] = Field(default=[], description="对话历史")
    debug_info: Optional[dict] = Field(default=None, description="调试信息")
    timestamp: datetime = Field(default_factory=datetime.now, description="时间戳")


class HealthResponse(BaseModel):
    """健康检查响应"""
    status: str
    version: str
    llm_provider: str
    lens_api_url: str


# ============================================================================
# Global Instances
# ============================================================================

# 全局缓存实例
_cache_instance = None

# 全局存储实例
_storage_instance = None


def get_cache():
    """获取缓存实例（单例）"""
    global _cache_instance
    
    if not CACHE_ENABLED:
        return None
    
    if _cache_instance is None:
        try:
            # 根据后端类型获取配置
            backend_config = get_config(f"cache.{CACHE_BACKEND}", {})
            
            _cache_instance = create_cache(
                backend=CACHE_BACKEND,
                **backend_config
            )
            logger.info(f"缓存实例创建成功: {CACHE_BACKEND}")
        except Exception as e:
            logger.error(f"创建缓存实例失败: {e}")
            return None
    
    return _cache_instance


def get_storage():
    """获取存储实例（单例）"""
    global _storage_instance
    
    if not STORAGE_ENABLED:
        return None
    
    if _storage_instance is None:
        try:
            # 根据后端类型获取配置
            backend_config = get_config(f"storage.{STORAGE_BACKEND}", {})
            
            _storage_instance = create_storage(
                backend=STORAGE_BACKEND,
                **backend_config
            )
            logger.info(f"存储实例创建成功: {STORAGE_BACKEND}")
        except Exception as e:
            logger.error(f"创建存储实例失败: {e}")
            return None
    
    return _storage_instance


# ============================================================================
# Dependencies
# ============================================================================

def get_llm():
    """获取语言模型实例"""
    if LLM_PROVIDER == "openai":
        # 创建自定义 HTTP 客户端以支持 SSL 配置
        http_client = None
        if not LLM_VERIFY_SSL:
            # 禁用 SSL 证书验证
            http_client = httpx.Client(verify=False)
            logger.warning("已禁用 LLM API 的 SSL 证书验证")
        
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
    """获取 Agent 实例"""
    # 获取缓存实例
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
    """根路径，返回 API 信息"""
    return HealthResponse(
        status="healthy",
        version=API_VERSION,
        llm_provider=LLM_PROVIDER,
        lens_api_url=LENS_API_URL
    )


@app.get("/health", response_model=HealthResponse)
async def health_check():
    """健康检查"""
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
    GPU 使用率分析对话接口（非流式）
    
    处理用户的自然语言查询，返回完整的分析结果
    """
    try:
        # 获取存储实例
        storage = get_storage()
        
        # 如果提供了 session_id，尝试加载历史对话
        if request.session_id and storage and not request.conversation_history:
            try:
                saved_conversation = storage.load_conversation(request.session_id)
                if saved_conversation:
                    request.conversation_history = saved_conversation.get("conversation", {}).get("history", [])
                    logger.info(f"已加载会话 {request.session_id} 的历史对话")
            except Exception as e:
                logger.warning(f"加载历史对话失败: {e}")
        
        # 如果请求中指定了集群名称，更新 agent 的集群名称
        if request.cluster_name:
            agent.cluster_name = request.cluster_name
        
        # 生成或使用提供的 session_id（提前生成以便在错误时也能保存）
        import uuid
        session_id = request.session_id or str(uuid.uuid4())
        
        # 调用 agent (agent 内部已经将 Message 对象转换为字典)
        result = agent.chat(
            user_query=request.query,
            conversation_history=request.conversation_history
        )
        
        # 在响应的 debug_info 中返回 session_id
        if result.get("debug_info"):
            result["debug_info"]["session_id"] = session_id
        else:
            result["debug_info"] = {"session_id": session_id}
        
        # 如果启用了存储且需要保存历史，保存对话记录
        if storage and request.save_history:
            try:
                # 保存对话，包含完整的调试信息
                conversation_data = {
                    "query": request.query,
                    "answer": result["answer"],
                    "history": result.get("conversation_history", []),
                    "insights": result.get("insights", []),
                    "debug_info": result.get("debug_info", {})  # 保存调试信息
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
                logger.info(f"已保存会话 {session_id} 的对话历史")
                
            except Exception as e:
                logger.warning(f"保存对话历史失败: {e}")
        
        return ChatResponse(
            answer=result["answer"],
            insights=result.get("insights", []),
            data_collected=result.get("data_collected", []),
            conversation_history=result.get("conversation_history", []),
            debug_info=result.get("debug_info")
        )
    
    except Exception as e:
        # 记录详细的错误信息
        import traceback
        import uuid
        error_type = type(e).__name__
        error_msg = str(e)
        error_traceback = traceback.format_exc()
        
        logger.error("="*60)
        logger.error(f"处理查询时发生错误")
        logger.error(f"错误类型: {error_type}")
        logger.error(f"错误消息: {error_msg}")
        logger.error(f"完整堆栈跟踪:\n{error_traceback}")
        logger.error("="*60)
        
        # 即使出错也保存 session，便于调试
        if storage and request.save_history:
            try:
                session_id = request.session_id or str(uuid.uuid4())
                conversation_data = {
                    "query": request.query,
                    "answer": f"处理查询时发生错误: {error_msg}",
                    "history": request.conversation_history or [],
                    "insights": [f"错误: {error_type}"],
                    "debug_info": {
                        "error": error_msg,
                        "error_type": error_type,
                        "error_traceback": error_traceback
                    }
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
                logger.info(f"已保存错误会话 {session_id}")
            except Exception as save_error:
                logger.warning(f"保存错误会话失败: {save_error}")
        
        # 检查是否是连接错误
        if "Connection" in error_type or "connection" in error_msg.lower():
            detail = f"LLM API 连接错误: {error_msg}. 请检查 LLM API 地址 ({LLM_BASE_URL}) 是否可访问。"
        else:
            detail = f"处理查询时发生错误 ({error_type}): {error_msg}"
        
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
    GPU 使用率分析对话接口（SSE 流式）
    
    处理用户的自然语言查询，以 Server-Sent Events 方式流式返回分析结果
    避免前端超时问题
    """
    
    async def generate_sse():
        """生成 SSE 事件流"""
        try:
            # 获取存储实例
            storage = get_storage()
            
            # 如果提供了 session_id，尝试加载历史对话
            if request.session_id and storage and not request.conversation_history:
                try:
                    saved_conversation = storage.load_conversation(request.session_id)
                    if saved_conversation:
                        request.conversation_history = saved_conversation.get("conversation", {}).get("history", [])
                        logger.info(f"已加载会话 {request.session_id} 的历史对话")
                except Exception as e:
                    logger.warning(f"加载历史对话失败: {e}")
            
            # 如果请求中指定了集群名称，更新 agent 的集群名称
            if request.cluster_name:
                agent.cluster_name = request.cluster_name
            
            # 生成或使用提供的 session_id
            import uuid
            session_id = request.session_id or str(uuid.uuid4())
            
            # 发送 session_id
            yield f"data: {json.dumps({'type': 'session', 'session_id': session_id}, ensure_ascii=False)}\n\n"
            
            # 存储最终结果用于保存对话历史
            final_result = None
            
            # 调用流式 agent
            async for chunk in agent.stream_chat(
                user_query=request.query,
                conversation_history=request.conversation_history
            ):
                # 发送 SSE 事件
                yield f"data: {json.dumps(chunk, ensure_ascii=False)}\n\n"
                
                # 如果是最终结果，保存以便后续存储
                if chunk.get("type") == "final":
                    final_result = chunk
                
                # 添加小延迟，确保客户端能够正确接收
                await asyncio.sleep(0.01)
            
            # 如果启用了存储且需要保存历史，保存对话记录
            if storage and request.save_history and final_result:
                try:
                    conversation_data = {
                        "query": request.query,
                        "answer": final_result.get("answer", ""),
                        "history": request.conversation_history or [],
                        "insights": [],
                        "debug_info": final_result.get("debug_info", {})
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
                    logger.info(f"已保存会话 {session_id} 的对话历史")
                    
                except Exception as e:
                    logger.warning(f"保存对话历史失败: {e}")
            
            # 发送结束信号
            yield f"data: {json.dumps({'type': 'done'}, ensure_ascii=False)}\n\n"
            
        except Exception as e:
            # 记录详细的错误信息
            import traceback
            error_type = type(e).__name__
            error_msg = str(e)
            error_traceback = traceback.format_exc()
            
            logger.error("="*60)
            logger.error(f"流式处理查询时发生错误")
            logger.error(f"错误类型: {error_type}")
            logger.error(f"错误消息: {error_msg}")
            logger.error(f"完整堆栈跟踪:\n{error_traceback}")
            logger.error("="*60)
            
            # 发送错误事件
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
            "X-Accel-Buffering": "no"  # 禁用 nginx 缓冲
        }
    )


@app.get("/api/gpu-analysis/capabilities")
async def get_capabilities():
    """
    获取 Agent 能力列表
    
    返回 Agent 支持的查询类型和功能
    """
    return {
        "capabilities": [
            {
                "type": "trend",
                "name": "趋势分析",
                "description": "查询不同维度、不同时间粒度的 GPU 使用率趋势",
                "examples": [
                    "最近几天的使用率变化趋势是怎么样的？",
                    "本月集群的GPU使用率趋势如何？"
                ]
            },
            {
                "type": "compare",
                "name": "对比分析",
                "description": "对比不同时间段、不同实体的使用情况",
                "examples": [
                    "本周和上周的使用率对比",
                    "ml-team 和 cv-team 的 GPU 使用情况对比"
                ]
            },
            {
                "type": "drill_down",
                "name": "根因下钻",
                "description": "从集群到 workspace 到用户到 workload 逐层分析",
                "examples": [
                    "为什么这周 ml-team 的使用率比上周低了？",
                    "是哪些 workload 导致的使用率下降？"
                ]
            },
            {
                "type": "realtime",
                "name": "实时状态",
                "description": "查询当前的 GPU 分配和使用情况",
                "examples": [
                    "当前集群的 GPU 使用情况如何？",
                    "现在有多少 GPU 在使用？"
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
    """获取缓存统计信息"""
    cache = get_cache()
    if not cache:
        return {"enabled": False, "message": "缓存未启用"}
    
    try:
        stats = cache.get_stats()
        return {
            "enabled": True,
            "backend": CACHE_BACKEND,
            "stats": stats
        }
    except Exception as e:
        logger.error(f"获取缓存统计失败: {e}")
        return {"enabled": True, "backend": CACHE_BACKEND, "error": str(e)}


@app.post("/api/gpu-analysis/cache/clear")
async def clear_cache():
    """清空缓存"""
    cache = get_cache()
    if not cache:
        return {"success": False, "message": "缓存未启用"}
    
    try:
        cache.clear()
        logger.info("缓存已清空")
        return {"success": True, "message": "缓存已清空"}
    except Exception as e:
        logger.error(f"清空缓存失败: {e}")
        return {"success": False, "error": str(e)}


@app.post("/api/gpu-analysis/cache/cleanup")
async def cleanup_cache():
    """清理过期的缓存条目"""
    cache = get_cache()
    if not cache:
        return {"success": False, "message": "缓存未启用"}
    
    try:
        if hasattr(cache, 'cleanup_expired'):
            removed = cache.cleanup_expired()
            logger.info(f"清理了 {removed} 个过期缓存条目")
            return {"success": True, "removed_count": removed}
        else:
            return {"success": False, "message": "当前缓存后端不支持清理操作"}
    except Exception as e:
        logger.error(f"清理缓存失败: {e}")
        return {"success": False, "error": str(e)}


@app.get("/api/gpu-analysis/storage/stats")
async def get_storage_stats():
    """获取存储统计信息"""
    storage = get_storage()
    if not storage:
        return {"enabled": False, "message": "存储未启用"}
    
    try:
        stats = storage.get_stats()
        return {
            "enabled": True,
            "backend": STORAGE_BACKEND,
            "retention_days": STORAGE_RETENTION_DAYS,
            "stats": stats
        }
    except Exception as e:
        logger.error(f"获取存储统计失败: {e}")
        return {"enabled": True, "backend": STORAGE_BACKEND, "error": str(e)}


@app.get("/api/gpu-analysis/storage/conversations")
async def list_conversations(
    limit: int = 20,
    offset: int = 0
):
    """列出对话记录"""
    storage = get_storage()
    if not storage:
        raise HTTPException(status_code=400, detail="存储未启用")
    
    try:
        conversations = storage.list_conversations(limit=limit, offset=offset)
        return {
            "conversations": conversations,
            "limit": limit,
            "offset": offset,
            "count": len(conversations)
        }
    except Exception as e:
        logger.error(f"列出对话记录失败: {e}")
        raise HTTPException(status_code=500, detail=str(e))


@app.get("/api/gpu-analysis/storage/conversations/{session_id}")
async def get_conversation(session_id: str):
    """获取指定的对话记录"""
    storage = get_storage()
    if not storage:
        raise HTTPException(status_code=400, detail="存储未启用")
    
    try:
        conversation = storage.load_conversation(session_id)
        if not conversation:
            raise HTTPException(status_code=404, detail="对话记录不存在")
        return conversation
    except HTTPException:
        raise
    except Exception as e:
        logger.error(f"获取对话记录失败: {e}")
        raise HTTPException(status_code=500, detail=str(e))


@app.delete("/api/gpu-analysis/storage/conversations/{session_id}")
async def delete_conversation(session_id: str):
    """删除指定的对话记录"""
    storage = get_storage()
    if not storage:
        raise HTTPException(status_code=400, detail="存储未启用")
    
    try:
        success = storage.delete_conversation(session_id)
        if success:
            logger.info(f"已删除会话 {session_id}")
            return {"success": True, "message": f"已删除会话 {session_id}"}
        else:
            raise HTTPException(status_code=404, detail="对话记录不存在")
    except HTTPException:
        raise
    except Exception as e:
        logger.error(f"删除对话记录失败: {e}")
        raise HTTPException(status_code=500, detail=str(e))


@app.post("/api/gpu-analysis/storage/cleanup")
async def cleanup_old_conversations(days: Optional[int] = None):
    """清理旧的对话记录"""
    storage = get_storage()
    if not storage:
        raise HTTPException(status_code=400, detail="存储未启用")
    
    retention_days = days or STORAGE_RETENTION_DAYS
    
    try:
        removed = storage.cleanup_old_conversations(days=retention_days)
        logger.info(f"清理了 {removed} 个旧的对话记录（保留 {retention_days} 天）")
        return {
            "success": True,
            "removed_count": removed,
            "retention_days": retention_days
        }
    except Exception as e:
        logger.error(f"清理对话记录失败: {e}")
        raise HTTPException(status_code=500, detail=str(e))


@app.get("/api/gpu-analysis/storage/search")
async def search_conversations(
    query: str,
    limit: int = 10
):
    """搜索对话记录"""
    storage = get_storage()
    if not storage:
        raise HTTPException(status_code=400, detail="存储未启用")
    
    try:
        results = storage.search_conversations(query=query, limit=limit)
        return {
            "results": results,
            "query": query,
            "count": len(results)
        }
    except Exception as e:
        logger.error(f"搜索对话记录失败: {e}")
        raise HTTPException(status_code=500, detail=str(e))


# ============================================================================
# Main
# ============================================================================

if __name__ == "__main__":
    import uvicorn
    
    logger.info("="*60)
    logger.info("启动 GPU Usage Analysis Agent API")
    logger.info(f"访问地址: http://{API_HOST}:{API_PORT}")
    logger.info(f"文档地址: http://{API_HOST}:{API_PORT}/docs")
    logger.info("="*60)
    
    uvicorn.run(
        "api.main:app",
        host=API_HOST,
        port=API_PORT,
        reload=True
    )

