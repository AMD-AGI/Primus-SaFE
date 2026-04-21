# Frontend TODO: render "Robust addon not installed" placeholder

## Background

SaFE's apiserver now serves the legacy `/lens/v1/*` URL prefix via a
per-request dynamic proxy (`apiserver/pkg/handlers/lens-compat`). On every
call it looks at the `?cluster=<name>` query parameter, resolves the
corresponding Cluster CR's `primus-safe.amd.com/robust-api-endpoint`
annotation through the shared `robustclient`, and reverse-proxies to that
data cluster's robust-analyzer.

If the user's currently-selected workspace belongs to a cluster that does
**not** have the primus-robust addon installed (e.g. the management
`control-plane` cluster, or any data cluster a site hasn't opted into
observability on yet), the handler returns a dedicated error:

```
HTTP 404
{
  "errorCode": "Primus.00050",
  "errorMessage": "primus-robust addon is not installed on cluster \"<name>\"; metrics and logs endpoints are unavailable"
}
```

The error code constant is defined in
`SaFE/common/pkg/errors/error_code.go` as
`RobustAddonNotInstalled = "Primus.00050"` and produced by
`NewRobustAddonNotInstalled(cluster string)`.

This is a **missing-feature signal, not a failure**. The frontend should
render an empty-state placeholder in affected panels ("This cluster does
not have Robust installed, metrics/logs are unavailable") rather than
bubbling a red `ElMessage` error toast.

## What the frontend needs to do

### 1. Expose a helper next to the existing axios instances

File: `Web/apps/safe/src/services/request.ts`

Add a shared constant and predicate so every caller uses the same code
point:

```ts
export const ROBUST_ADDON_NOT_INSTALLED_CODE = 'Primus.00050'

export function isRobustAddonNotInstalled(err: unknown): boolean {
  if (!err || typeof err !== 'object') return false
  const anyErr = err as any
  if (anyErr.errorCode === ROBUST_ADDON_NOT_INSTALLED_CODE) return true
  const data = anyErr?.response?.data
  if (data && typeof data === 'object') {
    if (data.errorCode === ROBUST_ADDON_NOT_INSTALLED_CODE) return true
    if (data.code === ROBUST_ADDON_NOT_INSTALLED_CODE) return true
  }
  return false
}
```

### 2. Silence the global error toast for this error code

File: `Web/apps/safe/src/services/request.ts` → inside
`attachInterceptors` → response error branch.

Right after the `skipErrorHandler` short-circuit, detect the code and
reject with a structured biz error **without** calling `ElMessage`:

```ts
if (isRobustAddonNotInstalled(error)) {
  const data = error?.response?.data ?? {}
  const bizErr = new Error(
    data.errorMessage || data.message || 'robust addon not installed',
  ) as any
  bizErr.errorCode = ROBUST_ADDON_NOT_INSTALLED_CODE
  bizErr.errorMessage = data.errorMessage || data.message || ''
  return Promise.reject(bizErr)
}
```

Do the same inside the 2xx branch of the interceptor — apiserver sometimes
wraps errors in a 200 + body envelope for legacy handlers, so check both
paths.

### 3. Per-panel empty-state handling

#### Home page GPU Utilization & Allocation

File: `Web/apps/safe/src/pages/Homepage/index.vue`

Add a `robustNotInstalled` ref. In `fetchGPUData`:

- Reset `robustNotInstalled.value = false` at the top.
- On success: clear it and render as today.
- On error: if `isRobustAddonNotInstalled(error)` is true, set it to
  `true`, clear `gpuData.value`, re-render chart with the placeholder
  branch. Otherwise fall through to the existing `console.error`.

In the chart empty state (`renderGPUChart` currently emits
`text: 'No Data'`), swap in a longer message when `robustNotInstalled` is
true, e.g. **"Robust is not installed on this cluster"** + sub-text
**"Install the primus-robust addon to see GPU utilization"**. Keep the
axis / tooltip / legend hidden in this state.

Also inline this state in any summary cards (Total Workloads / Avg
Allocation / Avg Utilization / Low Utilization) that depend on the same
data: show `—` with a small inline hint, don't show `0` which can be
confused with "genuinely zero utilization".

#### Other `/lens/v1/*` consumers

Run a search for `lensRequest.` to find every place that calls the
lens-compat proxy. Each one should either:

- Delegate to a wrapped API helper that already catches and re-exposes
  the robust-not-installed signal, or
- Handle it inline the same way the home page does.

Minimum set to cover:

| File | Feature |
| ---- | ------- |
| `Web/apps/safe/src/services/workload/index.ts` — `getGPUAggregation`, `getGPUAggregationByWorkload` | Home page GPU panels, Workload list GPU column |
| `Web/apps/safe/src/pages/Training/TrainingDetail.vue`, `TorchFT/TorchFTDetail.vue`, `RayJob/RayJobDetail.vue`, etc. — anywhere that embeds the Grafana training-workload dashboard or reads `training_perf_*` variables | Training detail pages |
| `Web/apps/lens/src/services/gpu-aggregation/index.ts` — Lens-only views | Standalone Lens app |

For embedded Grafana dashboards the apiserver-side `lens-compat` handler
already returns a clear JSON error; Grafana renders a panel-level error
automatically, but a top-of-page info banner explaining "Robust is not
installed on this cluster, dashboards are unavailable" keeps the signal
consistent with the home page.

### 4. Optional: disable menu entries that require Robust

Workload detail tabs that are pure Robust-backed (Training Performance,
GPU Utilization, Logs) could be greyed out at navigation time by reading
the current cluster's "has robust" flag. A pragmatic way to do that
without a new API:

1. On workspace switch (`useWorkspaceStore.setCurrentWorkspace`), probe
   `GET /lens/v1/health` (add a trivial robust-api endpoint that always
   returns 200) with `skipErrorHandler: true`.
2. Cache the outcome in the cluster store as `hasRobust: boolean`.
3. Use that flag in route guards / tab visibility.

This is a nice-to-have, not required for the initial fix.

## Backend contract reference

When updating the frontend, rely only on the error code, not the HTTP
status or message text — the status is currently 404 but may move and the
message is user-facing copy that will be adjusted.

```
errorCode:    "Primus.00050"
errorMessage: human-readable, safe to render as a fallback
HTTP status:  404 (current default, do not assert on it)
```

## Out of scope for this task

- Don't wire `?cluster=` into the Grafana iframe URL; the dashboard
  variable already covers that path.
- Don't try to auto-install the addon from the UI — provisioning is
  handled by `resource-manager` via the AddonController.
- No backend changes needed beyond the apiserver bits already merged in
  `fix/luochen/migrate-to-robust`.
