---
sidebar_position: 7
title: Run GitHub Actions runners (CI/CD)
---

# Run GitHub Actions runners (CI/CD)

> **Status:** Draft · **Owner:** _unassigned_ · **Source:**
> `SaFE/docs/apis/cicd-quickstart.md`, `workload.md`

Primus-SaFE can host **GitHub Actions self-hosted runners** as an autoscaling **runner scale set**:
GitHub triggers the workflow, and the platform spins up runner pods on your GPU cluster to execute
the jobs, then scales them back down. A runner scale set is a workload of kind
`AutoscalingRunnerSet` in a workspace with the `CICD` scope.

<!-- @test todo:
  - "ARC is installed on the test cluster, but a behavior test also needs a GitHub App/PAT + a repo, and the CICD console page is hidden in GA-scoped builds. Add a behavior block (create an AutoscalingRunnerSet, expect EphemeralRunner pods) once those fixtures + a CICD-visible build are available."
-->

## Before you start

- The **Actions Runner Controller (ARC)** add-on is installed on the cluster — the
  `gha-runner-scale-set` add-on (**System → Addons**). Runner scale sets will not start without it.
- You have access to a **workspace** with the **`CICD`** scope (and the **`Train`** scope too if you
  will use multi-node unified jobs). See [Manage access & quota](/administration/manage-access-and-quota).
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

> **Not yet covered (capture so we don't lose it):**
> - [ ] Console screenshots of the CI/CD create wizard (Clone/Create + Advanced Options).
> - [ ] Step-by-step for installing the ARC (`gha-runner-scale-set`) add-on from System → Addons.
> - [ ] Where to watch workflow-run history (today it lives in the Lens app, not the SaFE console).
