"""GPU Usage Agent - Utility Functions."""

import json
import logging
from typing import Dict, Any, Optional

logger = logging.getLogger(__name__)


def safe_json_parse(content: str) -> Optional[Dict[str, Any]]:
    """
    安全地解析JSON字符串，兼容多种格式
    
    该函数可以处理：
    1. JSON前后有空白字符（换行符、空格等）的情况
    2. JSON前后有额外文本的情况（会尝试提取JSON部分）
    
    Args:
        content: 待解析的JSON字符串
        
    Returns:
        解析后的字典，如果解析失败返回None
        
    Examples:
        >>> safe_json_parse('{"key": "value"}')
        {'key': 'value'}
        
        >>> safe_json_parse('\\n\\n\\n{"key": "value"}\\n\\n')
        {'key': 'value'}
        
        >>> safe_json_parse('Some text before {"key": "value"} some text after')
        {'key': 'value'}
    """
    if not content:
        return None
        
    try:
        # 先去除首尾空白字符（包括换行符、空格等）
        cleaned_content = content.strip()
        
        # 尝试直接解析
        return json.loads(cleaned_content)
    except json.JSONDecodeError as e:
        logger.debug(f"JSON直接解析失败: {e}, 尝试提取JSON内容")
        
        # 尝试查找JSON内容（有些模型可能在JSON前后添加了额外文本）
        try:
            # 查找第一个 '{' 和最后一个 '}'
            start_idx = cleaned_content.find('{')
            end_idx = cleaned_content.rfind('}')
            
            if start_idx != -1 and end_idx != -1 and start_idx < end_idx:
                json_str = cleaned_content[start_idx:end_idx+1]
                result = json.loads(json_str)
                logger.debug(f"成功从内容中提取并解析JSON")
                return result
        except json.JSONDecodeError as e2:
            logger.warning(f"JSON提取解析也失败: {e2}")
        
        logger.error(f"无法解析JSON内容: {content[:200]}...")
        return None


def format_json_error_message(content: str, max_length: int = 200) -> str:
    """
    格式化JSON解析错误消息
    
    Args:
        content: 失败的内容
        max_length: 最大显示长度
        
    Returns:
        格式化后的错误消息
    """
    if len(content) <= max_length:
        return f"无法解析的内容: {content}"
    else:
        return f"无法解析的内容 (前{max_length}字符): {content[:max_length]}..."

