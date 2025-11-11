"""Database storage implementation (optional) - Using SQLite"""

import sqlite3
import json
from pathlib import Path
from typing import Optional, List, Dict, Any
from datetime import datetime, timedelta
import threading
from .base import StorageBase


class DBStorage(StorageBase):
    """Database storage implementation - Using SQLite"""
    
    def __init__(self, db_path: str = ".storage/conversations.db"):
        """
        Initialize database storage
        
        Args:
            db_path: Database file path
        """
        self.db_path = Path(db_path)
        self.db_path.parent.mkdir(parents=True, exist_ok=True)
        
        self._lock = threading.Lock()
        self._init_db()
    
    def _init_db(self):
        """Initialize database tables"""
        with self._lock:
            conn = sqlite3.connect(str(self.db_path))
            cursor = conn.cursor()
            
            # Create conversations table
            cursor.execute("""
                CREATE TABLE IF NOT EXISTS conversations (
                    session_id TEXT PRIMARY KEY,
                    conversation_data TEXT NOT NULL,
                    metadata TEXT,
                    created_at TEXT NOT NULL,
                    updated_at TEXT NOT NULL
                )
            """)
            
            # Create indexes
            cursor.execute("""
                CREATE INDEX IF NOT EXISTS idx_updated_at 
                ON conversations(updated_at DESC)
            """)
            
            cursor.execute("""
                CREATE INDEX IF NOT EXISTS idx_created_at 
                ON conversations(created_at DESC)
            """)
            
            # Create full-text search table (if SQLite supports FTS5)
            try:
                cursor.execute("""
                    CREATE VIRTUAL TABLE IF NOT EXISTS conversations_fts 
                    USING fts5(session_id, conversation_data, metadata)
                """)
            except sqlite3.OperationalError:
                # FTS5 not available, skip
                pass
            
            conn.commit()
            conn.close()
    
    def _get_connection(self) -> sqlite3.Connection:
        """Get database connection"""
        conn = sqlite3.connect(str(self.db_path))
        conn.row_factory = sqlite3.Row  # Make results accessible by column name
        return conn
    
    def save_conversation(
        self,
        session_id: str,
        conversation_data: Dict[str, Any],
        metadata: Optional[Dict[str, Any]] = None
    ) -> bool:
        """Save conversation record"""
        with self._lock:
            try:
                conn = self._get_connection()
                cursor = conn.cursor()
                
                # Serialize data
                conversation_json = json.dumps(conversation_data, ensure_ascii=False)
                metadata_json = json.dumps(metadata or {}, ensure_ascii=False)
                
                now = datetime.now().isoformat()
                
                # Check if already exists
                cursor.execute(
                    "SELECT created_at FROM conversations WHERE session_id = ?",
                    (session_id,)
                )
                row = cursor.fetchone()
                created_at = row[0] if row else now
                
                # Insert or update
                cursor.execute("""
                    INSERT OR REPLACE INTO conversations 
                    (session_id, conversation_data, metadata, created_at, updated_at)
                    VALUES (?, ?, ?, ?, ?)
                """, (session_id, conversation_json, metadata_json, created_at, now))
                
                # Update full-text search table (if exists)
                try:
                    cursor.execute("""
                        INSERT OR REPLACE INTO conversations_fts
                        (session_id, conversation_data, metadata)
                        VALUES (?, ?, ?)
                    """, (session_id, conversation_json, metadata_json))
                except sqlite3.OperationalError:
                    pass  # FTS5 table doesn't exist
                
                conn.commit()
                conn.close()
                
                return True
            except Exception as e:
                print(f"Failed to save conversation {session_id}: {e}")
                return False
    
    def load_conversation(self, session_id: str) -> Optional[Dict[str, Any]]:
        """Load conversation record"""
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
        """List conversation records"""
        with self._lock:
            try:
                conn = self._get_connection()
                cursor = conn.cursor()
                
                # Build query
                query = """
                    SELECT session_id, metadata, created_at, updated_at
                    FROM conversations
                """
                
                params = []
                
                # Simple filtering (for more complex filtering, needs extension)
                if filter_by:
                    # Simplified handling, only supports exact metadata matching
                    # May need JSON function support in actual use
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
        """Delete conversation record"""
        with self._lock:
            try:
                conn = self._get_connection()
                cursor = conn.cursor()
                
                cursor.execute(
                    "DELETE FROM conversations WHERE session_id = ?",
                    (session_id,)
                )
                
                # Delete full-text search index
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
        """Search conversation records"""
        with self._lock:
            try:
                conn = self._get_connection()
                cursor = conn.cursor()
                
                # Try using full-text search
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
                    # FTS5 not available, use LIKE search
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
                
                # Load complete conversation data
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
        """Clean up old conversation records"""
        with self._lock:
            try:
                cutoff_date = datetime.now() - timedelta(days=days)
                cutoff_iso = cutoff_date.isoformat()
                
                conn = self._get_connection()
                cursor = conn.cursor()
                
                # Get session IDs to delete
                cursor.execute("""
                    SELECT session_id FROM conversations
                    WHERE updated_at < ?
                """, (cutoff_iso,))
                
                session_ids = [row[0] for row in cursor.fetchall()]
                
                # Delete conversations
                cursor.execute("""
                    DELETE FROM conversations
                    WHERE updated_at < ?
                """, (cutoff_iso,))
                
                # Delete full-text search index
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
        """Get storage statistics"""
        with self._lock:
            try:
                conn = self._get_connection()
                cursor = conn.cursor()
                
                # Total count
                cursor.execute("SELECT COUNT(*) FROM conversations")
                total_count = cursor.fetchone()[0]
                
                # Oldest and newest
                cursor.execute("""
                    SELECT MIN(created_at), MAX(updated_at)
                    FROM conversations
                """)
                row = cursor.fetchone()
                oldest = row[0] if row[0] else None
                newest = row[1] if row[1] else None
                
                # Database size
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

