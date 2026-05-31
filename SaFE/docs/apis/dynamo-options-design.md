# DynamoDeployment API: `dynamoOptions`

`dynamoOptions` is an optional structured field on `POST /api/v1/workloads`
(`CreateWorkloadRequest`) for the `DynamoDeployment` kind. The apiserver
translates each field into a `primus-safe.dynamo.*` annotation consumed by the
webhook and dispatcher. All fields are optional; an unset field keeps the
webhook's default inference.

## Fields

```go
type DynamoOptions struct {
    BackendFramework  string   `json:"backendFramework,omitempty"`
    KVTransferBackend string   `json:"kvTransferBackend,omitempty"`
    ServiceRoles      []string `json:"serviceRoles,omitempty"`
    MultinodeRoles    []string `json:"multinodeRoles,omitempty"`
}
```

| Field | Type | Annotation | Description |
|-------|------|------------|-------------|
| `backendFramework` | string | `primus-safe.dynamo.backend-framework` | Inference backend: `sglang` \| `vllm` \| `trtllm`. Default `sglang`. |
| `kvTransferBackend` | string | `primus-safe.dynamo.kv-transfer-backend` | KV transfer plane for PD disaggregation: `nixl` \| `mori` \| `mooncake`. Default `nixl`. |
| `serviceRoles` | []string | `primus-safe.dynamo.service-roles` | Role of each `resources[i]` slot, positional (comma-joined). Allowed: `frontend` \| `worker` \| `prefill` \| `decode` \| `planner` \| `epp`. |
| `multinodeRoles` | []string | `primus-safe.dynamo.multinode-roles` | Roles that run as a multi-node LeaderWorkerSet (comma-joined). See below. |

When `serviceRoles` is omitted, the webhook infers it from `len(resources)`:
2 → `frontend,worker`, 3 → `frontend,worker,planner`.

## Multinode vs. replica

`resources[i].replica` is interpreted according to `multinodeRoles`:

| Role listed in `multinodeRoles`? | `replica` means | Result |
|---|---|---|
| Yes | LWS node count | One LeaderWorkerSet group of `replica` pods — a single tensor-parallel model spanning nodes. |
| No | Deployment replica count | `replica` independent single-node instances, load-balanced by the frontend. |

For a role in `multinodeRoles` with `replica > 1`, the dispatcher:

1. Sets the DGD service `multinode.numberOfNodes = replica` and forces `replicas = 1`.
2. Appends `--nnodes <replica> --node-rank $LWS_WORKER_INDEX --dist-init-addr $LWS_LEADER_ADDRESS:5000` to the sglang launcher command. The `$LWS_*` variables are LWS-injected per pod and expanded by the launcher's bash at container start.

The caller's worker entrypoint only specifies parallelism (`--tp-size`, etc.) and
must NOT hand-write `--nnodes` / `--node-rank` / `--dist-init-addr` or wrap in
`bash -c`. Any of these flags already present is kept (per-flag dedup).

## Validation

Enforced by `validateDynamoDeployment`:

- `len(serviceRoles) == len(resources)`
- roles ∈ `frontend|worker|prefill|decode|planner|epp`; exactly one `frontend`; at most one `planner`
- `worker` is mutually exclusive with `prefill`/`decode`
- `count(prefill) == count(decode)`
- every role in `multinodeRoles` must appear in `serviceRoles`
- `backendFramework` / `kvTransferBackend` must be valid enum values
- `len(resources) <= 5`

## Examples

`entryPoints` are base64-encoded in real requests; shown here as plaintext.

### Aggregated, single node (TP=8)

```json
{
  "groupVersionKind": { "kind": "DynamoDeployment", "version": "v1" },
  "entryPoints": [
    "python3 -m dynamo.frontend --http-port 8000 --router-mode round-robin",
    "exec python3 -m dynamo.sglang --model-path /wekafs/models/DeepSeek-R1-0528 --tp-size 8 --ep-size 8 --attention-backend aiter --trust-remote-code --mem-fraction-static 0.75 --host 0.0.0.0"
  ],
  "resources": [
    { "replica": 1, "cpu": "4",  "memory": "16Gi" },
    { "replica": 1, "cpu": "64", "gpu": "8", "memory": "256Gi", "sharedMemory": "200Gi" }
  ],
  "dynamoOptions": { "serviceRoles": ["frontend", "worker"] }
}
```

### Aggregated, multi-node (TP=16 across 2 nodes)

```json
{
  "entryPoints": [
    "python3 -m dynamo.frontend --http-port 8000 --router-mode round-robin",
    "exec python3 -m dynamo.sglang --model-path /wekafs/models/DeepSeek-R1-0528 --tp-size 16 --ep-size 16 --enable-dp-attention --attention-backend aiter --trust-remote-code --mem-fraction-static 0.7 --host 0.0.0.0"
  ],
  "resources": [
    { "replica": 1, "cpu": "4",  "memory": "16Gi" },
    { "replica": 2, "cpu": "64", "gpu": "8", "memory": "256Gi", "sharedMemory": "200Gi", "rdmaResource": "1" }
  ],
  "dynamoOptions": {
    "serviceRoles":   ["frontend", "worker"],
    "multinodeRoles": ["worker"]
  }
}
```

`worker.replica = 2` means 2 nodes; the dispatcher injects the multi-node flags.

### PD disaggregation

```json
{
  "dynamoOptions": {
    "kvTransferBackend": "nixl",
    "serviceRoles":      ["frontend", "prefill", "decode"]
  }
}
```

Set `resources[prefill].replica` / `resources[decode].replica` > 1 for multiple
independent prefill/decode instances. To make prefill/decode span nodes instead,
add them to `multinodeRoles`.
