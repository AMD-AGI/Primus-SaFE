"""存储模块 - 用于持久化 chat history"""

from .base import StorageBase
from .file_storage import FileStorage
from .factory import create_storage

__all__ = [
    "StorageBase",
    "FileStorage",
    "create_storage",
]

