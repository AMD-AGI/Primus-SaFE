# GA v1.0.0 release gate — the negative surface

The docs are the spec, so anything **excluded** from GA must be **absent**. This gate makes
"absent" verifiable in the stable build, and is run as part of the docs-as-test **RELEASE** scope
(see [`RUN-DOCS-AS-TEST.md`](./RUN-DOCS-AS-TEST.md)) alongside the per-page verify/behavior/contract
blocks. It is not a published page (Docusaurus only builds `docs/`).

## Tier rules

- **Tier 1 removed + Tier 2 flag-off (default GA config):** the **backend route returns 404**.
- **Tier 3 hide-only:** the backend MAY still respond; only the **frontend route** is removed.
- **All tiers:** no excluded nav item, and no floating chatbot on any authenticated page.

## Backend endpoints that must 404 (default GA config)

Probe with the admin session; expect HTTP 404 (the standard NoRoute `"<uri> not found"` body).

| Feature | Probe | Why 404 |
|---------|-------|---------|
| A2A REST API | `GET /api/v1/a2a/services` | `a2a.enabled=false` (registration gated) |
| LLM Gateway | `GET /api/v1/llm-proxy/v1/models` | `llm_gateway.enabled=false` |
| Model Optimization | `GET /api/v1/optimization/tasks` | `model_optimization.enabled=false` (flag now honored) |
| MCP server | `GET /api/v1/safe-mcp/mcp` | `mcp.enabled=false` |
| InferenceX | any InferenceX endpoint | handler package removed entirely |

> Not 404 (Tier 3 — backend intentionally kept): post-train, evaluation, Dynamo/Optimus, RayJob,
> github-workflow. These are asserted at the frontend layer only. **CICD** (`cd-handlers`) is a
> **shipping GA feature** — both its backend and its frontend (`/cicd`) are included; it appears
> under **Workloads → CICD** for any workspace that has the `CICD` scope enabled.

## Frontend routes that must be unreachable

Navigating directly to each must land on the SaFE **NotFound/404** (the experimental page must NOT
render), and none may appear in the left nav:

`/rayjob` · `/monarch` · `/sandbox-workload` · `/dynamo` · `/optimus` · `/posttrain` ·
`/playground-agent` · `/model-square` · `/chatbot` · `/qabase` · `/feedback-management` ·
`/dataset` · `/evaluation` · `/model-optimization` · `/tools` · `/sandbox` · `/litellm-gateway` ·
`/a2a` · `/claw` (and their `/detail` variants).

Also:

- the **floating chatbot** widget is absent on every authenticated page;
- the homepage shows **no** "Go to Lens" / "Go to Hyperloom" buttons;
- the **Lens SPA** at `/lens/` does not load (while the `/lens/v1/*` API proxy and `/lens/grafana/*`
  embeds remain available).

## Must remain reachable (included surface sanity check)

Core API returns 200; these nav routes load: `/` · `/training` · `/torchft` · `/authoring` ·
`/infer` · `/workspace` · `/nodes` · `/clusters` · `/deploy` · `/download` (Datasync) · `/images` ·
`/registries` · `/secrets` · `/manageapikeys` · `/fault` · `/preflight`. `/cicd` loads for a
workspace with the `CICD` scope (and appears in the nav for it). The `/lens/v1/*` and
`/lens/grafana/*` proxies stay reachable.

<!-- @test
scope: page
mode: contract
priority: P0
targets: [console]
personas: [admin]
do: probe each excluded backend endpoint and navigate to each excluded frontend route per the tables above
expect:
  - every listed backend endpoint returns 404 in the default GA build
  - every listed frontend route renders the NotFound page and is absent from the nav
  - no floating chatbot on any authenticated page; homepage has no "Go to Lens" or "Go to Hyperloom"; the /lens/ SPA does not load
  - included surface still works: core API 200, core nav present (incl. /cicd for a CICD-scoped workspace), /lens/v1 and /lens/grafana proxies reachable
cleanup: none (read-only)
-->
