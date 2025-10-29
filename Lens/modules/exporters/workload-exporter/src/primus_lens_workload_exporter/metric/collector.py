import os
import time
import threading
import json
import socket
from typing import Callable, Dict, Any, List


class OTelMetric:
    def __init__(self, name: str, description: str, value: float,
                 labels: Dict[str, str] = None, ts: int = None):
        self.name = name
        self.description = description
        self.value = value
        self.labels = labels or {}
        self.ts = ts or int(time.time() * 1e9)  # nanosecond timestamp

    def to_otlp_json(self) -> Dict[str, Any]:
        return {
            "name": self.name,
            "description": self.description,
            "unit": "",
            "data": {
                "dataPoints": [
                    {
                        "attributes": [
                            {"key": k, "value": {"stringValue": v}}
                            for k, v in self.labels.items()
                        ],
                        "timeUnixNano": str(self.ts),
                        "asDouble": self.value
                    }
                ],
                "dataPointType": "gauge"
            }
        }


class Collector:
    def __init__(self, output_path: str, interval: int = 10):
        self.collectors: List[Callable[[], List[OTelMetric]]] = []
        self.output_path = output_path
        self.interval = interval
        self.running = False
        self._thread: threading.Thread | None = None


    def register_collector(self, func: Callable[[], List[OTelMetric]]):
        self.collectors.append(func)

    def run_once(self):
        resource_metrics = {
            "resourceMetrics": [
                {
                    "resource": {
                        "attributes": [
                            {"key": "service.name", "value": {"stringValue": "workload-exporter"}},
                            {"key": "host.name", "value": {"stringValue": socket.gethostname()}}
                        ]
                    },
                    "scopeMetrics": [
                        {
                            "scope": {"name": "custom.collector"},
                            "metrics": []
                        }
                    ]
                }
            ]
        }

        for fn in self.collectors:
            try:
                for m in fn():
                    resource_metrics["resourceMetrics"][0]["scopeMetrics"][0]["metrics"].append(
                        m.to_otlp_json()
                    )
            except Exception as e:
                print("Collection failed:", e)
        file_name = f"{self.output_path}/metrics.json"
        os.makedirs(self.output_path, exist_ok=True)
        with open(file_name, "a") as f:
            f.write(json.dumps(resource_metrics) + "\n")

    def start(self):
        self.running = True

        def loop():
            while self.running:
                self.run_once()
                time.sleep(self.interval)

        self._thread = threading.Thread(target=loop, daemon=True)
        self._thread.start()

    def stop(self):
        self.running = False

    def wait(self):
        """Block until the collection thread exits"""
        if self._thread:
            self._thread.join()

