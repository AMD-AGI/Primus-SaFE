---
sidebar_position: 2
title: Run a multi-node distributed job
---

# Run a multi-node distributed job

> **Status:** TODO · **Owner:** _unassigned_
> **Purpose:** run training across several nodes — the Primus-SaFE equivalent of the settings
> a user reaches for in a Slurm batch job.

:::note Content brief
- [ ] Start from the single-node how-to and show the multi-node delta only
      (link to [Run a single-node training job](/tasks/run-single-node-training)).
- [ ] **Master + worker roles**: the `resources[]` / `images[]` / `entryPoints[]` arrays —
      index 0 = master, index 1 = workers (with `replica` = worker count).
- [ ] **Pin to specific nodes**: `specifiedNodes` + `nodesAffinity` (`required` vs
      `preferred`); the replica auto-rules when `specifiedNodes` is set
      (PyTorchJob master=1, workers=len-1).
- [ ] **Gang + topology**: distributed pods start together (gang) and place with network
      locality — link to [Fault tolerance](/concepts/fault-tolerance).
- [ ] **Networking for collectives**: `forceHostNetwork` (auto-on for multi-node full-GPU),
      RDMA (`rdma/hca`), and the NCCL env (`NCCL_DEBUG`, `nccl_socket_ifname`/`nccl_ib_hca`
      set at install).
- [ ] **Reliability knobs**: `priority`, `timeout`, `maxRetry`, `dependencies`,
      `isSupervised` (hang detection).
- [ ] **Slurm → SaFE mapping table** (nodes/ntasks/gres/partition/dependency → SaFE fields).
- [ ] Mention **TorchFT** for elastic/fault-tolerant multi-node — link to
      [Beyond training](/tasks/beyond-training).

**Source:** `SaFE/docs/apis/workload.md` (resources, specifiedNodes, nodesAffinity,
forceHostNetwork, isSupervised, replica rules), `SaFE/charts/primus-safe/values.yaml`
(`net.nccl_socket_ifname`, `nccl_ib_hca`, `rdma_name`).
:::
