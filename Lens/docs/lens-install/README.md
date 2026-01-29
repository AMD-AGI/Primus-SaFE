# Primus-Lens Dataplane 安装机制

本文档描述 Primus-Lens 控制平面如何管理数据平面（Dataplane）的安装流程。

## 概述

Dataplane 安装涉及以下组件：

| 组件 | 职责 |
|------|------|
| `cluster_sync_service` | 从 Primus-SaFE 同步集群信息到控制平面数据库 |
| `multi_cluster_config_sync` | 从目标集群同步存储配置，自动检测 storage_mode |
| `TriggerDeploy API` | 接收用户安装请求，创建安装任务 |
| `dataplane_installer job` | 轮询待处理任务，创建 K8s Job 执行安装 |
| `installer` | 实际执行安装的容器，运行 Helm 命令 |

## 状态定义

### dataplane_status

| 状态 | 说明 |
|------|------|
| `pending` | 等待安装 |
| `deploying` | 正在部署 |
| `deployed` | 部署完成 |
| `failed` | 部署失败 |

### storage_mode

| 模式 | 说明 |
|------|------|
| `external` | 使用外部存储（已有 Postgres/OpenSearch/Prometheus） |
| `lens-managed` | 由 Lens 管理存储（完整安装） |

## 安装流程

### 1. 集群发现与同步

```
Primus-SaFE Cluster CR
        │
        ▼
cluster_sync_service (每60秒)
        │
        ├─ 新集群 → 创建 ClusterConfig (dataplane_status=pending)
        │
        └─ 已存在 → 更新 K8S 连接信息
```

### 2. Storage 配置同步

```
multi_cluster_config_sync (每30秒)
        │
        ▼
连接到目标集群，检查 primus-lens-storage-config Secret
        │
        ├─ Secret 存在 → 同步存储配置到 DB
        │              → 如果 storage_mode 为空，设为 external
        │
        └─ Secret 不存在 → 跳过
```

### 3. 触发安装

用户通过 UI 或 API 触发安装：

```
POST /lens/v1/release/clusters/:cluster_name/deploy
        │
        ▼
TriggerDeploy Handler
        │
        ├─ 读取 ClusterConfig.StorageMode
        │
        ├─ 创建 ReleaseHistory 记录
        │
        └─ 创建 DataplaneInstallTask
           (StorageMode 来自集群配置)
```

### 4. 任务调度

```
dataplane_installer job (每30秒)
        │
        ▼
查询 status=pending 的 DataplaneInstallTask
        │
        ▼
为每个任务创建 K8s Job (primus-lens-installer)
        │
        ▼
标记任务为 running
```

### 5. 安装执行

Installer Job 根据 `storage_mode` 执行不同的安装阶段：

#### external 模式（已有存储）

```
StageInit              → 跳过（假设 DB 已配置）
StageDatabaseMigration → 执行数据库迁移
StageStorageSecret     → 创建/更新存储配置 Secret
StageApplications      → 安装应用组件 (Helm)
StageWaitApps          → 等待应用就绪
```

#### lens-managed 模式（完整安装）

```
StageOperators         → 安装 Operators (CloudnativePG, OpenSearch Operator)
StageWaitOperators     → 等待 Operators 就绪
StageInfrastructure    → 安装存储组件 (Postgres, OpenSearch, VictoriaMetrics)
StageWaitInfra         → 等待存储组件就绪
StageInit              → 初始化（创建数据库等）
StageDatabaseMigration → 执行数据库迁移
StageStorageSecret     → 创建存储配置 Secret
StageApplications      → 安装应用组件
StageWaitApps          → 等待应用就绪
```

### 6. 状态更新

```
Installer 执行中
        │
        ├─ 开始 → dataplane_status = deploying
        │
        ├─ 成功 → dataplane_status = deployed
        │
        └─ 失败 → dataplane_status = failed
                  (支持重试)
```

## 关键代码位置

| 文件 | 说明 |
|------|------|
| `adapter/.../cluster_sync_service.go` | 集群同步服务 |
| `jobs/.../multi_cluster_config_sync/syncer.go` | 存储配置同步 |
| `jobs/.../dataplane_installer/job.go` | 安装任务调度器 |
| `installer/pkg/installer/installer.go` | 安装执行逻辑 |
| `installer/pkg/installer/stages.go` | 安装阶段定义 |
| `api/pkg/api/release/handler.go` | TriggerDeploy API |

## 常见场景

### 场景 1：纳管已有存储的集群

1. 集群已部署 Postgres/OpenSearch/Prometheus
2. 已创建 `primus-lens-storage-config` Secret
3. `multi_cluster_config_sync` 检测到 Secret → `storage_mode = external`
4. 用户触发安装 → 只安装 Applications

### 场景 2：全新集群完整安装

1. 新集群，无存储组件
2. 用户在 UI 选择 `storage_mode = lens-managed`
3. 配置 `ManagedStorageConfig`（存储大小等）
4. 用户触发安装 → 完整安装流程

### 场景 3：重新安装/升级

1. 将 `dataplane_status` 改为 `pending`
2. 通过 Release Management 触发 Deploy
3. Installer 执行（幂等操作，跳过已存在的组件）

## 幂等性

所有安装阶段都是幂等的：

```go
// 示例：Infrastructure Stage
exists, healthy, err := helm.ReleaseStatus(ctx, config.Kubeconfig, config.Namespace, ReleaseInfrastructure)
if exists && healthy {
    log.Infof("Infrastructure already installed and healthy, skipping")
    return nil
}
```

这意味着：
- 重复执行不会导致错误
- 已安装的组件会被跳过
- 可以安全地重新触发安装

## 故障排查

### 安装卡在 pending

1. 检查 `dataplane_installer` job 是否运行
2. 检查 `DataplaneInstallTask` 表中的任务状态
3. 检查 K8s Job 是否创建成功

### 安装失败

1. 查看 `DataplaneInstallTask.error_message`
2. 查看 K8s Job 的 Pod 日志
3. 检查目标集群的 K8s 连接是否正常

### storage_mode 未自动检测

1. 确认目标集群有 `primus-lens-storage-config` Secret
2. 检查 `multi_cluster_config_sync` job 日志
3. 检查 K8s 连接凭证是否有效
