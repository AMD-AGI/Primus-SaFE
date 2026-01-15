#!/usr/bin/env python3
# Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.

"""
Node Command Executor Script

This script creates an OpsJob and monitors its execution until completion.

Environment Variables:
    NODE_NAME: The target node name
    APIKEY: API key for authentication
    ADMIN_CONTROL_PLANE: The control plane endpoint (e.g., "l0.0.0.1")
    APISERVER_NODE_PORT: The apiserver node port (e.g., "8080")

Usage:
    python node_executor.py "<script content>"

Example:
    python node_executor.py "ulimit -n 65535"
"""

import base64
import json
import os
import sys
import time
import urllib.error
import urllib.request

def get_env_or_exit(name: str) -> str:
    """Get environment variable or exit with error if not set or empty."""
    value = os.environ.get(name)
    if value is None:
        print(f"Error: Environment variable {name} is not set", file=sys.stderr)
        sys.exit(1)
    value = value.strip()
    if not value:
        print(f"Error: Environment variable {name} is empty", file=sys.stderr)
        sys.exit(1)
    return value


def create_ops_job(endpoint: str, apikey: str, node_name: str, script_base64: str, timeout: int = 300) -> str:
    """
    Create an OpsJob and return the job ID.
    
    Args:
        endpoint: The control plane endpoint(including port)
        apikey: API key for authentication
        node_name: Target node name
        script_base64: Base64 encoded script content
        timeout: Job timeout in seconds
    
    Returns:
        The created job ID
    """
    url = f"http://{endpoint}/api/v1/opsjobs"
    
    body = {
        "inputs": [
            {"name": "node", "value": node_name},
            {"name": "script", "value": script_base64}
        ],
        "name": node_name + "-addon-script",
        "type": "addon",
        "securityUpgrade": False,
        "timeoutSecond": timeout
    }
    
    headers = {
        "Content-Type": "application/json",
        "Authorization": f"Bearer {apikey}"
    }
    
    data = json.dumps(body).encode("utf-8")
    req = urllib.request.Request(url, data=data, headers=headers, method="POST")
    
    try:
        with urllib.request.urlopen(req, timeout=30) as response:
            result = json.loads(response.read().decode("utf-8"))
            job_id = result.get("jobId")
            if not job_id:
                print(f"Error: No jobId in response: {result}", file=sys.stderr)
                sys.exit(1)
            return job_id
    except urllib.error.HTTPError as e:
        error_body = e.read().decode("utf-8") if e.fp else ""
        print(f"Error creating OpsJob: HTTP {e.code} - {error_body}", file=sys.stderr)
        sys.exit(1)
    except urllib.error.URLError as e:
        print(f"Error connecting to server: {e.reason}", file=sys.stderr)
        sys.exit(1)


def get_ops_job(endpoint: str, apikey: str, job_id: str, max_retries: int = 10, retry_interval: float = 0.5) -> dict:
    """
    Get OpsJob status by job ID with retry logic.
    
    Args:
        endpoint: The control plane endpoint
        apikey: API key for authentication
        job_id: The job ID to query
        max_retries: Maximum number of retries (default 10)
        retry_interval: Wait time between retries in seconds (default 0.5)
    
    Returns:
        The job response as dict
    """
    url = f"http://{endpoint}/api/v1/opsjobs/{job_id}"
    
    headers = {
        "Authorization": f"Bearer {apikey}"
    }
    
    last_error = None
    for attempt in range(max_retries):
        req = urllib.request.Request(url, headers=headers, method="GET")
        try:
            with urllib.request.urlopen(req, timeout=10) as response:
                return json.loads(response.read().decode("utf-8"))
        except urllib.error.HTTPError as e:
            error_body = e.read().decode("utf-8") if e.fp else ""
            last_error = f"HTTP {e.code} - {error_body}"
        except urllib.error.URLError as e:
            last_error = f"Connection error: {e.reason}"
        
        if attempt < max_retries - 1:
            time.sleep(retry_interval)
    
    print(f"Error querying OpsJob after {max_retries} retries: {last_error}", file=sys.stderr)
    return {}


def wait_for_completion(endpoint: str, apikey: str, job_id: str, timeout: int = 300, poll_interval: int = 1) -> bool:
    """
    Wait for OpsJob to complete.
    
    Args:
        endpoint: The control plane endpoint
        apikey: API key for authentication
        job_id: The job ID to monitor
        timeout: Maximum wait time in seconds
        poll_interval: Polling interval in seconds
    
    Returns:
        True if job succeeded, False otherwise
    """
    start_time = time.time()
    
    while True:
        elapsed = time.time() - start_time
        if elapsed > timeout:
            print(f"Error: Timeout waiting for execution after {timeout} seconds", file=sys.stderr)
            return False
        
        result = get_ops_job(endpoint, apikey, job_id)
        if not result:
            time.sleep(poll_interval)
            continue
        
        phase = result.get("phase", "")
        if phase == "Succeeded":
            print(f"\n✓ The script executed successfully!")
            return True
        elif phase == "Failed":
            print(f"\n✗ The script execution failed!", file=sys.stderr)
            return False
        
        # Still running (Pending, Running, etc.), continue polling
        time.sleep(poll_interval)


def main():
    if len(sys.argv) < 2:
        print("Usage: python run_ops_job.py \"<script content>\"", file=sys.stderr)
        print("Example: python run_ops_job.py \"ulimit -n 65535\"", file=sys.stderr)
        sys.exit(1)
    
    script_content = sys.argv[1]
    
    # Get environment variables
    node_name = get_env_or_exit("NODE_NAME")
    apikey = get_env_or_exit("APIKEY")
    endpoint = get_env_or_exit("ADMIN_CONTROL_PLANE")
    port = get_env_or_exit("APISERVER_NODE_PORT")
    endpoint = f"{endpoint}:{port}"
    
    # Base64 encode the script
    script_base64 = base64.b64encode(script_content.encode("utf-8")).decode("utf-8")    
    # Create the OpsJob
    job_id = create_ops_job(endpoint, apikey, node_name, script_base64)
    
    # Wait for completion
    success = wait_for_completion(endpoint, apikey, job_id)
    
    sys.exit(0 if success else 1)


if __name__ == "__main__":
    main()
