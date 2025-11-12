"""Storage factory - Create storage instances based on configuration"""

from typing import Optional
from .base import StorageBase
from .file_storage import FileStorage


def create_storage(
    backend: str = "file",
    **kwargs
) -> StorageBase:
    """
    Create storage instance
    
    Args:
        backend: Storage backend type (file, db, pg)
        **kwargs: Other parameters
    
    Returns:
        Storage instance
    
    Examples:
        >>> storage = create_storage("file", storage_dir=".storage/conversations")
        >>> storage = create_storage("db", db_path=".storage/conversations.db")
        >>> storage = create_storage("pg", host="localhost", port=5432, 
        ...                          database="agents", user="postgres", password="")
    """
    if backend == "file":
        storage_dir = kwargs.get("storage_dir", ".storage/conversations")
        return FileStorage(storage_dir=storage_dir)
    
    elif backend == "db":
        from .db_storage import DBStorage
        
        db_path = kwargs.get("db_path", ".storage/conversations.db")
        return DBStorage(db_path=db_path)
    
    elif backend == "pg" or backend == "postgres" or backend == "postgresql":
        from .pg_storage import PGStorage
        
        return PGStorage(
            host=kwargs.get("host", "localhost"),
            port=kwargs.get("port", 5432),
            database=kwargs.get("database", "agents"),
            user=kwargs.get("user", "postgres"),
            password=kwargs.get("password", ""),
            min_connections=kwargs.get("min_connections", 1),
            max_connections=kwargs.get("max_connections", 10),
            schema=kwargs.get("schema", "public"),
            sslmode=kwargs.get("sslmode", "prefer")
        )
    
    else:
        raise ValueError(f"Unknown storage backend: {backend}")
