"""磁盘缓存实现"""

import os
import json
import time
import pickle
from pathlib import Path
from typing import Optional, Any, Dict
import threading
from .base import CacheBase


class DiskCache(CacheBase):
    """磁盘缓存实现 - 将缓存持久化到文件系统"""
    
    def __init__(self, ttl: int = 300, cache_dir: str = ".cache/llm"):
        """
        初始化磁盘缓存
        
        Args:
            ttl: 缓存过期时间（秒）
            cache_dir: 缓存目录路径
        """
        super().__init__(ttl)
        self.cache_dir = Path(cache_dir)
        self.cache_dir.mkdir(parents=True, exist_ok=True)
        self._lock = threading.Lock()
        
        # 元数据文件，记录所有缓存条目
        self.metadata_file = self.cache_dir / "metadata.json"
        self._metadata: Dict[str, Dict[str, Any]] = self._load_metadata()
    
    def _load_metadata(self) -> Dict[str, Dict[str, Any]]:
        """加载元数据"""
        if not self.metadata_file.exists():
            return {}
        
        try:
            with open(self.metadata_file, 'r', encoding='utf-8') as f:
                return json.load(f)
        except Exception:
            return {}
    
    def _save_metadata(self):
        """保存元数据"""
        try:
            with open(self.metadata_file, 'w', encoding='utf-8') as f:
                json.dump(self._metadata, f, ensure_ascii=False, indent=2)
        except Exception as e:
            print(f"Failed to save metadata: {e}")
    
    def _get_cache_path(self, key: str) -> Path:
        """获取缓存文件路径"""
        return self.cache_dir / f"{key}.pkl"
    
    def get(self, key: str) -> Optional[Any]:
        """获取缓存值"""
        with self._lock:
            # 检查元数据
            if key not in self._metadata:
                return None
            
            metadata = self._metadata[key]
            
            # 检查是否过期
            if time.time() > metadata["expires_at"]:
                self.delete(key)
                return None
            
            # 读取缓存文件
            cache_path = self._get_cache_path(key)
            if not cache_path.exists():
                # 文件丢失，删除元数据
                del self._metadata[key]
                self._save_metadata()
                return None
            
            try:
                with open(cache_path, 'rb') as f:
                    value = pickle.load(f)
                
                # 更新访问统计
                metadata["hits"] = metadata.get("hits", 0) + 1
                metadata["last_accessed"] = time.time()
                self._save_metadata()
                
                return value
            except Exception as e:
                print(f"Failed to load cache {key}: {e}")
                self.delete(key)
                return None
    
    def set(self, key: str, value: Any, ttl: Optional[int] = None):
        """设置缓存值"""
        with self._lock:
            expires_at = time.time() + (ttl if ttl is not None else self.ttl)
            
            # 保存缓存值到文件
            cache_path = self._get_cache_path(key)
            try:
                with open(cache_path, 'wb') as f:
                    pickle.dump(value, f)
                
                # 更新元数据
                self._metadata[key] = {
                    "created_at": time.time(),
                    "expires_at": expires_at,
                    "hits": 0,
                    "last_accessed": time.time(),
                    "size": cache_path.stat().st_size
                }
                self._save_metadata()
            except Exception as e:
                print(f"Failed to save cache {key}: {e}")
    
    def delete(self, key: str):
        """删除缓存"""
        with self._lock:
            # 删除缓存文件
            cache_path = self._get_cache_path(key)
            if cache_path.exists():
                try:
                    cache_path.unlink()
                except Exception as e:
                    print(f"Failed to delete cache file {key}: {e}")
            
            # 删除元数据
            if key in self._metadata:
                del self._metadata[key]
                self._save_metadata()
    
    def clear(self):
        """清空所有缓存"""
        with self._lock:
            # 删除所有缓存文件
            for key in list(self._metadata.keys()):
                cache_path = self._get_cache_path(key)
                if cache_path.exists():
                    try:
                        cache_path.unlink()
                    except Exception:
                        pass
            
            # 清空元数据
            self._metadata.clear()
            self._save_metadata()
    
    def exists(self, key: str) -> bool:
        """检查缓存是否存在且未过期"""
        with self._lock:
            if key not in self._metadata:
                return False
            
            metadata = self._metadata[key]
            
            # 检查是否过期
            if time.time() > metadata["expires_at"]:
                self.delete(key)
                return False
            
            # 检查文件是否存在
            cache_path = self._get_cache_path(key)
            return cache_path.exists()
    
    def get_stats(self) -> Dict[str, Any]:
        """获取缓存统计信息"""
        with self._lock:
            total_hits = sum(meta.get("hits", 0) for meta in self._metadata.values())
            total_size = sum(meta.get("size", 0) for meta in self._metadata.values())
            
            return {
                "size": len(self._metadata),
                "total_hits": total_hits,
                "total_size_bytes": total_size,
                "cache_dir": str(self.cache_dir),
                "keys": list(self._metadata.keys())
            }
    
    def cleanup_expired(self) -> int:
        """清理过期的缓存条目"""
        with self._lock:
            current_time = time.time()
            expired_keys = [
                key for key, meta in self._metadata.items()
                if current_time > meta["expires_at"]
            ]
            
            for key in expired_keys:
                self.delete(key)
            
            return len(expired_keys)

