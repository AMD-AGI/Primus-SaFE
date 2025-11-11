"""PostgreSQL storage implementation"""

import json
import logging
from typing import Optional, List, Dict, Any
from datetime import datetime, timedelta
import threading
from contextlib import contextmanager

try:
    import psycopg2
    from psycopg2 import pool, sql
    from psycopg2.extras import RealDictCursor
except ImportError:
    raise ImportError(
        "psycopg2 is not installed. Please run: pip install psycopg2-binary"
    )

from .base import StorageBase

logger = logging.getLogger(__name__)


class PGStorage(StorageBase):
    """PostgreSQL storage implementation"""
    
    def __init__(
        self,
        host: str = "localhost",
        port: int = 5432,
        database: str = "agents",
        user: str = "postgres",
        password: str = "",
        min_connections: int = 1,
        max_connections: int = 10,
        schema: str = "public",
        sslmode: str = "prefer"
    ):
        """
        Initialize PostgreSQL storage
        
        Args:
            host: Database host address
            port: Database port
            database: Database name
            user: Database username
            password: Database password
            min_connections: Minimum connections
            max_connections: Maximum connections
            schema: Database schema
            sslmode: SSL mode (disable, allow, prefer, require, verify-ca, verify-full)
        """
        self.host = host
        self.port = port
        self.database = database
        self.user = user
        self.password = password
        self.schema = schema
        self.sslmode = sslmode
        
        self._lock = threading.Lock()
        
        # Create connection pool
        try:
            self.connection_pool = psycopg2.pool.ThreadedConnectionPool(
                minconn=min_connections,
                maxconn=max_connections,
                host=host,
                port=port,
                database=database,
                user=user,
                password=password,
                sslmode=sslmode
            )
            logger.info(f"PostgreSQL connection pool created successfully: {user}@{host}:{port}/{database} (sslmode={sslmode})")
        except Exception as e:
            logger.error(f"Failed to create PostgreSQL connection pool: {e}")
            raise
        
        # Initialize database tables
        self._init_db()
    
    @contextmanager
    def _get_connection(self):
        """Get database connection (context manager)"""
        conn = None
        try:
            conn = self.connection_pool.getconn()
            yield conn
            conn.commit()
        except Exception as e:
            if conn:
                conn.rollback()
            logger.error(f"Database operation failed: {e}")
            raise
        finally:
            if conn:
                self.connection_pool.putconn(conn)
    
    def _init_db(self):
        """Initialize database tables"""
        with self._lock:
            try:
                with self._get_connection() as conn:
                    cursor = conn.cursor()
                    
                    # Create schema (if not exists)
                    if self.schema != "public":
                        cursor.execute(
                            sql.SQL("CREATE SCHEMA IF NOT EXISTS {}").format(
                                sql.Identifier(self.schema)
                            )
                        )
                    
                    # Set search_path
                    cursor.execute(
                        sql.SQL("SET search_path TO {}, public").format(
                            sql.Identifier(self.schema)
                        )
                    )
                    
                    # Create conversations table
                    cursor.execute(f"""
                        CREATE TABLE IF NOT EXISTS {self.schema}.conversations (
                            session_id VARCHAR(255) PRIMARY KEY,
                            conversation_data JSONB NOT NULL,
                            metadata JSONB DEFAULT '{{}}',
                            created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
                            updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
                        )
                    """)
                    
                    # Create indexes
                    cursor.execute(f"""
                        CREATE INDEX IF NOT EXISTS idx_conversations_updated_at 
                        ON {self.schema}.conversations(updated_at DESC)
                    """)
                    
                    cursor.execute(f"""
                        CREATE INDEX IF NOT EXISTS idx_conversations_created_at 
                        ON {self.schema}.conversations(created_at DESC)
                    """)
                    
                    # Create GIN index for JSONB full-text search
                    cursor.execute(f"""
                        CREATE INDEX IF NOT EXISTS idx_conversations_data_gin 
                        ON {self.schema}.conversations USING GIN(conversation_data)
                    """)
                    
                    cursor.execute(f"""
                        CREATE INDEX IF NOT EXISTS idx_conversations_metadata_gin 
                        ON {self.schema}.conversations USING GIN(metadata)
                    """)
                    
                    # Create update time trigger
                    cursor.execute(f"""
                        CREATE OR REPLACE FUNCTION {self.schema}.update_updated_at_column()
                        RETURNS TRIGGER AS $$
                        BEGIN
                            NEW.updated_at = NOW();
                            RETURN NEW;
                        END;
                        $$ language 'plpgsql'
                    """)
                    
                    cursor.execute(f"""
                        DROP TRIGGER IF EXISTS update_conversations_updated_at 
                        ON {self.schema}.conversations
                    """)
                    
                    cursor.execute(f"""
                        CREATE TRIGGER update_conversations_updated_at 
                        BEFORE UPDATE ON {self.schema}.conversations 
                        FOR EACH ROW 
                        EXECUTE FUNCTION {self.schema}.update_updated_at_column()
                    """)
                    
                    cursor.close()
                    logger.info("PostgreSQL database tables initialized successfully")
                    
            except Exception as e:
                logger.error(f"Failed to initialize database tables: {e}")
                raise
    
    def save_conversation(
        self,
        session_id: str,
        conversation_data: Dict[str, Any],
        metadata: Optional[Dict[str, Any]] = None
    ) -> bool:
        """Save conversation record"""
        with self._lock:
            try:
                with self._get_connection() as conn:
                    cursor = conn.cursor()
                    
                    # Serialize data
                    conversation_json = json.dumps(conversation_data, ensure_ascii=False)
                    metadata_json = json.dumps(metadata or {}, ensure_ascii=False)
                    
                    # Insert or update (using ON CONFLICT)
                    cursor.execute(f"""
                        INSERT INTO {self.schema}.conversations 
                        (session_id, conversation_data, metadata, created_at, updated_at)
                        VALUES (%s, %s::jsonb, %s::jsonb, NOW(), NOW())
                        ON CONFLICT (session_id) 
                        DO UPDATE SET 
                            conversation_data = EXCLUDED.conversation_data,
                            metadata = EXCLUDED.metadata,
                            updated_at = NOW()
                    """, (session_id, conversation_json, metadata_json))
                    
                    cursor.close()
                    logger.debug(f"Conversation saved successfully: {session_id}")
                    return True
                    
            except Exception as e:
                logger.error(f"Failed to save conversation {session_id}: {e}")
                return False
    
    def load_conversation(self, session_id: str) -> Optional[Dict[str, Any]]:
        """Load conversation record"""
        with self._lock:
            try:
                with self._get_connection() as conn:
                    cursor = conn.cursor(cursor_factory=RealDictCursor)
                    
                    cursor.execute(f"""
                        SELECT session_id, conversation_data, metadata, 
                               created_at, updated_at
                        FROM {self.schema}.conversations
                        WHERE session_id = %s
                    """, (session_id,))
                    
                    row = cursor.fetchone()
                    cursor.close()
                    
                    if not row:
                        return None
                    
                    return {
                        "session_id": row["session_id"],
                        "conversation": row["conversation_data"],
                        "metadata": row["metadata"] or {},
                        "created_at": row["created_at"].isoformat(),
                        "updated_at": row["updated_at"].isoformat()
                    }
                    
            except Exception as e:
                logger.error(f"Failed to load conversation {session_id}: {e}")
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
                with self._get_connection() as conn:
                    cursor = conn.cursor(cursor_factory=RealDictCursor)
                    
                    # Build query
                    query = f"""
                        SELECT session_id, metadata, created_at, updated_at
                        FROM {self.schema}.conversations
                    """
                    params = []
                    
                    # Add filter conditions (using JSONB query)
                    if filter_by:
                        conditions = []
                        for key, value in filter_by.items():
                            conditions.append("metadata->>%s = %s")
                            params.extend([key, str(value)])
                        
                        if conditions:
                            query += " WHERE " + " AND ".join(conditions)
                    
                    query += " ORDER BY updated_at DESC LIMIT %s OFFSET %s"
                    params.extend([limit, offset])
                    
                    cursor.execute(query, params)
                    rows = cursor.fetchall()
                    cursor.close()
                    
                    results = []
                    for row in rows:
                        results.append({
                            "session_id": row["session_id"],
                            "metadata": row["metadata"] or {},
                            "created_at": row["created_at"].isoformat(),
                            "updated_at": row["updated_at"].isoformat()
                        })
                    
                    return results
                    
            except Exception as e:
                logger.error(f"Failed to list conversations: {e}")
                return []
    
    def delete_conversation(self, session_id: str) -> bool:
        """Delete conversation record"""
        with self._lock:
            try:
                with self._get_connection() as conn:
                    cursor = conn.cursor()
                    
                    cursor.execute(f"""
                        DELETE FROM {self.schema}.conversations 
                        WHERE session_id = %s
                    """, (session_id,))
                    
                    deleted_count = cursor.rowcount
                    cursor.close()
                    
                    logger.debug(f"Deleted conversation: {session_id}, affected rows: {deleted_count}")
                    return deleted_count > 0
                    
            except Exception as e:
                logger.error(f"Failed to delete conversation {session_id}: {e}")
                return False
    
    def search_conversations(
        self,
        query: str,
        limit: int = 10
    ) -> List[Dict[str, Any]]:
        """Search conversation records (using JSONB full-text search)"""
        with self._lock:
            try:
                with self._get_connection() as conn:
                    cursor = conn.cursor(cursor_factory=RealDictCursor)
                    
                    # Use JSONB's @> operator or to_tsvector for full-text search
                    # Using simple text matching here
                    cursor.execute(f"""
                        SELECT session_id, conversation_data, metadata, 
                               created_at, updated_at
                        FROM {self.schema}.conversations
                        WHERE 
                            conversation_data::text ILIKE %s 
                            OR metadata::text ILIKE %s
                        ORDER BY updated_at DESC
                        LIMIT %s
                    """, (f"%{query}%", f"%{query}%", limit))
                    
                    rows = cursor.fetchall()
                    cursor.close()
                    
                    results = []
                    for row in rows:
                        results.append({
                            "session_id": row["session_id"],
                            "conversation": row["conversation_data"],
                            "metadata": row["metadata"] or {},
                            "created_at": row["created_at"].isoformat(),
                            "updated_at": row["updated_at"].isoformat()
                        })
                    
                    return results
                    
            except Exception as e:
                logger.error(f"Failed to search conversations: {e}")
                return []
    
    def cleanup_old_conversations(self, days: int = 30) -> int:
        """Clean up old conversation records"""
        with self._lock:
            try:
                with self._get_connection() as conn:
                    cursor = conn.cursor()
                    
                    cutoff_date = datetime.now() - timedelta(days=days)
                    
                    cursor.execute(f"""
                        DELETE FROM {self.schema}.conversations
                        WHERE updated_at < %s
                    """, (cutoff_date,))
                    
                    deleted_count = cursor.rowcount
                    cursor.close()
                    
                    logger.info(f"Cleaned up {deleted_count} old conversation records")
                    return deleted_count
                    
            except Exception as e:
                logger.error(f"Failed to clean up old conversations: {e}")
                return 0
    
    def get_stats(self) -> Dict[str, Any]:
        """Get storage statistics"""
        with self._lock:
            try:
                with self._get_connection() as conn:
                    cursor = conn.cursor(cursor_factory=RealDictCursor)
                    
                    # Total count
                    cursor.execute(f"""
                        SELECT COUNT(*) as total_count
                        FROM {self.schema}.conversations
                    """)
                    total_count = cursor.fetchone()["total_count"]
                    
                    # Oldest and newest
                    cursor.execute(f"""
                        SELECT 
                            MIN(created_at) as oldest,
                            MAX(updated_at) as newest
                        FROM {self.schema}.conversations
                    """)
                    row = cursor.fetchone()
                    oldest = row["oldest"].isoformat() if row["oldest"] else None
                    newest = row["newest"].isoformat() if row["newest"] else None
                    
                    # Database size (approximate)
                    cursor.execute(f"""
                        SELECT pg_total_relation_size('{self.schema}.conversations') as size
                    """)
                    db_size = cursor.fetchone()["size"]
                    
                    cursor.close()
                    
                    return {
                        "total_conversations": total_count,
                        "database_size_bytes": db_size,
                        "database_info": f"{self.user}@{self.host}:{self.port}/{self.database}",
                        "schema": self.schema,
                        "oldest_conversation": oldest,
                        "newest_conversation": newest
                    }
                    
            except Exception as e:
                logger.error(f"Failed to get statistics: {e}")
                return {
                    "total_conversations": 0,
                    "error": str(e)
                }
    
    def close(self):
        """Close connection pool"""
        if self.connection_pool:
            self.connection_pool.closeall()
            logger.info("PostgreSQL connection pool closed")
    
    def __del__(self):
        """Destructor, ensure connection pool is closed"""
        try:
            self.close()
        except:
            pass

