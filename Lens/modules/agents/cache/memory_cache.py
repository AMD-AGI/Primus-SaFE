"""Memory cache implementation"""

import time
from typing import Optional, Any, Dict
from collections import OrderedDict
import threading
from .base import CacheBase


class MemoryCache(CacheBase):
    """Memory cache implementation - Using LRU strategy"""
    
    def __init__(self, ttl: int = 300, max_size: int = 1000):
        """
        Initialize memory cache
        
        Args:
            ttl: Cache expiration time (seconds)
            max_size: Maximum number of cache entries
        """
        super().__init__(ttl)
        self.max_size = max_size
        self._cache: OrderedDict[str, Dict[str, Any]] = OrderedDict()
        self._lock = threading.Lock()
    
    def get(self, key: str) -> Optional[Any]:
        """Get cache value"""
        with self._lock:
            if key not in self._cache:
                return None
            
            entry = self._cache[key]
            
            # Check if expired
            if time.time() > entry["expires_at"]:
                del self._cache[key]
                return None
            
            # Move to end (LRU)
            self._cache.move_to_end(key)
            
            # Update hit statistics
            entry["hits"] = entry.get("hits", 0) + 1
            entry["last_accessed"] = time.time()
            
            return entry["value"]
    
    def set(self, key: str, value: Any, ttl: Optional[int] = None):
        """Set cache value"""
        with self._lock:
            # If cache is full, delete oldest entry
            if len(self._cache) >= self.max_size and key not in self._cache:
                self._cache.popitem(last=False)
            
            expires_at = time.time() + (ttl if ttl is not None else self.ttl)
            
            self._cache[key] = {
                "value": value,
                "created_at": time.time(),
                "expires_at": expires_at,
                "hits": 0,
                "last_accessed": time.time()
            }
            
            # Move to end
            self._cache.move_to_end(key)
    
    def delete(self, key: str):
        """Delete cache"""
        with self._lock:
            if key in self._cache:
                del self._cache[key]
    
    def clear(self):
        """Clear all caches"""
        with self._lock:
            self._cache.clear()
    
    def exists(self, key: str) -> bool:
        """Check if cache exists and not expired"""
        with self._lock:
            if key not in self._cache:
                return False
            
            entry = self._cache[key]
            
            # Check if expired
            if time.time() > entry["expires_at"]:
                del self._cache[key]
                return False
            
            return True
    
    def get_stats(self) -> Dict[str, Any]:
        """Get cache statistics"""
        with self._lock:
            total_hits = sum(entry.get("hits", 0) for entry in self._cache.values())
            
            return {
                "size": len(self._cache),
                "max_size": self.max_size,
                "total_hits": total_hits,
                "keys": list(self._cache.keys())
            }
    
    def cleanup_expired(self):
        """Clean up expired cache entries"""
        with self._lock:
            current_time = time.time()
            expired_keys = [
                key for key, entry in self._cache.items()
                if current_time > entry["expires_at"]
            ]
            
            for key in expired_keys:
                del self._cache[key]
            
            return len(expired_keys)
