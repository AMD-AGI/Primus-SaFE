#!/usr/bin/env python3
import base64
import json
import os
import sys
import time
from typing import Any, Dict, Optional, Tuple
import requests

# Environment variable keys
NFS_PATH_ENV = "SAFE_NFS_PATH"
NFS_INPUT_ENV = "SAFE_NFS_INPUT"
NFS_OUTPUT_ENV = "SAFE_NFS_OUTPUT"

# Apiserver connectivity
ADMIN_CONTROL_PLANE_ENV = "ADMIN_CONTROL_PLANE"
APISERVER_NODE_PORT_ENV = "APISERVER_NODE_PORT"
USER_ID_ENV = "USER_ID"
WORKSPACE_ID_ENV = "WORKSPACE_ID"
SCALE_RUNNER_SET_ENV = "SCALE_RUNNER_SET"

# Optional overrides
GVK_KIND_ENV = "GVK_KIND"        # default: Deployment
GVK_VERSION_ENV = "GVK_VERSION"  # default: v1
POLL_INTERVAL_SECS = 5
DEFAULT_POLL_TIMEOUT_SECS = 36000


def getenv_str(name: str, default: Optional[str] = None) -> Optional[str]:
    val = os.environ.get(name)
    if val is None or val == "":
        return default
    return val


def is_base64(s: str) -> bool:
    try:
        raw = s.strip()
        if any(c.isspace() for c in raw):
            raw = "".join(raw.split())
        decoded = base64.b64decode(raw, validate=True)
        reencoded = base64.b64encode(decoded).decode("ascii")
        return reencoded.rstrip("=") == raw.rstrip("=")
    except Exception:
        return False


def ensure_base64(s: str) -> str:
    return s if is_base64(s) else base64.b64encode(s.encode("utf-8")).decode("ascii")


def wait_for_file(path: str, poll_interval: int = 2, timeout_secs: Optional[int] = None) -> bool:
    start_time = time.time()
    while not os.path.exists(path):
        if timeout_secs is not None and timeout_secs > 0 and (time.time() - start_time) >= timeout_secs:
            return False
        time.sleep(poll_interval)
    return True


def load_input_json(path: str) -> Dict[str, Any]:
    with open(path, "r", encoding="utf-8") as f:
        return json.load(f)


def build_payload_from_input(inp: Dict[str, Any]) -> Dict[str, Any]:
    model = inp.get("model")
    command = inp.get("command")
    image = inp.get("image")
    resources = inp.get("resources", {})
    env_map = inp.get("env", {}) or {}
    timeout = inp.get("timeout")  # seconds

    if not model:
        raise ValueError("input missing required field: model")
    if not command:
        raise ValueError("input missing required field: command")
    if not image:
        raise ValueError("input missing required field: image")
    for key in ("SCALE_RUNNER_SET"):
        val = getenv_str(key)
        if val is not None:
            env_map[key] = val

    workspace_id = getenv_str(WORKSPACE_ID_ENV)
    gvk_kind = "UnifiedJob"
    gvk_version = "v1"
    description = "scale-set-name:" + getenv_str("SCALE_RUNNER_SET")

    payload: Dict[str, Any] = {
        "displayName": model,
        "workspaceId": workspace_id,
        "resource": resources,
        "image": image,
        "entryPoint": ensure_base64(command),
        "env": env_map,
        "groupVersionKind": {"kind": gvk_kind, "version": gvk_version},
        "ttlSecondsAfterFinished": 300,
        "description": description,
    }
    if isinstance(timeout, int) and timeout > 0:
        payload["timeout"] = timeout
    return payload


def build_session() -> Tuple[requests.Session, str]:
    admin_ip = getenv_str(ADMIN_CONTROL_PLANE_ENV)
    node_port = getenv_str(APISERVER_NODE_PORT_ENV)
    if not admin_ip:
        raise ValueError("Missing ADMIN_CONTROL_PLANE (control plane IP address)")
    if not node_port:
        raise ValueError("Missing APISERVER_NODE_PORT (NodePort of apiserver pod)")
    base_url = f"http://{admin_ip}:{node_port}".rstrip("/")

    user_id = getenv_str(USER_ID_ENV)
    s = requests.Session()
    s.headers.update({"Content-Type": "application/json; charset=utf-8"})
    if user_id:
        s.headers.update({"userId": user_id})
    return s, base_url


def create_workload(s: requests.Session, base_url: str, payload: Dict[str, Any]) -> str:
    url = f"{base_url}/api/v1/workloads"
    resp = s.post(url, data=json.dumps(payload, ensure_ascii=False), timeout=30)
    if resp.status_code >= 300:
        raise RuntimeError(f"CreateWorkload failed: HTTP {resp.status_code} {resp.text}")
    data = resp.json()
    workload_id = data.get("workloadId")
    if not workload_id:
        raise RuntimeError(f"CreateWorkload returned no workloadId: {data}")
    return workload_id


def get_workload_phase(s: requests.Session, base_url: str, workload_id: str) -> str:
    url = f"{base_url}/api/v1/workloads/{workload_id}"
    resp = s.get(url, timeout=30)
    if resp.status_code >= 300:
        raise RuntimeError(f"GetWorkload failed: HTTP {resp.status_code} {resp.text}")
    data = resp.json()
    return data.get("phase")


def write_output(path: str, content: str) -> None:
    with open(path, "w", encoding="utf-8") as f:
        obj = {"phase": content if content is not None else ""}
        f.write(json.dumps(obj, ensure_ascii=False))


def main() -> int:
    nfs_root = getenv_str(NFS_PATH_ENV)
    nfs_input_rel = getenv_str(NFS_INPUT_ENV)
    nfs_output_rel = getenv_str(NFS_OUTPUT_ENV)
    if not nfs_root:
        print(f"[error] {NFS_PATH_ENV} not set", file=sys.stderr)
        return 2
    if not nfs_input_rel:
        print(f"[error] {NFS_INPUT_ENV} not set", file=sys.stderr)
        return 2
    if not nfs_output_rel:
        print(f"[error] {NFS_OUTPUT_ENV} not set", file=sys.stderr)
        return 2

    input_path = os.path.join(nfs_root, nfs_input_rel)
    output_path = os.path.join(nfs_root, nfs_output_rel)

    print(f"[info] waiting for input file: {input_path} (timeout: {DEFAULT_POLL_TIMEOUT_SECS}s)")
    if not wait_for_file(input_path, poll_interval=2, timeout_secs=DEFAULT_POLL_TIMEOUT_SECS):
        print(f"[error] waiting for input file timed out after {DEFAULT_POLL_TIMEOUT_SECS}s", file=sys.stderr)
        try:
            write_output(output_path, "Failed")
        except Exception:
            pass
        return 4

    try:
        inp = load_input_json(input_path)
    except Exception as e:
        print(f"[error] failed to parse input JSON: {e}", file=sys.stderr)
        return 3

    try:
        payload = build_payload_from_input(inp)
        session, base_url = build_session()
    except Exception as e:
        print(f"[error] initialization failed: {e}", file=sys.stderr)
        return 4

    try:
        workload_id = create_workload(session, base_url, payload)
        print(f"[info] workload created: {workload_id}")
    except Exception as e:
        print(f"[error] create workload failed: {e}", file=sys.stderr)
        write_output(output_path, "CreateFailed")
        return 5

    poll_timeout = inp.get("timeout") if isinstance(inp.get("timeout"), int) else DEFAULT_POLL_TIMEOUT_SECS
    start_time = time.time()
    terminal_phases = {"Succeeded", "Failed", "Stopped"}
    final_phase = None
    while True:
        try:
            phase = get_workload_phase(session, base_url, workload_id)
            if phase in terminal_phases:
                final_phase = phase
                break
        except Exception:
            pass

        if poll_timeout > 0 and (time.time() - start_time) >= poll_timeout:
            final_phase = "Failed"
            break
        time.sleep(POLL_INTERVAL_SECS)

    try:
        write_output(output_path, final_phase or "")
        print(f"[info] wrote final phase '{final_phase}' to {output_path}")
    except Exception as e:
        print(f"[error] failed to write output: {e}", file=sys.stderr)
        return 6

    return 0 if final_phase == "Succeeded" else 1


if __name__ == "__main__":
    sys.exit(main())


