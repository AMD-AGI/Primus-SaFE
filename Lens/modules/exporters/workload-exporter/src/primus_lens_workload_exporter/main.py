import os
from primus_lens_workload_exporter.metric.collector import Collector
from primus_lens_workload_exporter.collector.collector import gpu_collector, rdma_collector


def get_rank_info():
    """
    Read distributed rank information
    Priority:
    1. SLURM_NODEID
    2. PET_NODE_RANK
    3. NODE_RANK
    4. Default 0
    """
    # Handle NODE_RANK
    if "SLURM_NODEID" in os.environ:
        node_rank = int(os.environ["SLURM_NODEID"])
    elif "PET_NODE_RANK" in os.environ:
        node_rank = int(os.environ["PET_NODE_RANK"])
    elif "NODE_RANK" in os.environ:
        node_rank = int(os.environ["NODE_RANK"])
    else:
        node_rank = 0

    rank_info = {
        "RANK": int(os.environ.get("RANK", -1)),
        "LOCAL_RANK": int(os.environ.get("LOCAL_RANK", -1)),
        "NODE_RANK": node_rank,
        "WORLD_SIZE": int(os.environ.get("WORLD_SIZE", -1)),
    }
    return rank_info


def get_config():
    config = {
        "output_path": os.environ.get("PRIMUS_LENS_OUTPUT_PATH", None),
        "interval": int(os.environ.get("PRIMUS_LENS_INTERVAL", 10)),
    }
    return config


if __name__ == "__main__":
    info = get_rank_info()
    print(f"Rank info: {info}")
    print(f"My node rank is: {info['NODE_RANK']}")
    cfg = get_config()
    c = Collector(output_path=f"{cfg['output_path']}/{info['NODE_RANK']}", interval=cfg["interval"])
    c.register_collector(gpu_collector)
    c.register_collector(rdma_collector)
    c.start()
    c.wait()
