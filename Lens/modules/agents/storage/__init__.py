"""Storage module

Supports multiple storage backends:
- FileStorage: File storage
- DBStorage: SQLite database storage
- PGStorage: PostgreSQL database storage
"""

from .base import StorageBase
from .factory import create_storage
from .file_storage import FileStorage
from .db_storage import DBStorage

__all__ = [
    'StorageBase',
    'create_storage',
    'FileStorage',
    'DBStorage',
]

# PostgreSQL support is optional
try:
    from .pg_storage import PGStorage
    __all__.append('PGStorage')
except ImportError:
    pass
