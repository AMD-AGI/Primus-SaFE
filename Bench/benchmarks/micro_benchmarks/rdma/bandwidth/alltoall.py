# sanity/alltoall.py
import torch
import torch.distributed as dist
import time
from rdma.core import dist_utils
from rdma.core.logger import get_logger

logger = get_logger("sanity.alltoall")

def run(iters=50, tensor_size_mb=256, timeout=30.0):
    sizes = [tensor_size_mb * 1024 * 1024]
    rank = dist_utils.get_rank()
    world_size = dist_utils.get_world_size()
    rank_info = dist_utils.get_rank_info()
    logger.info(f"Start all-to-all P2P test. rank={rank}/{world_size}, iters={iters}, timeout={timeout}")

    local_world_size = torch.cuda.device_count()
    device_index = rank % torch.cuda.device_count()
    num_nodes = world_size // torch.cuda.device_count()
    device = torch.device(f"cuda:{device_index}")

        # ---------- deterministic cases (avoid list(set(...)) non-determinism) ----------
    # prefer explicit order: small groups first, then full num_nodes
    cases = {
        "allreduce": [2, 4, num_nodes],
        "alltoall":  [2, 4, num_nodes],
    }

    # Result structure: comm -> adjacent_nodes -> [group results]
    final_results = {}

    for comm, adjacent_node_list in cases.items():
        if rank == 0:
            logger.info(f"Start {comm} ")

        final_results.setdefault(comm, {})

        for adjacent_nodes in adjacent_node_list:
            if adjacent_nodes > num_nodes:
                continue

            num_procs = adjacent_nodes * local_world_size
            num_adjacent_groups = num_nodes // adjacent_nodes
            if rank == 0:
                logger.info(f"Start {comm} with {adjacent_nodes}")
            for i_group in range(num_adjacent_groups):
                group_ranks = [
                    i_group * adjacent_nodes * local_world_size + r
                    for r in range(adjacent_nodes * local_world_size)
                ]
                # IMPORTANT: all ranks must call new_group in the same order
                tmp_group = dist.new_group(ranks=group_ranks)
                adjacent_group = tmp_group if rank in group_ranks else None

                group_results = []

                for size in sizes:
                    if adjacent_group is None:
                        # not part of this group: skip measurement
                        break

                    # create tensors carefully
                    # size is bytes; create number of elements so that total bytes ~= size
                    # bfloat16 is 2 bytes, so num_elems = size // 2
                    num_elems = size // 2
                    # ensure per-rank chunk divisibility
                    if num_elems % num_procs != 0:
                        # reduce num_elems to nearest divisible by num_procs
                        num_elems = (num_elems // num_procs) * num_procs
                    per_rank_elems = num_elems // num_procs

                    if per_rank_elems == 0:
                        raise RuntimeError(f"per_rank_elems==0 for size {size} num_procs {num_procs}")

                    # send/recv tensors (do NOT use same tensor for input and output)
                    send = torch.rand(per_rank_elems * num_procs, dtype=torch.bfloat16, device=device)
                    recv = torch.empty_like(send)

                    dist.barrier(group=adjacent_group, device_ids=[torch.cuda.current_device()])

                    # warmup
                    for _ in range(10):
                        if comm == "allreduce":
                            dist.all_reduce(send, group=adjacent_group)
                        elif comm == "alltoall":
                            # use all_to_all_single with separate buffers
                            dist.all_to_all_single(recv, send, group=adjacent_group)
                    torch.cuda.synchronize()

                    # measurements
                    latency_list = []
                    bandwidth_list = []
                    size_key = f"{size//1024//1024}MB"
                    nodes = list(dict.fromkeys([rank_info[r]['hostname'] for r in group_ranks]))
                    try:
                        for _ in range(iters):
                            start = time.time()
                            if comm == "allreduce":
                                dist.all_reduce(send, group=adjacent_group)
                            elif comm == "alltoall":
                                dist.all_to_all_single(recv, send, group=adjacent_group)
                            torch.cuda.synchronize()
                            elapsed = time.time() - start
                            latency_list.append(elapsed * 1e6)

                            scale = 2 if comm == "allreduce" else 1
                            comm_size = scale * size * (num_procs - 1) / num_procs
                            gb_per_sec = comm_size / elapsed / 1e9
                            bandwidth_list.append(gb_per_sec)
                    except RuntimeError as e:
                        err_msg = str(e)
                        logger.error(f"Rank {rank} failed in {comm} group {i_group} nodes={nodes}: {err_msg}")
                        group_results.append({
                            "rank": rank,
                            "comm": comm,
                            "group": i_group,
                            "nodes": nodes,
                            "error": err_msg,
                        })
                        continue

                    group_results.append({
                        "group": i_group,
                        "nodes": nodes,
                        "size": size_key,
                        "latency": latency_list,
                        "bandwidth": bandwidth_list,
                    })

                # synchronize all ranks (global barrier)
                dist.barrier(device_ids=[torch.cuda.current_device()])

                if adjacent_group is not None:
                    dist.destroy_process_group(adjacent_group)

                # gather results to rank 0
                gathered = [None for _ in range(world_size)]
                dist.gather_object(group_results, gathered if rank == 0 else None, dst=0)

                if rank == 0:
                    key = str(adjacent_nodes)
                    final_results[comm].setdefault(key, [])
                    for g in gathered:
                        if g:
                            final_results[comm][key].extend(g)

    if rank == 0:
        return final_results
