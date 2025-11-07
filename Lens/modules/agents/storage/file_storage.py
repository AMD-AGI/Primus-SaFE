"""文件存储实现 - 将 chat history 保存到文件系统"""

import os
import json
from pathlib import Path
from typing import Optional, List, Dict, Any
from datetime import datetime, timedelta
import threading
from .base import StorageBase


class FileStorage(StorageBase):
    """文件存储实现"""
    
    def __init__(self, storage_dir: str = ".storage/conversations"):
        """
        初始化文件存储
        
        Args:
            storage_dir: 存储目录路径
        """
        self.storage_dir = Path(storage_dir)
        self.storage_dir.mkdir(parents=True, exist_ok=True)
        self._lock = threading.Lock()
        
        # 索引文件，用于快速查询
        self.index_file = self.storage_dir / "index.json"
        self._index: Dict[str, Dict[str, Any]] = self._load_index()
    
    def _load_index(self) -> Dict[str, Dict[str, Any]]:
        """加载索引"""
        if not self.index_file.exists():
            return {}
        
        try:
            with open(self.index_file, 'r', encoding='utf-8') as f:
                return json.load(f)
        except Exception:
            return {}
    
    def _save_index(self):
        """保存索引"""
        try:
            with open(self.index_file, 'w', encoding='utf-8') as f:
                json.dump(self._index, f, ensure_ascii=False, indent=2)
        except Exception as e:
            print(f"Failed to save index: {e}")
    
    def _get_conversation_path(self, session_id: str) -> Path:
        """获取对话文件路径"""
        # 使用子目录组织，避免单个目录文件过多
        # 例如：session_id = "abc123" -> a/b/abc123.json
        if len(session_id) >= 2:
            subdir = self.storage_dir / session_id[0] / session_id[1]
        else:
            subdir = self.storage_dir / "misc"
        
        subdir.mkdir(parents=True, exist_ok=True)
        return subdir / f"{session_id}.json"
    
    def save_conversation(
        self,
        session_id: str,
        conversation_data: Dict[str, Any],
        metadata: Optional[Dict[str, Any]] = None
    ) -> bool:
        """保存对话记录"""
        with self._lock:
            try:
                # 准备保存的数据
                save_data = {
                    "session_id": session_id,
                    "conversation": conversation_data,
                    "metadata": metadata or {},
                    "created_at": datetime.now().isoformat(),
                    "updated_at": datetime.now().isoformat()
                }
                
                # 如果已存在，保留创建时间
                if session_id in self._index:
                    save_data["created_at"] = self._index[session_id].get(
                        "created_at",
                        datetime.now().isoformat()
                    )
                
                # 保存对话文件
                conv_path = self._get_conversation_path(session_id)
                with open(conv_path, 'w', encoding='utf-8') as f:
                    json.dump(save_data, f, ensure_ascii=False, indent=2)
                
                # 更新索引
                self._index[session_id] = {
                    "session_id": session_id,
                    "created_at": save_data["created_at"],
                    "updated_at": save_data["updated_at"],
                    "metadata": metadata or {},
                    "file_path": str(conv_path),
                    "size": conv_path.stat().st_size
                }
                self._save_index()
                
                return True
            except Exception as e:
                print(f"Failed to save conversation {session_id}: {e}")
                return False
    
    def load_conversation(self, session_id: str) -> Optional[Dict[str, Any]]:
        """加载对话记录"""
        with self._lock:
            if session_id not in self._index:
                return None
            
            conv_path = self._get_conversation_path(session_id)
            if not conv_path.exists():
                # 文件丢失，删除索引
                del self._index[session_id]
                self._save_index()
                return None
            
            try:
                with open(conv_path, 'r', encoding='utf-8') as f:
                    return json.load(f)
            except Exception as e:
                print(f"Failed to load conversation {session_id}: {e}")
                return None
    
    def list_conversations(
        self,
        limit: int = 100,
        offset: int = 0,
        filter_by: Optional[Dict[str, Any]] = None
    ) -> List[Dict[str, Any]]:
        """列出对话记录"""
        with self._lock:
            # 获取所有对话的索引信息
            conversations = list(self._index.values())
            
            # 过滤
            if filter_by:
                filtered = []
                for conv in conversations:
                    match = True
                    for key, value in filter_by.items():
                        if key in conv.get("metadata", {}):
                            if conv["metadata"][key] != value:
                                match = False
                                break
                    if match:
                        filtered.append(conv)
                conversations = filtered
            
            # 按更新时间倒序排序
            conversations.sort(
                key=lambda x: x.get("updated_at", ""),
                reverse=True
            )
            
            # 分页
            return conversations[offset:offset + limit]
    
    def delete_conversation(self, session_id: str) -> bool:
        """删除对话记录"""
        with self._lock:
            try:
                # 删除文件
                conv_path = self._get_conversation_path(session_id)
                if conv_path.exists():
                    conv_path.unlink()
                
                # 删除索引
                if session_id in self._index:
                    del self._index[session_id]
                    self._save_index()
                
                return True
            except Exception as e:
                print(f"Failed to delete conversation {session_id}: {e}")
                return False
    
    def search_conversations(
        self,
        query: str,
        limit: int = 10
    ) -> List[Dict[str, Any]]:
        """搜索对话记录（简单的关键词匹配）"""
        with self._lock:
            query_lower = query.lower()
            results = []
            
            for session_id in self._index.keys():
                conv_data = self.load_conversation(session_id)
                if not conv_data:
                    continue
                
                # 搜索对话内容
                conv_str = json.dumps(conv_data, ensure_ascii=False).lower()
                if query_lower in conv_str:
                    results.append({
                        "session_id": session_id,
                        "conversation": conv_data["conversation"],
                        "metadata": conv_data.get("metadata", {}),
                        "created_at": conv_data.get("created_at"),
                        "updated_at": conv_data.get("updated_at")
                    })
                
                if len(results) >= limit:
                    break
            
            return results
    
    def cleanup_old_conversations(self, days: int = 30) -> int:
        """清理旧的对话记录"""
        with self._lock:
            cutoff_date = datetime.now() - timedelta(days=days)
            cutoff_iso = cutoff_date.isoformat()
            
            deleted_count = 0
            sessions_to_delete = []
            
            for session_id, info in self._index.items():
                updated_at = info.get("updated_at", "")
                if updated_at and updated_at < cutoff_iso:
                    sessions_to_delete.append(session_id)
            
            for session_id in sessions_to_delete:
                if self.delete_conversation(session_id):
                    deleted_count += 1
            
            return deleted_count
    
    def get_stats(self) -> Dict[str, Any]:
        """获取存储统计信息"""
        with self._lock:
            total_size = sum(info.get("size", 0) for info in self._index.values())
            
            return {
                "total_conversations": len(self._index),
                "total_size_bytes": total_size,
                "storage_dir": str(self.storage_dir),
                "oldest_conversation": min(
                    (info.get("created_at", "") for info in self._index.values()),
                    default=None
                ),
                "newest_conversation": max(
                    (info.get("updated_at", "") for info in self._index.values()),
                    default=None
                )
            }

