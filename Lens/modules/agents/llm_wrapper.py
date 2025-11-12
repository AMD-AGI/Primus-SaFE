"""LLM Wrapper - LLM calls with caching support"""

import logging
from typing import Optional, Any, List, Union
from langchain_core.messages import BaseMessage
from langchain_core.language_models import BaseChatModel
from cache.base import CacheBase

logger = logging.getLogger(__name__)


class CachedLLM:
    """LLM wrapper with caching"""
    
    def __init__(
        self,
        llm: BaseChatModel,
        cache: Optional[CacheBase] = None,
        cache_enabled: bool = True
    ):
        """
        Initialize cached LLM
        
        Args:
            llm: Language model instance
            cache: Cache instance
            cache_enabled: Whether to enable cache
        """
        self.llm = llm
        self.cache = cache
        self.cache_enabled = cache_enabled and cache is not None
        
        # Statistics
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
        Invoke LLM (with caching support)
        
        Args:
            messages: Message list or string
            **kwargs: Other parameters
        
        Returns:
            LLM response
        """
        self.stats["total_calls"] += 1
        
        # If cache is not enabled, call LLM directly
        if not self.cache_enabled:
            return self.llm.invoke(messages, **kwargs)
        
        # Generate cache key
        cache_key = self._generate_cache_key(messages, **kwargs)
        
        # Try to get from cache
        cached_response = self.cache.get(cache_key)
        if cached_response is not None:
            self.stats["cache_hits"] += 1
            logger.info(f"Cache hit: {cache_key[:16]}...")
            return cached_response
        
        # Cache miss, call LLM
        self.stats["cache_misses"] += 1
        logger.info(f"Cache miss: {cache_key[:16]}...")
        
        response = self.llm.invoke(messages, **kwargs)
        
        # Save to cache
        try:
            self.cache.set(cache_key, response)
            logger.info(f"Response cached: {cache_key[:16]}...")
        except Exception as e:
            logger.warning(f"Failed to save cache: {e}")
        
        return response
    
    def _generate_cache_key(
        self,
        messages: Union[List[BaseMessage], str],
        **kwargs
    ) -> str:
        """
        Generate cache key
        
        Args:
            messages: Message list or string
            **kwargs: Other parameters
        
        Returns:
            Cache key
        """
        # Convert messages to string
        if isinstance(messages, str):
            prompt = messages
        else:
            # Combine content of all messages
            prompt = "\n".join([
                f"{msg.__class__.__name__}: {msg.content}"
                for msg in messages
            ])
        
        # Extract relevant LLM parameters
        cache_params = {
            "model": getattr(self.llm, "model_name", None) or getattr(self.llm, "model", None),
            "temperature": kwargs.get("temperature") or getattr(self.llm, "temperature", None),
            "max_tokens": kwargs.get("max_tokens") or getattr(self.llm, "max_tokens", None),
        }
        
        # Use cache base class method to generate key
        return self.cache.generate_cache_key(prompt, **cache_params)
    
    def get_stats(self) -> dict:
        """Get cache statistics"""
        stats = self.stats.copy()
        
        if self.stats["total_calls"] > 0:
            stats["cache_hit_rate"] = self.stats["cache_hits"] / self.stats["total_calls"]
        else:
            stats["cache_hit_rate"] = 0.0
        
        if self.cache:
            stats["cache_info"] = self.cache.get_stats()
        
        return stats
    
    def clear_cache(self):
        """Clear cache"""
        if self.cache:
            self.cache.clear()
            logger.info("Cache cleared")
    
    def cleanup_cache(self):
        """Clean up expired cache"""
        if self.cache and hasattr(self.cache, "cleanup_expired"):
            removed = self.cache.cleanup_expired()
            logger.info(f"Cleaned up {removed} expired cache entries")
            return removed
        return 0
