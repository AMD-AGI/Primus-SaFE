"""Disk cache implementation"""

import os
import json
import time
import pickle
from pathlib import Path
from typing import Optional, Any, Dict
import threading
from .base import CacheBase


class DiskCache(CacheBase):
    """Disk cache implementation - Persist cache to file system"""
    
    def __init__(self, ttl: int = 300, cache_dir: str = ".cache/llm"):
        """
        Initialize disk cache
        
        Args:
            ttl: Cache expiration time (seconds)
            cache_dir: Cache directory path
        """
        super().__init__(ttl)
        self.cache_dir = Path(cache_dir)
        self.cache_dir.mkdir(parents=True, exist_ok=True)
        self._lock = threading.Lock()
        
        # Metadata file, recording all cache entries
        self.metadata_file = self.cache_dir / "metadata.json"
        self._metadata: Dict[str, Dict[str, Any]] = self._load_metadata()
    
    def _load_metadata(self) -> Dict[str, Dict[str, Any]]:
        """Load metadata"""
        if not self.metadata_file.exists():
            return {}
        
        try:
            with open(self.metadata_file, 'r', encoding='utf-8') as f:
                return json.load(f)
        except Exception:
            return {}
    
    def _save_metadata(self):
        """Save metadata"""
        try:
            with open(self.metadata_file, 'w', encoding='utf-8') as f:
                json.dump(self._metadata, f, ensure_ascii=False, indent=2)
        except Exception as e:
            print(f"Failed to save metadata: {e}")
    
    def _get_cache_path(self, key: str) -> Path:
        """Get cache file path"""
        return self.cache_dir / f"{key}.pkl"
    
    def get(self, key: str) -> Optional[Any]:
        """Get cache value"""
        with self._lock:
            # Check metadata
            if key not in self._metadata:
                return None
            
            metadata = self._metadata[key]
            
            # Check if expired
            if time.time() > metadata["expires_at"]:
                self.delete(key)
                return None
            
            # Read cache file
            cache_path = self._get_cache_path(key)
            if not cache_path.exists():
                # File is missing, delete metadata
                del self._metadata[key]
                self._save_metadata()
                return None
            
            try:
                with open(cache_path, 'rb') as f:
                    value = pickle.load(f)
                
                # Update access statistics
                metadata["hits"] = metadata.get("hits", 0) + 1
                metadata["last_accessed"] = time.time()
                self._save_metadata()
                
                return value
            except Exception as e:
                print(f"Failed to load cache {key}: {e}")
                self.delete(key)
                return None
    
    def set(self, key: str, value: Any, ttl: Optional[int] = None):
        """Set cache value"""
        with self._lock:
            expires_at = time.time() + (ttl if ttl is not None else self.ttl)
            
            # Save cache value to file
            cache_path = self._get_cache_path(key)
            try:
                with open(cache_path, 'wb') as f:
                    pickle.dump(value, f)
                
                # Update metadata
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
        """Delete cache"""
        with self._lock:
            # Delete cache file
            cache_path = self._get_cache_path(key)
            if cache_path.exists():
                try:
                    cache_path.unlink()
                except Exception as e:
                    print(f"Failed to delete cache file {key}: {e}")
            
            # Delete metadata
            if key in self._metadata:
                del self._metadata[key]
                self._save_metadata()
    
    def clear(self):
        """Clear all caches"""
        with self._lock:
            # Delete all cache files
            for key in list(self._metadata.keys()):
                cache_path = self._get_cache_path(key)
                if cache_path.exists():
                    try:
                        cache_path.unlink()
                    except Exception:
                        pass
            
            # Clear metadata
            self._metadata.clear()
            self._save_metadata()
    
    def exists(self, key: str) -> bool:
        """Check if cache exists and not expired"""
        with self._lock:
            if key not in self._metadata:
                return False
            
            metadata = self._metadata[key]
            
            # Check if expired
            if time.time() > metadata["expires_at"]:
                self.delete(key)
                return False
            
            # Check if file exists
            cache_path = self._get_cache_path(key)
            return cache_path.exists()
    
    def get_stats(self) -> Dict[str, Any]:
        """Get cache statistics"""
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
        """Clean up expired cache entries"""
        with self._lock:
            current_time = time.time()
            expired_keys = [
                key for key, meta in self._metadata.items()
                if current_time > meta["expires_at"]
            ]
            
            for key in expired_keys:
                self.delete(key)
            
            return len(expired_keys)
