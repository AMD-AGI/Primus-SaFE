#  Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
#  See LICENSE for license information.
import sys
import torch.distributed as dist
import os
from datetime import timedelta

def main():
    os.environ['MASTER_ADDR'] = os.getenv('MASTER_ADDR')
    os.environ['MASTER_PORT'] = os.getenv('MASTER_PORT')
    os.environ['WORLD_SIZE'] = os.getenv('WORLD_SIZE')
    os.environ['RANK'] = os.getenv('RANK')
    os.environ['TORCH_DISTRIBUTED_DEFAULT_TIMEOUT'] = os.getenv('TORCH_DISTRIBUTED_DEFAULT_TIMEOUT', '3600')

    dist.init_process_group(
        backend="gloo",
        init_method="env://",
        timeout=timedelta(hours=3)
    )
    rank = dist.get_rank()

    try:
        dist.barrier()
        print(f"[NODE-{rank}] Barrier passed. Diagnosis complete. Exiting.")
    except Exception as e:
        print(f"[NODE-{rank}] Barrier timeout or error: {e}")
        sys.exit(1)
    finally:
        dist.destroy_process_group()

if __name__ == "__main__":
    main()