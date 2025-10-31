import os
import time
import signal
import torch
import torch.distributed as dist
from rdma.core import dist_utils
from rdma.core.logger import get_logger

logger = get_logger()

# Timeout protection
class TimeoutError(Exception):
    pass

def timeout_handler(signum, frame):
    raise TimeoutError("Operation timed out")

def allreduce(
        sizes_mb=None,
    iters=10,
    timeout=30.0,
):
    if sizes_mb is None:
        sizes_mb = [1, 16, 64, 256]
    elif isinstance(sizes_mb, (int, float, str)):
        sizes_mb = [int(sizes_mb)]
    else:
        sizes_mb = list(sizes_mb)
    rank = dist_utils.get_rank()
    world_size = dist_utils.get_world_size()
    device = torch.device("cuda", rank % torch.cuda.device_count())

    results = []

    for size_mb in sizes_mb:
        num_elements = size_mb * 1024 * 1024 // 4  # float32
        tensor = torch.ones(num_elements, dtype=torch.float32, device=device)

        # warmup
        for _ in range(3):
            dist.all_reduce(tensor)
            torch.cuda.synchronize()

        times = []
        for i in range(iters):
            try:

                # Set timeout
                signal.signal(signal.SIGALRM, timeout_handler)
                signal.alarm(int(timeout))

                torch.cuda.synchronize()
                t0 = time.time()
                dist.all_reduce(tensor)
                torch.cuda.synchronize()
                t1 = time.time()

                times.append(t1 - t0)
                signal.alarm(0)  # Cancel timeout

            except TimeoutError:
                times.append(float("inf"))
                if rank == 0:
                    print(f"[WARN] Rank {rank} timeout at {size_mb}MB iter {i}")

        # Statistics
        valid_times = [t for t in times if t != float("inf")]
        avg = sum(valid_times) / len(valid_times) if valid_times else float("inf")
        bw = size_mb / avg if avg != float("inf") else 0.0
        std = (sum((t-avg)**2 for t in valid_times)/len(valid_times))**0.5 if valid_times else float("inf")

        results.append((rank, size_mb, avg, bw, std))

    # Collect results
    gathered = [None for _ in range(world_size)]
    dist.all_gather_object(gathered, results)

    return gathered

