"""
API Reporter - 异步上报数据到 telemetry-processor
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

# 导入日志模块
from .logger import debug_log, error_log, warning_log


class AsyncAPIReporter:
    """异步 API 上报器 - 使用后台线程上报数据"""
    
    def __init__(self, api_base_url: Optional[str] = None, batch_size: int = 10, flush_interval: float = 5.0):
        """
        初始化异步上报器
        
        Args:
            api_base_url: API 基础 URL，如 http://telemetry-processor:8080/api/v1
            batch_size: 批量上报的大小
            flush_interval: 刷新间隔（秒）
        """
        self.api_base_url = api_base_url or os.environ.get(
            "PRIMUS_LENS_API_BASE_URL", 
            "http://primus-lens-telemetry-processor:8080/api/v1"
        )
        self.batch_size = batch_size
        self.flush_interval = flush_interval
        
        # 数据队列
        self.detection_queue = Queue(maxsize=100)
        self.metrics_queue = Queue(maxsize=1000)
        self.logs_queue = Queue(maxsize=1000)
        
        # 后台线程
        self.worker_thread = None
        self.running = False
        self.lock = threading.Lock()
        
        # 统计信息
        self.stats = {
            "detection_sent": 0,
            "metrics_sent": 0,
            "logs_sent": 0,
            "errors": 0,
        }
    
    def start(self):
        """启动后台线程"""
        with self.lock:
            if self.running:
                return
            
            self.running = True
            self.worker_thread = threading.Thread(target=self._worker_loop, daemon=True)
            self.worker_thread.start()
            debug_log(f"[Primus Lens API Reporter] Started (API: {self.api_base_url})")
    
    def stop(self):
        """停止后台线程"""
        with self.lock:
            if not self.running:
                return
            
            self.running = False
        
        # 刷新所有待处理的数据
        self.flush_all()
        
        # 等待线程结束
        if self.worker_thread and self.worker_thread.is_alive():
            self.worker_thread.join(timeout=5.0)
        
        debug_log(f"[Primus Lens API Reporter] Stopped. Stats: {self.stats}")
    
    def report_detection(self, detection_data: Dict[str, Any]):
        """
        上报框架检测数据（异步）
        
        Args:
            detection_data: 框架检测数据
        """
        try:
            self.detection_queue.put_nowait(detection_data)
        except:
            # 队列满了，丢弃数据（避免阻塞）
            warning_log("[Primus Lens API Reporter] Detection queue full, dropping data")
    
    def report_metrics(self, metrics_data: Dict[str, Any]):
        """
        上报训练指标（异步）
        
        Args:
            metrics_data: 训练指标数据
        """
        try:
            self.metrics_queue.put_nowait(metrics_data)
        except:
            warning_log("[Primus Lens API Reporter] Metrics queue full, dropping data")
    
    def report_logs(self, logs_data: Dict[str, Any]):
        """
        上报训练日志（异步）
        
        Args:
            logs_data: 训练日志数据
        """
        try:
            self.logs_queue.put_nowait(logs_data)
        except:
            warning_log("[Primus Lens API Reporter] Logs queue full, dropping data")
    
    def _worker_loop(self):
        """后台线程工作循环"""
        last_flush_time = time.time()
        
        while self.running:
            try:
                # 检查是否需要刷新
                current_time = time.time()
                if current_time - last_flush_time >= self.flush_interval:
                    self._flush_queues()
                    last_flush_time = current_time
                
                # 休眠一小段时间，避免 CPU 占用过高
                time.sleep(0.1)
                
            except Exception as e:
                error_log(f"[Primus Lens API Reporter] Worker error: {e}")
                import traceback
                traceback.print_exc()
    
    def flush_all(self):
        """刷新所有队列（立即发送所有待处理数据）"""
        self._flush_queues()
    
    def _flush_queues(self):
        """刷新所有队列"""
        # 刷新框架检测数据
        self._flush_detection_queue()
        
        # 刷新指标数据
        self._flush_metrics_queue()
        
        # 刷新日志数据
        self._flush_logs_queue()
    
    def _flush_detection_queue(self):
        """刷新框架检测队列"""
        items = []
        try:
            while not self.detection_queue.empty() and len(items) < self.batch_size:
                item = self.detection_queue.get_nowait()
                items.append(item)
        except Empty:
            pass
        
        if not items:
            return
        
        # 发送检测数据（单个发送，因为通常只有一次）
        for item in items:
            success = self._send_detection(item)
            if success:
                self.stats["detection_sent"] += 1
            else:
                self.stats["errors"] += 1
    
    def _flush_metrics_queue(self):
        """刷新指标队列"""
        items = []
        try:
            while not self.metrics_queue.empty() and len(items) < self.batch_size:
                item = self.metrics_queue.get_nowait()
                items.append(item)
        except Empty:
            pass
        
        if not items:
            return
        
        # 批量发送指标（如果有多个）
        if len(items) == 1:
            success = self._send_metrics(items[0])
            if success:
                self.stats["metrics_sent"] += 1
            else:
                self.stats["errors"] += 1
        else:
            # 批量上报
            success = self._send_metrics_batch(items)
            if success:
                self.stats["metrics_sent"] += len(items)
            else:
                self.stats["errors"] += len(items)
    
    def _flush_logs_queue(self):
        """刷新日志队列"""
        items = []
        try:
            while not self.logs_queue.empty() and len(items) < self.batch_size:
                item = self.logs_queue.get_nowait()
                items.append(item)
        except Empty:
            pass
        
        if not items:
            return
        
        # 批量发送日志（如果有多个）
        if len(items) == 1:
            success = self._send_logs(items[0])
            if success:
                self.stats["logs_sent"] += 1
            else:
                self.stats["errors"] += 1
        else:
            # 批量上报
            success = self._send_logs_batch(items)
            if success:
                self.stats["logs_sent"] += len(items)
            else:
                self.stats["errors"] += len(items)
    
    def _send_detection(self, data: Dict[str, Any]) -> bool:
        """发送框架检测数据"""
        url = f"{self.api_base_url}/wandb/detection"
        return self._send_request(url, data)
    
    def _send_metrics(self, data: Dict[str, Any]) -> bool:
        """发送指标数据"""
        url = f"{self.api_base_url}/wandb/metrics"
        return self._send_request(url, data)
    
    def _send_logs(self, data: Dict[str, Any]) -> bool:
        """发送日志数据"""
        url = f"{self.api_base_url}/wandb/logs"
        return self._send_request(url, data)
    
    def _send_metrics_batch(self, items: List[Dict[str, Any]]) -> bool:
        """批量发送指标"""
        # 合并成一个请求
        if not items:
            return True
        
        # 使用第一个 item 的元信息
        first_item = items[0]
        merged_metrics = []
        
        for item in items:
            if "metrics" in item:
                merged_metrics.extend(item["metrics"])
        
        batch_data = {
            "workload_uid": first_item.get("workload_uid"),
            "pod_uid": first_item.get("pod_uid"),
            "run_id": first_item.get("run_id"),
            "metrics": merged_metrics,
            "timestamp": time.time(),
        }
        
        return self._send_metrics(batch_data)
    
    def _send_logs_batch(self, items: List[Dict[str, Any]]) -> bool:
        """批量发送日志"""
        if not items:
            return True
        
        # 使用第一个 item 的元信息
        first_item = items[0]
        merged_logs = []
        
        for item in items:
            if "logs" in item:
                merged_logs.extend(item["logs"])
        
        batch_data = {
            "workload_uid": first_item.get("workload_uid"),
            "pod_uid": first_item.get("pod_uid"),
            "run_id": first_item.get("run_id"),
            "logs": merged_logs,
            "timestamp": time.time(),
        }
        
        return self._send_logs(batch_data)
    
    def _send_request(self, url: str, data: Dict[str, Any], timeout: float = 5.0) -> bool:
        """
        发送 HTTP POST 请求
        
        Args:
            url: 请求 URL
            data: 请求数据
            timeout: 超时时间（秒）
        
        Returns:
            bool: 是否成功
        """
        try:
            json_data = json.dumps(data).encode('utf-8')
            
            req = Request(
                url,
                data=json_data,
                headers={
                    'Content-Type': 'application/json',
                    'User-Agent': 'Primus-Lens-WandB-Exporter/1.0',
                }
            )
            
            with urlopen(req, timeout=timeout) as response:
                if response.status == 200:
                    return True
                else:
                    warning_log(f"[Primus Lens API Reporter] Request failed: {response.status}")
                    return False
        
        except HTTPError as e:
            warning_log(f"[Primus Lens API Reporter] HTTP error: {e.code} {e.reason}")
            return False
        
        except URLError as e:
            warning_log(f"[Primus Lens API Reporter] URL error: {e.reason}")
            return False
        
        except Exception as e:
            warning_log(f"[Primus Lens API Reporter] Request error: {e}")
            return False


# 全局报告器实例
_global_reporter: Optional[AsyncAPIReporter] = None


def get_global_reporter() -> AsyncAPIReporter:
    """获取全局报告器实例"""
    global _global_reporter
    
    if _global_reporter is None:
        _global_reporter = AsyncAPIReporter()
        _global_reporter.start()
    
    return _global_reporter


def shutdown_reporter():
    """关闭全局报告器"""
    global _global_reporter
    
    if _global_reporter is not None:
        _global_reporter.stop()
        _global_reporter = None

