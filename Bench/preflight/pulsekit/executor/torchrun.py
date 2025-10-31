from pulsekit.executor.executor import Executor
from typing import Dict, Any, AsyncGenerator
import os
import subprocess
import asyncio
import json


# -------------------------
# Executors: Can run locally on worker or be called directly on orchestrator for local ip
# (Two simple implementations provided here: TorchrunExecutor and ShellExecutor)
# -------------------------

class TorchrunExecutor(Executor):
    def __init__(self, nproc_default=8):
        self.nproc_default = nproc_default

    async def run(self, params: Dict[str, Any]) -> AsyncGenerator[str, None]:
        global current_proc
        env = os.environ.copy()
        for k in ["MASTER_ADDR","MASTER_PORT","NNODES","NODE_RANK","RANK"]:
            env.pop(k, None)
        nproc = params.get("nproc_per_node", self.nproc_default)
        cmd = [
            "torchrun",
            f"--nproc_per_node={nproc}",
            f"--nnodes={params['nnodes']}",
            f"--node_rank={params['node_rank']}",
            f"--master_addr={params['master_addr']}",
            f"--master_port={params['master_port']}",
            params["script"],
            *params.get("script_args", [])
        ]
        proc = subprocess.Popen(cmd, stdout=subprocess.PIPE, stderr=subprocess.STDOUT, text=True, env=env)
        current_proc = proc
        runs = params.get("runs", 1)
        repeat_until_fail = params.get("repeat_until_fail", False)
        run_index = 0
        try:
            while True:
                yield format_sse("start", f"Started node_rank={params['node_rank']} run_index={run_index}")
                # stream lines
                while True:
                    line = proc.stdout.readline()
                    if line:
                        yield format_sse("log", line.rstrip("\n"))
                    else:
                        if proc.poll() is not None:
                            break
                        yield format_sse("heartbeat", f"node_rank={params['node_rank']}")
                        await asyncio.sleep(1)
                ret = await asyncio.get_event_loop().run_in_executor(None, proc.wait)
                success = (ret == 0)
                yield format_sse("result", json.dumps({
                    "node_rank": params['node_rank'],
                    "run_index": run_index,
                    "success": success
                }))
                run_index += 1
                if repeat_until_fail and success:
                    proc = subprocess.Popen(cmd, stdout=subprocess.PIPE, stderr=subprocess.STDOUT, text=True, env=env)
                    current_proc = proc
                    continue
                if runs and run_index < runs:
                    proc = subprocess.Popen(cmd, stdout=subprocess.PIPE, stderr=subprocess.STDOUT, text=True, env=env)
                    current_proc = proc
                    continue
                break
        finally:
            current_proc = None