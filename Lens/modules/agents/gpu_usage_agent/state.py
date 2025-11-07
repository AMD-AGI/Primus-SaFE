"""Agent State Management - Simplified."""

from typing import TypedDict, Dict, Any, Optional


class GPUQueryParams(TypedDict, total=False):
    """GPU 查询参数"""
    
    # 时间参数
    time_range: Dict[str, str]  # {"type": "relative", "value": "7d"}
    
    # 维度参数
    dimension: Optional[str]  # cluster/namespace/label/annotation
    dimension_value: Optional[str]  # 维度的具体值
    
    # 指标参数
    metric: str  # utilization/allocation_rate
    granularity: str  # hour/day/week


class GPUQueryResult(TypedDict):
    """GPU 查询结果"""
    
    # 响应内容
    answer: str  # 摘要信息
    needs_clarification: bool  # 是否需要澄清
    
    # 数据
    data: Dict[str, Any]  # 各维度的数据
    
    # 调试信息
    debug_info: Dict[str, Any]
