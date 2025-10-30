# orchestrator_tool.py
import os
import asyncio
import threading
import socket
import json
import sys
from typing import List, Dict, Any, Callable, Awaitable, cast
import torch.distributed as dist

import httpx

from pulsekit.core.node import NodeInfo
from pulsekit.executor.executor import Executor
from pulsekit.orchestrator.orchestrator import Orchestrator
from pulsekit.server.server import start_worker_server, shutdown


# -------------------------
# main: Keep the original worker startup and dist.all_gather_object sync logic; rank0 starts Orchestrator.run(...)
# -------------------------
def main():
    log_path = os.environ.get("LOG_DIR","/tmp")
    rank = int(os.environ.get("RANK","0"))
    world_size = int(os.environ.get("WORLD_SIZE","1"))
    # keep using gloo for small control plane
    dist.init_process_group(backend="gloo")

    node_info = NodeInfo(port=18789,node_rank=rank)

    server_ready_event = threading.Event()
    server_thread = threading.Thread(target=start_worker_server, args=(node_info,server_ready_event,), daemon=True)
    server_thread.start()
    server_ready_event.wait()
    print(f"[Rank {rank}] worker server started")

    all_ready: List[NodeInfo] = cast(List[NodeInfo], [None] * world_size)
    dist.all_gather_object(all_ready, node_info)
    print(f"[Rank {rank}] all nodes ready: {all_ready}")

    if rank == 0:
        orch = Orchestrator(all_ready, warmup_delay=5.0)
        # optionally register custom detectors/executors here: orch.register_detector(...), orch.register_executor(...)
        try:
        # Capture exceptions during runtime
            results = asyncio.run(orch.run())
            print("[Orchestrator] final results:", results)
        except Exception as e:
            print("[Orchestrator] encountered error:", e, file=sys.stderr)
            results = None
        print("[Orchestrator] final results:", results)
        # shutdown all workers
        for node in all_ready:
            if node.ip == node_info.ip: # local_ip
                continue
            shutdown(node.ip, node.port)
        shutdown(node_info.ip, node_info.port) # shutdown local server
    else:
        try:
            server_thread.join()
        except KeyboardInterrupt:
            print("Received Sig interrupt, exiting gracefully...")
            return 0
    return 0

if __name__ == "__main__":
    exit_code = main()
    sys.exit(exit_code)