import torch
import torch.distributed as dist
import time

def mini_tensor_ping(group=None, tensor_size=1024):
    """Test group communication with small tensor"""
    rank = dist.get_rank()
    world_size = dist.get_world_size()
    device = torch.device(f"cuda:{rank % torch.cuda.device_count()}")
    
    # Small tensor
    tensor = torch.ones(tensor_size, dtype=torch.float32, device=device) * rank
    result = {"rank": rank, "success": True, "errors": []}
    
    for peer in range(world_size):
        try:
            if peer == rank:
                continue
            # All ranks call all_to_all_single with same tensor size
            dist.barrier()
            dist.all_to_all_single(tensor, tensor, group=group)
            torch.cuda.synchronize()
        except Exception as e:
            result["success"] = False
            result["errors"].append({"peer": peer, "msg": str(e)})
    
    return result
