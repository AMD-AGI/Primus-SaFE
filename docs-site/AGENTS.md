# Docs-as-test: agent operating contract

This documentation site **is** the test suite. Each page is a **single, visible, dual-audience
walkthrough**: the same prose a customer reads is what an agent executes against a live
Primus-SaFE, and the expected outcome is written in plain language ("Healthy: … / if you instead
see …") that a human reads and an agent judges from. There is **no invisible test layer** — the
prose you read is the spec. The only things kept off the page are bookkeeping (each page's
`mode`/`priority`) and any known product bug, in the manifest at the bottom of this file.

A run **double-verifies the docs against the product**: a failure means either the doc lies or the
product regressed, and the report says which.

> This file is not part of the published site (Docusaurus only builds `docs/`). It is the standing
> contract for any agent asked to "test the docs."

## What the agent reads, and where

| Thing | Lives in | Why there |
|---|---|---|
| The procedure (what to click/do) | the page prose | single source; edit prose, the test follows |
| The pass/fail outcome | the page prose ("Healthy … / if you instead see … that means …") | visible to humans *and* agents; no hidden copy |
| Preconditions & personas | the page prose ("Before you start", "As a member …") | reads naturally; agent infers what to check before starting |
| Cleanup (user-meaningful) | the page prose ("delete the workload when done") | good UX; agent performs it after a mutating run |
| `mode` / `priority` | **this file's manifest** | CI selection only; no reader-facing form |
| Known product drift/bugs | **this file's manifest** | a bug ledger, not customer docs — must not leak into the page |

## How to run a page

1. **Read the whole page top to bottom** as if you were the reader. It is a walkthrough, not a
   reference — the steps are ordered and each states its own healthy result.
2. **Check preconditions first.** They're in the "Before you start" section and the `> **Agent:**`
   callouts. If one is missing, report **BLOCKED** naming the precondition — never a silent PASS
   and never a FAIL for a missing fixture.
3. **Perform the numbered steps through the console UI**, exactly as written. Use the
   `user-Playwright` MCP (navigate / click / type / snapshot). **Name every created resource
   `doc-as-test-<page>-<runid>`** so a sweep can safely delete only test-created objects. (The
   pages' human-facing text just says "use a unique name"; this scheme is the agent's authoritative
   naming rule.)
4. **Judge each outcome from the prose.** The "Healthy (pass)" / "Rejected/Failed (fail)" sentences
   are the predicate. Fill in the page's own **What you should see** table (the `Found` column)
   from what you observe.
5. **Honor the manifest's known-drift notes** — a documented product bug is not a new failure;
   note it, don't fail on it. A `kubectl` cross-check is allowed only as a ground-truth *aside*,
   never as the primary assertion.
6. **Always run cleanup** — the page tells you how (delete/stop the resource you created).
7. **Report PASS / FAIL / BLOCKED per row**, quoting *page says X, product does Y*, with a
   screenshot on FAIL.

## Test-scope exclusions (what the regression run skips, and why)

The pages are written as **complete, executable product docs** — an agent handed a page can perform
the real operation (install a cluster, register a node, set quota, upgrade) on a target
environment. The shared docs-as-test **regression** environment, however, is *provided and
long-lived*, so some documented operations are deliberately **not executed** there. **That skip
lives here, never on the page** — the page must stay a truthful, complete instruction for a real
operator or a provisioning agent.

On the shared regression env, do the **reduced** action. On a **disposable** environment the
operator has cleared, perform the **full** documented procedure.

| Page | Full doc procedure (real / disposable env) | Regression-run action (shared env) | Why reduced |
|---|---|---|---|
| getting-started/install | Bootstrap → storage → (gateway) → `install.sh` → sign in | **sign-in check only** (env is already installed) | bring-up is slow + destructive; env is provided |
| administration/manage-nodes | register / bind / taint / reboot / delete nodes | **presence-check the Nodes view** (read-only) | node mutations disrupt a live shared cluster |
| administration/manage-access-and-quota (Set quota) | edit a workspace's flavor / nodes / scopes | **presence-check the Workspaces form** | changing a live tenant's capacity |
| administration/upgrading | Create → approve → verify → roll back a deployment | **presence-check Deployment Management** | upgrades restart the platform |

Environment/driver constraints (regardless of env):

- `tasks/interact-with-your-job` → **SSH on port 2222** needs a terminal client outside the browser,
  so it's out of scope for a UI-only run. **WebShell is the in-UI equivalent and *is* exercised** —
  a same-origin WebSocket to `/api/v1/workloads/<id>/pods/<pod>/webshell`, proxied to the apiserver
  and authenticated by the **session cookie**. A WebShell **`1006`** is almost always the client
  rejecting the `wss://` handshake because the console uses a **self-signed TLS cert** (browsers
  fail WS to an untrusted cert *silently*). Mitigation: run the driver with HTTPS errors ignored
  (Playwright `ignoreHTTPSErrors: true` / `--ignore-certificate-errors`) or trust the cert;
  otherwise mark WebShell **BLOCKED (env: cert)**. If it shows `[Connected]` then instantly
  `[Disconnected]`, the image likely lacks `bash` — the page tells the user to pick `sh`.
- `tasks/run-cicd-runners` → needs the **ARC add-on + a GitHub App** and a **workspace with the
  `CICD` scope** (that scope is what surfaces **Workloads → CICD** in the nav). A full run also
  needs a repo to point a workflow at.
- `administration/observability` behavior → needs `observability-installed` (primus-robust +
  Grafana).

An agent running the **regression** scope applies these reductions. An agent asked to actually
**provision or operate** an environment ignores this table and follows the page in full.

## Repeatability protocol

Because the spec is free-form prose, verify it's read consistently. Spend the repeat budget where
verdicts are most likely to flip:

- **P0 behavior pages** (first-training-job, interact-with-your-job, run-single-node-training): run
  **3× in fresh threads**. They mutate state and have the most room to read ambiguously.
- **Other behavior / contract pages:** run **once**, re-running only if a verdict looks borderline
  or a row reads ambiguously.
- **verify / n/a / presence-only pages:** run **once** — presence checks are deterministic by
  construction.

Record, honestly, which pages got the full 3× and which were single-run. Then:

1. **Self-consistency:** a page run multiple times must yield the **same PASS/FAIL/BLOCKED per
   row**. Any row that flips is a defect — either the prose is ambiguous about what "healthy" means
   or the step is under-specified. Record the exact sentence read two ways.
2. **Ambiguity log:** for every place you had to *guess* what to click or what counted as success,
   quote the sentence and say what was unclear. If you can't write a crisp expectation from the
   page, the page is too vague — that's a doc defect to fix.

## Trigger — paste into a brand-new thread

Runtime values (console URL, admin/member login) live in the local, gitignored
`docs-site/.docs-test.env` — never in this file or the prompt.

```
Execute the docs-as-test suite. The docs-site IS the test spec.

1. Read docs-site/AGENTS.md (this contract) and its manifest.
2. Load runtime params from docs-site/.docs-test.env (PRIMUS_CONSOLE_URL, PRIMUS_MEMBER_LOGIN /
   PRIMUS_ADMIN_LOGIN). Do not print credentials.
3. DRIVER: use ONLY the "user-Playwright" MCP (navigate/click/type/snapshot/screenshot). If the
   console uses a self-signed cert, run with HTTPS errors ignored. Start by navigating to the
   console URL and signing in.
4. For each page under docs-site/docs/**/*.md: read it top to bottom, check its preconditions,
   perform its numbered steps in the UI (reference/explanation pages have no steps — just confirm
   the "What an agent verifies here" artifacts are present), judge each outcome from the page's own
   "Healthy/Fail" prose, fill its "What you should see" table, and run its cleanup. Use the manifest
   below for each page's mode/priority and to skip pages marked n/a. Apply the test-scope
   exclusions on a shared env.
5. Repeat by tier (see "Repeatability protocol"). Name created resources doc-as-test-<page>-<runid>
   and clean them up.
6. Honor the known-drift notes in the manifest below (documented bugs are not new failures).
7. Output: a summary table (page · level · result) + a findings/ambiguity list.
```

## Manifest — the only off-page bookkeeping

`mode`: verify = presence only · behavior = perform the golden path · contract = a documented
negative/permission limit (kept as visible prose) · n/a = reference/overview, nothing to execute
(agent only confirms it renders). `priority`: P0 (golden) … P2 (presence).

| Page | mode | priority | personas | preconditions | known drift (not a failure) |
|---|---|---|---|---|---|
| getting-started/prerequisites | verify | P2 | any | none | — |
| getting-started/install | verify | P2 | admin | env-provided (bring-up not tested) | higress console uses a self-signed cert → trust it / provide your own |
| getting-started/first-training-job | behavior | P0 | member | workspace-with-quota, pullable-image | console phase can stay **Pending** after the job actually ran/succeeded |
| administration/manage-users | contract | P0 | admin, member | running-cluster | workspace list can render "No Data" while the API returns the objects |
| administration/manage-access-and-quota | behavior | P1 | admin | running-cluster | — |
| administration/manage-nodes | contract | P1 | admin | running-cluster | mutating node ops are destructive → presence/read-only on a shared env |
| administration/preflight-and-monitoring | behavior + contract | P1 | admin | running-cluster | — |
| administration/observability | verify + behavior | P2 | admin | behavior gated on `observability-installed` (primus-robust + Grafana) | missing stack surfaces as raw 504 / "datasource not found" → **BLOCKED**, not FAIL |
| administration/upgrading | verify | P2 | admin | running-cluster | create/approve/rollback are destructive → presence-only on a shared env |
| concepts/workspace | verify | P2 | any | none | — |
| concepts/workload-types | verify | P2 | any | none | — |
| concepts/fault-tolerance | verify | P2 | any | none | — |
| concepts/storage-and-data | verify | P2 | any | none | — |
| tasks/run-single-node-training | behavior | P0 | member | workspace-with-quota, pullable-image | phase-lag Pending drift (as above) |
| tasks/run-multi-node-training | behavior | P1 | member | workspace-with-quota, multiple-ready-nodes | phase-lag drift; gang placement slower on first image pull |
| tasks/interact-with-your-job | behavior | P0 | member | running-cluster, workspace-with-authoring-scope | WebShell needs a trusted cert (see exclusions) and a shell present in the image (pick `sh` on minimal images); SSH:2222 is out of a UI-only run |
| tasks/speed-up-startup | behavior | P2 | member | workspace-with-quota, harbor-registry | — |
| tasks/beyond-training | n/a (overview/hub) | — | any | none | — |
| tasks/run-cicd-runners | behavior | P1 | admin, member | workspace with `CICD` scope (surfaces Workloads → CICD) + ARC add-on + GitHub App | **BLOCKED** unless the CICD scope + ARC add-on are present |
| intro | verify | P2 | any | none | — |
| architecture | verify | P2 | any | none | — |
| faq | n/a (Q&A reference) | — | any | none | — |
| troubleshooting | n/a (runbook reference) | — | any | none | — |
| contributing | n/a (not product behavior) | — | any | none | — |

## Notes

- **Reference/overview pages** (concepts, intro, architecture, faq, troubleshooting, contributing)
  carry no procedure — the agent only confirms the "What an agent verifies here" artifacts render,
  or skips `n/a` pages.
- **Negatives are visible prose too.** Documented negatives/permissions (RBAC on manage-users,
  node/upgrade restrictions) are stated as visible pass/fail outcomes, not a hidden layer.
- **No prose convention is imposed.** Outcomes are free-form ("Healthy … / if you instead see …").
  If a predicate is repeatedly read ambiguously (see the ambiguity log), tighten the prose.
