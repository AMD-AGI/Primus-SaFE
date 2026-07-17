---
sidebar_position: 4
title: View a job's logs
---

# View a job's logs

Every workload streams its **stdout/stderr** into the console, so you can read what a job is
doing without a `kubectl` or a shell. Open a workload's detail page, switch to the **Logs** tab,
pick a pod/node, and the lines show up — timestamped and grouped by pod. This is the quickest way
to confirm a job is alive and doing work.

Like the other task pages, the prose is the walkthrough for **both** a human and an agent, with
expectations stated inline and no invisible test layer. Bookkeeping (priority, known product bugs)
lives in the run contract `docs-site/AGENTS.md`.

This page has one golden path an agent should perform end to end — **open a job's Logs tab and
confirm log lines populate** (Steps 1–3).

## Before you start

- A **running cluster** you can reach in the console.
- A workload that is **Running** (or has recently run), so it has produced some log output. Any
  training job, inference job, or Authoring dev box works — logs are the same across workload types.

> **Agent:** the golden path needs `running-cluster`. Unlike most pages, do **not** report BLOCKED
> if there's no suitable workload — instead **create a trivial smoke job first** (see Step 1) so
> there is guaranteed log output, then clean it up at the end. Only report BLOCKED if there is no
> running cluster at all.

## Step 1 — Find or create a workload with logs

Open a workloads list (e.g. **Workloads → Training**, filter **All** if it looks empty) and pick a
job whose phase is **Running** or that has already run. That job is your target.

**If nothing suitable is available,** create a throwaway smoke job — this is the same recipe as
[Your first job](/getting-started/first-training-job): select your **workspace**, create a
**PyTorchJob** with any pullable image (e.g. `docker.io/rocm/pytorch:latest`) and an entrypoint
that prints something and stays up long enough to read, such as:

```bash
echo "hello from doc-as-test"; sleep 15
```

Submit it and wait for it to reach **Running** — now it has logs to show.

## Step 2 — Open the Logs tab

Open the workload's **detail page** (click its name/row), then click the **Logs** tab (alongside
**Overview / Pods / Timeline / Grafana / Images**).

## Step 3 — Select a pod/node and read the output

The Logs tab lists the workload's **nodes** on the left and auto-selects one (or two) for you; its
log lines load into the panel on the right. Here is what each outcome means — this is the pass/fail
for the walkthrough:

- **Healthy (pass):** with a node selected, the panel shows **at least one timestamped log line**
  for the pod/host (for the smoke job, you'll see your `hello from doc-as-test` line). You can also
  narrow the view with the **keyword** filter or the **date range**, and toggle **Wrap** — but a
  single populated line is enough to pass.
- **If instead you see** an empty panel, a spinner that never resolves, or an error, **that means**
  logs aren't reaching the UI **(fail)** — *unless* the pod genuinely hasn't produced output yet
  (just started / still **Pending**), in which case widen the **date range**, wait a moment, and
  retry before judging.

> **Agent:** perform Steps 1–3, fill the table below, show it to the user, and report **PASS** only
> if at least one real log line rendered in the Logs panel. Then **run cleanup**: if you created
> the smoke job in Step 1, delete it via its row action so the run is repeatable (leave
> pre-existing workloads alone).
>
> Known product drift to account for (do **not** count as a new failure): a just-started pod's log
> stream can lag, so an initially empty panel that fills after a widened date range / short wait is
> healthy, not a failure.

| Check | Healthy result | Found |
|---|---|---|
| Logs tab present on the detail page | yes | _fill in_ |
| A node is selected (auto or manual) | yes | _fill in_ |
| Log panel shows ≥1 timestamped line | yes | _fill in_ |
| Keyword filter narrows results | yes (optional) | _fill in_ |
| Cleanup (smoke job deleted, if created) | done / n/a | _fill in_ |

## Where to go next

| You want to… | Go to |
|--------------|-------|
| Shell into the pod / get results out | [Interact with your job](/tasks/interact-with-your-job) |
| All submit options & fields | [Run a single-node training job](/tasks/run-single-node-training) |
