# Copyright (c) Meta Platforms, Inc. and affiliates.
# All rights reserved.
#
# This source code is licensed under the BSD-style license found in the
# LICENSE file in the root directory of this source tree.

import asyncio
import ctypes
import logging
import os
import random
from enum import Enum
from typing import Dict, TYPE_CHECKING

import torch
from monarch.actor import Actor, current_rank, endpoint

logger = logging.getLogger()

if TYPE_CHECKING:
    from ..train_distributed import MonarchSlurm, Replica


class Failure(Enum):
    """
    A set of supported failures injected by FailureController
    """

    SEGFAULT = 0
    KILL_PROC = 1
    COMMS = 2
    KILL_SLURM = 3
    DEADLOCK = 4


class FailureActor(Actor):
    def _execute_failure(self, failure: Failure) -> None:
        rank = current_rank().rank
        logger.info(f"[FailureController] {rank} chosen to fail with {failure}")
        match failure:
            case Failure.SEGFAULT:
                self.segfault()
            case Failure.KILL_PROC:
                self.kill_proc()
            case Failure.COMMS:
                self.kill_comms()
            case Failure.DEADLOCK:
                self.deadlock()

    @endpoint
    def fail(self, failure: Failure) -> None:
        self._execute_failure(failure)

    @endpoint
    def fail_if_rank(self, target_rank: int, failure: Failure) -> None:
        rank = current_rank().rank
        if rank != target_rank:
            return
        self._execute_failure(failure)

    def segfault(self):
        """
        Triggers a SIGSEGV on the process
        """
        crash_func = ctypes.CFUNCTYPE(None)()
        crash_func()

    def kill_proc(self):
        """
        Immediately kills the process with non-zero exit code
        """
        os._exit(1)

    def kill_comms(self):
        """
        Forcefully aborts the ProcessGroup and NCCL communicator
        """
        torch.distributed.distributed_c10d._abort_process_group()

    def deadlock(self):
        """
        Deadlocks the GIL, resulting in ProcessGroupNCCL timeout and terminal failure
        """
        libc = ctypes.PyDLL(None)
        libc.sleep.argtypes = tuple([ctypes.c_uint])
        libc.sleep.restype = ctypes.c_uint
        libc.sleep(70)


class FailureController:
    @staticmethod
    def kill_slurm(scheduler: "MonarchSlurm") -> None:
        """
        Kills a random replica SLURM job
        """
        candidates = [
            mesh_name
            for mesh_name in scheduler.job_handles.keys()
            if "replica_" in mesh_name
        ]
        selected = random.choice(candidates)
        logger.info(f"[FailureController] Killing SLURM job for {selected}")
        scheduler.kill_job(selected)

    @staticmethod
    async def execute_failures(
        replicas: Dict[int, "Replica"],
        scheduler: "MonarchSlurm",
        startup_wait: int = 600,
        rest_time: int = 120,
    ) -> None:
        logger.info(
            f"[FailureController] Starting failure injection in {startup_wait}s, "
            f"then every {rest_time}s"
        )
        await asyncio.sleep(startup_wait)

        while True:
            if not replicas:
                return

            # Filter to replicas that are healthy and past the post-restart grace period.
            healthy = []
            for replica in replicas.values():
                if replica.rid == 0:
                    continue
                try:
                    if await replica.actor.is_healthy.call_one():
                        healthy.append(replica)
                except Exception:
                    pass  # replica actor may be temporarily unreachable mid-restart

            if not healthy:
                logger.info(
                    f"[FailureController] No healthy replicas available, "
                    f"skipping this cycle (retrying in {rest_time}s)"
                )
                await asyncio.sleep(rest_time)
                continue

            try:
                selected_replica = random.choice(healthy)
                selected_failure = random.choice(
                    [
                        Failure.COMMS,
                        # Failure.DEADLOCK,
                        # Failure.SEGFAULT,
                        # Failure.KILL_PROC,
                        # Failure.KILL_SLURM,
                    ]
                )
                try:
                    if selected_failure == Failure.KILL_SLURM:
                        FailureController.kill_slurm(scheduler)
                    else:
                        selected_replica.actor.inject_failure.broadcast(selected_failure)
                    logger.info(
                        f"[FailureController] Failure injection ({selected_failure}) "
                        f"sent to replica {selected_replica.rid}"
                    )
                except Exception as e:
                    logger.exception(
                        f"[FailureController] Failed to inject failure in replica "
                        f"{selected_replica.rid}: {e}"
                    )
            except Exception as e:
                logger.exception(
                    f"[FailureController] Something went wrong while injecting failure: {e}"
                )

            await asyncio.sleep(rest_time)
