"""Redis 缓存实现（可选）"""

import pickle
from typing import Optional, Any, Dict
from .base import CacheBase

try:
    import redis
    REDIS_AVAILABLE = True
except ImportError:
    REDIS_AVAILABLE = False


class RedisCache(CacheBase):
    """Redis 缓存实现"""
    
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
        初始化 Redis 缓存
        
        Args:
            ttl: 缓存过期时间（秒）
            host: Redis 主机
            port: Redis 端口
            db: Redis 数据库编号
            password: Redis 密码
            prefix: 缓存键前缀
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
            decode_responses=False  # 我们使用 pickle，需要二进制模式
        )
        
        # 测试连接
        try:
            self.client.ping()
        except redis.ConnectionError as e:
            raise ConnectionError(f"Failed to connect to Redis: {e}")
    
    def _make_key(self, key: str) -> str:
        """添加前缀"""
        return f"{self.prefix}{key}"
    
    def get(self, key: str) -> Optional[Any]:
        """获取缓存值"""
        try:
            redis_key = self._make_key(key)
            data = self.client.get(redis_key)
            
            if data is None:
                return None
            
            # 反序列化
            value = pickle.loads(data)
            
            # 更新访问统计（使用 Redis Hash）
            stats_key = f"{redis_key}:stats"
            self.client.hincrby(stats_key, "hits", 1)
            self.client.hset(stats_key, "last_accessed", str(int(time.time())))
            
            # 设置统计信息的过期时间
            ttl = self.client.ttl(redis_key)
            if ttl > 0:
                self.client.expire(stats_key, ttl)
            
            return value
        except Exception as e:
            print(f"Failed to get cache from Redis: {e}")
            return None
    
    def set(self, key: str, value: Any, ttl: Optional[int] = None):
        """设置缓存值"""
        try:
            redis_key = self._make_key(key)
            
            # 序列化
            data = pickle.dumps(value)
            
            # 设置缓存
            expire_time = ttl if ttl is not None else self.ttl
            self.client.setex(redis_key, expire_time, data)
            
            # 设置统计信息
            import time
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
        """删除缓存"""
        try:
            redis_key = self._make_key(key)
            self.client.delete(redis_key)
            self.client.delete(f"{redis_key}:stats")
        except Exception as e:
            print(f"Failed to delete cache from Redis: {e}")
    
    def clear(self):
        """清空所有带前缀的缓存"""
        try:
            # 获取所有匹配的键
            pattern = f"{self.prefix}*"
            keys = self.client.keys(pattern)
            
            if keys:
                self.client.delete(*keys)
        except Exception as e:
            print(f"Failed to clear cache from Redis: {e}")
    
    def exists(self, key: str) -> bool:
        """检查缓存是否存在"""
        try:
            redis_key = self._make_key(key)
            return self.client.exists(redis_key) > 0
        except Exception as e:
            print(f"Failed to check cache existence in Redis: {e}")
            return False
    
    def get_stats(self) -> Dict[str, Any]:
        """获取缓存统计信息"""
        try:
            pattern = f"{self.prefix}*"
            keys = self.client.keys(pattern)
            
            # 过滤掉统计键
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
        Redis 自动处理过期，此方法返回 0
        """
        return 0


# 导入 time 模块
import time

