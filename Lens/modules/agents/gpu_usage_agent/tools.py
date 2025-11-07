"""GPU Usage Analysis Tools."""

import json
import logging
from typing import Dict, Any, List, Optional
from datetime import datetime, timedelta
import requests
from langchain_core.tools import StructuredTool

logger = logging.getLogger(__name__)


class GPUAnalysisTools:
    """GPU utilization analysis toolset"""
    
    def __init__(self, api_base_url: str, cluster_name: Optional[str] = None):
        """
        Initialize toolset
        
        Args:
            api_base_url: API base URL (e.g., http://localhost:8080)
            cluster_name: Cluster name (optional)
        """
        self.api_base_url = api_base_url.rstrip('/')
        self.cluster_name = cluster_name
        
        # Test API connection on initialization
        self._test_api_connection()
    
    def _test_api_connection(self):
        """Test if API connection is working"""
        try:
            # Try to access a simple endpoint
            url = f"{self.api_base_url}/api/gpu-aggregation/clusters"
            response = requests.get(url, timeout=5)
            logger.info(f"API connection test: {url}, status code: {response.status_code}")
            if response.status_code == 200:
                logger.info("API connection is working")
            else:
                logger.warning(f"API returned non-200 status code: {response.status_code}")
        except Exception as e:
            logger.warning(f"API connection test failed: {str(e)}. Please ensure Lens API service is running at {self.api_base_url}")
    
    def _make_request(self, endpoint: str, params: Dict[str, Any]) -> Dict[str, Any]:
        """Make API request"""
        url = f"{self.api_base_url}{endpoint}"
        # Add cluster name (if specified)
        if self.cluster_name:
            params['cluster'] = self.cluster_name
        
        try:
            logger.info(f"Making API request: {url}, params: {params}")
            response = requests.get(url, params=params, timeout=30)
            logger.info(f"API response status code: {response.status_code}")
            
            response.raise_for_status()
            
            # Try to parse JSON
            try:
                result = response.json()
                logger.info(f"API response successful, returned data type: {type(result)}")
                return result
            except json.JSONDecodeError as e:
                # JSON parsing failed, log original response content
                logger.error(f"JSON parsing failed: {str(e)}")
                logger.error(f"Response status code: {response.status_code}")
                logger.error(f"Response headers: {dict(response.headers)}")
                logger.error(f"Response content (first 500 characters): {response.text[:500]}")
                return {
                    "meta": {
                        "code": -1,
                        "message": f"API returned non-JSON response: {str(e)}. Response content: {response.text[:200]}"
                    },
                    "data": None
                }
        except requests.RequestException as e:
            logger.error(f"API request failed: {str(e)}")
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
        Query GPU utilization trend data
        
        Args:
            dimension: Query dimension (cluster/namespace/label/annotation)
            granularity: Time granularity (hour/day), corresponds to API's hourly-stats
            time_range_days: Time range (days)
            dimension_value: Specific value of the dimension
                - namespace: namespace name
                - label: "key:value" format, e.g., "team:ml-team"
                - annotation: "key:value" format, e.g., "primus-safe.user.name:zhangsan"
            metric_type: Metric type (utilization/allocation_rate)
        
        Returns:
            JSON string containing trend data and statistics
        """
        # Calculate time range
        end_time = datetime.utcnow()
        start_time = end_time - timedelta(days=time_range_days)
        
        # Call different APIs based on dimension
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
            # dimension_value should be in "key:value" format
            if dimension_value and ":" in dimension_value:
                key, value = dimension_value.split(":", 1)
                params = {
                    "dimension_type": dimension,  # Use the passed dimension (label or annotation)
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
        
        # Check response status (success status code is 2000)
        meta = result.get("meta", {})
        if meta.get("code") != 2000:
            return json.dumps({
                "error": meta.get("message", "Unknown error"),
                "data": None
            })
        
        # Process data and calculate trend statistics
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
        
        # Detect and normalize data format
        # Check if avg_utilization is in percentage format
        util_values = [dp.get("avg_utilization", 0) for dp in data_points if dp.get("avg_utilization") is not None]
        is_percentage_format = util_values and sum(1 for v in util_values if v > 1) > len(util_values) / 2
        
        if is_percentage_format:
            logger.info(f"Detected API returns percentage format (0-100), converting data_points to decimal format (0-1)")
            # Normalize all percentage fields in data_points
            for dp in data_points:
                # Convert utilization-related fields
                if "avg_utilization" in dp and dp["avg_utilization"] is not None:
                    dp["avg_utilization"] = dp["avg_utilization"] / 100.0
                if "max_utilization" in dp and dp["max_utilization"] is not None:
                    dp["max_utilization"] = dp["max_utilization"] / 100.0
                if "min_utilization" in dp and dp["min_utilization"] is not None:
                    dp["min_utilization"] = dp["min_utilization"] / 100.0
                if "p50_utilization" in dp and dp["p50_utilization"] is not None:
                    dp["p50_utilization"] = dp["p50_utilization"] / 100.0
                if "p95_utilization" in dp and dp["p95_utilization"] is not None:
                    dp["p95_utilization"] = dp["p95_utilization"] / 100.0
                # Convert allocation rate field
                if "allocation_rate" in dp and dp["allocation_rate"] is not None:
                    dp["allocation_rate"] = dp["allocation_rate"] / 100.0
        
        # Extract metric values for statistical calculation
        if metric_type == "utilization":
            values = [dp.get("avg_utilization", 0) for dp in data_points]
        else:  # allocation_rate
            values = [dp.get("allocation_rate", 0) for dp in data_points]
        
        # Calculate statistics
        avg = sum(values) / len(values) if values else 0
        max_val = max(values) if values else 0
        min_val = min(values) if values else 0
        
        # Simple trend determination (compare first half and second half averages)
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
        Analyze workload history
        
        Args:
            time_range_days: Time range (days)
            namespace: Namespace filter (optional)
            kind: Workload type (optional)
            status: Workload status (optional)
            sort_by: Sort field (start_at/end_at)
            limit: Return count
        
        Returns:
            JSON string containing workload list and aggregated statistics
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
        
        # Check response status (success status code is 2000)
        meta = result.get("meta", {})
        if meta.get("code") != 2000:
            return json.dumps({
                "error": meta.get("message", "Unknown error"),
                "workloads": []
            })
        
        data = result.get("data", {})
        workloads = data.get("data", [])
        total = data.get("total", 0)
        
        # Calculate aggregated statistics
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
        Get latest GPU allocation snapshot (real-time status)
        
        Returns:
            JSON string containing current GPU allocation status
        """
        endpoint = "/v1/gpu-aggregation/snapshots/latest"
        params = {}
        
        result = self._make_request(endpoint, params)
        
        # Check response status (success status code is 2000)
        meta = result.get("meta", {})
        if meta.get("code") != 2000:
            return json.dumps({
                "error": meta.get("message", "Unknown error"),
                "data": None
            })
        
        return json.dumps(result.get("data", {}), ensure_ascii=False)
    
    def get_workload_metadata(self) -> str:
        """
        Get workload metadata (all namespaces and kinds)
        
        Returns:
            JSON string containing available namespaces and kinds
        """
        endpoint = "/v1/workloads/metadata"
        params = {}
        
        result = self._make_request(endpoint, params)
        
        # Check response status (success status code is 2000)
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
        Get all available cluster list
        
        Returns:
            JSON string containing cluster name list
        """
        endpoint = "/v1/gpu-aggregation/clusters"
        params = {}
        
        result = self._make_request(endpoint, params)
        
        # Check response status (success status code is 2000)
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
        Get namespaces with GPU allocation data within specified time range
        
        Args:
            time_range_days: Time range (days), default 7 days
            cluster: Cluster name (optional)
        
        Returns:
            JSON string containing namespace list
        """
        # Calculate time range
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
        
        # Check response status (success status code is 2000)
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
        Get available label or annotation keys within specified time range
        
        Args:
            dimension_type: Dimension type (label or annotation)
            time_range_days: Time range (days), default 7 days
            cluster: Cluster name (optional)
        
        Returns:
            JSON string containing dimension keys list
        """
        if dimension_type not in ["label", "annotation"]:
            return json.dumps({
                "error": f"Invalid dimension_type: {dimension_type}. Must be 'label' or 'annotation'",
                "dimension_keys": []
            })
        
        # Calculate time range
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
        
        # Check response status (success status code is 2000)
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
        Get all possible values for a specific label or annotation key within specified time range
        
        Args:
            dimension_type: Dimension type (label or annotation)
            dimension_key: Dimension key (e.g., "team", "primus-safe.user.name")
            time_range_days: Time range (days), default 7 days
            cluster: Cluster name (optional)
        
        Returns:
            JSON string containing dimension values list
        """
        if dimension_type not in ["label", "annotation"]:
            return json.dumps({
                "error": f"Invalid dimension_type: {dimension_type}. Must be 'label' or 'annotation'",
                "dimension_values": []
            })
        
        # Calculate time range
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
        
        # Check response status (success status code is 2000)
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
        Analyze GPU allocation and utilization for each user (based on annotation key "primus-safe.user.name")
        Find users who allocate many GPUs but have low utilization
        
        Args:
            time_range_days: Time range (days), default 7 days
            top_n: Return top N users, default 20
            cluster: Cluster name (optional)
        
        Returns:
            JSON string containing user list and their GPU allocation and utilization info
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
        For a specified dimension key, find top N values that allocate most GPUs but have lowest utilization
        
        Args:
            dimension_type: Dimension type (label or annotation)
            dimension_key: Dimension key (e.g., "primus-safe.user.name")
            time_range_days: Time range (days), default 7 days
            top_n: Return top N, default 20
            cluster: Cluster name (optional)
        
        Returns:
            JSON string containing sorted dimension values and their usage
        """
        if dimension_type not in ["label", "annotation"]:
            return json.dumps({
                "error": f"Invalid dimension_type: {dimension_type}. Must be 'label' or 'annotation'",
                "results": []
            })
        
        try:
            # 1. Get all values for this key
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
            
            # 2. Query usage for each value
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
                    
                    # Calculate average GPU count
                    avg_gpu_count = 0
                    if data_points:
                        total_gpu = sum(dp.get("allocated_gpu_count", 0) for dp in data_points)
                        avg_gpu_count = total_gpu / len(data_points)
                    
                    avg_utilization = stats.get("average", 0)
                    
                    # Calculate issue score: more GPUs and lower utilization = higher score
                    # Formula: GPU count × (1 - utilization) × 100
                    issue_score = 0
                    if avg_gpu_count > 0:
                        issue_score = avg_gpu_count * (1 - avg_utilization) * 100
                    
                    results.append({
                        "dimension_value": value,
                        "avg_utilization": round(avg_utilization * 100, 2),  # Convert to percentage
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
            
            # 3. Sort by issue score (higher score = more severe)
            results.sort(key=lambda x: x["issue_score"], reverse=True)
            
            # 4. Return top N
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
    
    def calculate_average_utilization(
        self,
        records_json: str
    ) -> str:
        """
        Calculate average utilization and statistics for a series of utilization records
        
        Args:
            records_json: JSON string containing utilization records array
                Supported record formats include:
                - ClusterGpuHourlyStats (cluster hourly statistics)
                - NamespaceGpuHourlyStats (namespace hourly statistics)
                - LabelGpuHourlyStats (label/annotation hourly statistics)
                
                Each record should contain the following fields (at least avg_utilization):
                - avg_utilization: Average utilization (0.0-1.0)
                - max_utilization: Maximum utilization (optional)
                - min_utilization: Minimum utilization (optional)
                - allocated_gpu_count: Allocated GPU count (optional)
        
        Returns:
            JSON string containing overall statistics:
            - overall_avg_utilization: Average utilization of all records
            - overall_max_utilization: Highest utilization among all records
            - overall_min_utilization: Lowest utilization among all records
            - weighted_avg_utilization: GPU count weighted average utilization (if allocated_gpu_count available)
            - record_count: Total record count
            - total_gpu_hours: Total GPU hours (if allocated_gpu_count available)
        """
        try:
            # Parse input JSON
            records = json.loads(records_json)
            
            if not isinstance(records, list):
                return json.dumps({
                    "error": "Input must be in JSON array format",
                    "data": None
                })
            
            if not records:
                return json.dumps({
                    "error": "Record list is empty",
                    "data": None
                })
            
            # Extract utilization data
            avg_utilizations = []
            max_utilizations = []
            min_utilizations = []
            gpu_counts = []
            
            for i, record in enumerate(records):
                if not isinstance(record, dict):
                    logger.warning(f"Record {i} is not in dictionary format, skipping")
                    continue
                
                # Extract average utilization (required field)
                avg_util = record.get("avg_utilization")
                if avg_util is not None:
                    avg_utilizations.append(float(avg_util))
                else:
                    logger.warning(f"Record {i} missing avg_utilization field, skipping")
                    continue
                
                # Extract maximum utilization (optional)
                max_util = record.get("max_utilization")
                if max_util is not None:
                    max_utilizations.append(float(max_util))
                
                # Extract minimum utilization (optional)
                min_util = record.get("min_utilization")
                if min_util is not None:
                    min_utilizations.append(float(min_util))
                
                # Extract GPU count (optional)
                gpu_count = record.get("allocated_gpu_count")
                if gpu_count is not None:
                    gpu_counts.append(float(gpu_count))
            
            if not avg_utilizations:
                return json.dumps({
                    "error": "No valid avg_utilization data found",
                    "data": None
                })
            
            # Detect data format: if most values > 1, data is in percentage (0-100), need to convert to decimal (0-1)
            if sum(1 for v in avg_utilizations if v > 1) > len(avg_utilizations) / 2:
                logger.info(f"Detected percentage format (0-100), converting to decimal format (0-1)")
                avg_utilizations = [v / 100.0 for v in avg_utilizations]
                max_utilizations = [v / 100.0 for v in max_utilizations]
                min_utilizations = [v / 100.0 for v in min_utilizations]
            
            # Calculate simple average utilization
            overall_avg = sum(avg_utilizations) / len(avg_utilizations)
            
            # Calculate overall maximum and minimum utilization
            overall_max = max(max_utilizations) if max_utilizations else max(avg_utilizations)
            overall_min = min(min_utilizations) if min_utilizations else min(avg_utilizations)
            
            # Build result
            result = {
                "overall_avg_utilization": round(overall_avg * 100, 2),  # Convert to percentage
                "overall_max_utilization": round(overall_max * 100, 2),
                "overall_min_utilization": round(overall_min * 100, 2),
                "record_count": len(avg_utilizations)
            }
            
            # If GPU count data available, calculate weighted average and total GPU hours
            if gpu_counts and len(gpu_counts) == len(avg_utilizations):
                total_gpu_hours = sum(gpu_counts)
                weighted_sum = sum(
                    util * count 
                    for util, count in zip(avg_utilizations, gpu_counts)
                )
                weighted_avg = weighted_sum / total_gpu_hours if total_gpu_hours > 0 else 0
                
                result["weighted_avg_utilization"] = round(weighted_avg * 100, 2)
                result["total_gpu_hours"] = round(total_gpu_hours, 2)
            
            # Calculate standard deviation of utilization
            if len(avg_utilizations) > 1:
                mean = sum(avg_utilizations) / len(avg_utilizations)
                variance = sum((x - mean) ** 2 for x in avg_utilizations) / len(avg_utilizations)
                std_dev = variance ** 0.5
                result["std_deviation"] = round(std_dev * 100, 2)  # Convert to percentage
            
            return json.dumps(result, ensure_ascii=False)
            
        except json.JSONDecodeError as e:
            logger.error(f"JSON parsing failed: {str(e)}")
            return json.dumps({
                "error": f"JSON parsing failed: {str(e)}",
                "data": None
            })
        except Exception as e:
            logger.error(f"Failed to calculate average utilization: {str(e)}")
            return json.dumps({
                "error": f"Calculation failed: {str(e)}",
                "data": None
            })
    
    def get_tools(self) -> List:
        """Return list of all tools"""
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
            ),
            StructuredTool.from_function(
                func=self.calculate_average_utilization,
                name="calculate_average_utilization",
                description=self.calculate_average_utilization.__doc__
            )
        ]
