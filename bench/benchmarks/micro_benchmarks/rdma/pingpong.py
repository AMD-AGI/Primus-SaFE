import torch
import torch.distributed as dist
import argparse
import socket

def parse_args():
    parser = argparse.ArgumentParser()
    parser.add_argument("--master_addr", type=str, default="127.0.0.1", help="Master node address")
    parser.add_argument("--master_port", type=str, default="29500", help="Master node port")
    return parser.parse_args()

def pingpong_offset(rank, world_size, offset):
    hostname = socket.gethostname()
    tensor = torch.zeros(1).cuda()
    group = rank // offset

    if group % 2 == 0:
        peer = rank + offset
    else:
        peer = rank - offset

    if peer < 0 or peer >= world_size:
        print(f"[Rank {rank}][{hostname}] No peer to ping-pong with")
        return

    print(f"[Rank {rank}][{hostname}] Ping-pong with Rank {peer} started")
    if rank < peer:
        tensor[0] = rank
        dist.send(tensor, dst=peer)
    else:
        dist.recv(tensor, src=peer)
    print(f"[Rank {rank}][{hostname}] Ping-pong with Rank {peer} succeeded, received {tensor.item()}")

def main():
    args = parse_args()

    dist.init_process_group(
        backend="nccl",
    )

    rank = dist.get_rank()
    world_size = dist.get_world_size()
    torch.cuda.set_device(rank % torch.cuda.device_count())

    print(f"[Rank {rank}]Starting ping-pong test with offset {torch.cuda.device_count()}...")
    pingpong_offset(rank, world_size, torch.cuda.device_count())

    dist.destroy_process_group()
    print(f"[Rank {rank}] Ping-pong test finished successfully.")

if __name__ == "__main__":
    main()