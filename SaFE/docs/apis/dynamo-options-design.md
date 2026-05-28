# DynamoDeployment API 增强：`dynamoOptions` 字段

## 1. 背景

`POST /api/v1/workloads`（`apiserver/pkg/handlers/resources/workload.go: createWorkload`）会在 `generateWorkload` 阶段把请求体里 `primus-safe.*` 前缀的 labels / annotations **全部 strip**：

```go
for key, val := range req.Annotations {
    if !strings.HasPrefix(key, v1.PrimusSafePrefix) {
        workload.Annotations[key] = val
    }
}
```

这套机制保护了 SaFE 内部命名空间，但**也封死了 DynamoDeployment 几个必需配置项**——它们恰好都是 `primus-safe.dynamo.*` 形式的 annotation：

| Annotation | 作用 | 当前 API 能否传 |
|------------|------|-----------------|
| `primus-safe.dynamo.service-roles` | 5 个 service role 槽位的实际身份（frontend / worker / prefill / decode / planner / epp） | ❌ |
| `primus-safe.dynamo.multinode.<role>` | 将某个 role 拉成跨节点 LeaderWorkerSet（值为节点数） | ❌ |
| `primus-safe.dynamo.backend-framework` | sglang / vllm / trtllm | ❌ |
| `primus-safe.dynamo.kv-transfer-backend` | nixl / mori / mooncake，PD 分离场景下 KV 传输平面 | ❌ |

结果是：**任何需要 multinode、PD 分离、显式角色组合的 DynamoDeployment，只能用 `kubectl apply` 直接喂 Workload CR**——前端、SDK、第三方调度都无法通过 SaFE REST API 创建这类工作负载。

---

## 2. 目标 / 非目标

### 目标
1. 让 DynamoDeployment 的 multinode、service-roles、backend-framework、kv-transfer-backend **可以通过 API 传**
2. 强类型字段（不要让调用方拼 annotation 字符串），减少错误
3. 校验逻辑**复用现有 webhook**（`validateDynamoDeployment`），不重复实现
4. 完全向后兼容：旧调用方不传新字段时行为不变

### 非目标
1. 不打破现有 `Annotations` 黑名单（`primus-safe.*` 仍然 strip）——这是 SaFE 的一致性原则
2. 不修改 webhook validator / mutator 的逻辑——它们读 annotation，与 annotation 的来源无关
3. 不为非 DynamoDeployment 的工作负载提供同类 API（如有需求再单独开口）

---

## 3. 设计选项对比

| 选项 | 实现方式 | 优点 | 缺点 |
|------|---------|------|------|
| **A. 加 `primus-safe.dynamo.*` 白名单** | `generateWorkload` 里增加 if 判断允许 dynamo.* annotation 通过 | 极简（3 行） | 调用方仍需拼字符串、无类型校验、未来扩展不友好 |
| **B. 加 `dynamoOptions` 结构化字段**（**采纳**） | `CreateWorkloadRequest` 新增 `*DynamoOptions`；`generateWorkload` 翻译到 annotation | 强类型、自文档化、validator 复用、未来易扩展 | 多一个 struct |
| C. raw passthrough | 直接接受 raw Workload yaml | 极致灵活 | 绕过所有 API 层校验，安全风险高 |

---

## 4. 采纳设计（选项 B）

### 4.1 API 结构

`apiserver/pkg/handlers/resources/view/workload_view.go` 中新增：

```go
type DynamoOptions struct {
    BackendFramework  string         `json:"backendFramework,omitempty"`
    KVTransferBackend string         `json:"kvTransferBackend,omitempty"`
    ServiceRoles      []string       `json:"serviceRoles,omitempty"`
    Multinode         map[string]int `json:"multinode,omitempty"`
}
```

并在 `CreateWorkloadRequest` 末尾添加：

```go
DynamoOptions *DynamoOptions `json:"dynamoOptions,omitempty"`
```

### 4.2 翻译逻辑

`apiserver/pkg/handlers/resources/workload.go: generateWorkload` 在所有现有字段处理完毕之后调用一个新增 helper：

```go
applyDynamoOptions(workload, req.DynamoOptions)
```

该 helper 把结构化字段写到对应 annotation：

| API 字段 | 翻译后的 annotation |
|----------|---------------------|
| `dynamoOptions.backendFramework` | `primus-safe.dynamo.backend-framework` |
| `dynamoOptions.kvTransferBackend` | `primus-safe.dynamo.kv-transfer-backend` |
| `dynamoOptions.serviceRoles=[...]` | `primus-safe.dynamo.service-roles="r0,r1,..."`（逗号拼接） |
| `dynamoOptions.multinode={k:v}` | 每对 `k -> v` 写为 `primus-safe.dynamo.multinode.<k>=<v>` |

未设置的字段不会写 annotation，让 `mutateDynamoDeployment`（mutating webhook）继续按 resources 数量推断默认值。

### 4.3 校验来源

完全复用现有 `validateDynamoDeployment`（[webhook](../../webhooks/pkg/workload_webhook.go)），它已经覆盖：

- service-roles 长度必须等于 `len(Resources)`
- service-roles 只能取 `frontend|worker|prefill|decode|planner|epp`，且 frontend 数量恰为 1
- worker 与 prefill/decode **互斥**（聚合 vs PD 分离）
- prefill 数量必须等于 decode 数量
- `multinode.<role>` 的 role 必须出现在 service-roles 中
- `multinode.<role>` 的值必须是 `>=1` 的整数
- backend-framework / kv-transfer-backend 的枚举值
- 资源数组上限 5（前一个 PR 加的 `maxDynamoResources=5`）

**API 增强不再做重复校验**——保持单一来源。

---

## 5. 调用示例

### 5.1 聚合 + 跨 2 节点 worker（用户当前实际场景）

```json
{
  "displayName": "dynamo-glm5-moe-2node",
  "workloadId": "dynamo-glm5-multinode",
  "workspaceId": "core42-hyperloom",
  "groupVersionKind": { "kind": "DynamoDeployment", "version": "v1" },
  "images": [
    "harbor.core42.primus-safe.amd.com/custom/sync/sglang-dynamo:1.1.0-rocm-202605271513",
    "harbor.core42.primus-safe.amd.com/custom/sync/sglang-dynamo:1.1.0-rocm-202605271513"
  ],
  "entryPoints": [
    "python3 -m dynamo.frontend --http-port 8000 --router-mode round-robin",
    "bash -c 'exec python3 -m dynamo.sglang --model-path /wekafs/models/DeepSeek-R1-0528 --tp-size 16 --ep-size 16 --nnodes 2 --node-rank $LWS_WORKER_INDEX --dist-init-addr $LWS_LEADER_ADDRESS:5000 --enable-dp-attention --attention-backend aiter --trust-remote-code --mem-fraction-static 0.7 --host 0.0.0.0'"
  ],
  "resources": [
    { "replica": 1, "cpu": "4",  "memory": "16Gi" },
    { "replica": 1, "cpu": "64", "gpu": "8", "memory": "256Gi", "sharedMemory": "200Gi", "rdmaResource": "1" }
  ],
  "env": { "HF_HOME": "/data/hf-cache", "NCCL_DEBUG": "INFO" },
  "service": { "protocol": "TCP", "port": 8000, "targetPort": 8000, "serviceType": "ClusterIP" },
  "dynamoOptions": {
    "serviceRoles": ["frontend", "worker"],
    "multinode":    { "worker": 2 }
  }
}
```

### 5.2 PD 分离（prefill 2 节点 + decode 2 节点）

```json
{
  "dynamoOptions": {
    "backendFramework":  "sglang",
    "kvTransferBackend": "nixl",
    "serviceRoles":      ["frontend", "prefill", "decode"],
    "multinode":         { "prefill": 2, "decode": 2 }
  }
}
```

### 5.3 简单单节点（不传 dynamoOptions，行为同旧版本）

```json
{ "dynamoOptions": null }
```

`mutateDynamoDeployment` 会按 `len(Resources)` 推断 `serviceRoles`（2 → frontend/worker、3 → frontend/prefill/decode），与旧行为完全一致。

---

## 6. 向后兼容

| 调用形式 | 兼容性 |
|----------|--------|
| 旧调用方不传 `dynamoOptions` | ✅ 行为完全不变（webhook 推断 + 不写 multinode = 单节点 Deployment） |
| 直接 `kubectl apply` 喂 Workload CR | ✅ 不走 API 路径，完全不受影响 |
| 新调用方传 `dynamoOptions` 但部分字段空 | ✅ 未填字段保持 webhook 推断 |

---

## 7. 实施变更范围

| 文件 | 改动 | 行数 |
|------|------|------|
| `apiserver/pkg/handlers/resources/view/workload_view.go` | 新增 `DynamoOptions` struct + `CreateWorkloadRequest.DynamoOptions` 字段 | +35 |
| `apiserver/pkg/handlers/resources/workload.go` | 新增 `applyDynamoOptions()` helper + 一行调用 | +28 |
| `docs/apis/dynamo-options-design.md` | 本设计文档 | 新建 |

总计 **3 个文件**，符合「单次改动 ≤ 5 个文件」要求。

---

## 8. 风险与缓解

| 风险 | 缓解 |
|------|------|
| `dynamoOptions.serviceRoles` 长度与 `resources` 不匹配 | webhook validator 已有检查，会 reject |
| `multinode` 引用了不存在的 role | webhook validator 已有检查 |
| 字段名拼写错（如 `backendFw`） | Go struct 标签强类型，JSON 不识别会被忽略；调用方在文档中能查到完整字段表 |
| 旧 client 老的 API 客户端不知道新字段 | 字段是可选的，不传无影响 |

---

## 9. 后续工作（不在本 PR 范围）

1. 给 SDK / Frontend 暴露同名字段
2. 在 OpenAPI / Swagger 文档自动生成中暴露 `DynamoOptions`
3. 给 `ListWorkload` 的 detail 视图回显这几个 annotation（如果对前端可见有需求）

---

## 10. 验证清单

实施后用以下场景跑通即可：

- [ ] 调 API 创建 `dynamoOptions.serviceRoles=["frontend","worker"], multinode={"worker":2}`，检查生成的 Workload CR 的 annotations 是否含 `primus-safe.dynamo.service-roles=frontend,worker` 和 `primus-safe.dynamo.multinode.worker=2`
- [ ] 调 API 创建不带 `dynamoOptions` 的 2-resource DynamoDeployment，检查 webhook 自动推断 `service-roles=frontend,worker`
- [ ] 调 API 时故意传 `serviceRoles=["frontend","frontend"]`（违反规则），检查 webhook validator reject
- [ ] 调 API 时故意传 `multinode={"unknown_role": 2}`（role 不在 ServiceRoles 中），检查 webhook validator reject
- [ ] 数据面看 dispatcher 是否正确产出 DCD + LWS（multinode 路径）
