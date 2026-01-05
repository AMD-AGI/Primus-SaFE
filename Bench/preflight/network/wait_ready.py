#  Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
#  See LICENSE for license information.
import sys
import signal
import torch.distributed as dist
import os
from datetime import timedelta, datetime

# Global flag for signal received
_signal_received = False

def sigusr1_handler(signum, frame):
    """Handler for SIGUSR1 signal from run.sh"""
    global _signal_received
    _signal_received = True

def wait_for_signal():
    """
    Rank 0 waits for SIGUSR1 signal from run.sh.
    Uses signal.pause() which is more efficient than polling.
    """
    global _signal_received
    
    # Register signal handler
    signal.signal(signal.SIGUSR1, sigusr1_handler)
    
    # Wait for signal (blocks until signal received)
    while not _signal_received:
        signal.pause()

def main():
    os.environ['MASTER_ADDR'] = os.getenv('MASTER_ADDR')
    os.environ['MASTER_PORT'] = os.getenv('MASTER_PORT')
    os.environ['WORLD_SIZE'] = os.getenv('WORLD_SIZE')
    os.environ['RANK'] = os.getenv('RANK')
    os.environ['TORCH_DISTRIBUTED_DEFAULT_TIMEOUT'] = os.getenv('TORCH_DISTRIBUTED_DEFAULT_TIMEOUT', '3600')
    
    rank = int(os.getenv('RANK', '0'))
    use_signal = os.getenv('USE_SIGNAL', 'false').lower() == 'true'

    # Step 1: Create TCPStore first (rank 0 is master)
    # Other ranks can connect immediately without backoff
    dist.init_process_group(
        backend="gloo",
        init_method="env://",
        timeout=timedelta(hours=3)
    )

    # Step 2: Rank 0 waits for SIGUSR1 signal AFTER init but BEFORE barrier
    # This ensures TCPStore exists for other ranks to connect
    if rank == 0 and use_signal:
        wait_for_signal()
    
    # Step 3: All ranks hit barrier together
    try:
        dist.barrier()
        print(f"[NODE-{rank}] Barrier passed. Exiting. {datetime.now().strftime('%Y-%m-%d %H:%M:%S')}")
    except Exception as e:
        print(f"[NODE-{rank}] Barrier error: {e}")
        sys.exit(1)
    finally:
        dist.destroy_process_group()

if __name__ == "__main__":
    main()