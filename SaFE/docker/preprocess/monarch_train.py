# Copyright (c) Meta Platforms, Inc. and affiliates.
# All rights reserved.
#
# This source code is licensed under the BSD-style license found in the
# LICENSE file in the root directory of this source tree.

import argparse
import asyncio
import atexit
import os
import random
import socket
import sys
import threading
import time
import traceback
from copy import deepcopy
from dataclasses import dataclass
from typing import Dict

import torch
from monarch.actor import Actor, current_rank, endpoint, HostMesh, MeshFailure, ProcMesh, this_host, enable_transport
from monarch.job.kubernetes import KubernetesJob
from monarch.spmd import setup_torch_elastic_env_async
from monarch.tools.network import AddrType
from torchtitan.config import ConfigManager
from torchtitan.tools.logging import init_logger, logger
from torchtitan.trainer import Trainer

# FaultTolerantTrainer may not exist in all versions of torchtitan
try:
    from torchtitan.experiments.ft.trainer import FaultTolerantTrainer
    HAS_FT_TRAINER = True
except ImportError:
    FaultTolerantTrainer = Trainer  # type: ignore
    HAS_FT_TRAINER = False

sys.path.insert(0, "/shared-data")
from utils.failure import Failure, FailureActor, FailureController


def _mesh_name(replica_id: int) -> str:
    """Build mesh name from MONARCH_MESH_PREFIX env var (set by dispatcher)
    or fall back to the legacy 'replica_<id>' convention."""
    prefix = os.environ.get("MONARCH_MESH_PREFIX", "")
    if prefix:
        return f"{prefix}{replica_id + 1}"
    return f"replica_{replica_id}"


# ==== Allocation boilerplate (Kubernetes) ====
class MonarchKubernetes:
    """Manages KubernetesJob instances for replica meshes."""
    job_name_prefix: str = "monarch-torchft"

    def __init__(
        self,
        namespace: str = "monarch-tests",
        gpus_per_node: int = 8,
    ) -> None:
        self.namespace = namespace
        self.gpus_per_node = gpus_per_node
        self.job_handles: Dict[str, KubernetesJob] = {}
        atexit.register(self.kill_jobs)

    def _normalize_mesh_name(self, mesh_name: str) -> str:
        """Convert mesh name to valid Kubernetes format (lowercase alphanumeric only)."""
        return mesh_name.replace("_", "")

    async def get_or_create_job(
        self, mesh_name: str, nodes_per_mesh: int = 1, gpus_per_node: int = 8
    ) -> None:
        """Create or reuse a KubernetesJob for the given mesh.
        
        In Kubernetes mode with pre-provisioned worker pods (via worker_meshes.yaml),
        this connects to existing MonarchMesh CRDs rather than creating new pods.
        """
        if mesh_name in self.job_handles:
            logger.info(f"Reusing existing job for mesh {mesh_name}")
            return

        k8s_mesh_name = self._normalize_mesh_name(mesh_name)
        job = KubernetesJob(namespace=self.namespace)
        job.add_mesh(k8s_mesh_name, num_replicas=nodes_per_mesh)
        self.job_handles[mesh_name] = job
        logger.info(f"Created Kubernetes job for mesh {mesh_name} (k8s: {k8s_mesh_name})")

    def kill_jobs(self):
        for mesh_name in list(self.job_handles.keys()):
            self.kill_job(mesh_name)

    def kill_job(self, mesh_name: str):
        try:
            if mesh_name not in self.job_handles:
                return
            job = self.job_handles[mesh_name]
            logger.info(f"Destroying job for mesh {mesh_name}")
            job.kill()
            del self.job_handles[mesh_name]
        except Exception as e:
            logger.exception(f"Failed to destroy job for {mesh_name}: {e}")

    def proc_mesh(self, mesh_name: str, num_procs: int) -> ProcMesh:
        job = self.job_handles[mesh_name]
        k8s_mesh_name = self._normalize_mesh_name(mesh_name)
        # Use per-mesh cache path to avoid race conditions when multiple replicas
        # try to access the same cache file concurrently
        cache_path = f".monarch/job_state_{k8s_mesh_name}.pkl"
        mesh: HostMesh = getattr(job.state(cached_path=cache_path), k8s_mesh_name)
        proc_mesh = mesh.spawn_procs({"gpus": num_procs})
        return proc_mesh


# ==== allocation boilerplate (Kubernetes) ====
class LighthouseActor(Actor):
    def __init__(self) -> None:
        self.lighthouse = None
    @endpoint
    def start_lighthouse(self, min_replicas: int = 1) -> str:
        # inline import because of https://github.com/meta-pytorch/monarch/issues/804
        from torchft.coordination import LighthouseServer
        self.lighthouse = LighthouseServer(
            bind="[::]:0",
            min_replicas=min_replicas,
            # Must be >= model initialization time so slow replicas aren't
            # excluded before they can join quorum. 10 min covers 8B models.
            join_timeout_ms=15000,
        )
        addr = self.lighthouse.address()
        # Replace hostname with IP so mesh workers on other nodes can resolve it
        import re, socket
        m = re.match(r"(https?://)([^:]+)(:\d+.*)", addr)
        if m:
            try:
                ip = socket.gethostbyname(m.group(2))
                addr = f"{m.group(1)}{ip}{m.group(3)}"
            except socket.gaierror:
                pod_ip = os.environ.get("POD_IP", "")
                if pod_ip:
                    addr = f"{m.group(1)}{pod_ip}{m.group(3)}"
        logger.info(f"[Lighthouse] Started with min_replicas={min_replicas}, address={addr}")
        return addr
    @endpoint
    def stop_lighthouse(self) -> None:
        if not self.lighthouse:
            raise RuntimeError("Lighthouse not started!")
        self.lighthouse.shutdown()
class TrainingActor(Actor):
    def __init__(
        self,
        job_config: Trainer.Config,
        replica_id: int,
        supervisor: "ReplicaActor",
        generation: int,
        wandb_project: str | None,
        wandb_group: str,
        wandb_run_prefix: str,
        run_start_time_epoch: float,
    ) -> None:
        self.job_config = job_config
        self.supervisor = supervisor
        self.generation = generation
        self.replica_id = replica_id
        self.wandb_project = wandb_project
        self.wandb_group = wandb_group
        self.wandb_run_prefix = wandb_run_prefix
        self.run_start_time_epoch = run_start_time_epoch
        rank = current_rank().rank
        self.uid = f"[replica_{replica_id}_trainer_{rank}]"

    def _get_real_ip(self) -> str | None:
        """Get the real IP address of this machine (not loopback)."""
        try:
            # Connect to external address to determine real IP
            with socket.socket(socket.AF_INET, socket.SOCK_DGRAM) as s:
                s.connect(("8.8.8.8", 80))
                return s.getsockname()[0]
        except Exception:
            pass
        # Fallback: try to resolve hostname
        try:
            hostname = socket.gethostname()
            ip = socket.gethostbyname(hostname)
            if not ip.startswith("127."):
                return ip
        except Exception:
            pass
        return None

    def _configure_wandb_env(self, rank: int) -> None:
        if self.wandb_project:
            os.environ["WANDB_PROJECT"] = self.wandb_project
        os.environ["WANDB_RUN_GROUP"] = self.wandb_group
        os.environ["WANDB_RUN_JOB_TYPE"] = "monarch-replica"
        if self.run_start_time_epoch > 0:
            os.environ["TORCHTITAN_RUN_START_TIME_EPOCH"] = str(
                self.run_start_time_epoch
            )
        else:
            os.environ.pop("TORCHTITAN_RUN_START_TIME_EPOCH", None)
        if self.generation > 1:
            os.environ["TORCHTITAN_SUPPRESS_POST_RECOVERY_WANDB_LOSS_STEPS"] = "5"
        else:
            os.environ.pop("TORCHTITAN_SUPPRESS_POST_RECOVERY_WANDB_LOSS_STEPS", None)
        os.environ.pop("TORCHTITAN_SUPPRESS_FIRST_POST_RECOVERY_WANDB_LOSS", None)
        os.environ["WANDB_RUN_NAME"] = (
            f"{self.wandb_run_prefix}-replica{self.replica_id:02d}"
            f"-gen{self.generation:02d}-rank{rank:02d}"
        )

    def _configure_nccl_timeout_env(self) -> None:
        # Keep NCCL nonblocking watchdog aligned with the configured torchft
        # process-group timeout so we do not abort earlier than FT expects.
        ft_cfg = getattr(self.job_config, "fault_tolerance", None)
        timeout_ms = getattr(ft_cfg, "process_group_timeout_ms", None)
        os.environ.setdefault("TORCH_NCCL_ASYNC_ERROR_HANDLING", "1")
        try:
            timeout_ms_int = int(timeout_ms) if timeout_ms is not None else None
        except (TypeError, ValueError):
            timeout_ms_int = None
        if timeout_ms_int is not None and timeout_ms_int > 0:
            timeout_s = max(1, (timeout_ms_int + 999) // 1000)
            os.environ.setdefault("TORCH_NCCL_NONBLOCKING_TIMEOUT", str(timeout_s))

    @endpoint
    async def start_training(self, lighthouse_address: str) -> None:
        init_logger()
        rank = current_rank().rank
        os.environ["TORCHFT_LIGHTHOUSE"] = lighthouse_address
        self._configure_nccl_timeout_env()
        self._configure_wandb_env(rank)

        # Fix MASTER_ADDR if it's loopback (common in Kubernetes hostNetwork mode)
        master_addr = os.environ.get("MASTER_ADDR", "")
        if master_addr.startswith("127.") or master_addr == "localhost":
            real_ip = self._get_real_ip()
            if real_ip:
                logger.info(
                    f"{self.uid} Replacing loopback MASTER_ADDR={master_addr} with {real_ip}"
                )
                os.environ["MASTER_ADDR"] = real_ip

        # Patch socket.gethostname to return IP instead of hostname so that
        # ft.ManagerServer advertises a routable IP to Lighthouse
        pod_ip = os.environ.get("POD_IP", "")
        if pod_ip:
            import socket as _socket
            _socket.gethostname = lambda: pod_ip
        trainer_cls = (
            FaultTolerantTrainer
            if isinstance(self.job_config, FaultTolerantTrainer.Config)
            else Trainer
        )
        trainer: Trainer | FaultTolerantTrainer | None = None
        try:
            trainer = trainer_cls(self.job_config)
            logger.info(f"{self.uid} initialized successfully on {os.getpid()}")
            logger.info(f"{self.uid} starting training")
            trainer.train()
        except Exception:
            # Send full traceback to supervisor before dying so __supervise__
            # has context to log. Then re-raise → process exits → __supervise__ fires.
            remote_tb = traceback.format_exc()
            summary = next(
                (line for line in reversed(remote_tb.splitlines()) if line.strip()),
                "trainer crashed without traceback",
            )
            try:
                # Fire-and-forget so reply delivery to a dying trainer does not
                # become a fatal supervision event on the replica actor.
                self.supervisor.report_training_error.broadcast(
                    self.generation,
                    rank,
                    summary,
                    remote_tb,
                )
            except Exception as e:
                logger.exception(
                    f"{self.uid} failed to report error to supervisor: {e}"
                )
            if trainer:
                trainer.close()
            raise  # re-raise so the actor process dies → __supervise__ fires
        else:
            # Rank 0 signals completion; supervisor resolves _done_future on receipt.
            if rank == 0:
                try:
                    await self.supervisor.training_completed.call_one(
                        self.generation, rank
                    )
                except Exception as e:
                    logger.exception(
                        f"{self.uid} failed to report completion to supervisor: {e}"
                    )
            if trainer:
                trainer.close()
        finally:
            if torch.distributed.is_initialized():
                torch.distributed.destroy_process_group()
            logger.info(f"{self.uid} trainer cleaned up")
@dataclass
class JobSpec:
    job_config: Trainer.Config
    remote_lighthouse: bool
    replica_count: int
    hosts_per_replica: int
    gpus_per_node: int
    namespace: str
    with_failures: bool
    failure_injection_delay: int
    min_replicas: int = 1
    wandb_project: str | None = None
    wandb_group: str = ""
    wandb_run_prefix: str = "monarch"
    run_start_time_epoch: float = 0.0
    lighthouse_address: str = ""
@dataclass
class Replica:
    rid: int
    proc_mesh: ProcMesh
    actor: "ReplicaActor"
# delay before re-creating proc mesh on existing job. change as needed.
PROC_ATTEMPT_DELAY = 10
# proc attempts before getting a new scheduler allocation. change as needed.
PROC_ATTEMPTS = 64
# attempts before failing training on replica. change as needed.
MAX_ATTEMPT = PROC_ATTEMPTS * 64
# seconds after restart completion before the replica is eligible for failure injection.
# covers 8B model load time (~4 min) so injections don't hit a freshly-restarted replica.
INJECTION_GRACE_PERIOD = 300
class MaxAttemptsExceeded(Exception):
    """Raised when a replica exhausts all retry attempts."""
# Supervision Tree:
#
#   OrchestrationManager  (asyncio client, top-level controller)
#     ├── LighthouseActor  (1 process on local host)
#     └── ReplicaActor × R  (1 process per replica)   ← fine-grained supervisor
#           └── trainers_proc_mesh  (N GPU processes, Kubernetes-allocated)
#                 ├── TrainingActor × N  (runs FaultTolerantTrainer)
#                 └── FailureActor  × N  (injects synthetic failures for demo)
#
# Trainers are started with broadcast(), making __supervise__ the SOLE recovery driver:
#   1. TrainingActor catches any exception, calls supervisor.report_training_error()
#      to deliver the full remote traceback, then re-raises so the process exits.
#   2. Monarch detects the process exit → fires __supervise__ on ReplicaActor.
#   3. __supervise__ logs the traceback from _last_reported_error and schedules restart.
#   4. On success, rank-0 TrainingActor calls supervisor.training_completed(),
#      resolving _done_future and unblocking start_replica.
#
# Generation tags on all callbacks drop stale events from torn-down generations.
class ReplicaActor(Actor):
    def __init__(self, spec: JobSpec, replica_id: int, scheduler: MonarchKubernetes) -> None:
        self.spec = deepcopy(spec)
        self.replica_id = replica_id
        self.uid = f"[replica_{replica_id}]"
        self.spec.job_config.fault_tolerance.replica_id = self.replica_id
        self.scheduler = scheduler
        self.failure_actors: FailureActor | None = None
        # Supervision-driven recovery state.
        self.attempt: int = 0
        self.generation: int = 0
        self._trainers_proc_mesh: ProcMesh | None = None
        self._training_actors = None
        self._done_future: asyncio.Future | None = None
        self._self_ref: "ReplicaActor | None" = None
        self._last_reported_error: str | None = None
        # Guards duplicate restarts when multiple ranks die simultaneously.
        # Lock because __supervise__ runs on a Rust thread.
        self._restarting: bool = False
        self._restart_lock: threading.Lock = threading.Lock()
        # True while a generation is live; blocks __supervise__ after teardown starts.
        self._training_active: bool = False
        # Timestamp when the last restart completed (0 = never restarted).
        # Used by is_healthy() to enforce INJECTION_GRACE_PERIOD after restart.
        self._last_restart_at: float = 0.0
        # __supervise__ runs on a Rust thread; asyncio ops go through this loop.
        self._loop: asyncio.AbstractEventLoop | None = None
    @endpoint
    async def start_replica(self, self_ref: "ReplicaActor") -> None:
        init_logger()
        self._self_ref = self_ref
        self._loop = asyncio.get_running_loop()
        self._done_future = self._loop.create_future()
        await self._start_attempt()
        try:
            await self._done_future
        finally:
            if self._trainers_proc_mesh is not None:
                try:
                    await self._trainers_proc_mesh.stop()
                except Exception as e:
                    logger.exception(
                        f"{self.uid} Failed to stop trainers proc mesh: {e}"
                    )
                self._trainers_proc_mesh = None
    async def _start_attempt(self) -> None:
        """Allocate (or re-allocate) a trainers proc mesh and start training."""
        if self.attempt != 0 and self.attempt % PROC_ATTEMPTS == 0:
            logger.info(
                f"{self.uid} Attempt {self.attempt}: failed {self.attempt} times. Getting new allocation."
            )
            self.scheduler.kill_job(_mesh_name(self.replica_id))
            await self.scheduler.get_or_create_job(
                _mesh_name(self.replica_id), self.spec.hosts_per_replica
            )
        delay = 0 if self.attempt == 0 else PROC_ATTEMPT_DELAY
        logger.info(
            f"{self.uid} Spinning up trainers for attempt {self.attempt} in {delay} seconds"
        )
        await asyncio.sleep(delay)
        self.generation += 1
        generation = self.generation
        self._last_reported_error = None
        logger.info(f"{self.uid} Spawning trainers for generation {generation}")
        if self._self_ref is None:
            raise RuntimeError(f"{self.uid} self reference not initialized")
        self._trainers_proc_mesh = self.scheduler.proc_mesh(
            _mesh_name(self.replica_id),
            num_procs=self.spec.gpus_per_node,
        )
        await self._trainers_proc_mesh.logging_option(stream_to_client=True)
        await setup_torch_elastic_env_async(
            self._trainers_proc_mesh, use_ipaddr=AddrType.IPv4
        )
        self._training_actors = self._trainers_proc_mesh.spawn(
            "training_actors",
            TrainingActor,
            self.spec.job_config,
            self.replica_id,
            self._self_ref,
            generation,
            self.spec.wandb_project,
            self.spec.wandb_group,
            self.spec.wandb_run_prefix,
            self.spec.run_start_time_epoch,
        )
        self.failure_actors = self._trainers_proc_mesh.spawn(
            "failure_actors", FailureActor
        )
        # Wait for actor objects to be fully initialized before broadcasting.
        await asyncio.sleep(10)
        logger.info(
            f"{self.uid} Starting trainers with broadcast (attempt {self.attempt}, generation {generation})"
        )
        # Set before broadcast so supervision events are handled immediately.
        self._training_active = True
        try:
            self._training_actors.start_training.broadcast(self.spec.lighthouse_address)
        except Exception as e:
            self._handle_failure(
                f"failed to broadcast start_training for generation {generation}: {e}",
                generation,
            )
    @endpoint
    async def training_completed(self, generation: int, rank: int) -> None:
        """Called by rank-0 TrainingActor when training finishes successfully."""
        if generation != self.generation:
            logger.info(
                f"{self.uid} Ignoring stale completion from generation {generation} (active {self.generation})"
            )
            return
        logger.info(
            f"{self.uid} Training completed successfully (generation {generation}, signaled by rank {rank})"
        )
        self._training_active = False
        if self._done_future is not None and not self._done_future.done():
            self._done_future.set_result(None)
    @endpoint
    async def report_training_error(
        self,
        generation: int,
        rank: int,
        summary: str,
        remote_traceback: str,
    ) -> None:
        """Stores the remote traceback so __supervise__ can log it on process death."""
        if generation != self.generation:
            logger.info(
                f"{self.uid} Ignoring stale error report from generation {generation} (active {self.generation})"
            )
            return
        self._last_reported_error = (
            f"[rank={rank}, generation={generation}] {summary}\n{remote_traceback}"
        )
        logger.error(
            f"{self.uid} Trainer error report received from rank {rank}, generation {generation}:\n{remote_traceback}"
        )
    @endpoint
    async def is_healthy(self) -> bool:
        """True if this replica is safe to inject a failure into.
        Returns False if restarting, not actively training, already done,
        or within INJECTION_GRACE_PERIOD seconds of a restart completing
        (covers model load time before training begins).
        """
        if self._restarting or not self._training_active:
            return False
        if self._done_future is None or self._done_future.done():
            return False
        if self._last_restart_at > 0 and (time.time() - self._last_restart_at) < INJECTION_GRACE_PERIOD:
            return False
        return True
    def _handle_failure(self, reason: str, generation: int) -> None:
        """Idempotent failure handler, safe to call from any thread.
        Stale events from old generations are dropped. The first caller acquires
        _restart_lock and schedules a restart; subsequent callers (e.g. 7 dying
        ranks after one NCCL abort) return immediately.
        """
        if generation != self.generation:
            logger.info(
                f"{self.uid} Ignoring stale failure for generation {generation} (active {self.generation})"
            )
            return
        with self._restart_lock:
            if self._restarting:
                return
            self._restarting = True
        self._training_active = False
        self.attempt += 1
        logger.warning(
            f"{self.uid} Failure (attempt now {self.attempt}, generation {generation}): {reason}"
        )
        if self.attempt >= MAX_ATTEMPT:
            logger.error(
                f"{self.uid} Replica {self.replica_id} has failed too many times."
            )
            def _set_exc():
                if self._done_future is not None and not self._done_future.done():
                    self._done_future.set_exception(
                        MaxAttemptsExceeded(
                            f"replica {self.replica_id} exhausted {MAX_ATTEMPT} attempts"
                        )
                    )
            self._loop.call_soon_threadsafe(_set_exc)
            return
        self._loop.call_soon_threadsafe(
            self._loop.create_task, self._stop_and_restart()
        )
    async def _stop_and_restart(self) -> None:
        """Tears down the current trainers proc mesh and starts a fresh attempt."""
        if self._trainers_proc_mesh is not None:
            logger.info(
                f"{self.uid} Stopping failed trainers (attempt {self.attempt}, waiting for processes to exit)..."
            )
            try:
                await self._trainers_proc_mesh.stop()
            except Exception as e:
                logger.exception(
                    f"{self.uid} Failed to stop trainers proc mesh, it may already be stopped. {e}"
                )
            logger.info(f"{self.uid} Failed trainers stopped.")
            self._trainers_proc_mesh = None
            self._training_actors = None
            self.failure_actors = None
        try:
            await self._start_attempt()
            self._restarting = False
            self._last_restart_at = time.time()
        except Exception as e:
            logger.exception(f"{self.uid} _start_attempt failed, giving up: {e}")
            self._restarting = False
            if self._done_future is not None and not self._done_future.done():
                self._done_future.set_exception(e)
    @endpoint
    async def inject_failure(self, failure_type: Failure):
        if self.failure_actors:
            try:
                target_rank = random.randrange(self.spec.gpus_per_node)
                logger.info(
                    f"{self.uid} Injecting failure ({failure_type}) into trainer rank {target_rank}"
                )
                self.failure_actors.fail_if_rank.broadcast(target_rank, failure_type)
            except Exception as e:
                logger.exception(f"{self.uid} Injected failure: {e}")
        else:
            logger.error(f"{self.uid} No failure actors available")
    def __supervise__(self, failure: MeshFailure) -> bool:
        """Sole recovery driver: fires when a training actor process dies.
        Guards (in order): training complete, restart already in progress,
        no active generation. Schedules restart via _handle_failure.
        Returns True so the failure is not propagated to the client.
        """
        report = failure.report()
        reason = next((l for l in reversed(report.splitlines()) if l.strip()), str(failure))
        logger.warning(
            f"{self.uid} Supervision event received (generation {self.generation}): {reason}"
        )
        if self._done_future is not None and self._done_future.done():
            logger.info(f"{self.uid} Supervision: training already complete, ignoring")
            return True
        if self._restarting:
            logger.info(f"{self.uid} Supervision: restart already in progress, ignoring")
            return True
        if not self._training_active:
            logger.info(f"{self.uid} Supervision: no active generation, ignoring")
            return True
        if self._last_reported_error:
            logger.error(
                f"{self.uid} Supervision: trainer exception (generation {self.generation}):\n{self._last_reported_error}"
            )
        self._handle_failure(reason, self.generation)
        return True
class OrchestrationManager:
    def __init__(self, spec: JobSpec) -> None:
        self.spec = spec
        self.replicas: Dict[int, Replica] = {}
        self.lighthouse_actor: LighthouseActor | None = None
        self.lighthouse_mesh: ProcMesh | None = None
        self.scheduler = MonarchKubernetes(
            namespace=self.spec.namespace,
            gpus_per_node=self.spec.gpus_per_node,
        )
        self._completed_replicas: set[int] = set()
        self._failed_replicas: set[int] = set()
        self._mesh_futures: Dict[int, asyncio.Task] = {}

    def _on_replica_done(self, replica_id: int) -> None:
        """Called when a replica completes successfully."""
        self._completed_replicas.add(replica_id)
        completed = len(self._completed_replicas)
        total = self.spec.replica_count
        logger.info(
            f"[Controller] Replica {replica_id} completed. "
            f"Progress: {completed}/{total} replicas done."
        )
        if completed >= total:
            logger.info("[Controller] All replicas completed successfully!")

    def _on_replica_failed(self, replica_id: int) -> None:
        """Called when a replica fails permanently."""
        self._failed_replicas.add(replica_id)
        logger.warning(
            f"[Controller] Replica {replica_id} failed permanently. "
            f"Failed: {len(self._failed_replicas)}, Completed: {len(self._completed_replicas)}"
        )

    async def start_training(self) -> None:
        logger.info(
            f"[Controller] Creating training system with {self.spec.replica_count} replicas"
        )
        self._completed_replicas.clear()
        self._failed_replicas.clear()

        for replica_id in range(self.spec.replica_count):
            await self.scheduler.get_or_create_job(
                _mesh_name(replica_id), self.spec.hosts_per_replica
            )
        
        self._mesh_futures = {}
        for i in range(self.spec.replica_count):
            self._mesh_futures[i] = asyncio.create_task(self._run_replica(i))

        failure_future = None
        if self.spec.with_failures:
            failure_future = asyncio.create_task(
                FailureController.execute_failures(
                    self.replicas,
                    self.scheduler,
                    startup_wait=self.spec.failure_injection_delay,
                )
            )

        # Wait for all replicas to complete (or be cancelled)
        results = await asyncio.gather(*self._mesh_futures.values(), return_exceptions=True)
        for i, result in enumerate(results):
            if isinstance(result, asyncio.CancelledError):
                logger.info(f"[Controller] replica {i} was cancelled")
            elif isinstance(result, Exception):
                logger.error(f"[Controller] replica {i} raised: {result}")

        if failure_future:
            failure_future.cancel()

        # Log final status
        completed = len(self._completed_replicas)
        total = self.spec.replica_count

        if completed == total:
            logger.info(f"[Controller] Training completed successfully! All {total} replicas finished.")
        else:
            logger.warning(
                f"[Controller] Training ended with {completed}/{total} replicas completed. "
                f"Failed replicas: {set(range(total)) - self._completed_replicas}"
            )
    async def start_lighthouse(self) -> None:
        if self.spec.remote_lighthouse:
            await self.scheduler.get_or_create_job("lighthouse")
            self.lighthouse_mesh = self.scheduler.proc_mesh("lighthouse", num_procs=1)
        else:
            self.lighthouse_mesh = this_host().spawn_procs({"gpus": 1})
        await self.lighthouse_mesh.logging_option(stream_to_client=False)
        self.lighthouse_actor = self.lighthouse_mesh.spawn(
            "lighthouse_actor", LighthouseActor
        )
        self.spec.lighthouse_address = (
            await self.lighthouse_actor.start_lighthouse.call_one(self.spec.min_replicas)
        )
    async def stop_lighthouse(self) -> None:
        try:
            if self.lighthouse_mesh:
                await self.lighthouse_actor.stop_lighthouse.call_one()
                await self.lighthouse_mesh.stop()
            logger.info("[Controller] Lighthouse stopped")
        except Exception as e:
            logger.exception(f"[Controller] Failed to stop lighthouse: {e}")
    async def _run_replica(self, replica_id: int) -> None:
        try:
            await self._spin_up_replica(replica_id)
            logger.info(f"[Controller] replica {replica_id} done")
            self._on_replica_done(replica_id)
            await self._teardown(replica_id)
        except asyncio.CancelledError:
            await self._teardown(replica_id)
            logger.info(f"[Controller] replica {replica_id} cancelled (straggler)")
            raise
        except MaxAttemptsExceeded as e:
            self._on_replica_failed(replica_id)
            await self._teardown(replica_id)
            logger.error(
                f"[Controller] replica {replica_id} gave up after max attempts: {e}"
            )
        except Exception as e:
            self._on_replica_failed(replica_id)
            await self._teardown(replica_id)
            logger.exception(f"[Controller] replica {replica_id} failed: {e}")
    async def _spin_up_replica(self, replica_id: int) -> None:
        logger.info(f"[Controller] Spinning up replica with ID {replica_id}")
        replica_proc_mesh = this_host().spawn_procs({"gpus": 1})
        await replica_proc_mesh.logging_option(aggregate_window_sec=None)
        replica_actor = replica_proc_mesh.spawn(
            "replica_actor", ReplicaActor, self.spec, replica_id, self.scheduler
        )
        replica = Replica(replica_id, replica_proc_mesh, replica_actor)
        self.replicas[replica_id] = replica
        await replica.actor.start_replica.call_one(replica.actor)
    async def _teardown(self, replica_id: int) -> None:
        replica = self.replicas.pop(replica_id, None)
        if replica is None:
            logger.info(
                f"[Controller] replica {replica_id} teardown skipped (not started)"
            )
            return
        try:
            await replica.proc_mesh.stop()
        except Exception as e:
            logger.exception(
                f"[Controller] Failed to stop replica {replica_id}, it may already be stopped. {e}"
            )
# === CLI / CONFIG === #
def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(
        description="Monarch-TorchFT Distributed Training - broadcast supervision demo"
    )
    script_dir = os.path.dirname(os.path.abspath(__file__))
    workspace_dir = os.environ.get("WORKSPACE_DIR", "/workspace")
    torchtitan_assets_dir = os.path.join(workspace_dir, "torchtitan", "tests", "assets")
    parser.add_argument(
        "--replica-count", type=int,
        default=int(os.environ.get("REPLICA_COUNT", "2")),
        help="Number of replicas (default: env REPLICA_COUNT or 2)",
    )
    parser.add_argument(
        "--gpu-per-node", type=int,
        default=int(os.environ.get("GPUS_PER_NODE", "8")),
        help="GPUs per replica (default: env GPUS_PER_NODE or 8)",
    )
    parser.add_argument(
        "--host-per-replica", type=int,
        default=int(os.environ.get("HOST_PER_REPLICA", "1")),
        help="Hosts per replica (default: env HOST_PER_REPLICA or 1)",
    )
    parser.add_argument(
        "--remote-lighthouse",
        action="store_true",
        help="Run the LighthouseServer on a worker node (default: False)",
    )
    parser.add_argument(
        "--training-steps",
        type=int,
        default=50,
        help="Number of training steps (default: 50)",
    )
    parser.add_argument(
        "--namespace",
        type=str,
        default=os.environ.get("WORKSPACE", "monarch-tests"),
        help="Kubernetes namespace for worker pods (default: env WORKSPACE or monarch-tests)",
    )
    parser.add_argument(
        "--model-config",
        type=str,
        default="llama3_ft_debugmodel",
        help="TorchTitan config_registry function name (default: llama3_ft_debugmodel)",
    )
    parser.add_argument(
        "--model-module",
        type=str,
        default="ft.llama3",
        help="TorchTitan module name for config loading (default: ft.llama3)",
    )
    parser.add_argument(
        "--dataset-path",
        type=str,
        default=os.path.join(torchtitan_assets_dir, "c4_test"),
        help="Path to training dataset",
    )
    parser.add_argument(
        "--tokenizer-path",
        type=str,
        default=os.path.join(torchtitan_assets_dir, "tokenizer"),
        help="Path to tokenizer / HF assets directory",
    )
    parser.add_argument(
        "--with-failures",
        action="store_true",
        help="Enable the failure injector utility (default: False)",
    )
    parser.add_argument(
        "--failure-injection-delay",
        type=int,
        default=900,
        help=(
            "Seconds to wait before injecting the first failure (default: 900). "
            "Must exceed model initialization time to avoid injecting during loading. "
            "For debugmodel use ~120; for 8B use ~600."
        ),
    )
    parser.add_argument(
        "--min-replicas",
        type=int,
        default=1,
        help="Minimum number of replicas required for quorum (default: 1)",
    )
    parser.add_argument(
        "--wandb-project",
        type=str,
        default=None,
        help="W&B project name override (default: use WANDB_PROJECT or torchtitan).",
    )
    parser.add_argument(
        "--wandb-group",
        type=str,
        default=None,
        help="W&B run group. If omitted, a demo group is auto-generated.",
    )
    parser.add_argument(
        "--wandb-run-prefix",
        type=str,
        default=None,
        help="Prefix for per-replica W&B run names (default: wandb-group).",
    )
    return parser.parse_args()


def make_job_spec(args: argparse.Namespace) -> JobSpec:
    data_parallel_shard_degree = args.gpu_per_node * args.host_per_replica

    run_timestamp = time.strftime("%Y%m%d-%H%M%S", time.gmtime())
    wandb_group = args.wandb_group or f"monarch-{args.model_config}-{run_timestamp}"
    wandb_run_prefix = args.wandb_run_prefix or wandb_group

    output_path = "./outputs"
    training_dataset = args.dataset_path.split("/")[-1]

    script_dir = os.path.dirname(os.path.abspath(__file__))
    
    # Handle absolute vs relative paths
    tokenizer_path = args.tokenizer_path if os.path.isabs(args.tokenizer_path) else os.path.join(script_dir, args.tokenizer_path)
    dataset_path = args.dataset_path if os.path.isabs(args.dataset_path) else os.path.join(script_dir, args.dataset_path)
    
    default_args = [
        "--module",
        args.model_module,
        "--config",
        args.model_config,
        "--hf_assets_path",
        tokenizer_path,
        "--comm.trace_buf_size",
        "0",
        "--metrics.log_freq",
        "1",
        "--fault_tolerance.enable",
        "--fault_tolerance.group_size",
        str(args.replica_count),
        "--fault_tolerance.process_group",
        "nccl",
        # 600s: covers DiLoCo cross-replica allreduce + peer checkpoint download (~75s for 8B).
        "--fault_tolerance.process_group_timeout_ms",
        "600000",
        "--parallelism.data_parallel_shard_degree",
        str(data_parallel_shard_degree),
        "--comm.train_timeout_seconds",
        "1200",
        "--training.steps",
        str(args.training_steps),
        "--dataloader.dataset",
        training_dataset,
        "--dataloader.dataset_path",
        dataset_path,
        "--dump_folder",
        output_path,
        "--metrics.enable_tensorboard",
        "--metrics.enable_wandb",
    ]

    try:
        import torchtitan.experiments.ft.llama3.config_registry
        logger.info("[Controller] ft.llama3.config_registry imported successfully")
    except Exception as e:
        logger.error(f"[Controller] Failed to import ft.llama3.config_registry: {e}", exc_info=True)

    config_manager = ConfigManager()
    job_config = config_manager.parse_args(default_args)

    logger.info(f"[Controller] default_args: {default_args}")
    logger.info(f"[Controller] job_config.hf_assets_path: {getattr(job_config, 'hf_assets_path', 'N/A')}")
    logger.info(f"[Controller] job_config: {job_config}")

    return JobSpec(
        job_config=job_config,
        remote_lighthouse=args.remote_lighthouse,
        replica_count=args.replica_count,
        hosts_per_replica=args.host_per_replica,
        gpus_per_node=args.gpu_per_node,
        namespace=args.namespace,
        with_failures=args.with_failures,
        failure_injection_delay=args.failure_injection_delay,
        min_replicas=args.min_replicas,
        wandb_project=args.wandb_project,
        wandb_group=wandb_group,
        wandb_run_prefix=wandb_run_prefix,
        run_start_time_epoch=time.time(),
    )
# === CLI / CONFIG === #
def _log_env_vars() -> None:
    env_keys = [
        "MONARCH_MESH_PREFIX", "MONARCH_PORT", "REPLICA_COUNT",
        "HOST_PER_REPLICA", "GPUS_PER_NODE", "WORKSPACE",
        "MASTER_ADDR", "MASTER_PORT",
    ]
    for key in env_keys:
        logger.info(f"[ENV] {key}={os.environ.get(key, '<unset>')}")


def _fix_loopback_hosts() -> None:
    """Replace 127.0.1.1 with real POD_IP in /etc/hosts so Monarch Rust
    runtime resolves hostname to the correct address instead of loopback."""
    pod_ip = os.environ.get("POD_IP", "")
    if not pod_ip:
        return
    try:
        with open("/etc/hosts", "r") as f:
            content = f.read()
        if "127.0.1.1" not in content:
            return
        fixed = content.replace("127.0.1.1", pod_ip)
        with open("/tmp/hosts.fixed", "w") as f:
            f.write(fixed)
        os.system("cp /tmp/hosts.fixed /etc/hosts 2>/dev/null")
        os.remove("/tmp/hosts.fixed")
        logger.info(f"[hosts] Replaced 127.0.1.1 with {pod_ip} in /etc/hosts")
    except Exception as e:
        logger.warning(f"[hosts] Failed to fix /etc/hosts: {e}")


async def main() -> None:
    init_logger()
    _fix_loopback_hosts()
    enable_transport("tcp")
    _log_env_vars()
    args = parse_args()
    job_spec = make_job_spec(args)
    orchestrator = OrchestrationManager(job_spec)
    try:
        await orchestrator.start_lighthouse()
        await orchestrator.start_training()
    finally:
        await orchestrator.stop_lighthouse()
if __name__ == "__main__":
    asyncio.run(main())
