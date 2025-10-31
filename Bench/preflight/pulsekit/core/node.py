import socket
import subprocess
import torch

from pulsekit.core.util import get_local_ip


class NodeInfo:
    def __init__(self, port: int, node_rank: int, ip: str = None, hostname: str = None):
        self.ip = ip or get_local_ip()
        self.hostname = hostname or socket.gethostname()
        self.port = port
        self.node_rank = node_rank
        self.ranks = []
        # Auto-detect information
        self.gpu_info = self.detect_gpus()
        self.rdma_device = self.detect_rdma_devices()
        for gpu_info in self.gpu_info:
            self.ranks.append(gpu_info["id"])

    @staticmethod
    def detect_gpus():
        """Detect local GPU information"""
        gpu_info = []
        try:
            if torch.cuda.is_available():
                for i in range(torch.cuda.device_count()):
                    props = torch.cuda.get_device_properties(i)
                    gpu_info.append({
                        "id": i,
                        "name": props.name,
                        "total_memory_GB": round(props.total_memory / 1024**3, 2)
                    })

        except Exception as e:
            gpu_info.append({"error": str(e)})
        return gpu_info

    @staticmethod
    def detect_rdma_devices():
        """Detect RDMA devices (ibv_devices or lspci)"""
        devices = []
        try:
            # Method 1: Using ibv_devices
            out = subprocess.check_output(["ibv_devices"], text=True)
            lines = out.strip().split("\n")[1:]  # Skip header line
            for line in lines:
                parts = line.split()
                if parts:
                    devices.append(parts[0])
        except Exception:
            try:
                # Method 2: lspci contains InfiniBand
                out = subprocess.check_output(["lspci"], text=True)
                for line in out.splitlines():
                    if "InfiniBand" in line or "Mellanox" in line:
                        devices.append(line.strip())
            except Exception:
                pass
        return devices

    def as_dict(self):
        return {
            "ip": self.ip,
            "hostname": self.hostname,
            "port": self.port,
            "node_rank": self.node_rank,
            "gpu_info": self.gpu_info,
            "rdma_device": self.rdma_device,
        }

    def __repr__(self):
        return str(self.as_dict())

