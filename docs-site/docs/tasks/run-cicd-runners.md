---
sidebar_position: 7
title: Run GitHub Actions runners (CI/CD)
---

# Run GitHub Actions runners (CI/CD)

Primus-SaFE can host **GitHub Actions self-hosted runners** as an autoscaling **runner scale set**:
GitHub triggers the workflow, and the platform spins up runner pods on your GPU cluster to execute
the jobs, then scales them back down. Under the hood the platform runs a **controller and a
listener** (the Actions Runner Controller, ARC) that register with GitHub, watch for queued jobs,
and create/tear down the runner pods. A runner scale set is a workload of kind
`AutoscalingRunnerSet`, and it lives in a **workspace that has the `CICD` scope enabled** — that
scope is what makes the **Workloads → CICD** section appear for the workspace.

This page is written to serve two audiences at once:

- **For you (the reader):** the sections below walk building a runner image, creating a scale set,
  and pointing a workflow at it, noting what a healthy result looks like along the way.
- **For an AI agent:** the feature has real preconditions. **CICD → Create** appears under
  **Workloads** only once the workspace has the **`CICD` scope**; the runner scale sets also need
  the **ARC (`gha-runner-scale-set`) add-on** on the cluster and **GitHub auth** (a GitHub App or
  PAT). If the workspace lacks the CICD scope, or the ARC add-on is missing, report **BLOCKED**
  naming the missing precondition rather than failing a step. There is no invisible test layer;
  bookkeeping lives in the run contract `docs-site/AGENTS.md`.

> **Agent:** a full end-to-end behavior run also needs a **GitHub App/PAT plus a repo** to point a
> workflow at. If those fixtures aren't available, verify the documented controls (the CICD create
> wizard and its fields) and report **BLOCKED** on the run itself — don't fabricate a workflow run.

## Before you start

- The **Actions Runner Controller (ARC)** `gha-runner-scale-set` add-on is present on the cluster —
  it may be enabled at bootstrap/install time or added later from **System → Addons**. Runner scale
  sets will not start without it.
- You have access to a **workspace** with the **`CICD`** scope enabled (and the **`Train`** scope
  too if you will use multi-node unified jobs). Enabling the `CICD` scope on the workspace is what
  makes **Workloads → CICD** appear in the left nav; if you enabled it and still don't see CICD,
  that's a bug, not expected. See [Manage access & quota](/administration/manage-access-and-quota).
- A **GitHub App** (recommended) or a **personal access token (PAT)** with permission to register
  runners for your repository or organization.
- A **runner container image** the cluster can pull (build one below).

## 1. Build a runner image

The runner image bundles the GitHub Actions runner. A minimal image:

```dockerfile
FROM ubuntu:22.04
ARG RUNNER_VERSION=2.333.1
ENV DEBIAN_FRONTEND=noninteractive
RUN apt-get update && apt-get install -y --no-install-recommends \
    curl git python3 python3-pip jq ca-certificates \
    libicu-dev libssl-dev libkrb5-dev zlib1g-dev \
 && apt-get clean && rm -rf /var/lib/apt/lists/*
# The runner refuses to run as root; use an unprivileged user.
RUN useradd --create-home --shell /bin/bash runner
WORKDIR /actions-runner
RUN curl -sL -o actions-runner.tar.gz \
      "https://github.com/actions/runner/releases/download/v${RUNNER_VERSION}/actions-runner-linux-x64-${RUNNER_VERSION}.tar.gz" \
 && tar xzf actions-runner.tar.gz && rm actions-runner.tar.gz \
 && ./bin/installdependencies.sh && chown -R runner:runner /actions-runner
USER runner
CMD ["bash"]
```

Build and push it to a registry the cluster can pull from (see
[Speed up startup → import an image](/tasks/speed-up-startup) if you use the in-cluster registry).
Note the **entry command** in the image — for this Dockerfile it is `/actions-runner/run.sh`.

:::tip You don't have to build from scratch
GitHub publishes officially maintained runner images at
[`ghcr.io/actions/actions-runner`](https://github.com/actions/runner/pkgs/container/actions-runner),
which you can use directly or as a base. The minimal Dockerfile above is just a starting point if
you need extra tooling in the runner.
:::

## 2. Create a runner scale set (console)

In the console, go to **Workloads → CICD → Create** and fill in the wizard:

| Field | What to enter |
|-------|---------------|
| **Name / description** | A label for the scale set. |
| **Image** | The runner image from step 1 (pullable from the cluster). |
| **Entry command** | The runner start command inside the image, e.g. `/actions-runner/run.sh`. |
| **GitHub URL** | The repo or org the runners serve, e.g. `https://github.com/OWNER/REPO`. |
| **GitHub auth** | **GitHub App** (`appId`, `installationId`, `privateKey`) — recommended — or a **PAT**. |
| **Runner resources** | Per-runner CPU / GPU / memory / ephemeral storage (this is the capacity CI jobs use). |
| **Workspace** | A workspace with the `CICD` scope. |
| **Priority** | Low / Medium / High. |

Under **Advanced Options** you can enable **multi-node unified jobs** (see step 4). Submit; the
scale set appears in the **CICD** list, and you can expand its row to see the per-job runner pods.

:::tip Two resource blocks
The wizard's **proxy** resources (control-plane pod, usually `replica: 1`) are separate from the
**runner** resources (the capacity each CI job actually gets). Size the runner resources for your
jobs.
:::

## 3. Point your workflow at the scale set

In your repository's workflow YAML, set `runs-on` to the scale set's name (the returned
`workloadId`):

```yaml
jobs:
  build:
    runs-on: <workloadId>
    steps:
      - uses: actions/checkout@v4
      - run: make test
```

Everything else follows standard [GitHub Actions](https://docs.github.com/en/actions) syntax.

## 4. (Optional) Multi-node unified jobs

Set **`UNIFIED_JOB_ENABLE = true`** to offload multi-node training/batch work to the cluster from a
single CI job. This requires the workspace to have **storage (NFS)** and the `Train` scope; your CI
script writes a request JSON to the injected NFS path and polls for a result. The full request/result
contract is in the repository CI/CD guide (`SaFE/docs/apis/cicd-quickstart.md`, §3).

## From the API (alternative)

The same scale set can be created with `POST /api/v1/workloads` (kind `AutoscalingRunnerSet`). The
console fields above map to `env` keys (`GITHUB_CONFIG_URL`, `IMAGE`, base64 `ENTRYPOINT`,
`RESOURCES`, `UNIFIED_JOB_ENABLE`) plus a root-level `githubAuth`:

```bash
curl -X POST https://<your-console>/api/v1/workloads \
  -H "Authorization: Bearer ak-..." -H "Content-Type: application/json" \
  -d '{
    "displayName": "ci-runners",
    "groupVersionKind": { "kind": "AutoscalingRunnerSet", "version": "v1" },
    "resources": [{ "replica": 1, "cpu": "1", "memory": "4Gi", "ephemeralStorage": "10Gi" }],
    "workspace": "<workspace-id>",
    "env": {
      "UNIFIED_JOB_ENABLE": "false",
      "GITHUB_CONFIG_URL": "https://github.com/OWNER/REPO",
      "IMAGE": "<runner-image>",
      "ENTRYPOINT": "'"$(echo -n /actions-runner/run.sh | base64 -w0)"'",
      "RESOURCES": "{\"replica\":1,\"cpu\":\"4\",\"gpu\":\"0\",\"memory\":\"16Gi\",\"ephemeralStorage\":\"100Gi\"}"
    },
    "githubAuth": { "type": "pat", "token": "<github-pat>" }
  }'
```

Rotate credentials later with `PATCH /api/v1/workloads/<workloadId>` (send just `githubAuth`).

## Watching workflow runs

Watch your workflow runs where you normally would — in **GitHub** (the repo's Actions tab), or in
the **CICD** tab of the console side panel, which lists the scale set's runs. A screenshot of that
CICD run list is usually enough; you don't need the Lens app for this.

*Still being documented: console screenshots of the CI/CD create wizard (Clone/Create + Advanced
Options) and the CICD run list, and confirmation of exactly how the ARC (`gha-runner-scale-set`)
add-on is enabled (bootstrap flag vs. System → Addons). Where the preconditions above are met, an
agent can drive the full create-and-run flow; where they aren't, presence-check the documented
controls and report BLOCKED on the run.*
