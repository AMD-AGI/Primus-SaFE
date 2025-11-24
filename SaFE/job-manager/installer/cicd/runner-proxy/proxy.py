#!/usr/bin/env python3
import base64
import atexit
import json
import os
import sys
import time
import shutil
from typing import Any, Dict, Optional, Tuple
import requests

timeout_secs = 36000

def getenv_str(name: str, default: Optional[str] = None) -> Optional[str]:
    val = os.environ.get(name)
    if val is None or val == "":
        return default
    return val


def getenv_bool(name: str, default: bool = False) -> bool:
    val = getenv_str(name)
    if val is None:
        return default
    return val.strip().lower() in {"1", "true", "yes", "y", "on"}


def is_base64(s: str) -> bool:
    try:
        # Normalize: strip whitespace and pad if necessary
        raw = s.strip()
        # Reject obviously non-b64 characters fast
        if any(c.isspace() for c in raw):
            raw = "".join(raw.split())
        # Try decode-encode roundtrip
        decoded = base64.b64decode(raw, validate=True)
        reencoded = base64.b64encode(decoded).decode("ascii")
        # Allow missing padding in input
        return reencoded.rstrip("=") == raw.rstrip("=")
    except Exception:
        return False


def ensure_base64(s: str) -> str:
    return s if is_base64(s) else base64.b64encode(s.encode("utf-8")).decode("ascii")


def parse_resources(env_value: str) -> Dict[str, Any]:
    try:
        obj = json.loads(env_value)
        if not isinstance(obj, dict):
            raise ValueError("RESOURCES is not a JSON object")
        return obj
    except json.JSONDecodeError as e:
        raise ValueError(f"Invalid RESOURCES JSON: {e}") from e


def build_payload() -> Dict[str, Any]:
    # Required inputs
    resources_env = getenv_str("RESOURCES")
    image_env = getenv_str("IMAGE")
    entrypoint_env = getenv_str("ENTRYPOINT")
    if not resources_env:
        raise ValueError("Missing required env: RESOURCES")
    if not image_env:
        raise ValueError("Missing required env: IMAGE")
    if not entrypoint_env:
        raise ValueError("Missing required env: ENTRYPOINT")

    resources = parse_resources(resources_env)
    entrypoint_b64 = ensure_base64(entrypoint_env)

    # Optional metadata/config
    workspace_id = getenv_str("WORKSPACE_ID")
    display_name = getenv_str("SCALE_RUNNER_SET") + "-runner"
    kind = "AutoscalingRunner"
    version = "v1"
    priority = 0

    # Inject only the two requested environment variables if present
    env_map: Dict[str, str] = {}
    for key in ("ACTIONS_RUNNER_INPUT_JITCONFIG", "GITHUB_ACTIONS_RUNNER_EXTRA_USER_AGENT", "SCALE_RUNNER_SET"):
        val = getenv_str(key)
        if val is not None:
            env_map[key] = val

    # Compose request (CreateWorkloadRequest embeds WorkloadSpec)
    payload: Dict[str, Any] = {
        "displayName": display_name,
        "workspaceId": workspace_id,
        # WorkloadSpec fields (top-level is fine; server will unmarshal into Spec)
        "resource": resources,
        "workspace": workspace_id,
        "image": image_env,
        "entryPoint": entrypoint_b64,
        "env": env_map,
        "groupVersionKind": {"kind": kind, "version": version},
        "priority": priority,
        "timeout": timeout_secs,
        "ttlSecondsAfterFinished": 300,
    }
    return payload


def build_session() -> Tuple[requests.Session, str]:
    admin_ip = getenv_str("ADMIN_CONTROL_PLANE")
    node_port = getenv_str("APISERVER_NODE_PORT")
    if not admin_ip:
        raise ValueError("Missing ADMIN_CONTROL_PLANE (control plane IP address)")
    if not node_port:
        raise ValueError("Missing APISERVER_NODE_PORT (NodePort of apiserver pod)")
    base_url = f"http://{admin_ip}:{node_port}".rstrip("/")

    userId = getenv_str("USER_ID")
    s = requests.Session()
    s.headers.update({"Content-Type": "application/json; charset=utf-8"})
    if userId:
        s.headers.update({"userId": userId})

    return s, base_url


def create_workload(s: requests.Session, base_url: str, payload: Dict[str, Any]) -> str:
    url = f"{base_url}/api/v1/workloads"
    body = json.dumps(payload, ensure_ascii=False)
    print(f"[debug] POST {url}")
    print(f"[debug] body: {body}")
    resp = s.post(url, data=body, timeout=30)
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
    # Phase is flattened in response; also allow nested reading fallback
    phase = data.get("phase")
    return phase


def main() -> int:
    # Unified build mode: extend timeout and manage NFS path lifecycle
    unified_build_enabled = getenv_bool("UnifiedJob", False)
    cleanup_path: Optional[str] = None
    if unified_build_enabled:
        global timeout_secs
        timeout_secs = 24 * 60 * 60  # 24h
        nfs_path = getenv_str("SAFE_NFS_PATH")
        if nfs_path:
            try:
                os.makedirs(nfs_path, exist_ok=True)
                cleanup_path = nfs_path
            except Exception as e:
                print(f"[warn] failed to create SAFE_NFS_PATH directory '{nfs_path}': {e}", file=sys.stderr)
        if cleanup_path:
            def _cleanup() -> None:
                try:
                    shutil.rmtree(cleanup_path, ignore_errors=True)
                except Exception:
                    pass
            atexit.register(_cleanup)

    try:
        payload = build_payload()
        session, base_url = build_session()
    except Exception as e:
        print(f"[error] initialization failed: {e}", file=sys.stderr)
        return 2

    try:
        workload_id = create_workload(session, base_url, payload)
        print(f"[info] workload created: {workload_id}")
    except Exception as e:
        print(f"[error] create workload failed: {e}", file=sys.stderr)
        return 3

 # 0 = no timeout
    start_time = time.time()

    terminal_phases = {"Succeeded", "Failed", "Stopped"}
    while True:
        try:
            phase = get_workload_phase(session, base_url, workload_id)
            if phase in terminal_phases:
                if phase == "Succeeded":
                    print(f"[info] workload {workload_id} completed successfully")
                    return 0
                print(f"[warn] workload {workload_id} finished with phase: {phase}")
                return 1
        except Exception as e:
            # Empty exception handler - does nothing
            pass

        if timeout_secs > 0 and (time.time() - start_time) >= timeout_secs:
            print(f"[error] polling timed out after {timeout_secs}s", file=sys.stderr)
            return 4
        time.sleep(5)


if __name__ == "__main__":
    sys.exit(main())


