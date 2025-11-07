"""缓存基类"""

from abc import ABC, abstractmethod
from typing import Optional, Any
import hashlib
import json


class CacheBase(ABC):
    """缓存基类"""
    
    def __init__(self, ttl: int = 300):
        """
        初始化缓存
        
        Args:
            ttl: 缓存过期时间（秒）
        """
        self.ttl = ttl
    
    @staticmethod
    def generate_cache_key(prompt: str, **kwargs) -> str:
        """
        生成缓存键
        
        Args:
            prompt: 提示词
            **kwargs: 其他参数（如 temperature, model 等）
        
        Returns:
            缓存键
        """
        # 将所有参数序列化为 JSON，然后计算哈希
        cache_data = {
            "prompt": prompt,
            **kwargs
        }
        cache_str = json.dumps(cache_data, sort_keys=True, ensure_ascii=False)
        return hashlib.sha256(cache_str.encode()).hexdigest()
    
    @abstractmethod
    def get(self, key: str) -> Optional[Any]:
        """
        获取缓存值
        
        Args:
            key: 缓存键
        
        Returns:
            缓存值，如果不存在或已过期则返回 None
        """
        pass
    
    @abstractmethod
    def set(self, key: str, value: Any, ttl: Optional[int] = None):
        """
        设置缓存值
        
        Args:
            key: 缓存键
            value: 缓存值
            ttl: 过期时间（秒），如果不指定则使用默认值
        """
        pass
    
    @abstractmethod
    def delete(self, key: str):
        """
        删除缓存
        
        Args:
            key: 缓存键
        """
        pass
    
    @abstractmethod
    def clear(self):
        """清空所有缓存"""
        pass
    
    @abstractmethod
    def exists(self, key: str) -> bool:
        """
        检查缓存是否存在
        
        Args:
            key: 缓存键
        
        Returns:
            是否存在
        """
        pass

