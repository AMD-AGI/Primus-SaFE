# Kubernetes API Discovery 超时问题

**日期**: 2026-01-30  
**影响范围**: Dataplane Installer  
**严重程度**: Medium  
**状态**: 已修复 (部分)

## 问题描述

Dataplane Installer 在执行 `operators` 阶段检测现有 Operator 时，kubectl 命令频繁出现 API Discovery 超时错误：

```
couldn't get current server API group list: Get "https://10.32.17.185:6443/apis?timeout=32s": 
net/http: request canceled (Client.Timeout exceeded while awaiting headers)
```

当晚 30+ 次安装尝试中，有 2-3 次出现此问题（task 30、31、32）。

## 现象分析

### 环境状态
- 集群有 13 个节点处于 NotReady 状态（Kubelet 停止发送心跳）
- 8 个节点正常（3 control-plane + 5 worker）

### 奇怪的现象

1. **curl 正常，kubectl 超时**
   ```bash
   # 从 installer pod 内部测试
   curl -k https://10.32.17.185:6443/version  # 200, 3ms
   kubectl get clusterrole pgo               # 超时 2.5 分钟
   ```

2. **手动 exec 正常，installer 调用超时**
   ```bash
   # 手动进入 pod 执行
   kubectl exec -it installer-pod -- kubectl get clusterrole pgo  # 正常 <1s
   
   # installer 进程内调用
   # 超时 2.5 分钟
   ```

3. **/apis 端点响应正常**
   ```bash
   curl -k https://10.32.17.185:6443/apis  # 403 (正常，匿名用户无权限)
   ```

## 根因分析

### 问题 1: 多次创建临时 kubeconfig 文件 (已修复)

原始代码在 `DetectOperators` 中检测 6 个 Operator 时，每次调用 `ClusterRoleExists` 都会创建新的临时 kubeconfig 文件：

```go
// 原始实现 - 每次调用都创建新文件
func (h *HelmClient) ClusterRoleExists(ctx context.Context, kubeconfig []byte, name string) (bool, error) {
    kubeconfigFile, err := os.CreateTemp("", "kubeconfig-*.yaml")  // 每次新文件
    // ...
}
```

kubectl 的 API Discovery 缓存是基于 kubeconfig 文件路径的，新文件意味着每次都要重新获取 `/apis` 端点数据。

**修复**: 在 `DetectOperators` 开始时只创建一次临时文件，6 次检测复用同一个文件。

```go
// 修复后 - 只创建一次
func (h *HelmClient) DetectOperators(ctx context.Context, kubeconfig []byte) (*OperatorStatus, error) {
    kubeconfigFile, err := os.CreateTemp("", "kubeconfig-*.yaml")  // 只创建一次
    defer os.Remove(kubeconfigFile.Name())
    // ...
    for _, check := range checks {
        exists, err := h.clusterRoleExistsWithKubeconfig(ctx, kubeconfigFile.Name(), check.resource)
        // 复用同一个文件
    }
}
```

**Commit**: `d4f3916e`

### 问题 2: API Server 负载高时 Discovery 请求超时 (未完全解决)

即使修复了问题 1，当 API Server 负载高时，首次 `/apis` 请求仍可能超时。

**现象**:
- kubectl 默认的 Discovery 超时是 32 秒
- 在集群有大量 NotReady 节点时，API Server 处理 Discovery 请求可能变慢
- 每次超时后会重试 5 次，每次 32 秒，总计约 2.5 分钟

**为什么手动执行正常**:
- 手动执行时，kubectl 会在 `~/.kube/cache` 目录缓存 API Discovery 结果
- installer pod 中没有这个缓存目录，或者每次启动都是新的

## 复现步骤

1. 确保集群有较多 NotReady 节点（高负载）
2. 启动 Dataplane Installer 任务
3. 观察 `operators` 阶段的日志

## 解决方案

### 已实施

1. **复用 kubeconfig 文件** - 减少 API Discovery 次数从 6 次到 1 次

### 建议后续优化

1. **使用 client-go 替代 kubectl CLI**
   - client-go 有内建的连接池和重试机制
   - 不依赖外部进程，更可靠
   
2. **预热 kubectl 缓存**
   - 在 installer 启动时先执行一次简单的 kubectl 命令
   - 让 kubectl 建立 Discovery 缓存
   
3. **增加 Discovery 超时时间**
   - 通过环境变量设置更长的超时
   
4. **跳过 Operator 检测**
   - 在确定部署到新集群时，可以跳过检测直接安装

## 相关文件

- `modules/installer/pkg/installer/helm_client.go` - DetectOperators, ClusterRoleExists
- `modules/installer/pkg/installer/cluster_client.go` - ClusterClient (使用 client-go)

## 时间线

| 时间 | 事件 |
|------|------|
| 19:31 | task32 开始执行 |
| 19:31:34 | 进入 operators 阶段 |
| 19:32:06 | 首次 API Discovery 超时 |
| 19:34:14 | PGO 检测失败（约 2.5 分钟） |
| ... | 继续检测其他 operators |

## 经验教训

1. 在高负载集群环境下，任何依赖 K8s API 的操作都需要考虑超时和重试
2. 使用 kubectl CLI 会引入额外的 Discovery 开销，优先使用 client-go
3. 临时文件的创建策略会影响缓存行为，需要谨慎设计
