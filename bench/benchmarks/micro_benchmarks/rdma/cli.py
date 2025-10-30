#!/usr/bin/env python3
# cli.py - project root directory

import argparse
import os
import sys
import json
from rdma.core import dist_utils
from rdma.bandwidth import allreduce
from rdma.bandwidth import alltoall
from rdma.sanity import pingpong
# from pulsekit.bandwidth import bandwidth_test
# from pulsekit.simulation import simulation_test
from rdma.core.logger import get_logger
from rdma.core import config  # Assume config.py defines OUTPUT_DIR
from rdma.flashattn.flashattn import run_flash_attention

logger = get_logger("cli")


def main():
    parser = argparse.ArgumentParser(
        description="PulseKit: GPU cluster communication test tool"
    )
    parser.add_argument(
        "--sanity", action="store_true", help="Run Sanity check"
    )
    parser.add_argument(
        "--bandwidth", action="store_true", help="Run Bandwidth test"
    )
    parser.add_argument(
        "--simulation", action="store_true", help="Run Simulation test (large model communication simulation)"
    )
    parser.add_argument(
        "--sizes-mb", type=int, nargs="+", default=[1, 16, 64, 256],
        help="Test tensor size (MB) list"
    )
    parser.add_argument(
        "--iters", type=int, default=20, help="Number of iterations to test for each size"
    )
    parser.add_argument(
        "--timeout", type=float, default=30.0, help="Single communication timeout (seconds)"
    )
    parser.add_argument(
        "--job-id", default="0",help="Job id, used to distinguish output files"
    )
    args = parser.parse_args()

    # Initialize distributed
    dist_utils.init_distributed()
    rank = dist_utils.get_rank()
    world_size = dist_utils.get_world_size()

    if rank == 0:
        logger.info(f"Running tests on {world_size} ranks")
        logger.info(f"ENV: {os.environ.copy()}")

    # Collect all results
    results = {
        "world_size": world_size,
        "tests": {},
        "topology": dist_utils.get_rank_info(),
        "ib_info": dist_utils.get_ib_info(),
    }

    # Sanity test
    if args.sanity:
        if rank == 0:
            logger.info("Starting Sanity Test...")
        result = pingpong.mini_tensor_ping()
        if rank == 0:
            results["tests"]["sanity"] = result
            logger.info("Bandwidth Test Finished")

    # Bandwidth test
    if args.bandwidth:
        if rank == 0:
            logger.info("Starting Bandwidth Test...")
        # bandwidth_results = bandwidth_test.run(sizes_mb=args.sizes_mb, iters=args.iters)
        all2all_results = alltoall.run(iters=args.iters, timeout=args.timeout)
        allreduce_results = allreduce.allreduce(iters=args.iters, timeout=args.timeout)
        if rank == 0:
            results["tests"]["bandwidth"] = {
                "alltoall": all2all_results,
                "allreduce": allreduce_results,
            }
            logger.info("Bandwidth Test Finished")
        

    # Simulation test
    if args.simulation:
        if rank == 0:
            logger.info("Starting Simulation Test...")
        # simulation_results = simulation_test.run()
        simulation_results = {"dummy": "not implemented"}  # placeholder
        if rank == 0:
            results["tests"]["simulation"] = simulation_results
            logger.info("Simulation Test Finished")

    logger.info("Starting test flash attn")
    flash_attn_result = run_flash_attention()
    results["tests"]["flash_attn"] = flash_attn_result

    # Cleanup
    dist_utils.cleanup_dist()

    

    # Output JSON only on rank0
    if rank == 0:
        os.makedirs(config.config.output_dir, exist_ok=True)
        output_file = os.path.join(config.config.output_dir, f"{args.job_id}.json")
        with open(output_file, "w") as f:
            json.dump(results, f, indent=2)
        os.chmod(output_file, 0o666)  # rw-rw-rw-
        logger.info(f"All tests completed. Results saved to {output_file}")


if __name__ == "__main__":
    main()
