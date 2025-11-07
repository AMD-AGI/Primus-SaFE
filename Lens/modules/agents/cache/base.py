"""Cache base class"""

from abc import ABC, abstractmethod
from typing import Optional, Any
import hashlib
import json


class CacheBase(ABC):
    """Cache base class"""
    
    def __init__(self, ttl: int = 300):
        """
        Initialize cache
        
        Args:
            ttl: Cache expiration time (seconds)
        """
        self.ttl = ttl
    
    @staticmethod
    def generate_cache_key(prompt: str, **kwargs) -> str:
        """
        Generate cache key
        
        Args:
            prompt: Prompt text
            **kwargs: Other parameters (such as temperature, model, etc.)
        
        Returns:
            Cache key
        """
        # Serialize all parameters to JSON, then calculate hash
        cache_data = {
            "prompt": prompt,
            **kwargs
        }
        cache_str = json.dumps(cache_data, sort_keys=True, ensure_ascii=False)
        return hashlib.sha256(cache_str.encode()).hexdigest()
    
    @abstractmethod
    def get(self, key: str) -> Optional[Any]:
        """
        Get cache value
        
        Args:
            key: Cache key
        
        Returns:
            Cache value, or None if not exists or expired
        """
        pass
    
    @abstractmethod
    def set(self, key: str, value: Any, ttl: Optional[int] = None):
        """
        Set cache value
        
        Args:
            key: Cache key
            value: Cache value
            ttl: Expiration time (seconds), uses default value if not specified
        """
        pass
    
    @abstractmethod
    def delete(self, key: str):
        """
        Delete cache
        
        Args:
            key: Cache key
        """
        pass
    
    @abstractmethod
    def clear(self):
        """Clear all caches"""
        pass
    
    @abstractmethod
    def exists(self, key: str) -> bool:
        """
        Check if cache exists
        
        Args:
            key: Cache key
        
        Returns:
            Whether it exists
        """
        pass
