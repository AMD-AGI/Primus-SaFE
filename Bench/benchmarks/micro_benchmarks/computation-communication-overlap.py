from itertools import product
import json
import os

import torch
import torch.distributed as dist
import torch.profiler


def benchmark(
    matmul_size,
    comm_size,
    matmul_stream,
    comm_stream,
    warmup_steps=10,
    benchmark_steps=100,
):
    torch.cuda.empty_cache()
    with torch.device("cuda"):
        A = torch.randn(*matmul_size, dtype=torch.bfloat16)
        B = torch.randn(*matmul_size, dtype=torch.bfloat16)
        comm_tensor = torch.randn(*comm_size, dtype=torch.bfloat16)

    # warmup matmul
    for _ in range(warmup_steps):
        with torch.cuda.stream(matmul_stream):
            torch.matmul(A, B)
    torch.cuda.synchronize()

    # benchmark matmul
    start_event = torch.cuda.Event(enable_timing=True)
    end_event = torch.cuda.Event(enable_timing=True)
    start_event.record(matmul_stream)
    for _ in range(benchmark_steps):
        with torch.cuda.stream(matmul_stream):
            torch.matmul(A, B)
    end_event.record(matmul_stream)
    torch.cuda.synchronize()

    matmul_time = start_event.elapsed_time(end_event) / benchmark_steps

    # warmup comm
    for _ in range(warmup_steps):
        with torch.cuda.stream(comm_stream):
            dist.all_reduce(comm_tensor)
    torch.cuda.synchronize()

    # benchmark comm
    start_event = torch.cuda.Event(enable_timing=True)
    end_event = torch.cuda.Event(enable_timing=True)
    start_event.record(comm_stream)
    for _ in range(benchmark_steps):
        with torch.cuda.stream(comm_stream):
            dist.all_reduce(comm_tensor)
    end_event.record(comm_stream)
    torch.cuda.synchronize()

    comm_time = start_event.elapsed_time(end_event) / benchmark_steps

    # warmup matmul-comm
    for _ in range(warmup_steps):
        with torch.cuda.stream(matmul_stream):
            torch.matmul(A, B)
        with torch.cuda.stream(comm_stream):
            dist.all_reduce(comm_tensor)
    torch.cuda.synchronize()

    # benchmark matmul-comm
    start_event = torch.cuda.Event(enable_timing=True)
    end_event = torch.cuda.Event(enable_timing=True)
    start_matmul_event = torch.cuda.Event(enable_timing=True)
    end_matmul_event = torch.cuda.Event(enable_timing=True)
    start_comm_event = torch.cuda.Event(enable_timing=True)
    end_comm_event = torch.cuda.Event(enable_timing=True)

    start_event.record()
    matmul_stream.wait_stream(torch.cuda.current_stream())
    comm_stream.wait_stream(torch.cuda.current_stream())

    start_matmul_event.record(matmul_stream)
    start_comm_event.record(comm_stream)

    for _ in range(benchmark_steps):
        with torch.cuda.stream(matmul_stream):
            torch.matmul(A, B)
        with torch.cuda.stream(comm_stream):
            dist.all_reduce(comm_tensor)

    end_matmul_event.record(matmul_stream)
    end_comm_event.record(comm_stream)

    torch.cuda.current_stream().wait_stream(matmul_stream)
    torch.cuda.current_stream().wait_stream(comm_stream)
    end_event.record()
    
    torch.cuda.synchronize()

    matmul_comm_time = start_event.elapsed_time(end_event) / benchmark_steps
    overlapped_matmul_time = start_matmul_event.elapsed_time(end_matmul_event) / benchmark_steps
    overlapped_comm_time = start_comm_event.elapsed_time(end_comm_event) / benchmark_steps

    return matmul_time, comm_time, matmul_comm_time, overlapped_matmul_time, overlapped_comm_time


if __name__ == "__main__":
    dist.init_process_group(backend="nccl")
    local_rank = int(os.environ["LOCAL_RANK"])
    torch.cuda.set_device(local_rank)

    rank = dist.get_rank()

    matmul_stream = torch.cuda.Stream()
    comm_stream = torch.cuda.Stream()

    matmul_sizes = [(2**i, 2**i) for i in range(14, 16)]
    comm_sizes = [(2**i, 2**i) for i in range(14, 16)]
    results = []
    # with torch.profiler.profile(
    #     activities=[torch.profiler.ProfilerActivity.CUDA, torch.profiler.ProfilerActivity.CPU],
    #     record_shapes=True,
    #     with_stack=True,
    # ) as prof:
    for matmul_size, comm_size in product(matmul_sizes, comm_sizes):
        matmul_time, comm_time, matmul_comm_time, overlapped_matmul_time, overlapped_comm_time = benchmark(
            matmul_size,
            comm_size,
            matmul_stream,
            comm_stream,
        )
        results.append({
            "matmul_size": matmul_size,
            "comm_size": comm_size,
            "matmul_time": matmul_time,
            "comm_time": comm_time,
            "matmul_comm_time": matmul_comm_time,
        })
        if rank == 0:
            print(
                f"matmul size: {matmul_size[0]}x{matmul_size[1]}, comm size: {comm_size[0]}x{comm_size[1]}",
            )
            print(f"  matmul alone:         {matmul_time:.4f}ms")
            print(f"  comm alone:           {comm_time:.4f}ms")
            print(f"  matmul + comm:        {matmul_comm_time:.4f}ms")
            print(f"  overlapped matmul:    {overlapped_matmul_time:.4f}ms")
            print(f"  overlapped comm:      {overlapped_comm_time:.4f}ms")
            print("-" * 60)

    if rank == 0:
        # prof.export_chrome_trace("amd_trace.json")
        with open("overlap_results.json", "w") as f:
            json.dump(results, f)

    dist.destroy_process_group()
