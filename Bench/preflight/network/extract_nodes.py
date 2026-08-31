#!/usr/bin/env python3
"""
Pull the unhealthy node IPs out of a diagnosis log, for run.sh to act on.

Two producers write that line, in two different shapes:

    ib_write_bw.py    unhealthy nodes: ['10.245.156.254']
    binary_diagnose.py unhealthy nodes: [crsuse2-m2m-182(10.245.156.254), ...]

The second shape carries both names for the node. Report the hostname and use
the ip only as a fallback, for when the hostname half is absent or unreadable.

The second is a human label, not a Python literal. This used to be parsed with
ast.literal_eval under a bare `except: return ''`, so every binary_diagnose
failure was read as "no unhealthy nodes" -- run.sh then saw an empty list, took
the "all nodes passed, exiting early" branch, and reported a run where both
nodes failed every RCCL test as 2/2 healthy with exit code 0.

So parse both shapes, and never answer "nothing is wrong" because the parse
failed: anything unrecognised is reported on stderr, which run.sh does not
capture into the node list but does show in the log.
"""
import re
import sys

# The bracketed list on the "unhealthy nodes:" line.
LIST_RE = re.compile(r"unhealthy nodes:\s*\[([^\]]*)\]")
# "hostname(ip)" -- capture both halves. The hostname is the identifier we
# report; the ip is only the fallback for when the hostname half is missing or
# not usable as a node name.
LABEL_RE = re.compile(r"^([^()]*)\(([^()]+)\)$")
# A bare IPv4 address, the other shape.
IP_RE = re.compile(r"^\d{1,3}(?:\.\d{1,3}){3}$")
# ...or a bare hostname. NODES_FILE is not required to hold IPs -- the
# documented default is /root/hosts, and binary_diagnose's node_label() falls
# back to the raw node string whenever HOSTNAME_MAP has no entry for it, so
# "unhealthy nodes: ['node-a']" is a shape that really occurs. run.sh matches
# NODES_FILE lines literally, so a hostname is just as usable to it as an IP;
# accepting only IPv4 here would drop those nodes on the floor and leave them
# in the list for the next test. (The old ast.literal_eval path handled them.)
HOSTNAME_RE = re.compile(r"^[A-Za-z0-9][A-Za-z0-9._-]*$")


def extract_unhealthy_nodes(text):
    """Return the unhealthy node IPs as a comma-separated string, in order."""
    nodes = []
    unparsed = []

    for match in LIST_RE.finditer(text):
        body = match.group(1).strip()
        if not body:
            continue
        for entry in body.split(","):
            entry = entry.strip().strip("'\"").strip()
            if not entry:
                continue
            label = LABEL_RE.match(entry)
            if label:
                # Prefer the hostname; fall back to the ip only when the
                # hostname half cannot be read.
                host = label.group(1).strip()
                node = host if HOSTNAME_RE.match(host) else label.group(2).strip()
            else:
                node = entry
            if not (IP_RE.match(node) or HOSTNAME_RE.match(node)):
                # Do not pass a value downstream that run.sh cannot match
                # against its node list -- say so instead of dropping it.
                unparsed.append(entry)
                continue
            if node not in nodes:
                nodes.append(node)

    if unparsed:
        print(
            "[extract_nodes] WARNING: could not read %d node(s) from the "
            "diagnosis output, they are NOT being removed: %s"
            % (len(unparsed), ", ".join(unparsed)),
            file=sys.stderr,
        )

    return ",".join(nodes)


if __name__ == "__main__":
    # Read from stdin if no arguments, otherwise from argument
    if len(sys.argv) > 1:
        input_data = sys.argv[1]
    else:
        # Read all input from stdin
        input_data = sys.stdin.read()

    result = extract_unhealthy_nodes(input_data)
    print(result)
