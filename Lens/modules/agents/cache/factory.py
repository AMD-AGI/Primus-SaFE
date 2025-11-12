"""Cache factory - Create cache instances based on configuration"""

from typing import Optional
from .base import CacheBase
from .memory_cache import MemoryCache
from .disk_cache import DiskCache


def create_cache(
    backend: str = "memory",
    ttl: int = 300,
    **kwargs
) -> CacheBase:
    """
    Create cache instance
    
    Args:
        backend: Cache backend type (memory, disk, redis)
        ttl: Cache expiration time (seconds)
        **kwargs: Other parameters
    
    Returns:
        Cache instance
    
    Examples:
        >>> cache = create_cache("memory", ttl=300, max_size=1000)
        >>> cache = create_cache("disk", ttl=600, cache_dir=".cache/llm")
        >>> cache = create_cache("redis", ttl=300, host="localhost", port=6379)
    """
    if backend == "memory":
        max_size = kwargs.get("max_size", 1000)
        return MemoryCache(ttl=ttl, max_size=max_size)
    
    elif backend == "disk":
        cache_dir = kwargs.get("cache_dir", ".cache/llm")
        return DiskCache(ttl=ttl, cache_dir=cache_dir)
    
    elif backend == "redis":
        try:
            from .redis_cache import RedisCache
            
            return RedisCache(
                ttl=ttl,
                host=kwargs.get("host", "localhost"),
                port=kwargs.get("port", 6379),
                db=kwargs.get("db", 0),
                password=kwargs.get("password"),
                prefix=kwargs.get("prefix", "llm_cache:")
            )
        except ImportError:
            raise ImportError(
                "Redis backend requires 'redis' package. "
                "Install it with: pip install redis"
            )
    
    else:
        raise ValueError(f"Unknown cache backend: {backend}")
