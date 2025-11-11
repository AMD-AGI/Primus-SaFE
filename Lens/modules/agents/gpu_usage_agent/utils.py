"""GPU Usage Agent - Utility Functions."""

import json
import logging
from typing import Dict, Any, Optional

logger = logging.getLogger(__name__)


def safe_json_parse(content: str) -> Optional[Dict[str, Any]]:
    """
    Safely parse JSON string, compatible with multiple formats
    
    This function can handle:
    1. JSON with whitespace (newlines, spaces, etc.) before or after
    2. JSON with extra text before or after (will try to extract JSON part)
    
    Args:
        content: JSON string to parse
        
    Returns:
        Parsed dictionary, returns None if parsing fails
        
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
        # First remove leading and trailing whitespace (including newlines, spaces, etc.)
        cleaned_content = content.strip()
        
        # Try to parse directly
        return json.loads(cleaned_content)
    except json.JSONDecodeError as e:
        logger.debug(f"Direct JSON parsing failed: {e}, trying to extract JSON content")
        
        # Try to find JSON content (some models may add extra text before or after JSON)
        try:
            # Find first '{' and last '}'
            start_idx = cleaned_content.find('{')
            end_idx = cleaned_content.rfind('}')
            
            if start_idx != -1 and end_idx != -1 and start_idx < end_idx:
                json_str = cleaned_content[start_idx:end_idx+1]
                result = json.loads(json_str)
                logger.debug(f"Successfully extracted and parsed JSON from content")
                return result
        except json.JSONDecodeError as e2:
            logger.warning(f"JSON extraction parsing also failed: {e2}")
        
        logger.error(f"Unable to parse JSON content: {content[:200]}...")
        return None


def format_json_error_message(content: str, max_length: int = 200) -> str:
    """
    Format JSON parsing error message
    
    Args:
        content: Failed content
        max_length: Maximum display length
        
    Returns:
        Formatted error message
    """
    if len(content) <= max_length:
        return f"Unable to parse content: {content}"
    else:
        return f"Unable to parse content (first {max_length} characters): {content[:max_length]}..."
