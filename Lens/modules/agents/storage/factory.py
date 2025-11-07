"""存储工厂 - 根据配置创建存储实例"""

from typing import Optional
from .base import StorageBase
from .file_storage import FileStorage


def create_storage(
    backend: str = "file",
    **kwargs
) -> StorageBase:
    """
    创建存储实例
    
    Args:
        backend: 存储后端类型（file, db）
        **kwargs: 其他参数
    
    Returns:
        存储实例
    
    Examples:
        >>> storage = create_storage("file", storage_dir=".storage/conversations")
        >>> storage = create_storage("db", db_path=".storage/conversations.db")
    """
    if backend == "file":
        storage_dir = kwargs.get("storage_dir", ".storage/conversations")
        return FileStorage(storage_dir=storage_dir)
    
    elif backend == "db":
        from .db_storage import DBStorage
        
        db_path = kwargs.get("db_path", ".storage/conversations.db")
        return DBStorage(db_path=db_path)
    
    else:
        raise ValueError(f"Unknown storage backend: {backend}")

