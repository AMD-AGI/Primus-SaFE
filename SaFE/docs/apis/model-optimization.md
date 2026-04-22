# Model Optimization API

The Model Optimization API wraps a PrimusClaw session running the Hyperloom
`inference-optimization` skill as a first-class SaFE resource.

## Summary

- Input: a downloaded SaFE `Model` (`phase=Ready`) plus optimization parameters
- Runtime: SaFE creates a Claw session, submits a Hyperloom prompt, and proxies
  the Claw SSE stream
- Output: persistent `OptimizationTask` records, structured events, artifacts,
  and an optional one-click `apply` to create an inference `Workload`

## Endpoints

Base path: `/api/v1/optimization`

### Create task

`POST /tasks`

Omitted fields use the same defaults as Hyperloom-Web `useInferOptTemplate.ts`
(`mode`: `local`, `kernelBackends`: `["Claude Code"]`, framework images, etc.).

```json
{
  "displayName": "qwen3-opt",
  "modelId": "model-abc",
  "workspace": "control-plane-sandbox",
  "mode": "local",
  "framework": "sglang",
  "precision": "FP4",
  "tp": 1,
  "ep": 1,
  "gpuType": "MI355X",
  "isl": 1024,
  "osl": 1024,
  "concurrency": 64,
  "kernelBackends": ["Claude Code"],
  "geakStepLimit": 100,
  "image": "harbor.oci-slc.primus-safe.amd.com/custom/lmsysorg/sglang:202603270958",
  "inferencexPath": "/hyperloom/InferenceX",
  "resultsPath": "/workspace/hyperloom/"
}
```

Use `"mode": "claw"` when you want the RayJob / workspace submission block in the
Hyperloom prompt (same as switching to claw in the UI).

Response:

```json
{
  "id": "opt-xxxx",
  "clawSessionId": "session_xxx"
}
```

### Batch create

`POST /tasks/batch`

```json
{
  "items": [
    {
      "displayName": "qwen3-opt",
      "modelId": "model-abc",
      "workspace": "control-plane-sandbox"
    }
  ]
}
```

### List / detail / delete

- `GET /tasks`
- `GET /tasks/{id}`
- `DELETE /tasks/{id}`

### Structured event stream

`GET /tasks/{id}/events`

SSE event types:

- `phase`
- `benchmark`
- `kernel`
- `log`
- `done`

The `data:` payload is the full event envelope:

```json
{
  "id": "opt-xxxx-1",
  "taskId": "opt-xxxx",
  "type": "phase",
  "timestamp": 1710000000000,
  "payload": {
    "phase": 2,
    "phaseName": "Baseline",
    "status": "started"
  }
}
```

### Artifacts

- `GET /tasks/{id}/artifacts`
- `GET /tasks/{id}/artifacts/download?path=<session-relative-path>`

Artifacts are proxied from the underlying Claw session. Typical files include:

- `claw-1/optimization_report.md`
- `claw-1/results/...`
- `claw-1/kernel_tasks.json`

### Lifecycle

- `POST /tasks/{id}/interrupt`
- `POST /tasks/{id}/retry`

`retry` clones the failed/interrupted task into a new task so the original
history remains intact.

### Apply optimized result

`POST /tasks/{id}/apply`

Optional overrides:

```json
{
  "displayName": "qwen3-optimized-infer",
  "workspace": "control-plane-sandbox",
  "image": "harbor.example/sglang:latest",
  "cpu": "16",
  "memory": "64Gi",
  "gpu": "1",
  "replica": 1,
  "port": 8888
}
```

Behavior:

1. Finds `optimization_report.md` from the Claw session artifacts
2. Extracts the recommended `sglang.launch_server` or `vllm serve` command
3. Falls back to a framework-derived default command if the report has no block
4. Creates a SaFE `Workload` (`Deployment`) with `PRIMUS_SOURCE_MODEL` and
   `MODEL_PATH` envs set

Response:

```json
{
  "taskId": "opt-xxxx",
  "workloadId": "qwen3-optimized-infer-abcde",
  "displayName": "qwen3-optimized-infer",
  "launchCommand": "python3 -m sglang.launch_server --model-path ...",
  "reportPath": "claw-1/optimization_report.md"
}
```
