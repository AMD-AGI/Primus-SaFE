"""Redis cache implementation (optional)"""

import time
import pickle
from typing import Optional, Any, Dict
from .base import CacheBase

try:
    import redis
    REDIS_AVAILABLE = True
except ImportError:
    REDIS_AVAILABLE = False


class RedisCache(CacheBase):
    """Redis cache implementation"""
    
    def __init__(
        self,
        ttl: int = 300,
        host: str = "localhost",
        port: int = 6379,
        db: int = 0,
        password: Optional[str] = None,
        prefix: str = "llm_cache:"
    ):
        """
        Initialize Redis cache
        
        Args:
            ttl: Cache expiration time (seconds)
            host: Redis host
            port: Redis port
            db: Redis database number
            password: Redis password
            prefix: Cache key prefix
        """
        if not REDIS_AVAILABLE:
            raise ImportError("Redis is not installed. Install it with: pip install redis")
        
        super().__init__(ttl)
        self.prefix = prefix
        
        self.client = redis.Redis(
            host=host,
            port=port,
            db=db,
            password=password,
            decode_responses=False  # We use pickle, need binary mode
        )
        
        # Test connection
        try:
            self.client.ping()
        except redis.ConnectionError as e:
            raise ConnectionError(f"Failed to connect to Redis: {e}")
    
    def _make_key(self, key: str) -> str:
        """Add prefix"""
        return f"{self.prefix}{key}"
    
    def get(self, key: str) -> Optional[Any]:
        """Get cache value"""
        try:
            redis_key = self._make_key(key)
            data = self.client.get(redis_key)
            
            if data is None:
                return None
            
            # Deserialize
            value = pickle.loads(data)
            
            # Update access statistics (using Redis Hash)
            stats_key = f"{redis_key}:stats"
            self.client.hincrby(stats_key, "hits", 1)
            self.client.hset(stats_key, "last_accessed", str(int(time.time())))
            
            # Set expiration time for statistics
            ttl = self.client.ttl(redis_key)
            if ttl > 0:
                self.client.expire(stats_key, ttl)
            
            return value
        except Exception as e:
            print(f"Failed to get cache from Redis: {e}")
            return None
    
    def set(self, key: str, value: Any, ttl: Optional[int] = None):
        """Set cache value"""
        try:
            redis_key = self._make_key(key)
            
            # Serialize
            data = pickle.dumps(value)
            
            # Set cache
            expire_time = ttl if ttl is not None else self.ttl
            self.client.setex(redis_key, expire_time, data)
            
            # Set statistics
            stats_key = f"{redis_key}:stats"
            self.client.hset(stats_key, mapping={
                "created_at": str(int(time.time())),
                "hits": "0",
                "last_accessed": str(int(time.time()))
            })
            self.client.expire(stats_key, expire_time)
            
        except Exception as e:
            print(f"Failed to set cache to Redis: {e}")
    
    def delete(self, key: str):
        """Delete cache"""
        try:
            redis_key = self._make_key(key)
            self.client.delete(redis_key)
            self.client.delete(f"{redis_key}:stats")
        except Exception as e:
            print(f"Failed to delete cache from Redis: {e}")
    
    def clear(self):
        """Clear all caches with prefix"""
        try:
            # Get all matching keys
            pattern = f"{self.prefix}*"
            keys = self.client.keys(pattern)
            
            if keys:
                self.client.delete(*keys)
        except Exception as e:
            print(f"Failed to clear cache from Redis: {e}")
    
    def exists(self, key: str) -> bool:
        """Check if cache exists"""
        try:
            redis_key = self._make_key(key)
            return self.client.exists(redis_key) > 0
        except Exception as e:
            print(f"Failed to check cache existence in Redis: {e}")
            return False
    
    def get_stats(self) -> Dict[str, Any]:
        """Get cache statistics"""
        try:
            pattern = f"{self.prefix}*"
            keys = self.client.keys(pattern)
            
            # Filter out statistics keys
            cache_keys = [k.decode() for k in keys if not k.decode().endswith(":stats")]
            
            total_hits = 0
            for key in cache_keys:
                stats_key = f"{key}:stats"
                hits = self.client.hget(stats_key, "hits")
                if hits:
                    total_hits += int(hits)
            
            return {
                "size": len(cache_keys),
                "total_hits": total_hits,
                "prefix": self.prefix,
                "keys": cache_keys
            }
        except Exception as e:
            print(f"Failed to get stats from Redis: {e}")
            return {
                "size": 0,
                "total_hits": 0,
                "error": str(e)
            }
    
    def cleanup_expired(self) -> int:
        """
        Redis automatically handles expiration, this method returns 0
        """
        return 0
