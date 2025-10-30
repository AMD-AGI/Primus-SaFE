import json
from typing import Dict, Any

from pulsekit.observability.log_analyser import parse_error_logs

class Detector:
    def detect(self, event_type: str, data: str) -> Dict[str, Any]:
        raise NotImplementedError

class ErrorLogDetector(Detector):
    def detect(self, event_type: str, data: str):
        if event_type == "log":
            errs = parse_error_logs(data)
            return {"has_error": len(errs) > 0, "errors": errs}
        return {"has_error": False}

class ResultDetector(Detector):
    def detect(self, event_type: str, data: str):
        if event_type == "result":
            try:
                r = json.loads(data)
                return {"success": r.get("success", False)}
            except:
                return {"success": False}
        return {}

DETECTOR_REGISTRY = {
    "error_logs": ErrorLogDetector(),
    "result": ResultDetector(),
}
