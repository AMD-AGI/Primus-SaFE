import asyncio
import json
import os
import threading
from typing import Dict, Any, Callable, Awaitable

import torch.distributed as dist

import httpx
import uvicorn
from fastapi import FastAPI, Request
from fastapi.middleware.cors import CORSMiddleware
from fastapi.responses import StreamingResponse

from pulsekit.core.node import NodeInfo
from pulsekit.detector.detector import Detector
from pulsekit.executor.executor import Executor
from pulsekit.executor.registry import EXECUTOR_REGISTRY

app = FastAPI()
app.add_middleware(
    CORSMiddleware,
    allow_origins=["*"],
    allow_methods=["*"],
    allow_headers=["*"],
)
server_should_exit = threading.Event()
current_proc = None

def format_sse(event: str, data: str) -> str:
    lines = data.splitlines() or [""]
    out = [f"event: {event}"]
    out += [f"data: {line}" for line in lines]
    return "\n".join(out) + "\n\n"



def get_executor(name: str) -> Executor:
    return EXECUTOR_REGISTRY.get(name, EXECUTOR_REGISTRY["torchrun"])

# worker endpoint: directly return executor.run's async generator (note: don't await executor.run)
@app.post("/run_task_sse")
async def run_task_sse(request: Request):
    params = await request.json()
    executor_type = params.get("executor_type", "torchrun")
    executor = get_executor(executor_type)
    generator = executor.run(params)   # async generator
    return StreamingResponse(generator, media_type="text/event-stream")

@app.post("/shutdown")
async def shutdown_worker():
    server_should_exit.set()
    dist.destroy_process_group()
    os.kill(os.getpid(), 2)  # SIGINT
    return {"success": True, "message": "Shutting down"}

def start_worker_server(nodeInfo:NodeInfo, server_ready_event: threading.Event):
    config = uvicorn.Config(app, host="0.0.0.0", port=nodeInfo.port, log_level="info")
    server = uvicorn.Server(config)
    server_ready_event.set()
    loop = asyncio.new_event_loop()
    asyncio.set_event_loop(loop)
    async def _serve():
        waiter = loop.run_in_executor(None, server_should_exit.wait)
        serve_task = asyncio.create_task(server.serve())
        done, pending = await asyncio.wait([serve_task, waiter], return_when=asyncio.FIRST_COMPLETED)
        if waiter in done:
            server.should_exit = True
            await serve_task
    loop.run_until_complete(_serve())




async def invoke_node(ip: str, params: Dict[str, Any], detector: Detector) -> Dict[str, Any]:
    node_result: Dict[str, Any] = {}
    url = f"http://{ip}:18789/run_task_sse"
    success = None
    current_event = None
    data_lines = []
    inner_id = params["inner_id"]
    node_rank = params["node_rank"]

    node_result["inner_id"] = inner_id
    node_result["node_rank"] = node_rank
    node_result["param"] = params
    node_result["detected_error"] = []

    # Define event handler table
    async def handle_default(event: str, data_str: str):
        #  do nothing
        pass

    async def handle_result(event: str, data_str: str):
        nonlocal success
        try:
            payload = json.loads(data_str)
            # Save original result data
            node_result["final_result"] = payload
            if "success" in payload:
                success = payload["success"]
        except Exception as e:
            node_result.setdefault("parse_errors", []).append(
                f"result parse error: {e}, raw={data_str}"
            )

    async def handle_log(event: str, data_str: str):
        det = detector.detect(event, data_str)
        if det.get("has_error"):
            node_result["detected_error"].append(det)


    event_handlers: Dict[str, Callable[[str, str], Awaitable[None]]] = {
        "log": handle_log,
        "result": handle_result,
    }
    try:
        async with httpx.AsyncClient(timeout=None) as client:
            async with client.stream("POST", url, json=params) as resp:
                async for line in resp.aiter_lines():
                    if line is None:
                        continue
                    if line.startswith("event:"):
                        current_event = line.split(":", 1)[1].strip()
                    elif line.startswith("data:"):
                        data_lines.append(line.split(":", 1)[1].strip())
                    elif line.strip() == "":  # SSE event end
                        if current_event:
                            data_str = "\n".join(data_lines)
                            handler = event_handlers.get(current_event, handle_default)
                            await handler(current_event, data_str)
                        # reset
                        current_event = None
                        data_lines = []

        final = True if success is None else success
        node_result["result"] = final
        return node_result

    except Exception as e:
        node_result["error"] = str(e)
        node_result["result"] = False
        return node_result


def shutdown(ip: str,port:int) -> None:
    try:
        resp = httpx.post(f"http://{ip}:{port}/shutdown", timeout=5)
        print("Shutdown", ip, resp.status_code)
    except Exception as e:
        print("Shutdown error", ip, e)