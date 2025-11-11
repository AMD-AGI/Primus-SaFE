"""Data Access Layer for GPU Analysis."""

from typing import Dict, Any, List, Optional
from datetime import datetime, timedelta
import requests


class GPUDataAccess:
    """GPU data access layer, encapsulates calls to Lens API"""
    
    def __init__(self, api_base_url: str, cluster_name: Optional[str] = None):
        """
        Initialize data access layer
        
        Args:
            api_base_url: API base URL
            cluster_name: Cluster name (optional)
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
        Make API request
        
        Args:
            endpoint: API endpoint
            params: Request parameters
            method: HTTP method
        
        Returns:
            API response data
        """
        url = f"{self.api_base_url}{endpoint}"
        
        # Add cluster name
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
        Get cluster hourly statistics
        
        Args:
            start_time: Start time
            end_time: End time
        
        Returns:
            Statistics data
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
        Get namespace hourly statistics
        
        Args:
            start_time: Start time
            end_time: End time
            namespace: Namespace name (optional)
        
        Returns:
            Statistics data
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
        Get label/annotation hourly statistics
        
        Args:
            dimension_key: label/annotation key
            start_time: Start time
            end_time: End time
            dimension_value: label/annotation value (optional)
            dimension_type: Dimension type (label/annotation)
        
        Returns:
            Statistics data
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
        Get latest GPU allocation snapshot
        
        Returns:
            Snapshot data
        """
        endpoint = "/api/gpu-aggregation/snapshots/latest"
        return self._make_request(endpoint)
    
    def list_snapshots(
        self,
        start_time: datetime,
        end_time: datetime
    ) -> Dict[str, Any]:
        """
        List snapshots within time range
        
        Args:
            start_time: Start time
            end_time: End time
        
        Returns:
            Snapshot list
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
        List workloads
        
        Args:
            page_num: Page number
            page_size: Page size
            namespace: Namespace filter
            kind: Workload type filter
            status: Status filter
            order_by: Sort field
        
        Returns:
            Workload list
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
        Get workload detailed information
        
        Args:
            uid: Workload UID
        
        Returns:
            Workload information
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
        Get workload metrics
        
        Args:
            uid: Workload UID
            start: Start timestamp (seconds)
            end: End timestamp (seconds)
            step: Step (seconds)
        
        Returns:
            Metrics data
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
        Get workload metadata (namespaces and kinds)
        
        Returns:
            Metadata
        """
        endpoint = "/api/workloads/metadata"
        return self._make_request(endpoint)
    
    def get_workload_hierarchy(self, uid: str) -> Dict[str, Any]:
        """
        Get workload hierarchy relationship
        
        Args:
            uid: Workload UID
        
        Returns:
            Hierarchy tree
        """
        endpoint = f"/api/workloads/{uid}/hierarchy"
        return self._make_request(endpoint)
    
    def get_clusters(self) -> Dict[str, Any]:
        """
        Get all available cluster list
        
        Returns:
            Cluster list
        """
        endpoint = "/api/gpu-aggregation/clusters"
        return self._make_request(endpoint)
    
    def get_namespaces(
        self,
        start_time: datetime,
        end_time: datetime
    ) -> Dict[str, Any]:
        """
        Get namespaces within specified time range
        
        Args:
            start_time: Start time
            end_time: End time
        
        Returns:
            Namespace list
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
        Get dimension keys within specified time range
        
        Args:
            dimension_type: Dimension type (label/annotation)
            start_time: Start time
            end_time: End time
        
        Returns:
            Dimension keys list
        """
        endpoint = "/api/gpu-aggregation/dimension-keys"
        params = {
            "dimension_type": dimension_type,
            "start_time": start_time.strftime("%Y-%m-%dT%H:%M:%SZ"),
            "end_time": end_time.strftime("%Y-%m-%dT%H:%M:%SZ")
        }
        return self._make_request(endpoint, params)
