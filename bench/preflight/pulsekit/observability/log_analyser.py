import re
from typing import List, Dict

# -----------------------------
# Precompiled regular expressions (compile only once)
# -----------------------------
IB_PATTERN = re.compile(
    r"NET/IB: Got completion from peer (?P<peer_ip>[\d\.]+)<(?P<peer_port>\d+)>.*?status=(?P<status>\d+).*?vendor err (?P<vendor_err>\d+).*?remoteGids::ffff:(?P<remote_ip>[\d\.]+)"
)

SOCKET_PATTERN = re.compile(
    r"waitForInput: socket .*?addr=\[(?P<local_host>[^\]]+)\]:(?P<local_port>\d+), remote=\[(?P<remote_host>[^\]]+)\]:(?P<remote_port>\d+)\).*?timed out after (?P<timeout>\d+)ms.*?node_rank\": (?P<node_rank>\d+)"
)

def parse_error_logs(log_text: str) -> List[Dict[str, str]]:
    """
    Input a complete log string, parse two types of error logs:
    1. NET/IB: Got completion from peer ...
    2. waitForInput: socket ... timed out ...
    Return error reason and peer information
    """
    results = []

    for line in log_text.splitlines():
        line = line.strip()
        if not line:
            continue

        # Match IB errors
        m1 = IB_PATTERN.search(line)
        if m1:
            results.append({
                "error_type": "IB Completion Error",
                "reason": f"status={m1.group('status')}, vendor_err={m1.group('vendor_err')}",
                "peer": f"{m1.group('peer_ip')}:{m1.group('peer_port')}",
                "remote": m1.group('remote_ip')
            })
            continue

        # Match socket timeout
        m2 = SOCKET_PATTERN.search(line)
        if m2:
            results.append({
                "error_type": "Socket Timeout",
                "reason": f"timed out after {m2.group('timeout')}ms",
                "peer": f"{m2.group('remote_host')}:{m2.group('remote_port')}",
                "local": f"{m2.group('local_host')}:{m2.group('local_port')}",
                "node_rank": m2.group('node_rank')
            })
            continue

    return results
