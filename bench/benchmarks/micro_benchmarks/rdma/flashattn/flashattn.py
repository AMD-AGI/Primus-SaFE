from flash_attn import flash_attn_func
import torch
import torch.distributed as dist
import socket
from rdma.core import dist_utils


def run_flash_attention(iteration=50):
    rank = dist_utils.get_rank()
    world_size = dist_utils.get_world_size()
    rank_info = dist_utils.get_rank_info()
    local_world_size = torch.cuda.device_count()
    device_index = rank % torch.cuda.device_count()
    num_nodes = world_size // torch.cuda.device_count()
    device = torch.device(f"cuda:{device_index}")

    sizes = [1024, 2048, 4096]  # Can be extended based on actual situation
    latency_results = {}
    flops_results = {}
    batch_size = 128
    num_head_q = 64
    num_head_kv = 64
    head_dim_qk = 64
    head_dim_v = 64
    causal = True
    for seq_len in sizes:
        fwd_tflops, fwd_time, bwd_tflops, bwd_time = flash_attention_profile(
            iteration = 50,
            batch_size=batch_size,
            seq_len=seq_len,
            num_head_q=num_head_q,
            num_head_kv=num_head_kv,
            head_dim_qk=head_dim_qk,
            head_dim_v=head_dim_v,
            causal=causal,
            dtype=torch.bfloat16,
            device=f"cuda:{device_index}",  # Multi-GPU support
        )

        key = f"{seq_len}_h{num_head_q}_d{head_dim_qk}"
        latency_results[key] = {
            "fwd_time": fwd_time,
            "bwd_time": bwd_time
        }
        flops_results[key] = {
            "fwd_tflops": fwd_tflops,
            "bwd_tflops": bwd_tflops
        }
        local_result = {
            'hostname': socket.gethostname(),
            'latency': latency_results,
            'tflops': flops_results
        }

    all_results = [None for _ in range(world_size)]
    dist.gather_object(local_result, all_results if rank ==0 else None, dst=0)

    if rank == 0:
        print("=======Flash Attention=======")
        json_output = []
        for r, result in enumerate(all_results):
            node_id = r // local_world_size
            entry = {
                "hostname": result["hostname"],
                "node_id": node_id,
                "rank": r,
                "tflops": result["tflops"],
                "latency_us": result["latency"],
            }
            json_output.append(entry)
        return json_output
    return None


def flash_attention_profile(
    iteration,
    batch_size,
    seq_len,
    num_head_q,
    num_head_kv,
    head_dim_qk,
    head_dim_v,
    causal,
    dtype=torch.bfloat16,
    device="cuda:0",
):
    #
    causal = causal

    #
    q = torch.randn(
        (batch_size, seq_len, num_head_q, head_dim_qk),
        dtype=dtype,
        device=device,
        requires_grad=True,
    )
    k = torch.randn(
        (batch_size, seq_len, num_head_kv, head_dim_v),
        dtype=dtype,
        device=device,
        requires_grad=True,
    )
    v = torch.randn(
        (batch_size, seq_len, num_head_kv, head_dim_v),
        dtype=dtype,
        device=device,
        requires_grad=True,
    )
    o = torch.randn(
        (batch_size, seq_len, num_head_q, head_dim_v),
        dtype=dtype,
        device=device,
    )
    o_grad = torch.randn_like(o)

    tflop_fwd = 2 * batch_size * seq_len * seq_len * num_head_q * (head_dim_qk + head_dim_v) / 1e12
    if causal is True:
        tflop_fwd = tflop_fwd * 0.5
    tflop_bwd = tflop_fwd * 2.5

    # Cuda Event
    start_event = torch.cuda.Event(enable_timing=True)
    end_event = torch.cuda.Event(enable_timing=True)
    # warm up
    for _ in range(10):
        q.grad = None
        k.grad = None
        v.grad = None
        o = flash_attn_func(
            q,
            k,
            v,
            causal=causal,
        )
        o.backward(o_grad)
    torch.cuda.synchronize()
    # FWD
    start_event.record()
    for _ in range(iteration):
        q.grad = None
        k.grad = None
        v.grad = None
        o = flash_attn_func(
            q,
            k,
            v,
            causal=causal,
        )
    end_event.record()
    torch.cuda.synchronize()
    fwd_time = start_event.elapsed_time(end_event) / iteration / 1000
    fwd_tflops = tflop_fwd / fwd_time

    # FWD + BWD
    start_event.record()
    for _ in range(iteration):
        q.grad = None
        k.grad = None
        v.grad = None
        o = flash_attn_func(
            q,
            k,
            v,
            causal=causal,
        )
        o.backward(o_grad)
    end_event.record()
    torch.cuda.synchronize()
    fwd_bwd_time = start_event.elapsed_time(end_event) / iteration / 1000
    bwd_time = fwd_bwd_time - fwd_time
    bwd_tflops = tflop_bwd / bwd_time
    return fwd_tflops, fwd_time, bwd_tflops, bwd_time

