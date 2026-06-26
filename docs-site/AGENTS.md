# Docs-as-test: agent operating contract

This documentation site **is** the test suite. The same pages customers read are the spec an
agent executes against a live Primus-SaFE. There is no separate scripted test to drift out of
sync — the only way a test is wrong is if the **doc** is wrong, which is exactly what we want to
catch. A run **double-verifies the docs against the product**: a failure means either the doc
lies or the product regressed, and the report says which.

> This file is not part of the published site (Docusaurus only builds `docs/`). It is the
> standing contract for any agent asked to "test the docs."

## The invisible test annotation

Test intent lives **inside the doc page**, next to the prose it governs, in HTML comments that
**never render** for customers (Docusaurus 3, classic Markdown — comments are already used and
build fine). One mechanism, two scopes:

A **page-scope** block right under the title sets defaults:

```markdown
<!-- @test
scope: page
mode: verify            # verify | behavior | contract  (default depth for this page)
priority: P2            # P0 (golden behavior) … P2 (presence-only)
targets: [console]      # console (live install) and/or docs (static site)
-->
```

A **claim-scope** block sits immediately after a falsifiable statement. It is a **thin overlay**,
not a re-description of the page. The **prose is the procedure**; the annotation only adds what
prose can't carry: the test metadata, the **pass/fail predicates**, and cleanup. `do:` is a
one-line *pointer* to the doc's own steps — never a copy of them:

```markdown
Create a local user; they can log in but have **no workspace access** yet.

<!-- @test
mode: contract
priority: P0
personas: [admin, member]
preconditions: [running-cluster]
do: follow this page's "From the console (UI)" steps to create a default user, then sign out and sign in as them
expect:
  - the new user can sign in
  - as that default user: no System admin section in the nav; Nodes/Clusters/Users not reachable; only public/granted workspaces listed
  - after an admin freezes them, they can no longer sign in
cleanup: as admin, delete the user via its row action
-->
```

### The single-source rule (this is what keeps it maintainable)

The annotation must **never restate steps the prose already contains.** If you are tempted to copy
a sentence of procedure into the block, stop — reference the section instead (`do: follow "…"`).
The agent reads the **prose** for *what to do* and the **annotation** for *what to check*. So:

- Steps live **once** (prose). Change the procedure → edit prose only; the test follows it
  automatically because the agent executes the prose as written.
- The annotation changes **only** when the *test intent* changes: a new falsifiable claim (add one
  `expect:` line), a level/priority/persona change, or different cleanup/fixtures.
- This is the anti-"two-directions" guarantee: there is no second copy of the procedure to drift.

Keep blocks small — typically `mode`, `priority`, a one-line `do:` pointer, 1–4 `expect:`
predicates, and `cleanup`. If a block is growing into a script, the prose is probably missing
steps; fix the prose, not the annotation.

Other rules:
- Plain YAML inside an HTML comment. **Never use a literal double-hyphen** inside a comment (it
  terminates the comment) — write "create/update/delete", not the dashed form.
- `{{baseUrl}}` is substituted from the selected target (see the install page's `targets`).
- A claim block overrides the page block for the lines it covers.
- A build-time remark plugin (`remarkStripTestAnnotations` in `docusaurus.config.ts`) removes
  these comments from the published HTML/JS — invisible to customers, present in `.md` for the agent.
- **Placement:** never make a `@test` comment the *first* body node on a page — put the page-scope
  block after the opening paragraph (Docusaurus derives the page/card description from the first
  content, so a leading comment would leak into it).

## Test levels (the standing answer to "what matters")

| `mode` | What the agent does | Cost | Use for |
|--------|---------------------|------|---------|
| `verify` | Assert the documented page/control/field/value **exists**. | cheap | Untouched features; install & setup (env is provided, never tested) |
| `behavior` | **Perform** the documented procedure; assert the documented **outcome**. | live env | Golden paths: submit training, authoring, create user |
| `contract` | Documented **negatives / limits / permissions**. | live env | RBAC ("no access until granted"), freeze, quota rejection, taints |

`priority` (P0…P2) + `mode` are how importance is encoded **once**. New feature → the author adds
a `behavior` block in the same PR and the test exists automatically. Untouched feature → it keeps
its `verify` block. Nobody re-explains what to test.

## Maintaining this over time

The maintenance burden is **the docs you were going to write anyway** plus a few lines of
overlay. The discipline:

1. **Write/edit the prose normally.** The numbered steps and the stated outcome are the spec.
2. **Touch the `@test` block only when test intent changes** — a new checkable claim, a changed
   level/priority/persona, or different cleanup. Most prose edits need **no** annotation change.
3. **Never duplicate prose into the annotation** (the single-source rule above). A `@test` block
   that reads like a rewrite of the section is a smell — thin it back to `do:` + `expect:`.
4. **If you can't write a crisp `expect:` from the page, the page is too vague** — add a
   `@test todo:` and improve the prose. Vague docs and missing tests are the same defect here.
5. **Keep the coverage map (bottom) honest** — it is the one place to see what is tested vs TODO.

A good review check on any docs PR: *did the prose change the procedure? then the test already
changed (the agent follows prose). Did it add a new promise to the reader? then it needs one
`expect:` line.*

## How to run

1. Walk `docs/**/*.md`. For each page, read its `@test` blocks.
2. Run each block at its `mode`/`priority`. On a **docs PR**, run `verify` (plus changed pages'
   `behavior`). On a **release**, run all `behavior` + `contract` against the provided env.
3. **Drive and assert through the UI by default.** Navigate, read the rendered page, click the
   documented controls — exercise the product the way the docs tell a user to. A direct API call
   (via the browser's `browser_evaluate` in-page `fetch`, reusing the session) or `kubectl` is
   allowed **only as an optional ground-truth cross-check** to expose a UI/API drift — never as
   the primary assertion. If a documented procedure has *no* UI path (API-only in the doc), mark
   it `@test todo:` until a console path is documented, rather than asserting via API.
4. Cross-check ground truth where useful (e.g. `kubectl` vs the console phase) and report drift.
5. Report **PASS / FAIL / BLOCKED** per `expect`, quoting *doc says X, product does Y*. BLOCKED
   (precondition missing) is never a silent PASS.
6. **Always run `cleanup`** — behavior/contract tests create uniquely-suffixed resources
   (`doc-as-test-<page>-<runid>`) and delete them so re-runs don't collide.

Driver: any browser MCP (Cursor's built-in tab, or `@playwright/mcp` with
`--browser=chromium`). The driver is interchangeable; the doc is the test.

## Environment contract

The test env is **provided, not built** — a running Primus-SaFE with seeded `root`/`root`, a few
nodes, one cluster, one workspace with quota. Install/bring-up is therefore `verify`-only.

## Findings this approach has already caught

- Doc said `hosts.yaml`; `Bootstrap/bootstrap.sh` reads `hosts.ini`. (fixed)
- Workspace list and PyTorch "My Workloads" list render "No Data" while the REST API returns the
  objects (only the "All" filter shows them).
- A submitted PyTorchJob ran to completion (`kubectl`: PyTorchJob `Succeeded`, pod `Completed`)
  while the console workload **phase stayed `Pending`** the entire lifecycle.

## Product-side changes that make this cheap and deterministic

- `data-testid` on key controls (nav items, Create-* buttons, form fields, the workload phase
  cell) so the agent targets stable hooks instead of giant snapshots / coordinate clicks.
- Stable, linkable routes for every page (today `/workspaces` and `/workloads` 404; the real
  routes are `/workspace` and `/training`). Make nav items real links.
- Fix the UI/API list discrepancies above so an agent does not burn turns deciding if its action
  failed.

## Coverage map (the in-repo tracker — maintain alongside the docs)

**Test status:** `Done` = runnable `@test` block landed · `TODO` = doc rich enough but no block
yet · `Partial` = only `@test todo:` (doc lacks a UI path or detail) · `Stub` = doc too thin to
test, write content first.
**Doc status:** `Rich` = substantial prose · `Draft` = has gaps ("Not yet covered") · `Stub` = placeholder.

| Page | intended mode / priority | doc | test |
|------|--------------------------|-----|------|
| getting-started/install | verify · P2 | Rich | **Done** |
| getting-started/first-training-job | behavior · P0 | Rich | **Done** |
| getting-started/prerequisites | n/a (static reference) | Rich | n/a |
| administration/manage-users | contract · P0 | Rich | **Done** |
| administration/manage-access-and-quota | behavior · P1 | Rich | **Done** |
| administration/manage-nodes | contract · P1 | Rich | **Done** (read-only; mutating ops TODO) |
| administration/preflight-and-monitoring | behavior + contract · P1 | Rich | **Done** (pre-flight submit + faults read-only) |
| administration/upgrading | n/a (CLI procedure) | Rich | n/a |
| tasks/run-single-node-training | n/a (submit covered by first-training-job) | Rich | n/a |
| tasks/run-multi-node-training | behavior · P1 | Rich | **Done** (needs ≥2 ready nodes) |
| tasks/interact-with-your-job (incl. Authoring) | behavior · P0 | Rich | **Done** |
| tasks/speed-up-startup | behavior · P2 | Rich | **Done** (import + preheat; gated on harbor-registry) |
| tasks/beyond-training | n/a (overview/hub) | Rich | n/a |
| concepts/workspace | verify · P2 | Rich | TODO (assert named artifacts only) |
| concepts/workload-types | verify · P2 | Rich | TODO (assert named artifacts only) |
| concepts/fault-tolerance | verify · P2 | Draft | TODO |
| concepts/storage-and-data | verify · P2 | Draft | TODO |
| architecture | verify · P2 | Rich | TODO |
| intro | verify · P2 | Rich | TODO |
| faq | n/a (Q&A reference) | Rich | n/a |
| troubleshooting | n/a (runbook) | Rich | n/a |
| contributing | n/a | Rich | n/a (not product behavior) |
