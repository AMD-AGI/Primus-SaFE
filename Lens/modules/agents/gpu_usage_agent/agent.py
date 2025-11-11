"""GPU Usage Analysis Agent - Enhanced with Analysis Features."""

import json
import logging
from typing import Dict, Any, Optional, List, Tuple
from datetime import datetime

from langchain_core.messages import SystemMessage
from langchain_core.language_models import BaseChatModel

from .tools import GPUAnalysisTools
from .utils import safe_json_parse
from .prompts import UNDERSTAND_PROMPT, CLUSTER_TREND_ANALYSIS_PROMPT

# Configure logging
logger = logging.getLogger(__name__)

# Import cache-related modules
try:
    from cache.base import CacheBase
    from llm_wrapper import CachedLLM
    CACHE_AVAILABLE = True
except ImportError:
    CACHE_AVAILABLE = False
    logger.warning("Cache module not available, LLM cache will not be used")


class GPUUsageAnalysisAgent:
    """GPU Usage Analysis Agent - Enhanced version with deep analysis support"""
    
    def __init__(
        self,
        llm: BaseChatModel,
        api_base_url: str,
        cluster_name: Optional[str] = None,
        cache: Optional[CacheBase] = None,
        cache_enabled: bool = True
    ):
        """
        Initialize Agent
        
        Args:
            llm: Language model
            api_base_url: Lens API base URL
            cluster_name: Cluster name (optional)
            cache: Cache instance (optional)
            cache_enabled: Whether to enable cache
        """
        # If cache is enabled and available, wrap with CachedLLM
        if cache_enabled and cache is not None and CACHE_AVAILABLE:
            self.llm = CachedLLM(llm, cache=cache, cache_enabled=True)
            self.cache_enabled = True
            logger.info("LLM cache enabled")
        else:
            self.llm = llm
            self.cache_enabled = False
            if cache_enabled and cache is not None:
                logger.warning("Cache module not available, LLM cache will not be used")
        
        self.api_base_url = api_base_url
        self.cluster_name = cluster_name
        
        # Initialize tools
        self.tools_manager = GPUAnalysisTools(api_base_url, cluster_name)
    
    def _understand_query(self, user_query: str) -> Dict[str, Any]:
        """Understand user query, identify dimensions and parameters to query"""
        prompt = UNDERSTAND_PROMPT.format(user_query=user_query)
        messages = [SystemMessage(content=prompt)]
        
        try:
            logger.info(f"Understanding user query: {user_query}")
            response = self.llm.invoke(messages)
            logger.info("Query understanding completed")
            
            # Parse JSON returned by LLM
            result = safe_json_parse(response.content)
            
            if result is None:
                logger.warning(f"Unable to parse JSON returned by LLM: {response.content[:200]}...")
                return {
                    "needs_clarification": True,
                    "clarification_question": "Sorry, I didn't understand your question. Could you please rephrase it?",
                    "entities": {}
                }
            
            return result
            
        except Exception as e:
            # Detailed error logging
            import traceback
            error_type = type(e).__name__
            error_msg = str(e)
            error_traceback = traceback.format_exc()
            
            logger.error("=" * 80)
            logger.error(f"Query understanding failed - User query: {user_query}")
            logger.error(f"Error type: {error_type}")
            logger.error(f"Error message: {error_msg}")
            logger.error(f"Full stack trace:\n{error_traceback}")
            logger.error("=" * 80)
            
            return {
                "needs_clarification": True,
                "clarification_question": f"Error occurred while processing query: {error_type} - {error_msg}",
                "entities": {},
                "error_details": {
                    "type": error_type,
                    "message": error_msg,
                    "traceback": error_traceback
                }
            }
    
    def _analyze_cluster_trend_with_chart(self, time_range_days: int, granularity: str = "hour") -> Dict[str, Any]:
        """
        Analyze cluster-level utilization and allocation rate trends, return line chart data
        
        Args:
            time_range_days: Time range in days
            granularity: Time granularity
        
        Returns:
            Dictionary containing line chart data and statistics
        """
        logger.info("Starting cluster trend analysis...")
        
        try:
            # Call API to get cluster hourly stats
            result = self.tools_manager.query_gpu_usage_trend(
                dimension="cluster",
                granularity=granularity,
                time_range_days=time_range_days,
                metric_type="utilization"
            )
            
            data = safe_json_parse(result)
            if not data or "data_points" not in data:
                return {"error": "Unable to retrieve cluster data"}
            
            data_points = data.get("data_points", [])
            statistics = data.get("statistics", {})
            
            # Build line chart data (include both utilization and allocation rate)
            chart_data = {
                "title": "Cluster GPU Utilization and Allocation Rate Trend",
                "x_axis": [],  # Time axis
                "series": [
                    {
                        "name": "Utilization",
                        "data": [],
                        "type": "line"
                    },
                    {
                        "name": "Allocation Rate", 
                        "data": [],
                        "type": "line"
                    }
                ]
            }
            
            for dp in data_points:
                timestamp = dp.get("stat_hour", "")
                avg_util = dp.get("avg_utilization", 0) * 100  # Convert to percentage
                alloc_rate = dp.get("allocation_rate", 0) * 100
                
                chart_data["x_axis"].append(timestamp)
                chart_data["series"][0]["data"].append(round(avg_util, 2))
                chart_data["series"][1]["data"].append(round(alloc_rate, 2))
            
            # === New: Use calculate_average_utilization tool to calculate detailed statistics ===
            # Pass data_points to tool for statistical calculation
            calculated_stats = {}
            if data_points:
                try:
                    records_json = json.dumps(data_points)
                    calc_result = self.tools_manager.calculate_average_utilization(records_json)
                    calculated_stats = safe_json_parse(calc_result)
                    logger.info(f"Statistical results calculated using calculate_average_utilization tool: {calculated_stats}")
                except Exception as e:
                    logger.warning(f"Failed to calculate statistics using tool: {str(e)}")
            # === End of new section ===
            
            # Calculate allocation rate statistics (extract from data_points)
            alloc_rates = [dp.get("allocation_rate", 0) for dp in data_points]
            avg_alloc_rate = sum(alloc_rates) / len(alloc_rates) if alloc_rates else 0
            max_alloc_rate = max(alloc_rates) if alloc_rates else 0
            min_alloc_rate = min(alloc_rates) if alloc_rates else 0
            
            # === New: Call LLM for deep trend analysis ===
            llm_analysis = {}
            try:
                # Prepare data points summary (select some data points to show to LLM)
                data_points_summary_lines = []
                sample_size = min(10, len(data_points))  # Show at most 10 data points
                step = len(data_points) // sample_size if sample_size > 0 else 1
                for i in range(0, len(data_points), max(1, step)):
                    if i < len(data_points):
                        dp = data_points[i]
                        data_points_summary_lines.append(
                            f"Time: {dp.get('stat_hour', '')}, "
                            f"Utilization: {round(dp.get('avg_utilization', 0) * 100, 2)}%, "
                            f"Allocation: {round(dp.get('allocation_rate', 0) * 100, 2)}%"
                        )
                data_points_summary = "\n".join(data_points_summary_lines[:sample_size])
                
                # Build prompt
                analysis_prompt = CLUSTER_TREND_ANALYSIS_PROMPT.format(
                    avg_utilization=round(statistics.get("average", 0) * 100, 2),
                    max_utilization=round(statistics.get("max", 0) * 100, 2),
                    min_utilization=round(statistics.get("min", 0) * 100, 2),
                    trend=statistics.get("trend", "unknown"),
                    time_range_days=time_range_days,
                    sample_count=statistics.get("sample_count", 0),
                    avg_allocation=round(avg_alloc_rate * 100, 2),
                    max_allocation=round(max_alloc_rate * 100, 2),
                    min_allocation=round(min_alloc_rate * 100, 2),
                    data_points_summary=data_points_summary
                )
                
                logger.info("Calling LLM for deep cluster trend analysis...")
                messages = [SystemMessage(content=analysis_prompt)]
                response = self.llm.invoke(messages)
                
                # Parse JSON returned by LLM
                llm_analysis = safe_json_parse(response.content)
                if llm_analysis:
                    logger.info("LLM trend analysis completed")
                else:
                    logger.warning("Unable to parse LLM analysis result as JSON")
                    llm_analysis = {"error": "Unable to parse LLM analysis result"}
                    
            except Exception as e:
                logger.error(f"LLM trend analysis failed: {str(e)}")
                llm_analysis = {"error": str(e)}
            # === End of new section ===
            
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
                    "time_range_days": time_range_days,
                    # Add detailed statistics calculated by tool (if successful)
                    "calculated_stats": calculated_stats
                },
                # Add LLM deep analysis results
                "llm_analysis": llm_analysis
            }
            
        except Exception as e:
            logger.error(f"Failed to analyze cluster trend: {str(e)}")
            return {"error": str(e)}
    
    def _analyze_namespace_usage(self, time_range_days: int, top_n: int = 10) -> Dict[str, Any]:
        """
        Analyze namespace-level utilization
        
        Args:
            time_range_days: Time range in days
            top_n: Return top N namespaces
            
        Returns:
            Dictionary containing namespace analysis results
        """
        logger.info("Starting namespace-level utilization analysis...")
        
        try:
            # Get all namespaces
            namespaces_result = self.tools_manager.get_available_namespaces(time_range_days)
            namespaces_data = safe_json_parse(namespaces_result)
            namespaces = namespaces_data.get("namespaces", [])
            
            if not namespaces:
                return {"error": "No namespace data found"}
            
            # Get utilization data for each namespace
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
                        
                        # Calculate average allocated GPU count
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
                    logger.error(f"Failed to get data for namespace {ns}: {str(e)}")
            
            # Sort by average utilization
            namespace_stats.sort(key=lambda x: x["avg_utilization"])
            
            return {
                "namespaces": namespace_stats,
                "total_count": len(namespace_stats),
                "summary": f"Analyzed {len(namespace_stats)} namespaces"
            }
            
        except Exception as e:
            logger.error(f"Failed to analyze namespace utilization: {str(e)}")
            return {"error": str(e)}
    
    def _find_low_utilization_annotations(
        self, 
        time_range_days: int,
        top_n_per_key: int = 20  # Return top N values per key
    ) -> Tuple[List[Dict[str, Any]], Dict[str, Any]]:
        """
        Find annotations with high GPU allocation but low utilization
        
        For each annotation key, find top N values with most GPU allocation but lowest utilization
        
        Args:
            time_range_days: Time range in days
            top_n_per_key: Top N values to return for each annotation key
            
        Returns:
            (List of low utilization annotations, all annotation data)
        """
        logger.info("Starting annotation usage analysis...")
        
        try:
            # Get all annotation keys
            keys_result = self.tools_manager.get_available_dimension_keys("annotation", time_range_days)
            keys_data = safe_json_parse(keys_result)
            annotation_keys = keys_data.get("dimension_keys", [])
            
            if not annotation_keys:
                return [], {"error": "No annotation data found"}
            
            all_results = []
            results_by_key = {}
            
            # For each annotation key, use tool method to find top N values with low utilization
            for key in annotation_keys[:10]:  # Limit to first 10 keys
                try:
                    logger.info(f"Analyzing annotation key: {key}")
                    
                    # Call tools method to get top N values for this key
                    result_str = self.tools_manager.find_low_utilization_dimension_values(
                        dimension_type="annotation",
                        dimension_key=key,
                        time_range_days=time_range_days,
                        top_n=top_n_per_key
                    )
                    
                    result_data = safe_json_parse(result_str)
                    
                    if result_data and "results" in result_data and result_data["results"]:
                        # Save results for this key
                        results_by_key[key] = result_data
                        
                        # Convert format for compatibility
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
                        
                        logger.info(f"Key {key}: found {len(result_data['results'])} low utilization values")
                    
                except Exception as e:
                    logger.error(f"Failed to analyze annotation key {key}: {str(e)}")
            
            # Sort globally by issue score (higher score = more serious)
            all_results.sort(key=lambda x: x["issue_score"], reverse=True)
            
            return all_results, {
                "results_by_key": results_by_key,
                "all_annotations": all_results[:100],  # Return top 100
                "total_count": len(all_results),
                "keys_analyzed": len(results_by_key)
            }
            
        except Exception as e:
            logger.error(f"Failed to analyze annotations: {str(e)}")
            return [], {"error": str(e)}
    
    def _get_workloads_by_annotations(
        self,
        low_util_annotations: List[Dict[str, Any]],
        limit: int = 20
    ) -> Dict[str, Any]:
        """
        Get workload list based on low utilization annotations found
        
        Args:
            low_util_annotations: List of low utilization annotations
            limit: Workload count limit per annotation
            
        Returns:
            Dictionary containing workload table data
        """
        logger.info("Starting query for workloads corresponding to low utilization annotations...")
        
        if not low_util_annotations:
            return {
                "table_data": [],
                "summary": "No low utilization annotations found"
            }
        
        try:
            # Note: Lens API's workloads interface currently doesn't support direct annotation filtering
            # We first get all workloads, then correlate based on namespace and other info
            # Here as an example, we get recent workloads
            
            workload_table = []
            
            # For each low utilization annotation, get related workloads
            for anno in low_util_annotations[:10]:  # Limit to first 10 annotations
                anno_key = anno["annotation_key"]
                anno_value = anno["annotation_value"]
                
                try:
                    # Get workloads (can filter by other conditions)
                    # Here we get recent workloads as an example
                    workloads_result = self.tools_manager.analyze_workload_history(
                        time_range_days=7,
                        namespace=None,
                        limit=limit
                    )
                    
                    workloads_data = safe_json_parse(workloads_result)
                    if workloads_data and "workloads" in workloads_data:
                        workloads = workloads_data["workloads"]
                        
                        # Add annotation info to each workload
                        for wl in workloads[:5]:  # Limit 5 workloads per annotation
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
                    logger.error(f"Failed to get workloads for annotation {anno_key}:{anno_value}: {str(e)}")
            
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
                "summary": f"Found {len(workload_table)} related workloads"
            }
            
        except Exception as e:
            logger.error(f"Failed to query workloads: {str(e)}")
            return {"error": str(e)}
    
    def _analyze_all_namespaces(self, time_range_days: int, top_n: int = 10) -> Dict[str, Any]:
        """Analyze usage of all namespaces"""
        logger.info("Starting analysis of all namespaces...")
        
        try:
            # Get all namespaces
            namespaces_result = self.tools_manager.get_available_namespaces(time_range_days)
            namespaces_data = safe_json_parse(namespaces_result)
            namespaces = namespaces_data.get("namespaces", [])
            
            if not namespaces:
                return {"error": "No namespace data found"}
            
            # Get utilization data for each namespace
            namespace_stats = []
            all_namespace_records = []  # For aggregating all namespace records
            
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
                        
                        # === New: Collect all namespace records for aggregate statistics ===
                        all_namespace_records.extend(data_points)
                        # === End of new section ===
                        
                        # Calculate average allocated GPU count
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
                    logger.error(f"Failed to get data for namespace {ns}: {str(e)}")
            
            # Sort by average utilization
            namespace_stats.sort(key=lambda x: x["avg_utilization"])
            
            # === New: Use tool to calculate aggregate statistics for all namespaces ===
            overall_stats = {}
            if all_namespace_records:
                try:
                    records_json = json.dumps(all_namespace_records)
                    calc_result = self.tools_manager.calculate_average_utilization(records_json)
                    overall_stats = safe_json_parse(calc_result)
                    logger.info(f"Aggregate statistics for all namespaces: {overall_stats}")
                except Exception as e:
                    logger.warning(f"Failed to calculate aggregate statistics: {str(e)}")
            # === End of new section ===
            
            return {
                "namespaces": namespace_stats,
                "total_count": len(namespace_stats),
                "overall_statistics": overall_stats  # Add aggregate statistics
            }
            
        except Exception as e:
            logger.error(f"Failed to analyze namespaces: {str(e)}")
            return {"error": str(e)}
    
    def _analyze_specific_namespace(self, namespace: str, time_range_days: int) -> Dict[str, Any]:
        """Analyze usage of a specific namespace"""
        logger.info(f"Starting analysis of namespace: {namespace}...")
        
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
                return {"error": f"Unable to get data for namespace {namespace}"}
            
            stats = ns_data["statistics"]
            data_points = ns_data.get("data_points", [])
            
            # Build line chart data
            chart_data = {
                "title": f"Namespace {namespace} GPU Utilization Trend",
                "x_axis": [],
                "series": [{
                    "name": "Utilization",
                    "data": [],
                    "type": "line"
                }]
            }
            
            for dp in data_points:
                timestamp = dp.get("stat_hour", "")
                avg_util = dp.get("avg_utilization", 0) * 100
                
                chart_data["x_axis"].append(timestamp)
                chart_data["series"][0]["data"].append(round(avg_util, 2))
            
            # Calculate average GPU count
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
            logger.error(f"Failed to analyze namespace {namespace}: {str(e)}")
            return {"error": str(e)}
    
    def _analyze_all_users(self, time_range_days: int, top_n: int = 20) -> Dict[str, Any]:
        """Analyze GPU allocation and utilization for all users"""
        logger.info("Starting analysis of all users...")
        
        try:
            result_str = self.tools_manager.analyze_user_gpu_usage(
                time_range_days=time_range_days,
                top_n=top_n
            )
            
            result_data = safe_json_parse(result_str)
            
            if not result_data or "results" not in result_data:
                return {"error": "Unable to get user data"}
            
            users = result_data.get("results", [])
            
            # Build table data
            table_data = {
                "columns": ["Username", "Avg GPU Allocation", "Avg Utilization(%)", "Max Utilization(%)", "Issue Score"],
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
            logger.error(f"Failed to analyze users: {str(e)}")
            return {"error": str(e)}
    
    def _analyze_specific_user(self, user_name: str, time_range_days: int) -> Dict[str, Any]:
        """Analyze GPU allocation for a specific user"""
        logger.info(f"Starting analysis of user: {user_name}...")
        
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
                return {"error": f"Unable to get data for user {user_name}"}
            
            stats = user_data["statistics"]
            data_points = user_data.get("data_points", [])
            
            # Build line chart data
            chart_data = {
                "title": f"User {user_name} GPU Utilization Trend",
                "x_axis": [],
                "series": [{
                    "name": "Utilization",
                    "data": [],
                    "type": "line"
                }]
            }
            
            for dp in data_points:
                timestamp = dp.get("stat_hour", "")
                avg_util = dp.get("avg_utilization", 0) * 100
                
                chart_data["x_axis"].append(timestamp)
                chart_data["series"][0]["data"].append(round(avg_util, 2))
            
            # Calculate average GPU count
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
            logger.error(f"Failed to analyze user {user_name}: {str(e)}")
            return {"error": str(e)}
    
    def _analyze_low_utilization_resources(self, time_range_days: int) -> Dict[str, Any]:
        """Analyze low utilization resources (including all annotations)"""
        logger.info("Starting low utilization resource analysis...")
        
        try:
            low_util_annos, all_anno_data = self._find_low_utilization_annotations(time_range_days)
            
            return {
                "low_utilization_annotations": low_util_annos,
                "all_annotations_summary": all_anno_data,
                "total_count": len(low_util_annos)
            }
            
        except Exception as e:
            logger.error(f"Failed to analyze low utilization resources: {str(e)}")
            return {"error": str(e)}
    
    def chat(
        self,
        user_query: str,
        conversation_history: Optional[List] = None
    ) -> Dict[str, Any]:
        """
        Process user query
        
        Args:
            user_query: User query
            conversation_history: Conversation history (optional)
        
        Returns:
            Dictionary containing analysis results
        """
        try:
            logger.info(f"Starting query processing: {user_query}")
            
            # 1. Understand user query
            understanding = self._understand_query(user_query)
            
            # 2. If clarification needed, return directly
            if understanding.get("needs_clarification"):
                return {
                    "answer": understanding.get("clarification_question", "Please provide more information"),
                    "needs_clarification": True,
                    "data": {},
                    "debug_info": {
                        "understanding": understanding
                    }
                }
            
            # 3. Parse query parameters
            entities = understanding.get("entities", {})
            time_range = entities.get("time_range", {})
            analysis_type = entities.get("analysis_type", "full")
            specific_dimension = entities.get("specific_dimension")
            output_format = entities.get("output_format", "both")
            
            # Calculate time range
            time_range_days = 7  # Default 7 days
            if time_range:
                time_value = time_range.get("value", "7d")
                if isinstance(time_value, str) and time_value.endswith("d"):
                    try:
                        time_range_days = int(time_value[:-1])
                    except:
                        time_range_days = 7
            
            # 4. Execute different analysis based on analysis type
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
                # Cluster trend analysis (with line chart)
                logger.info("Executing cluster trend analysis...")
                cluster_analysis = self._analyze_cluster_trend_with_chart(time_range_days)
                result["data"]["cluster_trend"] = cluster_analysis
                result["answer"] = self._generate_cluster_trend_summary(cluster_analysis)
                
            elif analysis_type == "namespace_analysis":
                # Namespace analysis
                logger.info("Executing namespace analysis...")
                if specific_dimension and specific_dimension.get("type") == "namespace":
                    # Analyze specific namespace
                    namespace_value = specific_dimension.get("value")
                    namespace_analysis = self._analyze_specific_namespace(namespace_value, time_range_days)
                else:
                    # Analyze all namespaces
                    namespace_analysis = self._analyze_all_namespaces(time_range_days)
                result["data"]["namespace_analysis"] = namespace_analysis
                result["answer"] = self._generate_namespace_summary(namespace_analysis)
                
            elif analysis_type == "user_analysis":
                # User allocation analysis (with table)
                logger.info("Executing user allocation analysis...")
                if specific_dimension and specific_dimension.get("type") == "user":
                    # Analyze specific user
                    user_name = specific_dimension.get("value")
                    user_analysis = self._analyze_specific_user(user_name, time_range_days)
                else:
                    # Analyze all users
                    user_analysis = self._analyze_all_users(time_range_days)
                result["data"]["user_analysis"] = user_analysis
                result["answer"] = self._generate_user_analysis_summary(user_analysis)
                
            elif analysis_type == "low_utilization":
                # Low utilization resource identification
                logger.info("Analyzing low utilization resources...")
                low_util_analysis = self._analyze_low_utilization_resources(time_range_days)
                result["data"]["low_utilization"] = low_util_analysis
                result["answer"] = self._generate_low_utilization_summary(low_util_analysis)
                
            else:  # "full" - Complete analysis
                logger.info("Executing full analysis...")
                
                # Cluster trend
                cluster_analysis = self._analyze_cluster_trend_with_chart(time_range_days)
                result["data"]["cluster_trend"] = cluster_analysis
                
                # Namespace analysis
                namespace_analysis = self._analyze_all_namespaces(time_range_days, top_n=10)
                result["data"]["namespace_analysis"] = namespace_analysis
                
                # User analysis
                user_analysis = self._analyze_all_users(time_range_days, top_n=20)
                result["data"]["user_analysis"] = user_analysis
                
                # Generate comprehensive summary
                result["answer"] = self._generate_full_analysis_summary(
                    cluster_analysis, namespace_analysis, user_analysis
                )
            
            logger.info("Query processing completed")
            return result
        
        except Exception as e:
            logger.error(f"Failed to process query: {str(e)}")
            import traceback
            return {
                "answer": f"Error occurred while processing query: {str(e)}",
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
        Asynchronously process user query
        
        Args:
            user_query: User query
            conversation_history: Conversation history (optional)
        
        Returns:
            Dictionary containing analysis results
        """
        # Simplified version calls synchronous method for now
        return self.chat(user_query, conversation_history)
    
    async def stream_chat(
        self,
        user_query: str,
        conversation_history: Optional[List] = None
    ):
        """
        Stream process user query, return analysis results progressively
        
        Args:
            user_query: User query
            conversation_history: Conversation history (optional)
        
        Yields:
            Dictionary containing analysis progress and results
        """
        try:
            logger.info(f"Starting stream processing of query: {user_query}")
            
            # 1. Understand user query
            yield {
                "type": "status",
                "stage": "understanding",
                "message": "Understanding your query..."
            }
            
            understanding = self._understand_query(user_query)
            
            # 2. If clarification needed, return directly
            if understanding.get("needs_clarification"):
                yield {
                    "type": "final",
                    "answer": understanding.get("clarification_question", "Please provide more information"),
                    "needs_clarification": True,
                    "data": {},
                    "debug_info": {
                        "understanding": understanding
                    }
                }
                return
            
            # 3. Parse query parameters
            entities = understanding.get("entities", {})
            time_range = entities.get("time_range", {})
            analysis_type = entities.get("analysis_type", "full")
            specific_dimension = entities.get("specific_dimension")
            
            # Calculate time range
            time_range_days = 7  # Default 7 days
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
                "message": f"Query understanding complete, analysis type: {analysis_type}, time range: {time_range_days} days"
            }
            
            # 4. Execute analysis
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
                # Cluster trend analysis
                yield {
                    "type": "status",
                    "stage": "cluster_analysis",
                    "message": "Analyzing cluster trend..."
                }
                
                cluster_analysis = self._analyze_cluster_trend_with_chart(time_range_days)
                result["data"]["cluster_trend"] = cluster_analysis
                
                yield {
                    "type": "data",
                    "stage": "cluster_analysis_complete",
                    "message": "Cluster trend analysis completed",
                    "data": {"cluster_trend": cluster_analysis}
                }
                
                result["answer"] = self._generate_cluster_trend_summary(cluster_analysis)
                
            elif analysis_type == "namespace_analysis":
                # Namespace analysis
                yield {
                    "type": "status",
                    "stage": "namespace_analysis",
                    "message": "Analyzing namespaces..."
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
                    "message": "Namespace analysis completed",
                    "data": {"namespace_analysis": namespace_analysis}
                }
                
                result["answer"] = self._generate_namespace_summary(namespace_analysis)
                
            elif analysis_type == "user_analysis":
                # User analysis
                yield {
                    "type": "status",
                    "stage": "user_analysis",
                    "message": "Analyzing user allocations..."
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
                    "message": "User analysis completed",
                    "data": {"user_analysis": user_analysis}
                }
                
                result["answer"] = self._generate_user_analysis_summary(user_analysis)
                
            elif analysis_type == "low_utilization":
                # Low utilization resource analysis
                yield {
                    "type": "status",
                    "stage": "low_utilization_analysis",
                    "message": "Analyzing low utilization resources..."
                }
                
                low_util_analysis = self._analyze_low_utilization_resources(time_range_days)
                result["data"]["low_utilization"] = low_util_analysis
                
                yield {
                    "type": "data",
                    "stage": "low_utilization_complete",
                    "message": "Low utilization resource analysis completed",
                    "data": {"low_utilization": low_util_analysis}
                }
                
                result["answer"] = self._generate_low_utilization_summary(low_util_analysis)
                
            else:  # "full" - Complete analysis
                # Cluster trend
                yield {
                    "type": "status",
                    "stage": "cluster_analysis",
                    "message": "Analyzing cluster trend..."
                }
                
                cluster_analysis = self._analyze_cluster_trend_with_chart(time_range_days)
                result["data"]["cluster_trend"] = cluster_analysis
                
                yield {
                    "type": "data",
                    "stage": "cluster_complete",
                    "message": "Cluster analysis completed",
                    "data": {"cluster_trend": cluster_analysis}
                }
                
                # Namespace analysis
                yield {
                    "type": "status",
                    "stage": "namespace_analysis",
                    "message": "Analyzing namespaces..."
                }
                
                namespace_analysis = self._analyze_all_namespaces(time_range_days, top_n=10)
                result["data"]["namespace_analysis"] = namespace_analysis
                
                yield {
                    "type": "data",
                    "stage": "namespace_complete",
                    "message": "Namespace analysis completed",
                    "data": {"namespace_analysis": namespace_analysis}
                }
                
                # User analysis
                yield {
                    "type": "status",
                    "stage": "user_analysis",
                    "message": "Analyzing user allocations..."
                }
                
                user_analysis = self._analyze_all_users(time_range_days, top_n=20)
                result["data"]["user_analysis"] = user_analysis
                
                yield {
                    "type": "data",
                    "stage": "user_complete",
                    "message": "User analysis completed",
                    "data": {"user_analysis": user_analysis}
                }
                
                # Generate comprehensive summary
                result["answer"] = self._generate_full_analysis_summary(
                    cluster_analysis, namespace_analysis, user_analysis
                )
            
            # Return final result
            yield {
                "type": "final",
                "answer": result["answer"],
                "needs_clarification": False,
                "data": result["data"],
                "debug_info": result["debug_info"]
            }
            
            logger.info("Stream query processing completed")
        
        except Exception as e:
            logger.error(f"Failed to stream process query: {str(e)}")
            import traceback
            yield {
                "type": "error",
                "answer": f"Error occurred while processing query: {str(e)}",
                "needs_clarification": False,
                "data": {},
                "debug_info": {
                    "error": str(e),
                    "traceback": traceback.format_exc()
                }
            }
    
    # ==================== Summary generation methods ====================
    
    def _generate_cluster_trend_summary(self, analysis: Dict[str, Any]) -> str:
        """Generate cluster trend analysis summary"""
        if "error" in analysis:
            return f"Analysis failed: {analysis['error']}"
        
        stats = analysis.get("statistics", {})
        util_stats = stats.get("utilization", {})
        alloc_stats = stats.get("allocation_rate", {})
        llm_analysis = analysis.get("llm_analysis", {})
        
        summary = f"""## Cluster GPU Usage Analysis

### 📊 Utilization Statistics
- Average Utilization: {util_stats.get('average', 0)}%
- Max Utilization: {util_stats.get('max', 0)}%
- Min Utilization: {util_stats.get('min', 0)}%
- Trend: {util_stats.get('trend', 'unknown')}

### 📈 Allocation Rate Statistics
- Average Allocation Rate: {alloc_stats.get('average', 0)}%
- Max Allocation Rate: {alloc_stats.get('max', 0)}%
- Min Allocation Rate: {alloc_stats.get('min', 0)}%

"""
        
        # === New: Add LLM deep analysis results ===
        if llm_analysis and "error" not in llm_analysis:
            summary += """### 🤖 AI Deep Analysis

"""
            # 1. Trend analysis
            trend_analysis = llm_analysis.get("trend_analysis", {})
            if trend_analysis:
                summary += f"""**Trend Assessment**
{trend_analysis.get('trend_description', '')}

"""
            
            # 2. Resource utilization efficiency evaluation
            efficiency = llm_analysis.get("efficiency_evaluation", {})
            if efficiency:
                overall_score = efficiency.get("overall_score", 0)
                efficiency_level = efficiency.get("efficiency_level", "Unknown")
                waste_pct = efficiency.get("waste_percentage", 0)
                
                # Choose appropriate emoji based on score
                score_emoji = "🟢" if overall_score >= 70 else "🟡" if overall_score >= 50 else "🔴"
                
                summary += f"""**Resource Utilization Efficiency Evaluation**
{score_emoji} Overall Score: **{overall_score}/100** ({efficiency_level})
- Resource Waste Level: {waste_pct}%
- Allocation Status: {efficiency.get('resource_allocation_status', 'Unknown')}

"""
            
            # 3. Problem diagnosis
            utilization_issues = llm_analysis.get("utilization_issues", {})
            problem_severity = llm_analysis.get("problem_severity", {})
            
            if problem_severity:
                needs_action = problem_severity.get("needs_immediate_action", False)
                critical_issues = problem_severity.get("critical_issues", [])
                warnings = problem_severity.get("warnings", [])
                
                if needs_action or critical_issues:
                    summary += """**⚠️ Issues Found**
"""
                    if critical_issues:
                        for issue in critical_issues:
                            summary += f"- 🔴 {issue}\n"
                    if warnings:
                        for warning in warnings:
                            summary += f"- 🟡 {warning}\n"
                    summary += "\n"
                else:
                    summary += "**✅ No Critical Issues Found**\n\n"
            
            # 4. Optimization recommendations
            recommendations = llm_analysis.get("recommendations", [])
            if recommendations:
                summary += """**💡 Optimization Recommendations**

"""
                for i, rec in enumerate(recommendations[:3], 1):  # Show at most 3 recommendations
                    priority = rec.get("priority", "Medium")
                    priority_emoji = "🔴" if priority == "High" else "🟡" if priority == "Medium" else "🟢"
                    
                    summary += f"""{i}. {priority_emoji} **[{priority} Priority]** {rec.get('issue', '')}
   - Recommendation: {rec.get('suggestion', '')}
   - Expected Improvement: {rec.get('expected_improvement', '')}

"""
            
            # 5. Summary
            llm_summary = llm_analysis.get("summary", "")
            if llm_summary:
                summary += f"""**📋 AI Summary**
{llm_summary}

"""
        elif llm_analysis and "error" in llm_analysis:
            summary += f"\n⚠️ AI deep analysis failed: {llm_analysis['error']}\n\n"
        # === End of new section ===
        
        summary += "📌 Line chart generated, please check visualization results.\n"
        
        return summary
    
    def _generate_namespace_summary(self, analysis: Dict[str, Any]) -> str:
        """Generate namespace analysis summary"""
        if "error" in analysis:
            return f"Analysis failed: {analysis['error']}"
        
        # If analyzing a single namespace
        if "namespace" in analysis:
            ns = analysis["namespace"]
            stats = analysis.get("statistics", {})
            return f"""## Namespace {ns} GPU Usage

### 📊 Statistics
- Average Utilization: {stats.get('avg_utilization', 0)}%
- Max Utilization: {stats.get('max_utilization', 0)}%
- Min Utilization: {stats.get('min_utilization', 0)}%
- Average GPU Allocation: {stats.get('avg_gpu_count', 0)} GPUs
- Trend: {stats.get('trend', 'unknown')}

📌 Line chart generated, please check visualization results.
"""
        
        # If analyzing all namespaces
        namespaces = analysis.get("namespaces", [])
        total = analysis.get("total_count", 0)
        overall_stats = analysis.get("overall_statistics", {})
        
        if not namespaces:
            return "No namespace data found."
        
        summary = f"""## Namespace GPU Usage Analysis

Analyzed {total} namespaces in total.
"""
        
        # === New: Show aggregate statistics if available ===
        if overall_stats and not overall_stats.get("error"):
            summary += f"""
### 📊 All Namespace Aggregate Statistics
- Overall Average Utilization: {overall_stats.get('overall_avg_utilization', 0)}%
- Overall Max Utilization: {overall_stats.get('overall_max_utilization', 0)}%
- Overall Min Utilization: {overall_stats.get('overall_min_utilization', 0)}%
- Utilization Standard Deviation: {overall_stats.get('std_deviation', 0)}%
"""
            if 'weighted_avg_utilization' in overall_stats:
                summary += f"- Weighted Average Utilization: {overall_stats.get('weighted_avg_utilization', 0)}%\n"
                summary += f"- Total GPU Hours: {overall_stats.get('total_gpu_hours', 0)}\n"
            summary += "\n"
        # === End of new section ===
        
        summary += """### Top 5 Namespaces with Lowest Utilization:
"""
        for i, ns in enumerate(namespaces[:5]):
            summary += f"{i+1}. **{ns['namespace']}**: Average utilization {ns['avg_utilization']}%, average allocation {ns['avg_gpu_count']} GPUs\n"
        
        return summary
    
    def _generate_user_analysis_summary(self, analysis: Dict[str, Any]) -> str:
        """Generate user analysis summary"""
        if "error" in analysis:
            return f"Analysis failed: {analysis['error']}"
        
        # If analyzing a single user
        if "user_name" in analysis:
            user = analysis["user_name"]
            stats = analysis.get("statistics", {})
            return f"""## User {user} GPU Usage

### 📊 Statistics
- Average Utilization: {stats.get('avg_utilization', 0)}%
- Max Utilization: {stats.get('max_utilization', 0)}%
- Min Utilization: {stats.get('min_utilization', 0)}%
- Average GPU Allocation: {stats.get('avg_gpu_count', 0)} GPUs
- Trend: {stats.get('trend', 'unknown')}

📌 Line chart generated, please check visualization results.
"""
        
        # If analyzing all users
        users = analysis.get("users", [])
        total = analysis.get("total_count", 0)
        
        if not users:
            return "No user data found."
        
        summary = f"""## User GPU Allocation Analysis

Analyzed {total} users in total.

### 🔍 Users with High GPU Allocation but Low Utilization (sorted by issue score):

| Username | Avg GPU Allocation | Avg Utilization | Max Utilization | Issue Score |
|----------|-------------------|-----------------|-----------------|-------------|
"""
        for user in users[:10]:
            summary += f"| {user['dimension_value']} | {user['avg_gpu_count']} | {user['avg_utilization']}% | {user['max_utilization']}% | {user['issue_score']} |\n"
        
        summary += "\n💡 **Recommendation**: Users with high issue scores should optimize GPU usage efficiency or reduce allocation.\n\n📊 See detailed data in table below."
        
        return summary
    
    def _generate_low_utilization_summary(self, analysis: Dict[str, Any]) -> str:
        """Generate low utilization resource analysis summary"""
        if "error" in analysis:
            return f"Analysis failed: {analysis['error']}"
        
        low_util_annos = analysis.get("low_utilization_annotations", [])
        total = analysis.get("total_count", 0)
        
        if not low_util_annos:
            return "✅ No obvious low utilization resource issues found."
        
        summary = f"""## Low Utilization Resource Analysis

Found {total} resources with high GPU allocation but low utilization.

### 🔴 Top 10 Most Critical Issues:

"""
        for i, anno in enumerate(low_util_annos[:10]):
            summary += f"{i+1}. **{anno['annotation_key']}={anno['annotation_value']}**\n"
            summary += f"   - Average GPU Allocation: {anno['avg_gpu_count']} GPUs\n"
            summary += f"   - Average Utilization: {anno['avg_utilization']}%\n"
            summary += f"   - Issue Score: {anno['issue_score']}\n\n"
        
        summary += "💡 **Recommendation**: Contact relevant resource owners to optimize GPU usage efficiency."
        
        return summary
    
    def _generate_full_analysis_summary(
        self,
        cluster_analysis: Dict[str, Any],
        namespace_analysis: Dict[str, Any],
        user_analysis: Dict[str, Any]
    ) -> str:
        """Generate complete analysis summary"""
        summary = "# GPU Usage Complete Analysis Report\n\n"
        
        # Cluster-level summary
        summary += "## 1. Cluster Overview\n\n"
        if "error" not in cluster_analysis:
            stats = cluster_analysis.get("statistics", {})
            util_stats = stats.get("utilization", {})
            alloc_stats = stats.get("allocation_rate", {})
            summary += f"- Average Utilization: {util_stats.get('average', 0)}%\n"
            summary += f"- Average Allocation Rate: {alloc_stats.get('average', 0)}%\n"
            summary += f"- Trend: {util_stats.get('trend', 'unknown')}\n\n"
            
            # === New: Add key information from LLM deep analysis ===
            llm_analysis = cluster_analysis.get("llm_analysis", {})
            if llm_analysis and "error" not in llm_analysis:
                efficiency = llm_analysis.get("efficiency_evaluation", {})
                problem_severity = llm_analysis.get("problem_severity", {})
                
                if efficiency:
                    overall_score = efficiency.get("overall_score", 0)
                    efficiency_level = efficiency.get("efficiency_level", "Unknown")
                    score_emoji = "🟢" if overall_score >= 70 else "🟡" if overall_score >= 50 else "🔴"
                    summary += f"{score_emoji} **AI Assessment**: {efficiency_level} ({overall_score}/100)\n\n"
                
                if problem_severity:
                    needs_action = problem_severity.get("needs_immediate_action", False)
                    critical_issues = problem_severity.get("critical_issues", [])
                    
                    if needs_action and critical_issues:
                        summary += "**⚠️ Critical Issues**:\n"
                        for issue in critical_issues[:2]:  # Show at most 2
                            summary += f"- {issue}\n"
                        summary += "\n"
                
                llm_summary = llm_analysis.get("summary", "")
                if llm_summary:
                    summary += f"**AI Summary**: {llm_summary}\n\n"
            # === End of new section ===
            
            summary += "📊 Cluster trend line chart generated.\n\n"
        else:
            summary += f"Cluster analysis failed: {cluster_analysis['error']}\n\n"
        
        # Namespace-level summary
        summary += "## 2. Namespace Analysis\n\n"
        if "error" not in namespace_analysis:
            namespaces = namespace_analysis.get("namespaces", [])
            total_ns = namespace_analysis.get("total_count", 0)
            summary += f"Analyzed {total_ns} namespaces in total.\n\n"
            if namespaces:
                summary += "Top 3 namespaces with lowest utilization:\n"
                for i, ns in enumerate(namespaces[:3]):
                    summary += f"{i+1}. {ns['namespace']}: {ns['avg_utilization']}% (allocates {ns['avg_gpu_count']} GPUs)\n"
        else:
            summary += f"Namespace analysis failed: {namespace_analysis['error']}\n\n"
        
        # User-level summary
        summary += "\n## 3. User Allocation Analysis\n\n"
        if "error" not in user_analysis:
            users = user_analysis.get("users", [])
            total_users = user_analysis.get("total_count", 0)
            summary += f"Analyzed {total_users} users in total.\n\n"
            if users:
                summary += "Top 5 users with high allocation but low utilization:\n\n"
                summary += "| Username | Avg GPU Allocation | Avg Utilization | Issue Score |\n"
                summary += "|----------|-------------------|-----------------|-------------|\n"
                for user in users[:5]:
                    summary += f"| {user['dimension_value']} | {user['avg_gpu_count']} | {user['avg_utilization']}% | {user['issue_score']} |\n"
                summary += "\n📊 See detailed user data in table.\n"
        else:
            summary += f"User analysis failed: {user_analysis['error']}\n\n"
        
        summary += "\n---\n\n💡 **Overall Recommendation**: Focus on users and namespaces with low utilization but high allocation to optimize resource efficiency."
        
        return summary