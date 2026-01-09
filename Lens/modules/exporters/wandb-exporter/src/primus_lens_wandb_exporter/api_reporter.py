# Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.

"""
API Reporter - Asynchronous data reporting to telemetry-processor
"""
import json
import os
import sys
import time
import threading
from queue import Queue, Empty
from typing import Dict, List, Any, Optional
from urllib.request import Request, urlopen
from urllib.error import URLError, HTTPError

# Import logger module
from .logger import debug_log, error_log, warning_log


class AsyncAPIReporter:
    """Asynchronous API Reporter - Report data using background thread"""
    
    def __init__(self, api_base_url: Optional[str] = None, batch_size: int = 10, flush_interval: float = 5.0):
        """
        Initialize asynchronous reporter
        
        Args:
            api_base_url: API base URL, e.g. http://telemetry-processor:8080/api/v1
            batch_size: Batch size for reporting
            flush_interval: Flush interval (seconds)
        """
        self.api_base_url = api_base_url or os.environ.get(
            "PRIMUS_LENS_API_BASE_URL", 
            "http://primus-lens-telemetry-processor:8080/api/v1"
        )
        self.batch_size = batch_size
        self.flush_interval = flush_interval
        
        # Data queues
        self.detection_queue = Queue(maxsize=100)
        self.metrics_queue = Queue(maxsize=1000)
        self.logs_queue = Queue(maxsize=1000)
        
        # Background thread
        self.worker_thread = None
        self.running = False
        self.lock = threading.Lock()
        
        # Statistics
        self.stats = {
            "detection_sent": 0,
            "metrics_sent": 0,
            "logs_sent": 0,
            "errors": 0,
        }
    
    def start(self):
        """Start background thread"""
        with self.lock:
            if self.running:
                return
            
            self.running = True
            self.worker_thread = threading.Thread(target=self._worker_loop, daemon=True)
            self.worker_thread.start()
            debug_log(f"[Primus Lens API Reporter] Started (API: {self.api_base_url})")
    
    def stop(self):
        """Stop background thread"""
        with self.lock:
            if not self.running:
                return
            
            self.running = False
        
        # Flush all pending data
        self.flush_all()
        
        # Wait for thread to finish
        if self.worker_thread and self.worker_thread.is_alive():
            self.worker_thread.join(timeout=5.0)
        
        debug_log(f"[Primus Lens API Reporter] Stopped. Stats: {self.stats}")
    
    def report_detection(self, detection_data: Dict[str, Any]):
        """
        Report framework detection data (asynchronous)
        
        Args:
            detection_data: Framework detection data
        """
        debug_log(f"[Primus Lens API Reporter] report_detection() called")
        debug_log(f"[Primus Lens API Reporter] Detection data keys: {list(detection_data.keys())}")
        try:
            self.detection_queue.put_nowait(detection_data)
            debug_log(f"[Primus Lens API Reporter] Detection data queued successfully, queue size: {self.detection_queue.qsize()}")
        except:
            # Queue is full, drop data (avoid blocking)
            warning_log("[Primus Lens API Reporter] Detection queue full, dropping data")
    
    def report_metrics(self, metrics_data: Dict[str, Any]):
        """
        Report training metrics (asynchronous)
        
        Args:
            metrics_data: Training metrics data
        """
        debug_log(f"[Primus Lens API Reporter] report_metrics() called")
        debug_log(f"[Primus Lens API Reporter] Metrics count: {len(metrics_data.get('metrics', []))}")
        try:
            self.metrics_queue.put_nowait(metrics_data)
            debug_log(f"[Primus Lens API Reporter] Metrics data queued successfully, queue size: {self.metrics_queue.qsize()}")
        except:
            warning_log("[Primus Lens API Reporter] Metrics queue full, dropping data")
    
    def report_logs(self, logs_data: Dict[str, Any]):
        """
        Report training logs (asynchronous)
        
        Args:
            logs_data: Training log data
        """
        debug_log(f"[Primus Lens API Reporter] report_logs() called")
        debug_log(f"[Primus Lens API Reporter] Logs count: {len(logs_data.get('logs', []))}")
        try:
            self.logs_queue.put_nowait(logs_data)
            debug_log(f"[Primus Lens API Reporter] Logs data queued successfully, queue size: {self.logs_queue.qsize()}")
        except:
            warning_log("[Primus Lens API Reporter] Logs queue full, dropping data")
    
    def _worker_loop(self):
        """Background thread work loop"""
        last_flush_time = time.time()
        
        while self.running:
            try:
                # Check if flush is needed
                current_time = time.time()
                if current_time - last_flush_time >= self.flush_interval:
                    self._flush_queues()
                    last_flush_time = current_time
                
                # Sleep briefly to avoid high CPU usage
                time.sleep(0.1)
                
            except Exception as e:
                error_log(f"[Primus Lens API Reporter] Worker error: {e}")
                import traceback
                traceback.print_exc()
    
    def flush_all(self):
        """Flush all queues (send all pending data immediately)"""
        self._flush_queues()
    
    def _flush_queues(self):
        """Flush all queues"""
        # Flush framework detection data
        self._flush_detection_queue()
        
        # Flush metrics data
        self._flush_metrics_queue()
        
        # Flush log data
        self._flush_logs_queue()
    
    def _flush_detection_queue(self):
        """Flush framework detection queue"""
        items = []
        try:
            while not self.detection_queue.empty() and len(items) < self.batch_size:
                item = self.detection_queue.get_nowait()
                items.append(item)
        except Empty:
            pass
        
        if not items:
            return
        
        debug_log(f"[Primus Lens API Reporter] Flushing {len(items)} detection item(s)")
        
        # Send detection data (send individually, as usually only once)
        for item in items:
            debug_log(f"[Primus Lens API Reporter] Sending detection data...")
            success = self._send_detection(item)
            if success:
                self.stats["detection_sent"] += 1
                debug_log(f"[Primus Lens API Reporter] Detection data sent successfully")
            else:
                self.stats["errors"] += 1
                debug_log(f"[Primus Lens API Reporter] Detection data failed to send")
    
    def _flush_metrics_queue(self):
        """Flush metrics queue"""
        items = []
        try:
            while not self.metrics_queue.empty() and len(items) < self.batch_size:
                item = self.metrics_queue.get_nowait()
                items.append(item)
        except Empty:
            pass
        
        if not items:
            return
        
        debug_log(f"[Primus Lens API Reporter] Flushing {len(items)} metrics item(s)")
        
        # Batch send metrics (if multiple)
        if len(items) == 1:
            debug_log(f"[Primus Lens API Reporter] Sending single metrics data...")
            success = self._send_metrics(items[0])
            if success:
                self.stats["metrics_sent"] += 1
                debug_log(f"[Primus Lens API Reporter] Metrics data sent successfully")
            else:
                self.stats["errors"] += 1
                debug_log(f"[Primus Lens API Reporter] Metrics data failed to send")
        else:
            # Batch reporting
            debug_log(f"[Primus Lens API Reporter] Sending batched metrics data ({len(items)} items)...")
            success = self._send_metrics_batch(items)
            if success:
                self.stats["metrics_sent"] += len(items)
                debug_log(f"[Primus Lens API Reporter] Batched metrics data sent successfully")
            else:
                self.stats["errors"] += len(items)
                debug_log(f"[Primus Lens API Reporter] Batched metrics data failed to send")
    
    def _flush_logs_queue(self):
        """Flush logs queue"""
        items = []
        try:
            while not self.logs_queue.empty() and len(items) < self.batch_size:
                item = self.logs_queue.get_nowait()
                items.append(item)
        except Empty:
            pass
        
        if not items:
            return
        
        debug_log(f"[Primus Lens API Reporter] Flushing {len(items)} logs item(s)")
        
        # Batch send logs (if multiple)
        if len(items) == 1:
            debug_log(f"[Primus Lens API Reporter] Sending single logs data...")
            success = self._send_logs(items[0])
            if success:
                self.stats["logs_sent"] += 1
                debug_log(f"[Primus Lens API Reporter] Logs data sent successfully")
            else:
                self.stats["errors"] += 1
                debug_log(f"[Primus Lens API Reporter] Logs data failed to send")
        else:
            # Batch reporting
            debug_log(f"[Primus Lens API Reporter] Sending batched logs data ({len(items)} items)...")
            success = self._send_logs_batch(items)
            if success:
                self.stats["logs_sent"] += len(items)
                debug_log(f"[Primus Lens API Reporter] Batched logs data sent successfully")
            else:
                self.stats["errors"] += len(items)
                debug_log(f"[Primus Lens API Reporter] Batched logs data failed to send")
    
    def _send_detection(self, data: Dict[str, Any]) -> bool:
        """Send framework detection data"""
        url = f"{self.api_base_url}/wandb/detection"
        return self._send_request(url, data)
    
    def _send_metrics(self, data: Dict[str, Any]) -> bool:
        """Send metrics data"""
        url = f"{self.api_base_url}/wandb/metrics"
        return self._send_request(url, data)
    
    def _send_logs(self, data: Dict[str, Any]) -> bool:
        """Send log data"""
        url = f"{self.api_base_url}/wandb/logs"
        return self._send_request(url, data)
    
    def _send_metrics_batch(self, items: List[Dict[str, Any]]) -> bool:
        """Batch send metrics"""
        # Merge into one request
        if not items:
            return True
        
        # Use meta information from first item
        first_item = items[0]
        merged_metrics = []
        
        for item in items:
            if "metrics" in item:
                merged_metrics.extend(item["metrics"])
        
        batch_data = {
            "source": first_item.get("source", "wandb"),  # Add: source field
            "workload_uid": first_item.get("workload_uid"),
            "pod_uid": first_item.get("pod_uid"),
            "pod_name": first_item.get("pod_name"),  # Add: missing pod_name field
            "run_id": first_item.get("run_id"),
            "metrics": merged_metrics,
            "timestamp": time.time(),
        }
        
        debug_log(f"[Primus Lens API Reporter] Batch metrics data - workload_uid={batch_data['workload_uid']}, pod_name={batch_data['pod_name']}, metrics_count={len(merged_metrics)}")
        
        return self._send_metrics(batch_data)
    
    def _send_logs_batch(self, items: List[Dict[str, Any]]) -> bool:
        """Batch send logs"""
        if not items:
            return True
        
        # Use meta information from first item
        first_item = items[0]
        merged_logs = []
        
        for item in items:
            if "logs" in item:
                merged_logs.extend(item["logs"])
        
        batch_data = {
            "source": first_item.get("source", "wandb"),  # Add: source field
            "workload_uid": first_item.get("workload_uid"),
            "pod_uid": first_item.get("pod_uid"),
            "pod_name": first_item.get("pod_name"),  # Add: missing pod_name field
            "run_id": first_item.get("run_id"),
            "logs": merged_logs,
            "timestamp": time.time(),
        }
        
        debug_log(f"[Primus Lens API Reporter] Batch logs data - workload_uid={batch_data['workload_uid']}, pod_name={batch_data['pod_name']}, logs_count={len(merged_logs)}")
        
        return self._send_logs(batch_data)
    
    def _send_request(self, url: str, data: Dict[str, Any], timeout: float = 5.0) -> bool:
        """
        Send HTTP POST request
        
        Args:
            url: Request URL
            data: Request data
            timeout: Timeout duration (seconds)
        
        Returns:
            bool: Whether successful
        """
        debug_log(f"[Primus Lens API Reporter] Preparing to send request to {url}")
        debug_log(f"[Primus Lens API Reporter] Data keys: {list(data.keys())}")
        
        try:
            json_data = json.dumps(data).encode('utf-8')
            debug_log(f"[Primus Lens API Reporter] Request payload size: {len(json_data)} bytes")
            
            req = Request(
                url,
                data=json_data,
                headers={
                    'Content-Type': 'application/json',
                    'User-Agent': 'Primus-Lens-WandB-Exporter/1.0',
                }
            )
            
            debug_log(f"[Primus Lens API Reporter] Sending HTTP POST request...")
            with urlopen(req, timeout=timeout) as response:
                if response.status == 200:
                    debug_log(f"[Primus Lens API Reporter] Request successful (200 OK)")
                    return True
                else:
                    warning_log(f"[Primus Lens API Reporter] Request failed: {response.status}")
                    return False
        
        except HTTPError as e:
            warning_log(f"[Primus Lens API Reporter] HTTP error: {e.code} {e.reason}")
            debug_log(f"[Primus Lens API Reporter] Failed URL: {url}")
            return False
        
        except URLError as e:
            warning_log(f"[Primus Lens API Reporter] URL error: {e.reason}")
            debug_log(f"[Primus Lens API Reporter] Failed URL: {url}")
            return False
        
        except Exception as e:
            warning_log(f"[Primus Lens API Reporter] Request error: {e}")
            debug_log(f"[Primus Lens API Reporter] Failed URL: {url}")
            return False


# Global reporter instance
_global_reporter: Optional[AsyncAPIReporter] = None


def get_global_reporter() -> AsyncAPIReporter:
    """Get global reporter instance"""
    global _global_reporter
    
    if _global_reporter is None:
        _global_reporter = AsyncAPIReporter()
        _global_reporter.start()
    
    return _global_reporter


def shutdown_reporter():
    """Shutdown global reporter"""
    global _global_reporter
    
    if _global_reporter is not None:
        _global_reporter.stop()
        _global_reporter = None

