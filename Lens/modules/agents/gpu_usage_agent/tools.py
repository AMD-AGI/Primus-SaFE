"""GPU Usage Analysis Tools."""

import json
import logging
from typing import Dict, Any, List, Optional
from datetime import datetime, timedelta
import requests
from langchain_core.tools import StructuredTool

logger = logging.getLogger(__name__)


class GPUAnalysisTools:
    """GPU 使用率分析工具集"""
    
    def __init__(self, api_base_url: str, cluster_name: Optional[str] = None):
        """
        初始化工具集
        
        Args:
            api_base_url: API 基础 URL（如 http://localhost:8080）
            cluster_name: 集群名称（可选）
        """
        self.api_base_url = api_base_url.rstrip('/')
        self.cluster_name = cluster_name
        
        # 初始化时测试 API 连接
        self._test_api_connection()
    
    def _test_api_connection(self):
        """测试 API 连接是否正常"""
        try:
            # 尝试访问一个简单的端点
            url = f"{self.api_base_url}/api/gpu-aggregation/clusters"
            response = requests.get(url, timeout=5)
            logger.info(f"API 连接测试: {url}, 状态码: {response.status_code}")
            if response.status_code == 200:
                logger.info("API 连接正常")
            else:
                logger.warning(f"API 返回非 200 状态码: {response.status_code}")
        except Exception as e:
            logger.warning(f"API 连接测试失败: {str(e)}。请确保 Lens API 服务正在运行在 {self.api_base_url}")
    
    def _make_request(self, endpoint: str, params: Dict[str, Any]) -> Dict[str, Any]:
        """发起 API 请求"""
        url = f"{self.api_base_url}{endpoint}"
        # 添加集群名称（如果指定）
        if self.cluster_name:
            params['cluster'] = self.cluster_name
        
        try:
            logger.info(f"发起 API 请求: {url}, 参数: {params}")
            response = requests.get(url, params=params, timeout=30)
            logger.info(f"API 响应状态码: {response.status_code}")
            
            response.raise_for_status()
            
            # 尝试解析 JSON
            try:
                result = response.json()
                logger.info(f"API 响应成功，返回数据类型: {type(result)}")
                return result
            except json.JSONDecodeError as e:
                # JSON 解析失败，记录原始响应内容
                logger.error(f"JSON 解析失败: {str(e)}")
                logger.error(f"响应状态码: {response.status_code}")
                logger.error(f"响应头: {dict(response.headers)}")
                logger.error(f"响应内容（前500字符）: {response.text[:500]}")
                return {
                    "meta": {
                        "code": -1,
                        "message": f"API 返回了非 JSON 响应: {str(e)}。响应内容: {response.text[:200]}"
                    },
                    "data": None
                }
        except requests.RequestException as e:
            logger.error(f"API 请求失败: {str(e)}")
            return {
                "meta": {
                    "code": -1,
                    "message": f"API request failed: {str(e)}"
                },
                "data": None
            }
    
    def query_gpu_usage_trend(
        self,
        dimension: str,
        granularity: str,
        time_range_days: int,
        dimension_value: Optional[str] = None,
        metric_type: str = "utilization"
    ) -> str:
        """
        查询 GPU 使用率趋势数据
        
        Args:
            dimension: 查询维度（cluster/namespace/label/annotation）
            granularity: 时间粒度（hour/day），对应 API 的 hourly-stats
            time_range_days: 时间范围（天数）
            dimension_value: 维度的具体值
                - namespace: namespace 名称
                - label: "key:value" 格式，如 "team:ml-team"
                - annotation: "key:value" 格式，如 "primus-safe.user.name:zhangsan"
            metric_type: 指标类型（utilization/allocation_rate）
        
        Returns:
            JSON 字符串，包含趋势数据和统计信息
        """
        # 计算时间范围
        end_time = datetime.utcnow()
        start_time = end_time - timedelta(days=time_range_days)
        
        # 根据维度调用不同的 API
        if dimension == "cluster":
            endpoint = "/v1/gpu-aggregation/cluster/hourly-stats"
            params = {
                "start_time": start_time.strftime("%Y-%m-%dT%H:%M:%SZ"),
                "end_time": end_time.strftime("%Y-%m-%dT%H:%M:%SZ")
            }
        elif dimension == "namespace":
            endpoint = "/v1/gpu-aggregation/namespaces/hourly-stats"
            params = {
                "start_time": start_time.strftime("%Y-%m-%dT%H:%M:%SZ"),
                "end_time": end_time.strftime("%Y-%m-%dT%H:%M:%SZ")
            }
            if dimension_value:
                params["namespace"] = dimension_value
        elif dimension in ["label", "annotation"]:
            endpoint = "/v1/gpu-aggregation/labels/hourly-stats"
            # dimension_value 应该是 "key:value" 格式
            if dimension_value and ":" in dimension_value:
                key, value = dimension_value.split(":", 1)
                params = {
                    "dimension_type": dimension,  # 使用传入的 dimension（label 或 annotation）
                    "dimension_key": key,
                    "dimension_value": value,
                    "start_time": start_time.strftime("%Y-%m-%dT%H:%M:%SZ"),
                    "end_time": end_time.strftime("%Y-%m-%dT%H:%M:%SZ")
                }
            else:
                return json.dumps({
                    "error": f"{dimension} dimension_value must be in 'key:value' format, e.g., 'team:ml-team' or 'primus-safe.user.name:zhangsan'"
                })
        else:
            return json.dumps({"error": f"Unsupported dimension: {dimension}. Supported dimensions: cluster, namespace, label, annotation"})
        
        result = self._make_request(endpoint, params)
        
        # 检查响应状态（成功状态码为 2000）
        meta = result.get("meta", {})
        if meta.get("code") != 2000:
            return json.dumps({
                "error": meta.get("message", "Unknown error"),
                "data": None
            })
        
        # 处理数据，计算趋势统计
        data_points = result.get("data", [])
        if not data_points:
            return json.dumps({
                "data_points": [],
                "statistics": {
                    "average": 0,
                    "max": 0,
                    "min": 0,
                    "trend": "no_data"
                }
            })
        
        # 提取指标值
        if metric_type == "utilization":
            values = [dp.get("avg_utilization", 0) for dp in data_points]
        else:  # allocation_rate
            values = [dp.get("allocation_rate", 0) for dp in data_points]
        
        # 计算统计信息
        avg = sum(values) / len(values) if values else 0
        max_val = max(values) if values else 0
        min_val = min(values) if values else 0
        
        # 简单趋势判断（比较前后半段平均值）
        if len(values) >= 2:
            mid = len(values) // 2
            first_half_avg = sum(values[:mid]) / mid
            second_half_avg = sum(values[mid:]) / (len(values) - mid)
            if second_half_avg > first_half_avg * 1.1:
                trend = "increasing"
            elif second_half_avg < first_half_avg * 0.9:
                trend = "decreasing"
            else:
                trend = "stable"
        else:
            trend = "insufficient_data"
        
        return json.dumps({
            "data_points": data_points,
            "statistics": {
                "average": avg,
                "max": max_val,
                "min": min_val,
                "trend": trend,
                "sample_count": len(data_points)
            }
        }, ensure_ascii=False)
    
    def analyze_workload_history(
        self,
        time_range_days: int,
        namespace: Optional[str] = None,
        kind: Optional[str] = None,
        status: Optional[str] = None,
        sort_by: str = "start_at",
        limit: int = 20
    ) -> str:
        """
        分析 workload 历史记录
        
        Args:
            time_range_days: 时间范围（天数）
            namespace: 筛选的 namespace（可选）
            kind: workload 类型（可选）
            status: workload 状态（可选）
            sort_by: 排序字段（start_at/end_at）
            limit: 返回数量
        
        Returns:
            JSON 字符串，包含 workload 列表和聚合统计
        """
        endpoint = "/v1/workloads"
        params = {
            "pageNum": 1,
            "pageSize": limit,
            "orderBy": sort_by
        }
        
        if namespace:
            params["namespace"] = namespace
        if kind:
            params["kind"] = kind
        if status:
            params["status"] = status
        
        result = self._make_request(endpoint, params)
        
        # 检查响应状态（成功状态码为 2000）
        meta = result.get("meta", {})
        if meta.get("code") != 2000:
            return json.dumps({
                "error": meta.get("message", "Unknown error"),
                "workloads": []
            })
        
        data = result.get("data", {})
        workloads = data.get("data", [])
        total = data.get("total", 0)
        
        # 计算聚合统计
        total_gpu_allocated = sum(w.get("gpuAllocated", 0) for w in workloads)
        namespaces = set(w.get("namespace") for w in workloads if w.get("namespace"))
        
        return json.dumps({
            "workloads": workloads,
            "total_count": total,
            "aggregated_stats": {
                "total_gpu_allocated": total_gpu_allocated,
                "unique_namespaces": len(namespaces),
                "workload_count": len(workloads)
            }
        }, ensure_ascii=False)
    
    def get_latest_snapshot(self) -> str:
        """
        获取最新的 GPU 分配快照（实时状态）
        
        Returns:
            JSON 字符串，包含当前 GPU 分配情况
        """
        endpoint = "/v1/gpu-aggregation/snapshots/latest"
        params = {}
        
        result = self._make_request(endpoint, params)
        
        # 检查响应状态（成功状态码为 2000）
        meta = result.get("meta", {})
        if meta.get("code") != 2000:
            return json.dumps({
                "error": meta.get("message", "Unknown error"),
                "data": None
            })
        
        return json.dumps(result.get("data", {}), ensure_ascii=False)
    
    def get_workload_metadata(self) -> str:
        """
        获取 workload 元数据（所有 namespaces 和 kinds）
        
        Returns:
            JSON 字符串，包含可用的 namespaces 和 kinds
        """
        endpoint = "/v1/workloads/metadata"
        params = {}
        
        result = self._make_request(endpoint, params)
        
        # 检查响应状态（成功状态码为 2000）
        meta = result.get("meta", {})
        if meta.get("code") != 2000:
            return json.dumps({
                "error": meta.get("message", "Unknown error"),
                "namespaces": [],
                "kinds": []
            })
        
        data = result.get("data", {})
        return json.dumps({
            "namespaces": data.get("namespaces", []),
            "kinds": data.get("kinds", [])
        }, ensure_ascii=False)
    
    def get_available_clusters(self) -> str:
        """
        获取所有可用的集群列表
        
        Returns:
            JSON 字符串，包含集群名称列表
        """
        endpoint = "/v1/gpu-aggregation/clusters"
        params = {}
        
        result = self._make_request(endpoint, params)
        
        # 检查响应状态（成功状态码为 2000）
        meta = result.get("meta", {})
        if meta.get("code") != 2000:
            return json.dumps({
                "error": meta.get("message", "Unknown error"),
                "clusters": []
            })
        
        clusters = result.get("data", [])
        return json.dumps({
            "clusters": clusters,
            "count": len(clusters)
        }, ensure_ascii=False)
    
    def get_available_namespaces(
        self,
        time_range_days: int = 7,
        cluster: Optional[str] = None
    ) -> str:
        """
        获取指定时间范围内有 GPU 分配数据的 namespaces
        
        Args:
            time_range_days: 时间范围（天数），默认 7 天
            cluster: 集群名称（可选）
        
        Returns:
            JSON 字符串，包含 namespace 列表
        """
        # 计算时间范围
        end_time = datetime.utcnow()
        start_time = end_time - timedelta(days=time_range_days)
        
        endpoint = "/v1/gpu-aggregation/namespaces"
        params = {
            "start_time": start_time.strftime("%Y-%m-%dT%H:%M:%SZ"),
            "end_time": end_time.strftime("%Y-%m-%dT%H:%M:%SZ")
        }
        
        if cluster:
            params["cluster"] = cluster
        
        result = self._make_request(endpoint, params)
        
        # 检查响应状态（成功状态码为 2000）
        meta = result.get("meta", {})
        if meta.get("code") != 2000:
            return json.dumps({
                "error": meta.get("message", "Unknown error"),
                "namespaces": []
            })
        
        namespaces = result.get("data", [])
        return json.dumps({
            "namespaces": namespaces,
            "count": len(namespaces),
            "time_range_days": time_range_days
        }, ensure_ascii=False)
    
    def get_available_dimension_keys(
        self,
        dimension_type: str,
        time_range_days: int = 7,
        cluster: Optional[str] = None
    ) -> str:
        """
        获取指定时间范围内可用的 label 或 annotation keys
        
        Args:
            dimension_type: 维度类型（label 或 annotation）
            time_range_days: 时间范围（天数），默认 7 天
            cluster: 集群名称（可选）
        
        Returns:
            JSON 字符串，包含 dimension keys 列表
        """
        if dimension_type not in ["label", "annotation"]:
            return json.dumps({
                "error": f"Invalid dimension_type: {dimension_type}. Must be 'label' or 'annotation'",
                "dimension_keys": []
            })
        
        # 计算时间范围
        end_time = datetime.utcnow()
        start_time = end_time - timedelta(days=time_range_days)
        
        endpoint = "/v1/gpu-aggregation/dimension-keys"
        params = {
            "dimension_type": dimension_type,
            "start_time": start_time.strftime("%Y-%m-%dT%H:%M:%SZ"),
            "end_time": end_time.strftime("%Y-%m-%dT%H:%M:%SZ")
        }
        
        if cluster:
            params["cluster"] = cluster
        
        result = self._make_request(endpoint, params)
        
        # 检查响应状态（成功状态码为 2000）
        meta = result.get("meta", {})
        if meta.get("code") != 2000:
            return json.dumps({
                "error": meta.get("message", "Unknown error"),
                "dimension_keys": []
            })
        
        dimension_keys = result.get("data", [])
        return json.dumps({
            "dimension_type": dimension_type,
            "dimension_keys": dimension_keys,
            "count": len(dimension_keys),
            "time_range_days": time_range_days
        }, ensure_ascii=False)
    
    def get_available_dimension_values(
        self,
        dimension_type: str,
        dimension_key: str,
        time_range_days: int = 7,
        cluster: Optional[str] = None
    ) -> str:
        """
        获取指定时间范围内某个 label 或 annotation key 的所有可能值
        
        Args:
            dimension_type: 维度类型（label 或 annotation）
            dimension_key: 维度key（如 "team", "primus-safe.user.name"）
            time_range_days: 时间范围（天数），默认 7 天
            cluster: 集群名称（可选）
        
        Returns:
            JSON 字符串，包含 dimension values 列表
        """
        if dimension_type not in ["label", "annotation"]:
            return json.dumps({
                "error": f"Invalid dimension_type: {dimension_type}. Must be 'label' or 'annotation'",
                "dimension_values": []
            })
        
        # 计算时间范围
        end_time = datetime.utcnow()
        start_time = end_time - timedelta(days=time_range_days)
        
        endpoint = "/v1/gpu-aggregation/dimension-values"
        params = {
            "dimension_type": dimension_type,
            "dimension_key": dimension_key,
            "start_time": start_time.strftime("%Y-%m-%dT%H:%M:%SZ"),
            "end_time": end_time.strftime("%Y-%m-%dT%H:%M:%SZ")
        }
        
        if cluster:
            params["cluster"] = cluster
        
        result = self._make_request(endpoint, params)
        
        # 检查响应状态（成功状态码为 2000）
        meta = result.get("meta", {})
        if meta.get("code") != 2000:
            return json.dumps({
                "error": meta.get("message", "Unknown error"),
                "dimension_values": []
            })
        
        dimension_values = result.get("data", [])
        return json.dumps({
            "dimension_type": dimension_type,
            "dimension_key": dimension_key,
            "dimension_values": dimension_values,
            "count": len(dimension_values),
            "time_range_days": time_range_days
        }, ensure_ascii=False)
    
    def analyze_user_gpu_usage(
        self,
        time_range_days: int = 7,
        top_n: int = 20,
        cluster: Optional[str] = None
    ) -> str:
        """
        分析每个用户的GPU占用和使用率情况（基于annotation key "primus-safe.user.name"）
        找出占用GPU多但使用率低的用户
        
        Args:
            time_range_days: 时间范围（天数），默认 7 天
            top_n: 返回前N个用户，默认 20
            cluster: 集群名称（可选）
        
        Returns:
            JSON 字符串，包含用户列表及其GPU占用和使用率信息
        """
        return self.find_low_utilization_dimension_values(
            dimension_type="annotation",
            dimension_key="primus-safe.user.name",
            time_range_days=time_range_days,
            top_n=top_n,
            cluster=cluster
        )
    
    def find_low_utilization_dimension_values(
        self,
        dimension_type: str,
        dimension_key: str,
        time_range_days: int = 7,
        top_n: int = 20,
        cluster: Optional[str] = None
    ) -> str:
        """
        对于指定的dimension key，找出其values中占用GPU最多但利用率最低的top N
        
        Args:
            dimension_type: 维度类型（label 或 annotation）
            dimension_key: 维度key（如 "primus-safe.user.name"）
            time_range_days: 时间范围（天数），默认 7 天
            top_n: 返回前N个，默认 20
            cluster: 集群名称（可选）
        
        Returns:
            JSON 字符串，包含排序后的 dimension values 及其使用情况
        """
        if dimension_type not in ["label", "annotation"]:
            return json.dumps({
                "error": f"Invalid dimension_type: {dimension_type}. Must be 'label' or 'annotation'",
                "results": []
            })
        
        try:
            # 1. 获取该key的所有values
            values_result = self.get_available_dimension_values(
                dimension_type=dimension_type,
                dimension_key=dimension_key,
                time_range_days=time_range_days,
                cluster=cluster
            )
            
            values_data = json.loads(values_result)
            if "error" in values_data:
                return json.dumps({
                    "error": values_data["error"],
                    "results": []
                })
            
            dimension_values = values_data.get("dimension_values", [])
            if not dimension_values:
                return json.dumps({
                    "dimension_type": dimension_type,
                    "dimension_key": dimension_key,
                    "results": [],
                    "message": "No values found for this dimension key"
                })
            
            # 2. 查询每个value的使用情况
            results = []
            for value in dimension_values:
                try:
                    dimension_value_str = f"{dimension_key}:{value}"
                    trend_result = self.query_gpu_usage_trend(
                        dimension=dimension_type,
                        dimension_value=dimension_value_str,
                        granularity="hour",
                        time_range_days=time_range_days,
                        metric_type="utilization"
                    )
                    
                    trend_data = json.loads(trend_result)
                    if "error" in trend_data or "statistics" not in trend_data:
                        continue
                    
                    stats = trend_data["statistics"]
                    data_points = trend_data.get("data_points", [])
                    
                    # 计算平均GPU数量
                    avg_gpu_count = 0
                    if data_points:
                        total_gpu = sum(dp.get("allocated_gpu_count", 0) for dp in data_points)
                        avg_gpu_count = total_gpu / len(data_points)
                    
                    avg_utilization = stats.get("average", 0)
                    
                    # 计算问题评分：GPU数量越多、利用率越低，分数越高
                    # 公式: GPU数量 × (1 - 利用率) × 100
                    issue_score = 0
                    if avg_gpu_count > 0:
                        issue_score = avg_gpu_count * (1 - avg_utilization) * 100
                    
                    results.append({
                        "dimension_value": value,
                        "avg_utilization": round(avg_utilization * 100, 2),  # 转换为百分比
                        "avg_gpu_count": round(avg_gpu_count, 2),
                        "max_utilization": round(stats.get("max", 0) * 100, 2),
                        "min_utilization": round(stats.get("min", 0) * 100, 2),
                        "trend": stats.get("trend", "unknown"),
                        "issue_score": round(issue_score, 2),
                        "sample_count": stats.get("sample_count", 0)
                    })
                    
                except Exception as e:
                    logger.error(f"Failed to query dimension value {dimension_key}:{value}: {str(e)}")
                    continue
            
            # 3. 按问题评分排序（分数越高越严重）
            results.sort(key=lambda x: x["issue_score"], reverse=True)
            
            # 4. 返回top N
            top_results = results[:top_n]
            
            return json.dumps({
                "dimension_type": dimension_type,
                "dimension_key": dimension_key,
                "results": top_results,
                "total_count": len(results),
                "top_n": top_n,
                "time_range_days": time_range_days,
                "summary": f"Found {len(results)} values, returning top {len(top_results)}"
            }, ensure_ascii=False)
            
        except Exception as e:
            logger.error(f"Failed to find low utilization dimension values: {str(e)}")
            return json.dumps({
                "error": str(e),
                "results": []
            })
    
    def get_tools(self) -> List:
        """返回所有工具的列表"""
        return [
            StructuredTool.from_function(
                func=self.query_gpu_usage_trend,
                name="query_gpu_usage_trend",
                description=self.query_gpu_usage_trend.__doc__
            ),
            StructuredTool.from_function(
                func=self.analyze_workload_history,
                name="analyze_workload_history",
                description=self.analyze_workload_history.__doc__
            ),
            StructuredTool.from_function(
                func=self.get_latest_snapshot,
                name="get_latest_snapshot",
                description=self.get_latest_snapshot.__doc__
            ),
            StructuredTool.from_function(
                func=self.get_workload_metadata,
                name="get_workload_metadata",
                description=self.get_workload_metadata.__doc__
            ),
            StructuredTool.from_function(
                func=self.get_available_clusters,
                name="get_available_clusters",
                description=self.get_available_clusters.__doc__
            ),
            StructuredTool.from_function(
                func=self.get_available_namespaces,
                name="get_available_namespaces",
                description=self.get_available_namespaces.__doc__
            ),
            StructuredTool.from_function(
                func=self.get_available_dimension_keys,
                name="get_available_dimension_keys",
                description=self.get_available_dimension_keys.__doc__
            ),
            StructuredTool.from_function(
                func=self.get_available_dimension_values,
                name="get_available_dimension_values",
                description=self.get_available_dimension_values.__doc__
            )
        ]

