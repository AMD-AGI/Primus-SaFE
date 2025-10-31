import asyncio
from typing import List, Dict, Any, Callable, Awaitable
from pulsekit.core.node import NodeInfo
from pulsekit.core.util import get_local_ip
from pulsekit.detector.detector import Detector, DETECTOR_REGISTRY
from pulsekit.executor.executor import Executor
from pulsekit.executor.registry import EXECUTOR_REGISTRY


class Orchestrator:
    def __init__(self, nodes: List[NodeInfo],  warmup_delay: float = 5.0):
        self.nodes = nodes[:]  # copy
        self.warmup_delay = warmup_delay
        self.results: Dict[Any, Any] = {}
        self.failed: List[str] = []
        self.local_ip = get_local_ip()
        self.executor_registry = EXECUTOR_REGISTRY
        self.detector_registry = DETECTOR_REGISTRY

    def register_executor(self, name: str, executor: Executor):
        self.executor_registry[name] = executor

    def register_detector(self, name: str, detector: Detector):
        self.detector_registry[name] = detector

    async def run(self):
        for executor_name in self.executor_registry.keys():
            print(f"[Orchestrator] running {executor_name}...")
            executor = self.executor_registry[executor_name]
            result = await executor.schedule(self.nodes)
            print(f"[Orchestrator] {executor_name} execute success")
            self.results[executor_name] = result
        return self.results
