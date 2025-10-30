import json
import os
import subprocess
import time

import torch.distributed as dist
from pulsekit.core.node import NodeInfo
from pulsekit.server.server import invoke_node
from pulsekit.executor.executor import Executor
import asyncio
import random
from typing import List, Dict, Any, Tuple, Set, AsyncGenerator


class Peer:
    def __init__(self, node_a: NodeInfo, node_b: NodeInfo):
        self.node_a = node_a
        self.node_b = node_b

class PingpongExecutor(Executor):
    async def run(self, params: Dict[str, Any]) -> AsyncGenerator[str, None]:
        """Launch torchrun and return async generator for SSE"""
        env = os.environ.copy()
        for k in ["MASTER_ADDR", "MASTER_PORT", "NNODES", "NODE_RANK", "RANK"]:
            env.pop(k, None)

        cmd = [
            "torchrun",
            "--nproc_per_node=1",
            f"--nnodes={params['nnodes']}",
            f"--node_rank={params['node_rank']}",
            f"--master_addr={params['master_addr']}",
            f"--master_port={params['master_port']}",
            params["script"],
            *params.get("script_args", [])
        ]
        print(f"[PingPongExecutor] Executing: {' '.join(cmd)}")

        proc = subprocess.Popen(
            cmd, stdout=subprocess.PIPE, stderr=subprocess.STDOUT, text=True, env=env
        )

        def format_sse(event: str, data: str) -> str:
            """Format as SSE message, supporting multiline data"""
            lines = data.splitlines() or [""]
            msg = [f"event: {event}"]
            msg.extend(f"data: {line}" for line in lines)
            return "\n".join(msg) + "\n\n"

        async def event_generator():
            # Task start
            yield format_sse("start", f"data: [Task] Started node_rank={params['node_rank']}")

            # Async polling for logs + heartbeat
            while True:
                line = proc.stdout.readline()
                if line:
                    # print(f"[LOG]{line.strip()}")
                    yield format_sse("log", line.strip())
                else:
                    if proc.poll() is not None:
                        break
                    # Send heartbeat when there are no logs
                    yield format_sse("heartbeat", f"node_rank={params['node_rank']}")
                    await asyncio.sleep(1)

            ret = await asyncio.get_event_loop().run_in_executor(None, proc.wait)
            success = (ret == 0)
            yield format_sse("result", json.dumps({
                "node_rank": params['node_rank'],
                "success": success
            }))

        return event_generator()


    async def _invoke_pair(self, node_a: NodeInfo, node_b: NodeInfo, local_a: int, local_b: int, pair_id: int,
                           detector: Any,  retries: int = 0, timeout: float = None) -> Dict[str, Any]:
            """
            Invoke a single rank-pair test by calling both node HTTP servers in parallel.

            node_a/node_b are NodeInfo objects. local_a/local_b are the local_rank ints.
            Returns the dictionary describing results for this pair.
            """
            print(f"[{time.time():.3f}][PingPongExecutor] Invoking pair {pair_id}...")
            print(f"[{time.time():.3f}][PingPongExecutor] running node {node_a.hostname}:{local_a} {node_b.hostname}:{local_b}...]")
            print(f"[{time.time():.3f}][PingPongExecutor] running  {pair_id}...]")
            # Choose a master addr and port. Use node_a.ip as master by default.
            master_addr = node_a.ip
            master_port = random.randint(20000, 30000)

            # For deterministic rank assignment inside the 2-process world, we choose order by (node_idx, local_rank)
            # so that the smaller tuple receives rank=0 and the other rank=1.
            a_id = (getattr(node_a, "_idx", None), local_a)
            b_id = (getattr(node_b, "_idx", None), local_b)
            # but since node objects may not have _idx, just compare (node_a.ip, local_a)
            if (node_a.ip, local_a) <= (node_b.ip, local_b):
                left_rank, right_rank = 0, 1
                left_node, right_node = node_a, node_b
                left_local, right_local = local_a, local_b
            else:
                left_rank, right_rank = 0, 1
                left_node, right_node = node_b, node_a
                left_local, right_local = local_b, local_a

            params_left = {
                "master_addr": master_addr,
                "master_port": master_port,
                "rank": 0,
                "local_rank": 0,
                "nnodes":2,
                "script": "test/pingpong.py",
                "executor_type":'pingpong',
            }
            params_right = {
                "master_addr": master_addr,
                "master_port": master_port,
                "rank": 1,
                "local_rank": 1,
                "nnodes":2,
                "script": "test/pingpong.py",
                "executor_type":'pingpong',
            }

            attempt = 0
            while attempt <= retries:
                attempt += 1
                try:
                    print(f"[{time.time():.3f}][PingPongExecutor] Do Invoking pair {pair_id}...")
                    left_res, right_res = await asyncio.gather(
                        self.invoke_node(left_node.ip, params_left, detector),
                        self.invoke_node(right_node.ip, params_right, detector)
                    )
                    return {
                        "pair_id": pair_id,
                        "node_a": getattr(node_a, "hostname", None),
                        "node_a_ip": getattr(node_a, "ip", None),
                        "local_a": local_a,
                        "node_b": getattr(node_b, "hostname", None),
                        "node_b_ip": getattr(node_b, "ip", None),
                        "local_b": local_b,
                        "attempt": attempt,
                        "left_result": left_res,
                        "right_result": right_res,
                    }

                except Exception as e:
                    # catch to possibly retry
                    last_err = str(e)
                    if attempt > retries:
                        return {
                            "pair_id": pair_id,
                            "node_a": getattr(node_a, "ip", None),
                            "local_a": local_a,
                            "node_b": getattr(node_b, "ip", None),
                            "local_b": local_b,
                            "attempt": attempt,
                            "error": last_err,
                        }
                    # otherwise retry after short backoff
                    await asyncio.sleep(min(1.0 * attempt, 3.0))
            return {}

    async def invoke_node(self, ip: str, params_dict: Dict[str, Any], detector: Any) -> Any:
        print(f"[{time.time():.3f}][PingPongExecutor] Invoking node {ip}")
        # Async simulation delay
        node_result = await invoke_node(ip, params_dict, detector)
        result = {
            'success':node_result['result'],
            'original_result': node_result
        }
        print(f"[{time.time():.3f}][PingPongExecutor] Node {ip} finished with {result}")
        return result


    async def schedule(self, nodes: List[Any], detector: Any = None, concurrency_limit: int = 0,
                       retries: int = 0, shuffle_rounds: bool = True) -> Dict[str, Any]:
        """
        High-level schedule that:
          - builds all cross-node rank pairs
          - partitions them into disjoint rounds (each round is a matching)
          - for each round, invokes all pairs concurrently (optionally limited by concurrency_limit)

        Returns a dict with full per-pair results and a summary.

        Notes:
          - nodes is a list of NodeInfo-like objects. Each node must have `.ip` and `.ranks` attributes.
          - concurrency_limit (int): maximum number of *pairs* running at the same time across a round.
            If 0 or None, no explicit limit is applied (all pairs in the round are dispatched concurrently).
        """
        # index nodes for stable identification (used in pair ordering)
        for idx, n in enumerate(nodes):
            setattr(n, "_idx", idx)

        all_pairs = build_cross_node_pairs(nodes)
        if not all_pairs:
            return {"rounds": [], "summary": {"total_pairs": 0}}

        rounds = partition_into_rounds(all_pairs, shuffle=shuffle_rounds)

        overall_results = []
        pair_counter = 0

        # global semaphore controlling concurrent pairs (0 means unlimited)

        for round_idx, r_pairs in enumerate(rounds):
            print(f"[PingPongExecutor] Scheduling round {round_idx}")
            # dispatch all pairs in this round concurrently (subject to semaphore)
            tasks = []
            for (n1, lr1), (n2, lr2) in r_pairs:
                node_a = nodes[n1]
                node_b = nodes[n2]
                pair_counter += 1
                tid = pair_counter
                # Schedule using create_task
                tasks.append(
                    asyncio.create_task(
                        self._invoke_pair(node_a, node_b, lr1, lr2, tid, detector, retries=retries)
                    )
                )

            print(f"[PingPongExecutor] Scheduling round {round_idx} complete. Got {len(tasks)} tasks.")
            # Wait for all in-round tasks
            round_results = await asyncio.gather(*tasks)
            overall_results.append({"round_idx": round_idx, "pairs": round_results})

        # summarize
        total = 0
        failed = 0
        for r in overall_results:
            for p in r["pairs"]:
                total += 1
                if p.get("left_result", {}).get("success") is False or p.get("right_result", {}).get("success") is False:
                    failed += 1
                if p.get("error"):
                    failed += 1

        summary = {"total_pairs": total, "failed_pairs": failed, "rounds": len(overall_results)}
        return {"rounds_detail": overall_results, "summary": summary}


    def _run(self, peer: Peer):
        pass

# Helper unlimited semaphore (no-op context manager)
class _UnlimitedSemaphore:
    def __init__(self):
        pass

    async def __aenter__(self):
        return None

    async def __aexit__(self, exc_type, exc, tb):
        return False

    async def acquire(self):
        return True

    def release(self):
        return True

    # make it compatible with `async with semaphore:` pattern
    def __await__(self):
        async def _d():
            return None
        return _d().__await__()


# Helper types
RankID = Tuple[int, int]  # (node_index, local_rank)
Pair = Tuple[RankID, RankID]


def _normalized_pair(a: RankID, b: RankID) -> Pair:
    # deterministic ordering so pairs are hashable/unique
    return (a, b) if a <= b else (b, a)


def build_cross_node_pairs(nodes: List[Any]) -> List[Pair]:
    """
    Build all cross-node rank pairs. nodes is a list of NodeInfo-like objects that have at least:
      - .ip (str)
      - .ranks (iterable of local_rank ints)
    Returns list of unique normalized pairs ((node_idx, local_rank), (node_idx2, local_rank2)).
    """
    rank_items: List[RankID] = []
    for ni, node in enumerate(nodes):
        for lr in node.ranks:
            rank_items.append((ni, lr))

    pairs: List[Pair] = []
    n = len(rank_items)
    for i in range(n):
        for j in range(i + 1, n):
            a = rank_items[i]
            b = rank_items[j]
            if a[0] == b[0]:
                # same node -> skip
                continue
            pairs.append(_normalized_pair(a, b))
    return pairs


def partition_into_rounds(pairs: List[Pair], shuffle: bool = True) -> List[List[Pair]]:
    """
    Greedy partitioning of the set of pairs into rounds where pairs within a round are vertex-disjoint
    (no RankID appears more than once in a round). This produces a sequence of maximal matchings.

    This algorithm is simple and efficient. It does not necessarily minimize number of rounds but
    in practice is fast and produces good packings.
    """
    remaining: Set[Pair] = set(pairs)
    rounds: List[List[Pair]] = []

    while remaining:
        used: Set[RankID] = set()
        round_pairs: List[Pair] = []
        # iterate in a stable order; optionally shuffle to balance load across rounds
        candidate_list = list(remaining)
        if shuffle:
            random.shuffle(candidate_list)
        for p in candidate_list:
            a, b = p
            if a in used or b in used:
                continue
            round_pairs.append(p)
            used.add(a)
            used.add(b)
        if not round_pairs:
            # should not normally happen; break to avoid infinite loop
            break
        # remove selected pairs
        for p in round_pairs:
            remaining.remove(p)
        rounds.append(round_pairs)
    return rounds
