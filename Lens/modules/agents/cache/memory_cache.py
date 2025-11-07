"""内存缓存实现"""

import time
from typing import Optional, Any, Dict
from collections import OrderedDict
import threading
from .base import CacheBase


class MemoryCache(CacheBase):
    """内存缓存实现 - 使用 LRU 策略"""
    
    def __init__(self, ttl: int = 300, max_size: int = 1000):
        """
        初始化内存缓存
        
        Args:
            ttl: 缓存过期时间（秒）
            max_size: 最大缓存条目数
        """
        super().__init__(ttl)
        self.max_size = max_size
        self._cache: OrderedDict[str, Dict[str, Any]] = OrderedDict()
        self._lock = threading.Lock()
    
    def get(self, key: str) -> Optional[Any]:
        """获取缓存值"""
        with self._lock:
            if key not in self._cache:
                return None
            
            entry = self._cache[key]
            
            # 检查是否过期
            if time.time() > entry["expires_at"]:
                del self._cache[key]
                return None
            
            # 移到最后（LRU）
            self._cache.move_to_end(key)
            
            # 更新命中统计
            entry["hits"] = entry.get("hits", 0) + 1
            entry["last_accessed"] = time.time()
            
            return entry["value"]
    
    def set(self, key: str, value: Any, ttl: Optional[int] = None):
        """设置缓存值"""
        with self._lock:
            # 如果缓存已满，删除最旧的条目
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
            
            # 移到最后
            self._cache.move_to_end(key)
    
    def delete(self, key: str):
        """删除缓存"""
        with self._lock:
            if key in self._cache:
                del self._cache[key]
    
    def clear(self):
        """清空所有缓存"""
        with self._lock:
            self._cache.clear()
    
    def exists(self, key: str) -> bool:
        """检查缓存是否存在且未过期"""
        with self._lock:
            if key not in self._cache:
                return False
            
            entry = self._cache[key]
            
            # 检查是否过期
            if time.time() > entry["expires_at"]:
                del self._cache[key]
                return False
            
            return True
    
    def get_stats(self) -> Dict[str, Any]:
        """获取缓存统计信息"""
        with self._lock:
            total_hits = sum(entry.get("hits", 0) for entry in self._cache.values())
            
            return {
                "size": len(self._cache),
                "max_size": self.max_size,
                "total_hits": total_hits,
                "keys": list(self._cache.keys())
            }
    
    def cleanup_expired(self):
        """清理过期的缓存条目"""
        with self._lock:
            current_time = time.time()
            expired_keys = [
                key for key, entry in self._cache.items()
                if current_time > entry["expires_at"]
            ]
            
            for key in expired_keys:
                del self._cache[key]
            
            return len(expired_keys)

