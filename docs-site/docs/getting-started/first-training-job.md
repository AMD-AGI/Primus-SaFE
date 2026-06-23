---
sidebar_position: 3
title: Your first job
---

# Your first job

> **Status:** Draft · **Owner:** _unassigned_ · **Source:** `SaFE/docs/apis/workload.md`

A two-minute path from a working install to a running job. For the full how-to (all options
and fields) see [Tasks → Run a single-node training job](/tasks/run-single-node-training).

## Prerequisites

- Primus-SaFE is installed and you can reach the console (see [Install](/getting-started/install)).
- You have access to a **workspace** with quota (see [Workspace](/concepts/workspace)).

## Submit a job (console)

1. Sign in and select your **workspace**.
2. Create a **workload**, choose **PyTorchJob**, and set an **image** + **entrypoint**
   (e.g. `python train.py`) and the **resources** for one replica.
3. Submit.

That's it — you've launched a single-node training job.

## Where to go next

| You want to… | Go to |
|--------------|-------|
| All submit options & fields (UI/API) | [Run a single-node training job](/tasks/run-single-node-training) |
| Scale across nodes | [Run a multi-node distributed job](/tasks/run-multi-node-training) |
| Watch logs / shell in / get results | [Interact with your job](/tasks/interact-with-your-job) |
| Make jobs start faster | [Speed up workload startup](/tasks/speed-up-startup) |
