"""GPU Usage Analysis Agent - Enhanced with Analysis Features."""

import json
import logging
from typing import Dict, Any, Optional, List, Tuple
from datetime import datetime

from langchain_core.messages import SystemMessage
from langchain_core.language_models import BaseChatModel

from .tools import GPUAnalysisTools
from .utils import safe_json_parse
from .prompts import UNDERSTAND_PROMPT

# 配置日志
logger = logging.getLogger(__name__)

# 导入缓存相关模块
try:
    from cache.base import CacheBase
    from llm_wrapper import CachedLLM
    CACHE_AVAILABLE = True
except ImportError:
    CACHE_AVAILABLE = False
    logger.warning("缓存模块不可用，将不使用 LLM 缓存")


class GPUUsageAnalysisAgent:
    """GPU 使用率分析 Agent - 增强版本，支持深度分析"""
    
    def __init__(
        self,
        llm: BaseChatModel,
        api_base_url: str,
        cluster_name: Optional[str] = None,
        cache: Optional[CacheBase] = None,
        cache_enabled: bool = True
    ):
        """
        初始化 Agent
        
        Args:
            llm: 语言模型
            api_base_url: Lens API 基础 URL
            cluster_name: 集群名称（可选）
            cache: 缓存实例（可选）
            cache_enabled: 是否启用缓存
        """
        # 如果启用缓存且缓存可用，使用 CachedLLM 包装
        if cache_enabled and cache is not None and CACHE_AVAILABLE:
            self.llm = CachedLLM(llm, cache=cache, cache_enabled=True)
            self.cache_enabled = True
            logger.info("LLM 缓存已启用")
        else:
            self.llm = llm
            self.cache_enabled = False
            if cache_enabled and cache is not None:
                logger.warning("缓存模块不可用，将不使用 LLM 缓存")
        
        self.api_base_url = api_base_url
        self.cluster_name = cluster_name
        
        # 初始化工具集
        self.tools_manager = GPUAnalysisTools(api_base_url, cluster_name)
    
    def _understand_query(self, user_query: str) -> Dict[str, Any]:
        """理解用户查询，识别需要查询的维度和参数"""
        prompt = UNDERSTAND_PROMPT.format(user_query=user_query)
        messages = [SystemMessage(content=prompt)]
        
        try:
            logger.info(f"正在理解用户查询: {user_query}")
            response = self.llm.invoke(messages)
            logger.info("查询理解完成")
            
            # 解析 LLM 返回的 JSON
            result = safe_json_parse(response.content)
            
            if result is None:
                logger.warning(f"无法解析 LLM 返回的 JSON: {response.content[:200]}...")
                return {
                    "needs_clarification": True,
                    "clarification_question": "抱歉，我没有理解您的问题，能否重新描述一下？",
                    "entities": {}
                }
            
            return result
            
        except Exception as e:
            # 详细的错误日志
            import traceback
            error_type = type(e).__name__
            error_msg = str(e)
            error_traceback = traceback.format_exc()
            
            logger.error("=" * 80)
            logger.error(f"查询理解失败 - 用户查询: {user_query}")
            logger.error(f"错误类型: {error_type}")
            logger.error(f"错误消息: {error_msg}")
            logger.error(f"完整堆栈跟踪:\n{error_traceback}")
            logger.error("=" * 80)
            
            return {
                "needs_clarification": True,
                "clarification_question": f"处理查询时发生错误: {error_type} - {error_msg}",
                "entities": {},
                "error_details": {
                    "type": error_type,
                    "message": error_msg,
                    "traceback": error_traceback
                }
            }
    
    def _analyze_cluster_trend_with_chart(self, time_range_days: int, granularity: str = "hour") -> Dict[str, Any]:
        """
        分析cluster级别的使用率和占用率趋势，返回折线图数据
        
        Args:
            time_range_days: 时间范围（天数）
            granularity: 时间粒度
        
        Returns:
            包含折线图数据和统计信息的字典
        """
        logger.info("开始分析集群趋势...")
        
        try:
            # 调用API获取cluster hourly stats
            result = self.tools_manager.query_gpu_usage_trend(
                dimension="cluster",
                granularity=granularity,
                time_range_days=time_range_days,
                metric_type="utilization"
            )
            
            data = safe_json_parse(result)
            if not data or "data_points" not in data:
                return {"error": "无法获取集群数据"}
            
            data_points = data.get("data_points", [])
            statistics = data.get("statistics", {})
            
            # 构建折线图数据（同时包含使用率和占用率）
            chart_data = {
                "title": "集群GPU使用率和占用率趋势",
                "x_axis": [],  # 时间轴
                "series": [
                    {
                        "name": "使用率 (Utilization)",
                        "data": [],
                        "type": "line"
                    },
                    {
                        "name": "占用率 (Allocation Rate)", 
                        "data": [],
                        "type": "line"
                    }
                ]
            }
            
            for dp in data_points:
                timestamp = dp.get("stat_hour", "")
                avg_util = dp.get("avg_utilization", 0) * 100  # 转换为百分比
                alloc_rate = dp.get("allocation_rate", 0) * 100
                
                chart_data["x_axis"].append(timestamp)
                chart_data["series"][0]["data"].append(round(avg_util, 2))
                chart_data["series"][1]["data"].append(round(alloc_rate, 2))
            
            # 计算占用率统计（从 data_points 中提取）
            alloc_rates = [dp.get("allocation_rate", 0) for dp in data_points]
            avg_alloc_rate = sum(alloc_rates) / len(alloc_rates) if alloc_rates else 0
            max_alloc_rate = max(alloc_rates) if alloc_rates else 0
            min_alloc_rate = min(alloc_rates) if alloc_rates else 0
            
            return {
                "chart_data": chart_data,
                "statistics": {
                    "utilization": {
                        "average": round(statistics.get("average", 0) * 100, 2),
                        "max": round(statistics.get("max", 0) * 100, 2),
                        "min": round(statistics.get("min", 0) * 100, 2),
                        "trend": statistics.get("trend", "unknown")
                    },
                    "allocation_rate": {
                        "average": round(avg_alloc_rate * 100, 2),
                        "max": round(max_alloc_rate * 100, 2),
                        "min": round(min_alloc_rate * 100, 2)
                    },
                    "sample_count": statistics.get("sample_count", 0),
                    "time_range_days": time_range_days
                }
            }
            
        except Exception as e:
            logger.error(f"分析集群趋势失败: {str(e)}")
            return {"error": str(e)}
    
    def _analyze_namespace_usage(self, time_range_days: int, top_n: int = 10) -> Dict[str, Any]:
        """
        分析namespace级别的使用率
        
        Args:
            time_range_days: 时间范围（天）
            top_n: 返回前N个namespace
            
        Returns:
            包含namespace分析结果的字典
        """
        logger.info("开始分析namespace级别使用率...")
        
        try:
            # 获取所有namespaces
            namespaces_result = self.tools_manager.get_available_namespaces(time_range_days)
            namespaces_data = safe_json_parse(namespaces_result)
            namespaces = namespaces_data.get("namespaces", [])
            
            if not namespaces:
                return {"error": "未找到namespace数据"}
            
            # 获取每个namespace的使用率数据
            namespace_stats = []
            for ns in namespaces[:top_n]:
                try:
                    ns_result = self.tools_manager.query_gpu_usage_trend(
                        dimension="namespace",
                        dimension_value=ns,
                        granularity="hour",
                        time_range_days=time_range_days,
                        metric_type="utilization"
                    )
                    
                    ns_data = safe_json_parse(ns_result)
                    if ns_data and "statistics" in ns_data:
                        stats = ns_data["statistics"]
                        data_points = ns_data.get("data_points", [])
                        
                        # 计算平均分配的GPU数量
                        avg_gpu_count = 0
                        if data_points:
                            total_gpu = sum(dp.get("allocated_gpu_count", 0) for dp in data_points)
                            avg_gpu_count = total_gpu / len(data_points)
                        
                        namespace_stats.append({
                            "namespace": ns,
                            "avg_utilization": round(stats.get("average", 0) * 100, 2),
                            "max_utilization": round(stats.get("max", 0) * 100, 2),
                            "min_utilization": round(stats.get("min", 0) * 100, 2),
                            "trend": stats.get("trend", "unknown"),
                            "avg_gpu_count": round(avg_gpu_count, 2)
                        })
                except Exception as e:
                    logger.error(f"获取namespace {ns} 数据失败: {str(e)}")
            
            # 按平均使用率排序
            namespace_stats.sort(key=lambda x: x["avg_utilization"])
            
            return {
                "namespaces": namespace_stats,
                "total_count": len(namespace_stats),
                "summary": f"分析了 {len(namespace_stats)} 个namespaces"
            }
            
        except Exception as e:
            logger.error(f"分析namespace使用率失败: {str(e)}")
            return {"error": str(e)}
    
    def _find_low_utilization_annotations(
        self, 
        time_range_days: int,
        top_n_per_key: int = 20  # 每个key返回top N个values
    ) -> Tuple[List[Dict[str, Any]], Dict[str, Any]]:
        """
        找出占用GPU多但使用率低的annotations
        
        对于每个annotation key，找出其values中占用GPU最多但利用率最低的top N
        
        Args:
            time_range_days: 时间范围
            top_n_per_key: 每个annotation key返回的top N个values
            
        Returns:
            (低使用率annotation列表, 所有annotation数据)
        """
        logger.info("开始分析annotation使用情况...")
        
        try:
            # 获取所有annotation keys
            keys_result = self.tools_manager.get_available_dimension_keys("annotation", time_range_days)
            keys_data = safe_json_parse(keys_result)
            annotation_keys = keys_data.get("dimension_keys", [])
            
            if not annotation_keys:
                return [], {"error": "未找到annotation数据"}
            
            all_results = []
            results_by_key = {}
            
            # 对每个annotation key，使用工具方法找出低使用率的top N values
            for key in annotation_keys[:10]:  # 限制处理前10个key
                try:
                    logger.info(f"分析annotation key: {key}")
                    
                    # 调用tools方法，获取该key下top N的values
                    result_str = self.tools_manager.find_low_utilization_dimension_values(
                        dimension_type="annotation",
                        dimension_key=key,
                        time_range_days=time_range_days,
                        top_n=top_n_per_key
                    )
                    
                    result_data = safe_json_parse(result_str)
                    
                    if result_data and "results" in result_data and result_data["results"]:
                        # 保存该key的结果
                        results_by_key[key] = result_data
                        
                        # 转换格式以保持兼容性
                        for item in result_data["results"]:
                            all_results.append({
                                "annotation_key": key,
                                "annotation_value": item["dimension_value"],
                                "avg_utilization": item["avg_utilization"],
                                "avg_gpu_count": item["avg_gpu_count"],
                                "max_utilization": item.get("max_utilization", 0),
                                "min_utilization": item.get("min_utilization", 0),
                                "trend": item.get("trend", "unknown"),
                                "issue_score": item["issue_score"]
                            })
                        
                        logger.info(f"Key {key}: 找到 {len(result_data['results'])} 个低使用率values")
                    
                except Exception as e:
                    logger.error(f"分析annotation key {key} 失败: {str(e)}")
            
            # 按问题评分全局排序（分数越高越严重）
            all_results.sort(key=lambda x: x["issue_score"], reverse=True)
            
            return all_results, {
                "results_by_key": results_by_key,
                "all_annotations": all_results[:100],  # 返回前100个
                "total_count": len(all_results),
                "keys_analyzed": len(results_by_key)
            }
            
        except Exception as e:
            logger.error(f"分析annotation失败: {str(e)}")
            return [], {"error": str(e)}
    
    def _get_workloads_by_annotations(
        self,
        low_util_annotations: List[Dict[str, Any]],
        limit: int = 20
    ) -> Dict[str, Any]:
        """
        根据找到的低使用率annotations获取对应的workload列表
        
        Args:
            low_util_annotations: 低使用率annotation列表
            limit: 每个annotation返回的workload数量限制
            
        Returns:
            包含workload表格数据的字典
        """
        logger.info("开始查询低使用率annotations对应的workloads...")
        
        if not low_util_annotations:
            return {
                "table_data": [],
                "summary": "未找到低使用率的annotations"
            }
        
        try:
            # 注意：Lens API的workloads接口目前不支持直接按annotation过滤
            # 我们先获取所有workload，然后根据namespace等信息关联
            # 这里作为示例，获取最近的workloads
            
            workload_table = []
            
            # 对于每个低使用率annotation，获取相关workloads
            for anno in low_util_annotations[:10]:  # 限制前10个annotation
                anno_key = anno["annotation_key"]
                anno_value = anno["annotation_value"]
                
                try:
                    # 获取workloads（可以按其他条件过滤）
                    # 这里我们获取最近的workloads作为示例
                    workloads_result = self.tools_manager.analyze_workload_history(
                        time_range_days=7,
                        namespace=None,
                        limit=limit
                    )
                    
                    workloads_data = safe_json_parse(workloads_result)
                    if workloads_data and "workloads" in workloads_data:
                        workloads = workloads_data["workloads"]
                        
                        # 为每个workload添加annotation信息
                        for wl in workloads[:5]:  # 每个annotation限制5个workload
                            workload_table.append({
                                "annotation_key": anno_key,
                                "annotation_value": anno_value,
                                "annotation_avg_utilization": anno["avg_utilization"],
                                "annotation_avg_gpu_count": anno["avg_gpu_count"],
                                "workload_name": wl.get("name", ""),
                                "workload_namespace": wl.get("namespace", ""),
                                "workload_kind": wl.get("kind", ""),
                                "workload_status": wl.get("status", ""),
                                "workload_gpu_allocated": wl.get("gpuAllocated", 0),
                                "workload_start_time": wl.get("startAt", 0)
                            })
                            
                except Exception as e:
                    logger.error(f"获取annotation {anno_key}:{anno_value} 的workloads失败: {str(e)}")
            
            return {
                "table_data": workload_table,
                "columns": [
                    "annotation_key",
                    "annotation_value", 
                    "annotation_avg_utilization",
                    "annotation_avg_gpu_count",
                    "workload_name",
                    "workload_namespace",
                    "workload_kind",
                    "workload_status",
                    "workload_gpu_allocated"
                ],
                "total_count": len(workload_table),
                "summary": f"找到 {len(workload_table)} 个相关workloads"
            }
            
        except Exception as e:
            logger.error(f"查询workloads失败: {str(e)}")
            return {"error": str(e)}
    
    def _analyze_all_namespaces(self, time_range_days: int, top_n: int = 10) -> Dict[str, Any]:
        """分析所有namespace的使用情况"""
        logger.info("开始分析所有namespaces...")
        
        try:
            # 获取所有namespaces
            namespaces_result = self.tools_manager.get_available_namespaces(time_range_days)
            namespaces_data = safe_json_parse(namespaces_result)
            namespaces = namespaces_data.get("namespaces", [])
            
            if not namespaces:
                return {"error": "未找到namespace数据"}
            
            # 获取每个namespace的使用率数据
            namespace_stats = []
            for ns in namespaces[:top_n]:
                try:
                    ns_result = self.tools_manager.query_gpu_usage_trend(
                        dimension="namespace",
                        dimension_value=ns,
                        granularity="hour",
                        time_range_days=time_range_days,
                        metric_type="utilization"
                    )
                    
                    ns_data = safe_json_parse(ns_result)
                    if ns_data and "statistics" in ns_data:
                        stats = ns_data["statistics"]
                        data_points = ns_data.get("data_points", [])
                        
                        # 计算平均分配的GPU数量
                        avg_gpu_count = 0
                        if data_points:
                            total_gpu = sum(dp.get("allocated_gpu_count", 0) for dp in data_points)
                            avg_gpu_count = total_gpu / len(data_points)
                        
                        namespace_stats.append({
                            "namespace": ns,
                            "avg_utilization": round(stats.get("average", 0) * 100, 2),
                            "max_utilization": round(stats.get("max", 0) * 100, 2),
                            "min_utilization": round(stats.get("min", 0) * 100, 2),
                            "trend": stats.get("trend", "unknown"),
                            "avg_gpu_count": round(avg_gpu_count, 2)
                        })
                except Exception as e:
                    logger.error(f"获取namespace {ns} 数据失败: {str(e)}")
            
            # 按平均使用率排序
            namespace_stats.sort(key=lambda x: x["avg_utilization"])
            
            return {
                "namespaces": namespace_stats,
                "total_count": len(namespace_stats)
            }
            
        except Exception as e:
            logger.error(f"分析namespaces失败: {str(e)}")
            return {"error": str(e)}
    
    def _analyze_specific_namespace(self, namespace: str, time_range_days: int) -> Dict[str, Any]:
        """分析特定namespace的使用情况"""
        logger.info(f"开始分析namespace: {namespace}...")
        
        try:
            ns_result = self.tools_manager.query_gpu_usage_trend(
                dimension="namespace",
                dimension_value=namespace,
                granularity="hour",
                time_range_days=time_range_days,
                metric_type="utilization"
            )
            
            ns_data = safe_json_parse(ns_result)
            if not ns_data or "statistics" not in ns_data:
                return {"error": f"无法获取namespace {namespace} 的数据"}
            
            stats = ns_data["statistics"]
            data_points = ns_data.get("data_points", [])
            
            # 构建折线图数据
            chart_data = {
                "title": f"Namespace {namespace} GPU使用率趋势",
                "x_axis": [],
                "series": [{
                    "name": "使用率",
                    "data": [],
                    "type": "line"
                }]
            }
            
            for dp in data_points:
                timestamp = dp.get("stat_hour", "")
                avg_util = dp.get("avg_utilization", 0) * 100
                
                chart_data["x_axis"].append(timestamp)
                chart_data["series"][0]["data"].append(round(avg_util, 2))
            
            # 计算平均GPU数量
            avg_gpu_count = 0
            if data_points:
                total_gpu = sum(dp.get("allocated_gpu_count", 0) for dp in data_points)
                avg_gpu_count = total_gpu / len(data_points)
            
            return {
                "namespace": namespace,
                "chart_data": chart_data,
                "statistics": {
                    "avg_utilization": round(stats.get("average", 0) * 100, 2),
                    "max_utilization": round(stats.get("max", 0) * 100, 2),
                    "min_utilization": round(stats.get("min", 0) * 100, 2),
                    "trend": stats.get("trend", "unknown"),
                    "avg_gpu_count": round(avg_gpu_count, 2)
                }
            }
            
        except Exception as e:
            logger.error(f"分析namespace {namespace} 失败: {str(e)}")
            return {"error": str(e)}
    
    def _analyze_all_users(self, time_range_days: int, top_n: int = 20) -> Dict[str, Any]:
        """分析所有用户的GPU占用和使用率情况"""
        logger.info("开始分析所有用户...")
        
        try:
            result_str = self.tools_manager.analyze_user_gpu_usage(
                time_range_days=time_range_days,
                top_n=top_n
            )
            
            result_data = safe_json_parse(result_str)
            
            if not result_data or "results" not in result_data:
                return {"error": "无法获取用户数据"}
            
            users = result_data.get("results", [])
            
            # 构建表格数据
            table_data = {
                "columns": ["用户名", "平均GPU占用", "平均使用率(%)", "最大使用率(%)", "问题评分"],
                "rows": []
            }
            
            for user in users:
                table_data["rows"].append([
                    user.get("dimension_value", ""),
                    user.get("avg_gpu_count", 0),
                    user.get("avg_utilization", 0),
                    user.get("max_utilization", 0),
                    user.get("issue_score", 0)
                ])
            
            return {
                "table_data": table_data,
                "users": users,
                "total_count": len(users),
                "summary": result_data.get("summary", "")
            }
            
        except Exception as e:
            logger.error(f"分析用户失败: {str(e)}")
            return {"error": str(e)}
    
    def _analyze_specific_user(self, user_name: str, time_range_days: int) -> Dict[str, Any]:
        """分析特定用户的GPU占用情况"""
        logger.info(f"开始分析用户: {user_name}...")
        
        try:
            dimension_value = f"primus-safe.user.name:{user_name}"
            user_result = self.tools_manager.query_gpu_usage_trend(
                dimension="annotation",
                dimension_value=dimension_value,
                granularity="hour",
                time_range_days=time_range_days,
                metric_type="utilization"
            )
            
            user_data = safe_json_parse(user_result)
            if not user_data or "statistics" not in user_data:
                return {"error": f"无法获取用户 {user_name} 的数据"}
            
            stats = user_data["statistics"]
            data_points = user_data.get("data_points", [])
            
            # 构建折线图数据
            chart_data = {
                "title": f"用户 {user_name} GPU使用率趋势",
                "x_axis": [],
                "series": [{
                    "name": "使用率",
                    "data": [],
                    "type": "line"
                }]
            }
            
            for dp in data_points:
                timestamp = dp.get("stat_hour", "")
                avg_util = dp.get("avg_utilization", 0) * 100
                
                chart_data["x_axis"].append(timestamp)
                chart_data["series"][0]["data"].append(round(avg_util, 2))
            
            # 计算平均GPU数量
            avg_gpu_count = 0
            if data_points:
                total_gpu = sum(dp.get("allocated_gpu_count", 0) for dp in data_points)
                avg_gpu_count = total_gpu / len(data_points)
            
            return {
                "user_name": user_name,
                "chart_data": chart_data,
                "statistics": {
                    "avg_utilization": round(stats.get("average", 0) * 100, 2),
                    "max_utilization": round(stats.get("max", 0) * 100, 2),
                    "min_utilization": round(stats.get("min", 0) * 100, 2),
                    "trend": stats.get("trend", "unknown"),
                    "avg_gpu_count": round(avg_gpu_count, 2)
                }
            }
            
        except Exception as e:
            logger.error(f"分析用户 {user_name} 失败: {str(e)}")
            return {"error": str(e)}
    
    def _analyze_low_utilization_resources(self, time_range_days: int) -> Dict[str, Any]:
        """分析低使用率资源（包含所有annotations）"""
        logger.info("开始分析低使用率资源...")
        
        try:
            low_util_annos, all_anno_data = self._find_low_utilization_annotations(time_range_days)
            
            return {
                "low_utilization_annotations": low_util_annos,
                "all_annotations_summary": all_anno_data,
                "total_count": len(low_util_annos)
            }
            
        except Exception as e:
            logger.error(f"分析低使用率资源失败: {str(e)}")
            return {"error": str(e)}
    
    def chat(
        self,
        user_query: str,
        conversation_history: Optional[List] = None
    ) -> Dict[str, Any]:
        """
        处理用户查询
        
        Args:
            user_query: 用户查询
            conversation_history: 对话历史（可选）
        
        Returns:
            包含分析结果的字典
        """
        try:
            logger.info(f"开始处理查询: {user_query}")
            
            # 1. 理解用户查询
            understanding = self._understand_query(user_query)
            
            # 2. 如果需要澄清，直接返回
            if understanding.get("needs_clarification"):
                return {
                    "answer": understanding.get("clarification_question", "请提供更多信息"),
                    "needs_clarification": True,
                    "data": {},
                    "debug_info": {
                        "understanding": understanding
                    }
                }
            
            # 3. 解析查询参数
            entities = understanding.get("entities", {})
            time_range = entities.get("time_range", {})
            analysis_type = entities.get("analysis_type", "full")
            specific_dimension = entities.get("specific_dimension")
            output_format = entities.get("output_format", "both")
            
            # 计算时间范围
            time_range_days = 7  # 默认7天
            if time_range:
                time_value = time_range.get("value", "7d")
                if isinstance(time_value, str) and time_value.endswith("d"):
                    try:
                        time_range_days = int(time_value[:-1])
                    except:
                        time_range_days = 7
            
            # 4. 根据分析类型执行不同的分析
            result = {
                "answer": "",
                "needs_clarification": False,
                "data": {},
                "debug_info": {
                    "understanding": understanding,
                    "time_range_days": time_range_days,
                    "analysis_type": analysis_type
                }
            }
            
            if analysis_type == "cluster_trend":
                # 集群趋势分析（带折线图）
                logger.info("执行集群趋势分析...")
                cluster_analysis = self._analyze_cluster_trend_with_chart(time_range_days)
                result["data"]["cluster_trend"] = cluster_analysis
                result["answer"] = self._generate_cluster_trend_summary(cluster_analysis)
                
            elif analysis_type == "namespace_analysis":
                # Namespace分析
                logger.info("执行namespace分析...")
                if specific_dimension and specific_dimension.get("type") == "namespace":
                    # 分析特定namespace
                    namespace_value = specific_dimension.get("value")
                    namespace_analysis = self._analyze_specific_namespace(namespace_value, time_range_days)
                else:
                    # 分析所有namespace
                    namespace_analysis = self._analyze_all_namespaces(time_range_days)
                result["data"]["namespace_analysis"] = namespace_analysis
                result["answer"] = self._generate_namespace_summary(namespace_analysis)
                
            elif analysis_type == "user_analysis":
                # 用户占用分析（带表格）
                logger.info("执行用户占用分析...")
                if specific_dimension and specific_dimension.get("type") == "user":
                    # 分析特定用户
                    user_name = specific_dimension.get("value")
                    user_analysis = self._analyze_specific_user(user_name, time_range_days)
                else:
                    # 分析所有用户
                    user_analysis = self._analyze_all_users(time_range_days)
                result["data"]["user_analysis"] = user_analysis
                result["answer"] = self._generate_user_analysis_summary(user_analysis)
                
            elif analysis_type == "low_utilization":
                # 低使用率资源识别
                logger.info("分析低使用率资源...")
                low_util_analysis = self._analyze_low_utilization_resources(time_range_days)
                result["data"]["low_utilization"] = low_util_analysis
                result["answer"] = self._generate_low_utilization_summary(low_util_analysis)
                
            else:  # "full" - 完整分析
                logger.info("执行完整分析...")
                
                # 集群趋势
                cluster_analysis = self._analyze_cluster_trend_with_chart(time_range_days)
                result["data"]["cluster_trend"] = cluster_analysis
                
                # Namespace分析
                namespace_analysis = self._analyze_all_namespaces(time_range_days, top_n=10)
                result["data"]["namespace_analysis"] = namespace_analysis
                
                # 用户分析
                user_analysis = self._analyze_all_users(time_range_days, top_n=20)
                result["data"]["user_analysis"] = user_analysis
                
                # 生成综合摘要
                result["answer"] = self._generate_full_analysis_summary(
                    cluster_analysis, namespace_analysis, user_analysis
                )
            
            logger.info("查询处理完成")
            return result
        
        except Exception as e:
            logger.error(f"处理查询失败: {str(e)}")
            import traceback
            return {
                "answer": f"处理查询时发生错误: {str(e)}",
                "needs_clarification": False,
                "data": {},
                "debug_info": {
                    "error": str(e),
                    "traceback": traceback.format_exc()
                }
            }
    
    async def achat(
        self,
        user_query: str,
        conversation_history: Optional[List] = None
    ) -> Dict[str, Any]:
        """
        异步处理用户查询
        
        Args:
            user_query: 用户查询
            conversation_history: 对话历史（可选）
        
        Returns:
            包含分析结果的字典
        """
        # 简化版本暂时直接调用同步方法
        return self.chat(user_query, conversation_history)
    
    async def stream_chat(
        self,
        user_query: str,
        conversation_history: Optional[List] = None
    ):
        """
        流式处理用户查询，逐步返回分析结果
        
        Args:
            user_query: 用户查询
            conversation_history: 对话历史（可选）
        
        Yields:
            包含分析进度和结果的字典
        """
        try:
            logger.info(f"开始流式处理查询: {user_query}")
            
            # 1. 理解用户查询
            yield {
                "type": "status",
                "stage": "understanding",
                "message": "正在理解您的查询..."
            }
            
            understanding = self._understand_query(user_query)
            
            # 2. 如果需要澄清，直接返回
            if understanding.get("needs_clarification"):
                yield {
                    "type": "final",
                    "answer": understanding.get("clarification_question", "请提供更多信息"),
                    "needs_clarification": True,
                    "data": {},
                    "debug_info": {
                        "understanding": understanding
                    }
                }
                return
            
            # 3. 解析查询参数
            entities = understanding.get("entities", {})
            time_range = entities.get("time_range", {})
            analysis_type = entities.get("analysis_type", "full")
            specific_dimension = entities.get("specific_dimension")
            
            # 计算时间范围
            time_range_days = 7  # 默认7天
            if time_range:
                time_value = time_range.get("value", "7d")
                if isinstance(time_value, str) and time_value.endswith("d"):
                    try:
                        time_range_days = int(time_value[:-1])
                    except:
                        time_range_days = 7
            
            yield {
                "type": "status",
                "stage": "understanding_complete",
                "message": f"查询理解完成，分析类型: {analysis_type}，时间范围: {time_range_days}天"
            }
            
            # 4. 执行分析
            result = {
                "answer": "",
                "needs_clarification": False,
                "data": {},
                "debug_info": {
                    "understanding": understanding,
                    "time_range_days": time_range_days,
                    "analysis_type": analysis_type
                }
            }
            
            if analysis_type == "cluster_trend":
                # 集群趋势分析
                yield {
                    "type": "status",
                    "stage": "cluster_analysis",
                    "message": "正在分析集群趋势..."
                }
                
                cluster_analysis = self._analyze_cluster_trend_with_chart(time_range_days)
                result["data"]["cluster_trend"] = cluster_analysis
                
                yield {
                    "type": "data",
                    "stage": "cluster_analysis_complete",
                    "message": "集群趋势分析完成",
                    "data": {"cluster_trend": cluster_analysis}
                }
                
                result["answer"] = self._generate_cluster_trend_summary(cluster_analysis)
                
            elif analysis_type == "namespace_analysis":
                # Namespace分析
                yield {
                    "type": "status",
                    "stage": "namespace_analysis",
                    "message": "正在分析namespace..."
                }
                
                if specific_dimension and specific_dimension.get("type") == "namespace":
                    namespace_value = specific_dimension.get("value")
                    namespace_analysis = self._analyze_specific_namespace(namespace_value, time_range_days)
                else:
                    namespace_analysis = self._analyze_all_namespaces(time_range_days)
                
                result["data"]["namespace_analysis"] = namespace_analysis
                
                yield {
                    "type": "data",
                    "stage": "namespace_analysis_complete",
                    "message": "Namespace分析完成",
                    "data": {"namespace_analysis": namespace_analysis}
                }
                
                result["answer"] = self._generate_namespace_summary(namespace_analysis)
                
            elif analysis_type == "user_analysis":
                # 用户分析
                yield {
                    "type": "status",
                    "stage": "user_analysis",
                    "message": "正在分析用户占用情况..."
                }
                
                if specific_dimension and specific_dimension.get("type") == "user":
                    user_name = specific_dimension.get("value")
                    user_analysis = self._analyze_specific_user(user_name, time_range_days)
                else:
                    user_analysis = self._analyze_all_users(time_range_days)
                
                result["data"]["user_analysis"] = user_analysis
                
                yield {
                    "type": "data",
                    "stage": "user_analysis_complete",
                    "message": "用户分析完成",
                    "data": {"user_analysis": user_analysis}
                }
                
                result["answer"] = self._generate_user_analysis_summary(user_analysis)
                
            elif analysis_type == "low_utilization":
                # 低使用率资源分析
                yield {
                    "type": "status",
                    "stage": "low_utilization_analysis",
                    "message": "正在分析低使用率资源..."
                }
                
                low_util_analysis = self._analyze_low_utilization_resources(time_range_days)
                result["data"]["low_utilization"] = low_util_analysis
                
                yield {
                    "type": "data",
                    "stage": "low_utilization_complete",
                    "message": "低使用率资源分析完成",
                    "data": {"low_utilization": low_util_analysis}
                }
                
                result["answer"] = self._generate_low_utilization_summary(low_util_analysis)
                
            else:  # "full" - 完整分析
                # 集群趋势
                yield {
                    "type": "status",
                    "stage": "cluster_analysis",
                    "message": "正在分析集群趋势..."
                }
                
                cluster_analysis = self._analyze_cluster_trend_with_chart(time_range_days)
                result["data"]["cluster_trend"] = cluster_analysis
                
                yield {
                    "type": "data",
                    "stage": "cluster_complete",
                    "message": "集群分析完成",
                    "data": {"cluster_trend": cluster_analysis}
                }
                
                # Namespace分析
                yield {
                    "type": "status",
                    "stage": "namespace_analysis",
                    "message": "正在分析namespaces..."
                }
                
                namespace_analysis = self._analyze_all_namespaces(time_range_days, top_n=10)
                result["data"]["namespace_analysis"] = namespace_analysis
                
                yield {
                    "type": "data",
                    "stage": "namespace_complete",
                    "message": "Namespace分析完成",
                    "data": {"namespace_analysis": namespace_analysis}
                }
                
                # 用户分析
                yield {
                    "type": "status",
                    "stage": "user_analysis",
                    "message": "正在分析用户占用情况..."
                }
                
                user_analysis = self._analyze_all_users(time_range_days, top_n=20)
                result["data"]["user_analysis"] = user_analysis
                
                yield {
                    "type": "data",
                    "stage": "user_complete",
                    "message": "用户分析完成",
                    "data": {"user_analysis": user_analysis}
                }
                
                # 生成综合摘要
                result["answer"] = self._generate_full_analysis_summary(
                    cluster_analysis, namespace_analysis, user_analysis
                )
            
            # 返回最终结果
            yield {
                "type": "final",
                "answer": result["answer"],
                "needs_clarification": False,
                "data": result["data"],
                "debug_info": result["debug_info"]
            }
            
            logger.info("流式查询处理完成")
        
        except Exception as e:
            logger.error(f"流式处理查询失败: {str(e)}")
            import traceback
            yield {
                "type": "error",
                "answer": f"处理查询时发生错误: {str(e)}",
                "needs_clarification": False,
                "data": {},
                "debug_info": {
                    "error": str(e),
                    "traceback": traceback.format_exc()
                }
            }
    
    # ==================== 摘要生成方法 ====================
    
    def _generate_cluster_trend_summary(self, analysis: Dict[str, Any]) -> str:
        """生成集群趋势分析摘要"""
        if "error" in analysis:
            return f"分析失败: {analysis['error']}"
        
        stats = analysis.get("statistics", {})
        util_stats = stats.get("utilization", {})
        alloc_stats = stats.get("allocation_rate", {})
        
        summary = f"""## 集群GPU使用情况分析

### 📊 使用率统计
- 平均使用率: {util_stats.get('average', 0)}%
- 最大使用率: {util_stats.get('max', 0)}%
- 最小使用率: {util_stats.get('min', 0)}%
- 趋势: {util_stats.get('trend', 'unknown')}

### 📈 占用率统计
- 平均占用率: {alloc_stats.get('average', 0)}%
- 最大占用率: {alloc_stats.get('max', 0)}%
- 最小占用率: {alloc_stats.get('min', 0)}%

📌 已生成折线图，请查看可视化结果。
"""
        return summary
    
    def _generate_namespace_summary(self, analysis: Dict[str, Any]) -> str:
        """生成namespace分析摘要"""
        if "error" in analysis:
            return f"分析失败: {analysis['error']}"
        
        # 如果是单个namespace分析
        if "namespace" in analysis:
            ns = analysis["namespace"]
            stats = analysis.get("statistics", {})
            return f"""## Namespace {ns} GPU使用情况

### 📊 统计信息
- 平均使用率: {stats.get('avg_utilization', 0)}%
- 最大使用率: {stats.get('max_utilization', 0)}%
- 最小使用率: {stats.get('min_utilization', 0)}%
- 平均GPU占用: {stats.get('avg_gpu_count', 0)} 张
- 趋势: {stats.get('trend', 'unknown')}

📌 已生成折线图，请查看可视化结果。
"""
        
        # 如果是所有namespace分析
        namespaces = analysis.get("namespaces", [])
        total = analysis.get("total_count", 0)
        
        if not namespaces:
            return "未找到namespace数据。"
        
        summary = f"""## Namespace GPU使用情况分析

共分析了 {total} 个namespaces。

### 使用率最低的前5个Namespaces：
"""
        for i, ns in enumerate(namespaces[:5]):
            summary += f"{i+1}. **{ns['namespace']}**: 平均使用率 {ns['avg_utilization']}%, 平均占用 {ns['avg_gpu_count']} 张GPU\n"
        
        return summary
    
    def _generate_user_analysis_summary(self, analysis: Dict[str, Any]) -> str:
        """生成用户分析摘要"""
        if "error" in analysis:
            return f"分析失败: {analysis['error']}"
        
        # 如果是单个用户分析
        if "user_name" in analysis:
            user = analysis["user_name"]
            stats = analysis.get("statistics", {})
            return f"""## 用户 {user} GPU使用情况

### 📊 统计信息
- 平均使用率: {stats.get('avg_utilization', 0)}%
- 最大使用率: {stats.get('max_utilization', 0)}%
- 最小使用率: {stats.get('min_utilization', 0)}%
- 平均GPU占用: {stats.get('avg_gpu_count', 0)} 张
- 趋势: {stats.get('trend', 'unknown')}

📌 已生成折线图，请查看可视化结果。
"""
        
        # 如果是所有用户分析
        users = analysis.get("users", [])
        total = analysis.get("total_count", 0)
        
        if not users:
            return "未找到用户数据。"
        
        summary = f"""## 用户GPU占用分析

共分析了 {total} 个用户。

### 🔍 占用GPU多但使用率低的用户（按问题评分排序）：

| 用户名 | 平均GPU占用 | 平均使用率 | 最大使用率 | 问题评分 |
|--------|-------------|------------|------------|----------|
"""
        for user in users[:10]:
            summary += f"| {user['dimension_value']} | {user['avg_gpu_count']} | {user['avg_utilization']}% | {user['max_utilization']}% | {user['issue_score']} |\n"
        
        summary += "\n💡 **建议**: 问题评分高的用户建议优化GPU使用效率或减少占用。\n\n📊 详细数据见下方表格。"
        
        return summary
    
    def _generate_low_utilization_summary(self, analysis: Dict[str, Any]) -> str:
        """生成低使用率资源分析摘要"""
        if "error" in analysis:
            return f"分析失败: {analysis['error']}"
        
        low_util_annos = analysis.get("low_utilization_annotations", [])
        total = analysis.get("total_count", 0)
        
        if not low_util_annos:
            return "✅ 未发现明显的低使用率资源问题。"
        
        summary = f"""## 低使用率资源分析

发现 {total} 个占用GPU多但使用率低的资源。

### 🔴 问题最严重的前10个：

"""
        for i, anno in enumerate(low_util_annos[:10]):
            summary += f"{i+1}. **{anno['annotation_key']}={anno['annotation_value']}**\n"
            summary += f"   - 平均GPU占用: {anno['avg_gpu_count']} 张\n"
            summary += f"   - 平均使用率: {anno['avg_utilization']}%\n"
            summary += f"   - 问题评分: {anno['issue_score']}\n\n"
        
        summary += "💡 **建议**: 联系相关资源负责人，优化GPU使用效率。"
        
        return summary
    
    def _generate_full_analysis_summary(
        self,
        cluster_analysis: Dict[str, Any],
        namespace_analysis: Dict[str, Any],
        user_analysis: Dict[str, Any]
    ) -> str:
        """生成完整分析摘要"""
        summary = "# GPU使用情况完整分析报告\n\n"
        
        # 集群级别摘要
        summary += "## 1. 集群整体情况\n\n"
        if "error" not in cluster_analysis:
            stats = cluster_analysis.get("statistics", {})
            util_stats = stats.get("utilization", {})
            alloc_stats = stats.get("allocation_rate", {})
            summary += f"- 平均使用率: {util_stats.get('average', 0)}%\n"
            summary += f"- 平均占用率: {alloc_stats.get('average', 0)}%\n"
            summary += f"- 趋势: {util_stats.get('trend', 'unknown')}\n\n"
            summary += "📊 已生成集群趋势折线图。\n\n"
        else:
            summary += f"集群分析失败: {cluster_analysis['error']}\n\n"
        
        # Namespace级别摘要
        summary += "## 2. Namespace分析\n\n"
        if "error" not in namespace_analysis:
            namespaces = namespace_analysis.get("namespaces", [])
            total_ns = namespace_analysis.get("total_count", 0)
            summary += f"共分析了 {total_ns} 个namespaces。\n\n"
            if namespaces:
                summary += "使用率最低的3个namespaces:\n"
                for i, ns in enumerate(namespaces[:3]):
                    summary += f"{i+1}. {ns['namespace']}: {ns['avg_utilization']}% (占用{ns['avg_gpu_count']}张GPU)\n"
        else:
            summary += f"Namespace分析失败: {namespace_analysis['error']}\n\n"
        
        # 用户级别摘要
        summary += "\n## 3. 用户占用分析\n\n"
        if "error" not in user_analysis:
            users = user_analysis.get("users", [])
            total_users = user_analysis.get("total_count", 0)
            summary += f"共分析了 {total_users} 个用户。\n\n"
            if users:
                summary += "占用多但使用率低的前5个用户:\n\n"
                summary += "| 用户名 | 平均GPU占用 | 平均使用率 | 问题评分 |\n"
                summary += "|--------|-------------|------------|----------|\n"
                for user in users[:5]:
                    summary += f"| {user['dimension_value']} | {user['avg_gpu_count']} | {user['avg_utilization']}% | {user['issue_score']} |\n"
                summary += "\n📊 详细用户数据见表格。\n"
        else:
            summary += f"用户分析失败: {user_analysis['error']}\n\n"
        
        summary += "\n---\n\n💡 **总体建议**: 重点关注使用率低但占用多的用户和namespace，优化资源利用效率。"
        
        return summary