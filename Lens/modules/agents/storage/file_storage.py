"""File storage implementation - Save chat history to file system"""

import os
import json
from pathlib import Path
from typing import Optional, List, Dict, Any
from datetime import datetime, timedelta
import threading
from .base import StorageBase


class FileStorage(StorageBase):
    """File storage implementation"""
    
    def __init__(self, storage_dir: str = ".storage/conversations"):
        """
        Initialize file storage
        
        Args:
            storage_dir: Storage directory path
        """
        self.storage_dir = Path(storage_dir)
        self.storage_dir.mkdir(parents=True, exist_ok=True)
        self._lock = threading.Lock()
        
        # Index file for fast queries
        self.index_file = self.storage_dir / "index.json"
        self._index: Dict[str, Dict[str, Any]] = self._load_index()
    
    def _load_index(self) -> Dict[str, Dict[str, Any]]:
        """Load index"""
        if not self.index_file.exists():
            return {}
        
        try:
            with open(self.index_file, 'r', encoding='utf-8') as f:
                return json.load(f)
        except Exception:
            return {}
    
    def _save_index(self):
        """Save index"""
        try:
            with open(self.index_file, 'w', encoding='utf-8') as f:
                json.dump(self._index, f, ensure_ascii=False, indent=2)
        except Exception as e:
            print(f"Failed to save index: {e}")
    
    def _get_conversation_path(self, session_id: str) -> Path:
        """Get conversation file path"""
        # Use subdirectories to avoid too many files in a single directory
        # For example: session_id = "abc123" -> a/b/abc123.json
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
        """Save conversation record"""
        with self._lock:
            try:
                # Prepare data to save
                save_data = {
                    "session_id": session_id,
                    "conversation": conversation_data,
                    "metadata": metadata or {},
                    "created_at": datetime.now().isoformat(),
                    "updated_at": datetime.now().isoformat()
                }
                
                # If already exists, keep creation time
                if session_id in self._index:
                    save_data["created_at"] = self._index[session_id].get(
                        "created_at",
                        datetime.now().isoformat()
                    )
                
                # Save conversation file
                conv_path = self._get_conversation_path(session_id)
                with open(conv_path, 'w', encoding='utf-8') as f:
                    json.dump(save_data, f, ensure_ascii=False, indent=2)
                
                # Update index
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
        """Load conversation record"""
        with self._lock:
            if session_id not in self._index:
                return None
            
            conv_path = self._get_conversation_path(session_id)
            if not conv_path.exists():
                # File is missing, delete index
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
        """List conversation records"""
        with self._lock:
            # Get all conversation index information
            conversations = list(self._index.values())
            
            # Filter
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
            
            # Sort by update time in descending order
            conversations.sort(
                key=lambda x: x.get("updated_at", ""),
                reverse=True
            )
            
            # Pagination
            return conversations[offset:offset + limit]
    
    def delete_conversation(self, session_id: str) -> bool:
        """Delete conversation record"""
        with self._lock:
            try:
                # Delete file
                conv_path = self._get_conversation_path(session_id)
                if conv_path.exists():
                    conv_path.unlink()
                
                # Delete index
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
        """Search conversation records (simple keyword matching)"""
        with self._lock:
            query_lower = query.lower()
            results = []
            
            for session_id in self._index.keys():
                conv_data = self.load_conversation(session_id)
                if not conv_data:
                    continue
                
                # Search conversation content
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
        """Clean up old conversation records"""
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
        """Get storage statistics"""
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

