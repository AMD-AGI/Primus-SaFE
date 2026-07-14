---
sidebar_position: 6
title: Beyond training (the LLM lifecycle)
---

# Beyond training (the LLM lifecycle)

Primus-SaFE isn't only for training runs. The same platform — and the same **Workload** model in
the console — covers the whole LLM lifecycle on one cluster: develop, train, make training
fault-tolerant, and serve. This page is a map; each stage links to its detail.

Unlike the executable task walkthroughs, this is an **overview/hub** page: it serves
two audiences at once but has **no numbered steps**.

- **For you (the reader):** a single map of the lifecycle, with each stage pointing to the page
  that walks it end to end.
- **For an AI agent:** there is nothing to *perform* here. The healthy result is a **presence
  check** — the workload kinds and links below exist and resolve. There is no invisible test
  layer; bookkeeping (priority, known product bugs) lives in the run contract
  `docs-site/AGENTS.md`.

> **What an agent verifies here:** this is a hub, so the check is presence only — confirm each
> lifecycle stage below is described and that its links (Develop, Train single/multi-node,
> TorchFT / Fault tolerance, Infer / Deployment, Workload types) are present and resolve. Do
> **not** fabricate steps or a pass/fail run on this page; the underlying flows are tested on
> their own pages.

## Develop

Prototype and debug interactively in an **Authoring** dev box (a personal pod with GPUs and your
workspace storage), then graduate to a full job. See
[Develop & interact with your jobs](/tasks/interact-with-your-job#authoring-a-personal-dev-box).

## Train

Submit a **PyTorchJob** on one node, or scale across nodes with gang scheduling and topology-aware
placement:

- [Run a single-node training job](/tasks/run-single-node-training)
- [Run a multi-node distributed job](/tasks/run-multi-node-training)

## Train with fault tolerance

For elastic, group-based fault tolerance — replica groups that can fail and recover independently
— submit a **TorchFT** job (**Workloads → Training → TorchFT**) instead of a plain PyTorchJob. The
platform's automatic node-fault recovery applies to training jobs generally; see
[Fault tolerance](/concepts/fault-tolerance).

## Serve / host inference

After training, host your model as a long-running inference service: create a **Deployment** under
**Workloads → Infer**, exposed as a service with liveness/readiness so it stays available. It uses
the same workload model as training — an image, an entry point, and resources.

## Other workload kinds

The platform supports more kinds for different jobs — **TorchFT**, **StatefulSet**, and **CICD**
among them. Rather than detail each here, see the [Workload types](/concepts/workload-types)
concept for what each is and when to use it.
