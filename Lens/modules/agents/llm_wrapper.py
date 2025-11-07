"""LLM 包装器 - 支持缓存的 LLM 调用"""

import logging
from typing import Optional, Any, List, Union
from langchain_core.messages import BaseMessage
from langchain_core.language_models import BaseChatModel
from cache.base import CacheBase

logger = logging.getLogger(__name__)


class CachedLLM:
    """带缓存的 LLM 包装器"""
    
    def __init__(
        self,
        llm: BaseChatModel,
        cache: Optional[CacheBase] = None,
        cache_enabled: bool = True
    ):
        """
        初始化缓存 LLM
        
        Args:
            llm: 语言模型实例
            cache: 缓存实例
            cache_enabled: 是否启用缓存
        """
        self.llm = llm
        self.cache = cache
        self.cache_enabled = cache_enabled and cache is not None
        
        # 统计信息
        self.stats = {
            "total_calls": 0,
            "cache_hits": 0,
            "cache_misses": 0
        }
    
    def invoke(
        self,
        messages: Union[List[BaseMessage], str],
        **kwargs
    ) -> Any:
        """
        调用 LLM（支持缓存）
        
        Args:
            messages: 消息列表或字符串
            **kwargs: 其他参数
        
        Returns:
            LLM 响应
        """
        self.stats["total_calls"] += 1
        
        # 如果缓存未启用，直接调用 LLM
        if not self.cache_enabled:
            return self.llm.invoke(messages, **kwargs)
        
        # 生成缓存键
        cache_key = self._generate_cache_key(messages, **kwargs)
        
        # 尝试从缓存获取
        cached_response = self.cache.get(cache_key)
        if cached_response is not None:
            self.stats["cache_hits"] += 1
            logger.info(f"缓存命中: {cache_key[:16]}...")
            return cached_response
        
        # 缓存未命中，调用 LLM
        self.stats["cache_misses"] += 1
        logger.info(f"缓存未命中: {cache_key[:16]}...")
        
        response = self.llm.invoke(messages, **kwargs)
        
        # 保存到缓存
        try:
            self.cache.set(cache_key, response)
            logger.info(f"响应已缓存: {cache_key[:16]}...")
        except Exception as e:
            logger.warning(f"缓存保存失败: {e}")
        
        return response
    
    def _generate_cache_key(
        self,
        messages: Union[List[BaseMessage], str],
        **kwargs
    ) -> str:
        """
        生成缓存键
        
        Args:
            messages: 消息列表或字符串
            **kwargs: 其他参数
        
        Returns:
            缓存键
        """
        # 将 messages 转换为字符串
        if isinstance(messages, str):
            prompt = messages
        else:
            # 组合所有消息的内容
            prompt = "\n".join([
                f"{msg.__class__.__name__}: {msg.content}"
                for msg in messages
            ])
        
        # 提取相关的 LLM 参数
        cache_params = {
            "model": getattr(self.llm, "model_name", None) or getattr(self.llm, "model", None),
            "temperature": kwargs.get("temperature") or getattr(self.llm, "temperature", None),
            "max_tokens": kwargs.get("max_tokens") or getattr(self.llm, "max_tokens", None),
        }
        
        # 使用缓存基类的方法生成键
        return self.cache.generate_cache_key(prompt, **cache_params)
    
    def get_stats(self) -> dict:
        """获取缓存统计信息"""
        stats = self.stats.copy()
        
        if self.stats["total_calls"] > 0:
            stats["cache_hit_rate"] = self.stats["cache_hits"] / self.stats["total_calls"]
        else:
            stats["cache_hit_rate"] = 0.0
        
        if self.cache:
            stats["cache_info"] = self.cache.get_stats()
        
        return stats
    
    def clear_cache(self):
        """清空缓存"""
        if self.cache:
            self.cache.clear()
            logger.info("缓存已清空")
    
    def cleanup_cache(self):
        """清理过期缓存"""
        if self.cache and hasattr(self.cache, "cleanup_expired"):
            removed = self.cache.cleanup_expired()
            logger.info(f"清理了 {removed} 个过期缓存条目")
            return removed
        return 0

