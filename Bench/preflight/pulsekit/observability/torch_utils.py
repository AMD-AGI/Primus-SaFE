# torch_utils.py
import subprocess
from typing import List, Dict
from rdma import RankMonitor  # Your existing RankMonitor


def launch_and_monitor_torchrun(
    script_path: str,
    nproc_per_node: int,
    extra_args: List[str] = None,
    check_interval: float = 1.0
) -> Dict[int, float]:
    """
    Launch a torchrun task and start RankMonitor to monitor the exit status of all rank processes.

    Args:
        script_path (str): Path to the training script to run
        nproc_per_node (int): Number of processes per node
        extra_args (List[str], optional): Extra arguments, e.g. ["--arg1", "value1"]
        check_interval (float, optional): RankMonitor check interval, default 2.0 seconds

    Returns:
        Dict[int, float]: rank pid -> exit timestamp
    """
    cmd = ["torchrun", f"--nproc_per_node={nproc_per_node}", script_path]
    if extra_args:
        cmd.extend(extra_args)

    # Start torchrun subprocess
    proc = subprocess.Popen(cmd)
    print(f"[Launcher] torchrun started with PID {proc.pid}")

    # Start rank monitor
    monitor = RankMonitor(proc.pid, check_interval=check_interval)
    exit_info = monitor.run()

    # Wait for torchrun to fully exit
    proc.wait()
    print("[Launcher] torchrun finished.")

    return exit_info


# -------------------
# Example usage
# -------------------
if __name__ == "__main__":
    exit_info = launch_and_monitor_torchrun(
        script_path="train.py",  # Replace with your training script
        nproc_per_node=2,
        extra_args=["--epochs", "10"],
        check_interval=1.0
    )

    print("Rank processes exited with timestamps:", exit_info)
