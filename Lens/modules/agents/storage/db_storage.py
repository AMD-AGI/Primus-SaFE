"""数据库存储实现（可选）- 使用 SQLite"""

import sqlite3
import json
from pathlib import Path
from typing import Optional, List, Dict, Any
from datetime import datetime, timedelta
import threading
from .base import StorageBase


class DBStorage(StorageBase):
    """数据库存储实现 - 使用 SQLite"""
    
    def __init__(self, db_path: str = ".storage/conversations.db"):
        """
        初始化数据库存储
        
        Args:
            db_path: 数据库文件路径
        """
        self.db_path = Path(db_path)
        self.db_path.parent.mkdir(parents=True, exist_ok=True)
        
        self._lock = threading.Lock()
        self._init_db()
    
    def _init_db(self):
        """初始化数据库表"""
        with self._lock:
            conn = sqlite3.connect(str(self.db_path))
            cursor = conn.cursor()
            
            # 创建对话表
            cursor.execute("""
                CREATE TABLE IF NOT EXISTS conversations (
                    session_id TEXT PRIMARY KEY,
                    conversation_data TEXT NOT NULL,
                    metadata TEXT,
                    created_at TEXT NOT NULL,
                    updated_at TEXT NOT NULL
                )
            """)
            
            # 创建索引
            cursor.execute("""
                CREATE INDEX IF NOT EXISTS idx_updated_at 
                ON conversations(updated_at DESC)
            """)
            
            cursor.execute("""
                CREATE INDEX IF NOT EXISTS idx_created_at 
                ON conversations(created_at DESC)
            """)
            
            # 创建全文搜索表（如果 SQLite 支持 FTS5）
            try:
                cursor.execute("""
                    CREATE VIRTUAL TABLE IF NOT EXISTS conversations_fts 
                    USING fts5(session_id, conversation_data, metadata)
                """)
            except sqlite3.OperationalError:
                # FTS5 不可用，跳过
                pass
            
            conn.commit()
            conn.close()
    
    def _get_connection(self) -> sqlite3.Connection:
        """获取数据库连接"""
        conn = sqlite3.connect(str(self.db_path))
        conn.row_factory = sqlite3.Row  # 使结果可以通过列名访问
        return conn
    
    def save_conversation(
        self,
        session_id: str,
        conversation_data: Dict[str, Any],
        metadata: Optional[Dict[str, Any]] = None
    ) -> bool:
        """保存对话记录"""
        with self._lock:
            try:
                conn = self._get_connection()
                cursor = conn.cursor()
                
                # 序列化数据
                conversation_json = json.dumps(conversation_data, ensure_ascii=False)
                metadata_json = json.dumps(metadata or {}, ensure_ascii=False)
                
                now = datetime.now().isoformat()
                
                # 检查是否已存在
                cursor.execute(
                    "SELECT created_at FROM conversations WHERE session_id = ?",
                    (session_id,)
                )
                row = cursor.fetchone()
                created_at = row[0] if row else now
                
                # 插入或更新
                cursor.execute("""
                    INSERT OR REPLACE INTO conversations 
                    (session_id, conversation_data, metadata, created_at, updated_at)
                    VALUES (?, ?, ?, ?, ?)
                """, (session_id, conversation_json, metadata_json, created_at, now))
                
                # 更新全文搜索表（如果存在）
                try:
                    cursor.execute("""
                        INSERT OR REPLACE INTO conversations_fts
                        (session_id, conversation_data, metadata)
                        VALUES (?, ?, ?)
                    """, (session_id, conversation_json, metadata_json))
                except sqlite3.OperationalError:
                    pass  # FTS5 表不存在
                
                conn.commit()
                conn.close()
                
                return True
            except Exception as e:
                print(f"Failed to save conversation {session_id}: {e}")
                return False
    
    def load_conversation(self, session_id: str) -> Optional[Dict[str, Any]]:
        """加载对话记录"""
        with self._lock:
            try:
                conn = self._get_connection()
                cursor = conn.cursor()
                
                cursor.execute("""
                    SELECT conversation_data, metadata, created_at, updated_at
                    FROM conversations
                    WHERE session_id = ?
                """, (session_id,))
                
                row = cursor.fetchone()
                conn.close()
                
                if not row:
                    return None
                
                return {
                    "session_id": session_id,
                    "conversation": json.loads(row[0]),
                    "metadata": json.loads(row[1]) if row[1] else {},
                    "created_at": row[2],
                    "updated_at": row[3]
                }
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
            try:
                conn = self._get_connection()
                cursor = conn.cursor()
                
                # 构建查询
                query = """
                    SELECT session_id, metadata, created_at, updated_at
                    FROM conversations
                """
                
                params = []
                
                # 简单过滤（如果需要更复杂的过滤，需要扩展）
                if filter_by:
                    # 这里简化处理，仅支持元数据的精确匹配
                    # 实际使用中可能需要 JSON 函数支持
                    pass
                
                query += " ORDER BY updated_at DESC LIMIT ? OFFSET ?"
                params.extend([limit, offset])
                
                cursor.execute(query, params)
                rows = cursor.fetchall()
                conn.close()
                
                results = []
                for row in rows:
                    results.append({
                        "session_id": row[0],
                        "metadata": json.loads(row[1]) if row[1] else {},
                        "created_at": row[2],
                        "updated_at": row[3]
                    })
                
                return results
            except Exception as e:
                print(f"Failed to list conversations: {e}")
                return []
    
    def delete_conversation(self, session_id: str) -> bool:
        """删除对话记录"""
        with self._lock:
            try:
                conn = self._get_connection()
                cursor = conn.cursor()
                
                cursor.execute(
                    "DELETE FROM conversations WHERE session_id = ?",
                    (session_id,)
                )
                
                # 删除全文搜索索引
                try:
                    cursor.execute(
                        "DELETE FROM conversations_fts WHERE session_id = ?",
                        (session_id,)
                    )
                except sqlite3.OperationalError:
                    pass
                
                conn.commit()
                conn.close()
                
                return True
            except Exception as e:
                print(f"Failed to delete conversation {session_id}: {e}")
                return False
    
    def search_conversations(
        self,
        query: str,
        limit: int = 10
    ) -> List[Dict[str, Any]]:
        """搜索对话记录"""
        with self._lock:
            try:
                conn = self._get_connection()
                cursor = conn.cursor()
                
                # 尝试使用全文搜索
                try:
                    cursor.execute("""
                        SELECT session_id
                        FROM conversations_fts
                        WHERE conversations_fts MATCH ?
                        LIMIT ?
                    """, (query, limit))
                    
                    rows = cursor.fetchall()
                    session_ids = [row[0] for row in rows]
                    
                except sqlite3.OperationalError:
                    # FTS5 不可用，使用 LIKE 搜索
                    cursor.execute("""
                        SELECT session_id
                        FROM conversations
                        WHERE conversation_data LIKE ? OR metadata LIKE ?
                        ORDER BY updated_at DESC
                        LIMIT ?
                    """, (f"%{query}%", f"%{query}%", limit))
                    
                    rows = cursor.fetchall()
                    session_ids = [row[0] for row in rows]
                
                conn.close()
                
                # 加载完整的对话数据
                results = []
                for session_id in session_ids:
                    conv_data = self.load_conversation(session_id)
                    if conv_data:
                        results.append(conv_data)
                
                return results
            except Exception as e:
                print(f"Failed to search conversations: {e}")
                return []
    
    def cleanup_old_conversations(self, days: int = 30) -> int:
        """清理旧的对话记录"""
        with self._lock:
            try:
                cutoff_date = datetime.now() - timedelta(days=days)
                cutoff_iso = cutoff_date.isoformat()
                
                conn = self._get_connection()
                cursor = conn.cursor()
                
                # 获取要删除的会话ID
                cursor.execute("""
                    SELECT session_id FROM conversations
                    WHERE updated_at < ?
                """, (cutoff_iso,))
                
                session_ids = [row[0] for row in cursor.fetchall()]
                
                # 删除对话
                cursor.execute("""
                    DELETE FROM conversations
                    WHERE updated_at < ?
                """, (cutoff_iso,))
                
                # 删除全文搜索索引
                try:
                    for session_id in session_ids:
                        cursor.execute("""
                            DELETE FROM conversations_fts
                            WHERE session_id = ?
                        """, (session_id,))
                except sqlite3.OperationalError:
                    pass
                
                conn.commit()
                deleted_count = len(session_ids)
                conn.close()
                
                return deleted_count
            except Exception as e:
                print(f"Failed to cleanup old conversations: {e}")
                return 0
    
    def get_stats(self) -> Dict[str, Any]:
        """获取存储统计信息"""
        with self._lock:
            try:
                conn = self._get_connection()
                cursor = conn.cursor()
                
                # 总数
                cursor.execute("SELECT COUNT(*) FROM conversations")
                total_count = cursor.fetchone()[0]
                
                # 最早和最新
                cursor.execute("""
                    SELECT MIN(created_at), MAX(updated_at)
                    FROM conversations
                """)
                row = cursor.fetchone()
                oldest = row[0] if row[0] else None
                newest = row[1] if row[1] else None
                
                # 数据库大小
                db_size = self.db_path.stat().st_size if self.db_path.exists() else 0
                
                conn.close()
                
                return {
                    "total_conversations": total_count,
                    "database_size_bytes": db_size,
                    "database_path": str(self.db_path),
                    "oldest_conversation": oldest,
                    "newest_conversation": newest
                }
            except Exception as e:
                print(f"Failed to get stats: {e}")
                return {
                    "total_conversations": 0,
                    "error": str(e)
                }

