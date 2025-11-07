"""Data Access Layer for GPU Analysis."""

from typing import Dict, Any, List, Optional
from datetime import datetime, timedelta
import requests


class GPUDataAccess:
    """GPU 数据访问层，封装对 Lens API 的调用"""
    
    def __init__(self, api_base_url: str, cluster_name: Optional[str] = None):
        """
        初始化数据访问层
        
        Args:
            api_base_url: API 基础 URL
            cluster_name: 集群名称（可选）
        """
        self.api_base_url = api_base_url.rstrip('/')
        self.cluster_name = cluster_name
    
    def _make_request(
        self,
        endpoint: str,
        params: Optional[Dict[str, Any]] = None,
        method: str = "GET"
    ) -> Dict[str, Any]:
        """
        发起 API 请求
        
        Args:
            endpoint: API 端点
            params: 请求参数
            method: HTTP 方法
        
        Returns:
            API 响应数据
        """
        url = f"{self.api_base_url}{endpoint}"
        
        # 添加集群名称
        if self.cluster_name:
            params = params or {}
            params['cluster'] = self.cluster_name
        
        try:
            if method.upper() == "GET":
                response = requests.get(url, params=params, timeout=30)
            elif method.upper() == "POST":
                response = requests.post(url, json=params, timeout=30)
            else:
                raise ValueError(f"Unsupported HTTP method: {method}")
            
            response.raise_for_status()
            return response.json()
        
        except requests.RequestException as e:
            return {
                "code": -1,
                "message": f"API request failed: {str(e)}",
                "data": None
            }
    
    def get_cluster_hourly_stats(
        self,
        start_time: datetime,
        end_time: datetime
    ) -> Dict[str, Any]:
        """
        获取集群小时级统计数据
        
        Args:
            start_time: 开始时间
            end_time: 结束时间
        
        Returns:
            统计数据
        """
        endpoint = "/api/gpu-aggregation/cluster/hourly-stats"
        params = {
            "start_time": start_time.strftime("%Y-%m-%dT%H:%M:%SZ"),
            "end_time": end_time.strftime("%Y-%m-%dT%H:%M:%SZ")
        }
        return self._make_request(endpoint, params)
    
    def get_namespace_hourly_stats(
        self,
        start_time: datetime,
        end_time: datetime,
        namespace: Optional[str] = None
    ) -> Dict[str, Any]:
        """
        获取 namespace 小时级统计数据
        
        Args:
            start_time: 开始时间
            end_time: 结束时间
            namespace: namespace 名称（可选）
        
        Returns:
            统计数据
        """
        endpoint = "/api/gpu-aggregation/namespaces/hourly-stats"
        params = {
            "start_time": start_time.strftime("%Y-%m-%dT%H:%M:%SZ"),
            "end_time": end_time.strftime("%Y-%m-%dT%H:%M:%SZ")
        }
        if namespace:
            params["namespace"] = namespace
        
        return self._make_request(endpoint, params)
    
    def get_label_hourly_stats(
        self,
        dimension_key: str,
        start_time: datetime,
        end_time: datetime,
        dimension_value: Optional[str] = None,
        dimension_type: str = "label"
    ) -> Dict[str, Any]:
        """
        获取 label/annotation 小时级统计数据
        
        Args:
            dimension_key: label/annotation key
            start_time: 开始时间
            end_time: 结束时间
            dimension_value: label/annotation value（可选）
            dimension_type: 维度类型（label/annotation）
        
        Returns:
            统计数据
        """
        endpoint = "/api/gpu-aggregation/labels/hourly-stats"
        params = {
            "dimension_type": dimension_type,
            "dimension_key": dimension_key,
            "start_time": start_time.strftime("%Y-%m-%dT%H:%M:%SZ"),
            "end_time": end_time.strftime("%Y-%m-%dT%H:%M:%SZ")
        }
        if dimension_value:
            params["dimension_value"] = dimension_value
        
        return self._make_request(endpoint, params)
    
    def get_latest_snapshot(self) -> Dict[str, Any]:
        """
        获取最新的 GPU 分配快照
        
        Returns:
            快照数据
        """
        endpoint = "/api/gpu-aggregation/snapshots/latest"
        return self._make_request(endpoint)
    
    def list_snapshots(
        self,
        start_time: datetime,
        end_time: datetime
    ) -> Dict[str, Any]:
        """
        列出时间范围内的快照
        
        Args:
            start_time: 开始时间
            end_time: 结束时间
        
        Returns:
            快照列表
        """
        endpoint = "/api/gpu-aggregation/snapshots"
        params = {
            "start_time": start_time.strftime("%Y-%m-%dT%H:%M:%SZ"),
            "end_time": end_time.strftime("%Y-%m-%dT%H:%M:%SZ")
        }
        return self._make_request(endpoint, params)
    
    def list_workloads(
        self,
        page_num: int = 1,
        page_size: int = 20,
        namespace: Optional[str] = None,
        kind: Optional[str] = None,
        status: Optional[str] = None,
        order_by: Optional[str] = None
    ) -> Dict[str, Any]:
        """
        列出 workloads
        
        Args:
            page_num: 页码
            page_size: 每页大小
            namespace: namespace 筛选
            kind: workload 类型筛选
            status: 状态筛选
            order_by: 排序字段
        
        Returns:
            workload 列表
        """
        endpoint = "/api/workloads"
        params = {
            "pageNum": page_num,
            "pageSize": page_size
        }
        if namespace:
            params["namespace"] = namespace
        if kind:
            params["kind"] = kind
        if status:
            params["status"] = status
        if order_by:
            params["orderBy"] = order_by
        
        return self._make_request(endpoint, params)
    
    def get_workload_info(self, uid: str) -> Dict[str, Any]:
        """
        获取 workload 详细信息
        
        Args:
            uid: workload UID
        
        Returns:
            workload 信息
        """
        endpoint = f"/api/workloads/{uid}"
        return self._make_request(endpoint)
    
    def get_workload_metrics(
        self,
        uid: str,
        start: int,
        end: int,
        step: int = 60
    ) -> Dict[str, Any]:
        """
        获取 workload 指标
        
        Args:
            uid: workload UID
            start: 开始时间戳（秒）
            end: 结束时间戳（秒）
            step: 步长（秒）
        
        Returns:
            指标数据
        """
        endpoint = f"/api/workloads/{uid}/metrics"
        params = {
            "start": start,
            "end": end,
            "step": step
        }
        return self._make_request(endpoint, params)
    
    def get_workloads_metadata(self) -> Dict[str, Any]:
        """
        获取 workload 元数据（namespaces 和 kinds）
        
        Returns:
            元数据
        """
        endpoint = "/api/workloads/metadata"
        return self._make_request(endpoint)
    
    def get_workload_hierarchy(self, uid: str) -> Dict[str, Any]:
        """
        获取 workload 层级关系
        
        Args:
            uid: workload UID
        
        Returns:
            层级关系树
        """
        endpoint = f"/api/workloads/{uid}/hierarchy"
        return self._make_request(endpoint)
    
    def get_clusters(self) -> Dict[str, Any]:
        """
        获取所有可用的集群列表
        
        Returns:
            集群列表
        """
        endpoint = "/api/gpu-aggregation/clusters"
        return self._make_request(endpoint)
    
    def get_namespaces(
        self,
        start_time: datetime,
        end_time: datetime
    ) -> Dict[str, Any]:
        """
        获取指定时间范围内的 namespaces
        
        Args:
            start_time: 开始时间
            end_time: 结束时间
        
        Returns:
            namespace 列表
        """
        endpoint = "/api/gpu-aggregation/namespaces"
        params = {
            "start_time": start_time.strftime("%Y-%m-%dT%H:%M:%SZ"),
            "end_time": end_time.strftime("%Y-%m-%dT%H:%M:%SZ")
        }
        return self._make_request(endpoint, params)
    
    def get_dimension_keys(
        self,
        dimension_type: str,
        start_time: datetime,
        end_time: datetime
    ) -> Dict[str, Any]:
        """
        获取指定时间范围内的 dimension keys
        
        Args:
            dimension_type: 维度类型（label/annotation）
            start_time: 开始时间
            end_time: 结束时间
        
        Returns:
            dimension keys 列表
        """
        endpoint = "/api/gpu-aggregation/dimension-keys"
        params = {
            "dimension_type": dimension_type,
            "start_time": start_time.strftime("%Y-%m-%dT%H:%M:%SZ"),
            "end_time": end_time.strftime("%Y-%m-%dT%H:%M:%SZ")
        }
        return self._make_request(endpoint, params)

