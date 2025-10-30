import subprocess
import torch
import torch.distributed as dist
import json

def get_ib_info():
    """
    Collect local IB HCA information
    Return list, each device contains port status and GID
    """
    try:
        result = subprocess.check_output(["ibv_devinfo"], stderr=subprocess.STDOUT).decode()
    except Exception as e:
        return {"error": str(e)}

    devices = []
    device = None
    for line in result.splitlines():
        line = line.strip()
        if line.startswith("hca_id:"):
            if device:
                devices.append(device)
            device = {"name": line.split()[1], "ports": []}
        elif line.startswith("port:"):
            port_info = {"port_num": int(line.split()[1])}
            device["ports"].append(port_info)
        elif line.startswith("state:") and device and device["ports"]:
            device["ports"][-1]["state"] = line.split()[1]
        elif line.startswith("phys_state:") and device and device["ports"]:
            device["ports"][-1]["phys_state"] = line.split()[1]
        elif line.startswith("gid:") and device and device["ports"]:
            device["ports"][-1]["gid"] = line.split()[1]
    if device:
        devices.append(device)
    return devices

def gather_ib_info():
    """
    Collect IB information on all ranks and gather to rank0
    Return complete information collected by rank0, otherwise return None
    """
    if not dist.is_initialized():
        raise RuntimeError("torch.distributed not initialized")

    rank = dist.get_rank()
    world_size = dist.get_world_size()

    local_ib_info = get_ib_info()
    # Can add rank information
    report = {"rank": rank, "ib_info": local_ib_info}

    # Use gather_object to gather to rank0
    gathered = [None for _ in range(world_size)] if rank == 0 else None
    dist.gather_object(report, gathered, dst=0)

    if rank == 0:
        # Return complete list
        return gathered
    else:
        return None

# Example usage:
# After initializing distributed environment
# dist.init_process_group(backend="nccl", init_method=...)
# ib_reports = gather_ib_info()
# if dist.get_rank() == 0:
#     print(json.dumps(ib_reports, indent=2))
