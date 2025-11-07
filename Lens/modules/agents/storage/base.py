"""存储基类"""

from abc import ABC, abstractmethod
from typing import Optional, List, Dict, Any
from datetime import datetime


class StorageBase(ABC):
    """Chat History 存储基类"""
    
    @abstractmethod
    def save_conversation(
        self,
        session_id: str,
        conversation_data: Dict[str, Any],
        metadata: Optional[Dict[str, Any]] = None
    ) -> bool:
        """
        保存对话记录
        
        Args:
            session_id: 会话 ID
            conversation_data: 对话数据
            metadata: 元数据（如用户ID、时间戳等）
        
        Returns:
            是否保存成功
        """
        pass
    
    @abstractmethod
    def load_conversation(self, session_id: str) -> Optional[Dict[str, Any]]:
        """
        加载对话记录
        
        Args:
            session_id: 会话 ID
        
        Returns:
            对话数据，如果不存在则返回 None
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
        列出对话记录
        
        Args:
            limit: 返回数量限制
            offset: 偏移量
            filter_by: 过滤条件
        
        Returns:
            对话记录列表
        """
        pass
    
    @abstractmethod
    def delete_conversation(self, session_id: str) -> bool:
        """
        删除对话记录
        
        Args:
            session_id: 会话 ID
        
        Returns:
            是否删除成功
        """
        pass
    
    @abstractmethod
    def search_conversations(
        self,
        query: str,
        limit: int = 10
    ) -> List[Dict[str, Any]]:
        """
        搜索对话记录
        
        Args:
            query: 搜索关键词
            limit: 返回数量限制
        
        Returns:
            匹配的对话记录列表
        """
        pass
    
    @abstractmethod
    def cleanup_old_conversations(self, days: int = 30) -> int:
        """
        清理旧的对话记录
        
        Args:
            days: 保留天数
        
        Returns:
            删除的对话数量
        """
        pass

