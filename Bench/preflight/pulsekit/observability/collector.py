# collector.py
import time
import os
import json
from datetime import datetime
from gpu import get_rocm_smi_info
from rdma import get_rdma_statistics


def append_metrics_to_file(file_path: str, interval: float = 5.0):
    """
    Get GPU + RDMA metrics every interval seconds and write to file
    """
    os.makedirs(os.path.dirname(file_path), exist_ok=True) 
    while True:
        timestamp = datetime.now().isoformat()

        # Get GPU information
        gpu_info = get_rocm_smi_info()

        # Get RDMA information
        rdma_info = get_rdma_statistics()

        # Combine into a dict
        metrics = {
            "timestamp": timestamp,
            "gpu": gpu_info,
            "rdma": rdma_info
        }

        # Append to file
        with open(file_path, "a") as f:
            f.write(json.dumps(metrics) + "\n")
        time.sleep(interval)


if __name__ == "__main__":
    output_file = "/tmp/gpu_rdma_metrics.log"  # Can be changed to any path
    interval_seconds = 5
    append_metrics_to_file(output_file, interval_seconds)
