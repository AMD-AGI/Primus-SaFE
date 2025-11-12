"""Agent State Management - Simplified."""

from typing import TypedDict, Dict, Any, Optional


class GPUQueryParams(TypedDict, total=False):
    """GPU query parameters"""
    
    # Time parameters
    time_range: Dict[str, str]  # {"type": "relative", "value": "7d"}
    
    # Dimension parameters
    dimension: Optional[str]  # cluster/namespace/label/annotation
    dimension_value: Optional[str]  # Specific value of the dimension
    
    # Metric parameters
    metric: str  # utilization/allocation_rate
    granularity: str  # hour/day/week


class GPUQueryResult(TypedDict):
    """GPU query result"""
    
    # Response content
    answer: str  # Summary information
    needs_clarification: bool  # Whether clarification is needed
    
    # Data
    data: Dict[str, Any]  # Data for each dimension
    
    # Debug info
    debug_info: Dict[str, Any]
