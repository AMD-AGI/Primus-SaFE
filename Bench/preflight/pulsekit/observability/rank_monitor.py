import psutil
import time
from typing import Dict, Set, Tuple


class RankMonitor:
    def __init__(self, torchrun_pid: int, check_interval: float = 2.0):
        """
        torchrun_pid: torchrun main process PID
        check_interval: monitoring interval (seconds)
        """
        self.torchrun_pid = torchrun_pid
        self.check_interval = check_interval
        self.prev_rank_pids: Set[int] = set()
        self.exit_info: Dict[str, Tuple[int, float]] = {}  # rank -> (pid, exit_timestamp)
        self.rank_map: Dict[int, str] = {}  # pid -> rank

    def _get_current_rank_pids(self) -> Set[int]:
        """Get all rank subprocess pids under torchrun"""
        try:
            parent = psutil.Process(self.torchrun_pid)
            children = parent.children(recursive=True)
            return {child.pid for child in children}
        except psutil.NoSuchProcess:
            return set()

    def _update_rank_map(self, pids: Set[int]):
        """Get rank from environment variables when process is first discovered"""
        for pid in pids:
            if pid not in self.rank_map:
                try:
                    proc = psutil.Process(pid)
                    env = proc.environ()
                    rank = env.get("RANK", f"unknown-{pid}")
                    self.rank_map[pid] = rank
                    self._log(f"[PID {pid}] detected with RANK={rank}")
                except (psutil.NoSuchProcess, psutil.AccessDenied):
                    self.rank_map[pid] = f"unknown-{pid}"
                    self._log(f"[PID {pid}] detected but failed to read RANK")

    def _log(self, msg: str):
        ts = time.strftime("%Y-%m-%d %H:%M:%S", time.localtime())
        print(f"[{ts}] {msg}")

    def _monitor_loop(self):
        while True:
            current_pids = self._get_current_rank_pids()
            self._update_rank_map(current_pids)

            # Detect exited ranks
            disappeared = self.prev_rank_pids - current_pids
            for pid in disappeared:
                rank = self.rank_map.get(pid, f"unknown-{pid}")
                if rank not in self.exit_info:
                    self.exit_info[rank] = (pid, time.time())
                    self._log(f"[Rank {rank} / PID {pid}] exited")

            # Update prev
            self.prev_rank_pids = current_pids

            # Exit condition: torchrun main process has ended and there are no subprocesses
            if not current_pids:
                try:
                    psutil.Process(self.torchrun_pid)
                except psutil.NoSuchProcess:
                    break

            time.sleep(self.check_interval)

    def run(self) -> Dict[str, Tuple[int, float]]:
        """Start monitoring, return rank -> (pid, exit_time)"""
        self.prev_rank_pids = self._get_current_rank_pids()
        self._update_rank_map(self.prev_rank_pids)
        self._monitor_loop()
        return self.exit_info


# -----------------------
# Example usage
# -----------------------
if __name__ == "__main__":
    torchrun_pid = 12345  # Actual torchrun PID
    monitor = RankMonitor(torchrun_pid)
    exit_info = monitor.run()
    print("Exit info:", exit_info)
