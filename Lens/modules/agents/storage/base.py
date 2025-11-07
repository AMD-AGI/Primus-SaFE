"""Storage base class"""

from abc import ABC, abstractmethod
from typing import Optional, List, Dict, Any
from datetime import datetime


class StorageBase(ABC):
    """Chat History storage base class"""
    
    @abstractmethod
    def save_conversation(
        self,
        session_id: str,
        conversation_data: Dict[str, Any],
        metadata: Optional[Dict[str, Any]] = None
    ) -> bool:
        """
        Save conversation record
        
        Args:
            session_id: Session ID
            conversation_data: Conversation data
            metadata: Metadata (such as user ID, timestamp, etc.)
        
        Returns:
            Whether the save was successful
        """
        pass
    
    @abstractmethod
    def load_conversation(self, session_id: str) -> Optional[Dict[str, Any]]:
        """
        Load conversation record
        
        Args:
            session_id: Session ID
        
        Returns:
            Conversation data, or None if not exists
        """
        pass
    
    @abstractmethod
    def list_conversations(
        self,
        limit: int = 100,
        offset: int = 0,
        filter_by: Optional[Dict[str, Any]] = None
    ) -> List[Dict[str, Any]]:
        """
        List conversation records
        
        Args:
            limit: Return count limit
            offset: Offset
            filter_by: Filter conditions
        
        Returns:
            List of conversation records
        """
        pass
    
    @abstractmethod
    def delete_conversation(self, session_id: str) -> bool:
        """
        Delete conversation record
        
        Args:
            session_id: Session ID
        
        Returns:
            Whether the deletion was successful
        """
        pass
    
    @abstractmethod
    def search_conversations(
        self,
        query: str,
        limit: int = 10
    ) -> List[Dict[str, Any]]:
        """
        Search conversation records
        
        Args:
            query: Search keyword
            limit: Return count limit
        
        Returns:
            List of matching conversation records
        """
        pass
    
    @abstractmethod
    def cleanup_old_conversations(self, days: int = 30) -> int:
        """
        Clean up old conversation records
        
        Args:
            days: Retention days
        
        Returns:
            Number of deleted conversations
        """
        pass
