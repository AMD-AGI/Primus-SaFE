#!/usr/bin/env python3

#  Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
#  See LICENSE for license information.

import atexit
import base64
import json
import os
import shutil
import signal
import sys
import threading
import time
from typing import Any, Dict, List, Optional, Tuple

import requests

# Global cleanup context - set after workload creation
_cleanup_context: Dict[str, Any] = {
    "session": None,
    "base_url": None,
    "workload_id": None,
    "cleanup_path": None,
    "cleaned_up": False,
    "lock": threading.Lock(),
}

timeout_secs = 604800

def _do_cleanup() -> None:
    """
    Perform cleanup: stop workload and remove NFS directory.
    Thread-safe and idempotent - can be called multiple times safely.
    """
    with _cleanup_context["lock"]:
        if _cleanup_context["cleaned_up"]:
            return
        _cleanup_context["cleaned_up"] = True

        # Stop workload if created
        session = _cleanup_context["session"]
        base_url = _cleanup_context["base_url"]
        workload_id = _cleanup_context["workload_id"]
        if session and base_url and workload_id:
            print(f"[info] stopping workload on exit: {workload_id}", file=sys.stderr)
            try:
                url = f"{base_url}/api/v1/workloads/{workload_id}/stop"
                print(f"[debug] POST {url}", file=sys.stderr)
                resp = session.post(url, timeout=10)  # Shorter timeout for cleanup
                if resp.status_code >= 300:
                    print(f"[warn] stop workload failed: HTTP {resp.status_code} {resp.text}", file=sys.stderr)
            except Exception as e:
                print(f"[warn] stop workload exception: {e}", file=sys.stderr)

        # Clean up NFS directory if created
        cleanup_path = _cleanup_context["cleanup_path"]
        if cleanup_path:
            print(f"[info] cleaning up NFS directory: {cleanup_path}", file=sys.stderr)
            try:
                shutil.rmtree(cleanup_path, ignore_errors=True)
            except Exception:
                pass


def _signal_handler(signum: int, frame: Any) -> None:
    """
    Handle SIGTERM/SIGINT by performing cleanup directly, then exit.
    This ensures cleanup happens even if atexit handlers don't run.
    """
    sig_name = signal.Signals(signum).name if hasattr(signal, 'Signals') else str(signum)
    print(f"[info] received {sig_name}, performing cleanup...", file=sys.stderr)
    _do_cleanup()
    print(f"[info] cleanup done, exiting...", file=sys.stderr)
    sys.exit(128 + signum)


# Register signal handlers early
signal.signal(signal.SIGTERM, _signal_handler)
signal.signal(signal.SIGINT, _signal_handler)

# Also register atexit as a fallback for normal exits
atexit.register(_do_cleanup)

def getenv_str(name: str, default: Optional[str] = None) -> Optional[str]:
    val = os.environ.get(name)
    if val is None or val == "":
        return default
    return val

def getenv_int(name: str, default: Optional[int] = None) -> Optional[int]:
    val = getenv_str(name)
    if val is None:
        return default
    try:
        return int(val)
    except ValueError:
        return default

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


# Path where Kubernetes downwardAPI mounts pod metadata
PODINFO_DIR = "/etc/podinfo"
PODINFO_LABELS_FILE = os.path.join(PODINFO_DIR, "labels")
PODINFO_ANNOTATIONS_FILE = os.path.join(PODINFO_DIR, "annotations")


def parse_podinfo_file(filepath: str) -> Dict[str, str]:
    """
    Parse a Kubernetes downwardAPI metadata file (labels or annotations).
    Format is: key="value" per line.
    Returns a dict of key-value pairs, filtering out keys that should not be copied
    to new runner pods (unique identifiers, controller-managed fields, etc.).
    """
    # Keys that should NOT be copied (exact match)
    EXCLUDED_KEYS = {
        # Annotations - unique identifiers that cause signature validation failure
        "actions.github.com/patch-id",
        "actions.github.com/runner-spec-hash",
        "kubernetes.io/config.seen",
        "kubernetes.io/config.source",
        # Labels - controller-managed unique identifiers
        "batch.kubernetes.io/controller-uid",
        "batch.kubernetes.io/job-name",
        "controller-uid",
        "job-name",
        "pod-template-hash",
    }

    result: Dict[str, str] = {}
    if not os.path.isfile(filepath):
        return result
    try:
        with open(filepath, "r", encoding="utf-8") as f:
            for line in f:
                line = line.strip()
                if not line or "=" not in line:
                    continue
                # Format: key="value"
                eq_idx = line.index("=")
                key = line[:eq_idx]
                value = line[eq_idx + 1:]
                # Remove surrounding quotes if present
                if value.startswith('"') and value.endswith('"'):
                    value = value[1:-1]
                # Filter out excluded keys (exact match)
                if key in EXCLUDED_KEYS:
                    continue
                result[key] = value
    except Exception as e:
        print(f"[warn] failed to parse podinfo file '{filepath}': {e}", file=sys.stderr)
    return result


def get_pod_labels() -> Dict[str, str]:
    """Read user-defined labels from downwardAPI mounted file."""
    return parse_podinfo_file(PODINFO_LABELS_FILE)


def get_pod_annotations() -> Dict[str, str]:
    """Read user-defined annotations from downwardAPI mounted file."""
    return parse_podinfo_file(PODINFO_ANNOTATIONS_FILE)


def parse_resources(env_value: str) -> List[Dict[str, Any]]:
    try:
        obj = json.loads(env_value)
        if not isinstance(obj, dict):
            raise ValueError("RESOURCES is not a JSON object")
        return [obj]
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
    priority = getenv_int("PRIORITY", 0)
    display_name = getenv_str("SCALE_RUNNER_SET_ID")
    if display_name and len(display_name) > 30:
        display_name = display_name[:30]
    kind = "EphemeralRunner"
    version = "v1"

    env_map: Dict[str, str] = {}
    for key in ("SCALE_RUNNER_SET_ID", "SAFE_NFS_INPUT", "SAFE_NFS_OUTPUT", "GITHUB_CONFIG_URL", "GITHUB_SECRET_ID"):
        val = getenv_str(key)
        if val is not None:
            env_map[key] = val

    unified_build_enabled = getenv_bool("UNIFIED_JOB_ENABLE", False)
    if unified_build_enabled:
        nfs_path = get_unified_nfs_path()
        if nfs_path is not None:
            env_map["SAFE_NFS_PATH"] = nfs_path
    val = getenv_str("POD_NAME")
    if val is not None:
        env_map["SCALE_RUNNER_ID"] = val

    # Read user-defined labels and annotations from downwardAPI mounted files
    pod_labels = get_pod_labels()
    pod_annotations = get_pod_annotations()

    # Compose request (CreateWorkloadRequest embeds WorkloadSpec)
    payload: Dict[str, Any] = {
        "displayName": display_name,
        "workspaceId": workspace_id,
        "resources": resources,
        "workspace": workspace_id,
        "image": image_env,
        "entryPoint": entrypoint_b64,
        "env": env_map,
        "groupVersionKind": {"kind": kind, "version": version},
        "priority": priority,
        "timeout": timeout_secs,
        "ttlSecondsAfterFinished": 20,
    }
    # Add user-defined labels and annotations if present
    if pod_labels:
        payload["labels"] = pod_labels
    if pod_annotations:
        payload["annotations"] = pod_annotations

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
    print(f"[debug] POST {url}", flush=True)
    print(f"[debug] body: {body}", flush=True)
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


def stop_workload(s: requests.Session, base_url: str, workload_id: str) -> bool:
    """Stop workload and return True if successful."""
    try:
        url = f"{base_url}/api/v1/workloads/{workload_id}/stop"
        print(f"[debug] POST {url}", flush=True)
        resp = s.post(url, timeout=30)
        if resp.status_code >= 300:
            print(f"[warn] stop workload failed: HTTP {resp.status_code} {resp.text}", file=sys.stderr)
            return False
        return True
    except Exception as e:
        print(f"[warn] stop workload exception: {e}", file=sys.stderr)
        return False

def get_unified_nfs_path() -> Optional[str]:
    nfs_path = getenv_str("SAFE_NFS_PATH")
    pod_name = getenv_str("POD_NAME")
    if nfs_path and pod_name:
        return os.path.join(nfs_path, pod_name)
    return None

def main() -> int:
    print(f"[info] runner-proxy starting... (pid={os.getpid()}), timeout set to {timeout_secs}s", flush=True)
    
    # Unified build mode: extend timeout and manage NFS path lifecycle
    unified_build_enabled = getenv_bool("UNIFIED_JOB_ENABLE", False)
    if unified_build_enabled:
        print(f"[info] unified build mode enabled", flush=True)
        nfs_path = get_unified_nfs_path()
        print(f"[info] unified NFS path: {nfs_path}", flush=True)
        # Handle case where NFS path could not be constructed
        if nfs_path is None:
            print("[error] Failed to construct NFS path: missing SAFE_NFS_PATH or POD_NAME", file=sys.stderr)
            return 5
        if nfs_path:
            try:
                os.makedirs(nfs_path, exist_ok=True)
                # Store cleanup path in global context for signal handler
                _cleanup_context["cleanup_path"] = nfs_path
                print(f"[info] NFS directory created: {nfs_path}", flush=True)
            except Exception as e:
                print(f"[warn] failed to create SAFE_NFS_PATH directory '{nfs_path}': {e}", file=sys.stderr)

    print("[info] building payload and session...", flush=True)
    try:
        payload = build_payload()
        session, base_url = build_session()
        print(f"[info] session established, base_url: {base_url}", flush=True)
    except Exception as e:
        print(f"[error] initialization failed: {e}", file=sys.stderr)
        return 2

    print("[info] creating workload...", flush=True)
    try:
        workload_id = create_workload(session, base_url, payload)
        print(f"[info] workload created: {workload_id}", flush=True)
        # Store cleanup context for signal handler and atexit
        _cleanup_context["session"] = session
        _cleanup_context["base_url"] = base_url
        _cleanup_context["workload_id"] = workload_id
        print("[info] cleanup context registered for signal handler", flush=True)
    except Exception as e:
        print(f"[error] create workload failed: {e}", file=sys.stderr)
        return 3

    # 0 = no timeout
    start_time = time.time()
    last_phase = None
    poll_count = 0

    print(f"[info] starting to poll workload status...", flush=True)
    terminal_phases = {"Succeeded", "Failed", "Stopped"}
    while True:
        try:
            phase = get_workload_phase(session, base_url, workload_id)
            poll_count += 1
            # Log phase changes or periodically every 60 polls (~5 min)
            if phase != last_phase:
                print(f"[info] workload {workload_id} phase: {phase}", flush=True)
                last_phase = phase
            elif poll_count % 60 == 0:
                elapsed = int(time.time() - start_time)
                print(f"[info] workload {workload_id} still in phase: {phase} (elapsed: {elapsed}s)", flush=True)
            
            if phase in terminal_phases:
                elapsed = int(time.time() - start_time)
                # Workload already in terminal state, mark as cleaned up to skip stop on exit
                with _cleanup_context["lock"]:
                    _cleanup_context["workload_id"] = None  # Don't stop already-finished workload
                if phase == "Succeeded" or phase == "Stopped":
                    print(f"[info] workload {workload_id} completed successfully (elapsed: {elapsed}s)", flush=True)
                    return 0
                print(f"[warn] workload {workload_id} finished with phase: {phase} (elapsed: {elapsed}s)", flush=True)
                return 1
        except Exception as e:
            if (time.time() - start_time) >= 10:
                print(f"[warn] failed to get workload phase: {e}", file=sys.stderr)

        if timeout_secs > 0 and (time.time() - start_time) >= timeout_secs:
            print(f"[error] polling timed out after {timeout_secs}s", file=sys.stderr)
            return 4
        time.sleep(5)


if __name__ == "__main__":
    sys.exit(main())


