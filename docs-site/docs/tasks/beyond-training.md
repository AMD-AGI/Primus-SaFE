---
sidebar_position: 6
title: Beyond training (the LLM lifecycle)
---

# Beyond training (the LLM lifecycle)

Primus-SaFE isn't only for training runs. The same platform — and the same **Workload** model in
the console — covers the whole LLM lifecycle on one cluster: develop, train, make training
fault-tolerant, and serve. This page is a map; each stage links to its detail.

<!-- @test none: overview/hub page — the underlying flows are tested on their own pages. -->

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

The platform supports more kinds for different jobs — **Ray**, **StatefulSet**, and **CICD** among
them. Rather than detail each here, see the [Workload types](/concepts/workload-types) concept for
what each is and when to use it.
