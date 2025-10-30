# pingpong.py
import argparse
import json
import sys
import time
import torch
import torch.distributed as dist


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--master_addr", required=True)
    parser.add_argument("--master_port", required=True)
    parser.add_argument("--rank", type=int, required=True)
    parser.add_argument("--local_rank", type=int, required=True)
    args = parser.parse_args()

    # Initialize process group
    dist.init_process_group(
        backend="nccl",
        init_method=f"tcp://{args.master_addr}:{args.master_port}",
        world_size=2,
        rank=args.rank,
    )

    device = torch.device(f"cuda:{args.local_rank}")
    tensor = torch.ones(1, device=device)

    try:
        if args.rank == 0:
            t0 = time.time()
            dist.send(tensor, dst=1)
            dist.recv(tensor, src=1)
            t1 = time.time()
            print(json.dumps({
                "success": True,
                "latency_us": (t1 - t0) * 1e6
            }))
        elif args.rank == 1:
            dist.recv(tensor, src=0)
            dist.send(tensor, dst=0)

        dist.destroy_process_group()
        sys.exit(0)
    except Exception as e:
        print(json.dumps({
            "success": False,
            "error": str(e)
        }))
        dist.destroy_process_group()
        sys.exit(1)

if __name__ == "__main__":
    main()