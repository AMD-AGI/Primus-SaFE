# Cluster Overview Cache Job

## 概述

此 Job 用于定期采集集群概览数据并缓存到数据库，以提升 API 性能，特别是在大规模集群环境中。

## 背景

原有的 `getClusterOverview` API 在大规模集群中存在严重的性能问题：

### 原有性能瓶颈

对于一个有 **N 个节点**，每个节点平均 **P 个 GPU Pod** 的集群：

- **K8s API 调用**: `3N + 2N*P` 次（极其昂贵）
- **Prometheus 查询**: ~5 次
- **时间复杂度**: `O(N*P)`

详细调用链：
1. `gpu.GetGpuNodes` - O(N) K8s API List 调用
2. `fault.GetFaultyNodes` - **O(N)** - N 次 K8s API Get 调用 ⚠️
3. `gpu.GetGpuNodeIdleInfo` - **O(N*P)** - 每个节点调用 kubelet API ⚠️
4. `gpu.CalculateGpuUsage` - 1 次 Prometheus 查询
5. `gpu.GetClusterGpuAllocationRate` - **O(N*P)** - 重复计算 ⚠️
6. `storage.GetStorageStat` - 1 次数据库 + 2 次 Prometheus
7. `rdma.GetRdmaClusterStat` - 2 次 Prometheus

## 解决方案

### 架构设计

```
┌─────────────────┐         ┌──────────────────┐         ┌─────────────┐
│   Cluster       │ 30s周期 │  Database        │         │   API       │
│   Overview Job  │────────>│  Cache Table     │<────────│  Endpoint   │
│                 │  写入    │                  │  读取    │             │
└─────────────────┘         └──────────────────┘         └─────────────┘
```

### 核心组件

1. **数据库表**: `cluster_overview_cache`
   - 存储预计算的集群概览数据
   - 按 `cluster_name` 作为唯一索引
   - 包含所有统计指标

2. **定时 Job**: `ClusterOverviewJob`
   - 每 30 秒执行一次
   - 采集所有指标并存储到缓存表
   - 支持多集群环境

3. **API 增强**: `getClusterOverview`
   - 优先从缓存读取
   - 缓存未命中时回退到实时计算（向后兼容）

## 性能提升

### 优化后性能

- **API 响应时间**: 从 `O(N*P)` 降至 `O(1)` - **常数时间**
- **数据库查询**: 仅 1 次，基于索引查询
- **K8s API 调用**: 0 次（由 Job 异步处理）
- **响应速度**: 预期提升 **100-1000倍**（取决于集群规模）

### 对比示例

假设集群规模：1000 个节点，每节点 10 个 Pod

| 指标 | 原方案 | 新方案 | 提升 |
|------|--------|--------|------|
| K8s API 调用 | ~23,000 次 | 0 次 | ∞ |
| 响应时间 | 30-60秒 | <50ms | 600-1200x |
| API 服务器负载 | 高 | 极低 | - |

## 使用方法

### 1. 数据库迁移

运行 SQL 迁移脚本创建缓存表：

```bash
psql -h <host> -U <user> -d <database> -f Lens/modules/core/pkg/database/migrations/cluster_overview_cache.sql
```

### 2. 启动 Jobs 服务

Job 会自动注册并开始运行：

```bash
cd Lens/modules/jobs
go run ./cmd/primus-lens-jobs
```

Job 将每 30 秒更新一次缓存。

### 3. API 自动使用缓存

无需修改 API 调用方式，现有的 API 端点会自动使用缓存：

```bash
curl http://localhost:8080/api/clusters/overview
```

或指定集群：

```bash
curl http://localhost:8080/api/clusters/overview?cluster=prod-cluster
```

## 多集群支持

系统完全支持多集群环境：

1. **Job 端**: 每个集群运行独立的 Job 实例
   - Job 使用当前集群名称
   - 数据写入对应的 `cluster_name` 行

2. **数据库**: 使用 `cluster_name` 作为唯一键
   - 每个集群一行记录
   - 独立更新，互不影响

3. **API 端**: 通过查询参数指定集群
   - `?cluster=<name>` - 查询指定集群
   - 不指定则使用默认集群

## 配置选项

### 调整更新频率

在 `cluster_overview.go` 中修改 `Schedule()` 方法：

```go
func (j *ClusterOverviewJob) Schedule() string {
    // 选项：
    // "@every 30s" - 30秒（默认，推荐）
    // "@every 1m"  - 1分钟（中等规模集群）
    // "@every 2m"  - 2分钟（超大规模集群）
    return "@every 30s"
}
```

### 数据一致性

- **延迟**: 最多 30 秒（取决于 Job 周期）
- **准确性**: 与实时数据一致
- **失败处理**: 
  - Job 失败不影响 API
  - API 自动回退到实时计算

## 监控

### 查看缓存状态

```sql
-- 查看所有集群缓存
SELECT cluster_name, updated_at, total_nodes, allocation_rate, utilization
FROM cluster_overview_cache
ORDER BY updated_at DESC;

-- 查看特定集群
SELECT * FROM cluster_overview_cache WHERE cluster_name = 'prod-cluster';
```

### 日志监控

Job 会输出详细日志：

```
INFO Starting cluster overview cache job for cluster: prod-cluster
INFO Cluster overview cache job completed successfully for cluster: prod-cluster, took: 2.5s
```

失败时会记录错误：

```
ERROR Failed to get GPU nodes: connection refused
```

## 故障排除

### 问题：缓存未更新

1. 检查 Job 是否运行：
   ```bash
   # 查看进程
   ps aux | grep primus-lens-jobs
   
   # 查看日志
   tail -f /var/log/primus-lens-jobs.log
   ```

2. 检查数据库连接：
   ```bash
   psql -h <host> -U <user> -d <database> -c "SELECT * FROM cluster_overview_cache;"
   ```

### 问题：API 响应慢

1. 验证缓存是否命中：
   - 查看 API 日志
   - 检查数据库中是否有对应集群的记录

2. 如果缓存未命中，API 会回退到实时计算（较慢）

### 问题：数据不一致

缓存数据最多有 30 秒延迟（取决于 Job 周期），这是预期行为。

如需实时数据，可以：
1. 临时删除缓存记录，强制 API 使用实时计算
2. 减小 Job 更新周期

## 向后兼容性

完全向后兼容：

- ✅ API 接口不变
- ✅ 缓存未命中时自动回退
- ✅ 可以逐步迁移（先启动 Job，不影响现有 API）
- ✅ 无需修改客户端代码

## 最佳实践

1. **分阶段部署**:
   - 第一步：部署数据库迁移
   - 第二步：启动 Job 服务
   - 第三步：验证缓存正常工作
   - 第四步：部署新 API（自动使用缓存）

2. **监控告警**:
   - 监控 Job 执行时间（正常应在几秒内完成）
   - 监控缓存更新时间（不应超过 Job 周期的 2 倍）
   - 设置告警阈值

3. **性能调优**:
   - 小集群（<100节点）：30秒周期
   - 中等集群（100-500节点）：1分钟周期
   - 大集群（>500节点）：2分钟周期

## 未来改进

可能的优化方向：

1. **Redis 缓存**: 进一步降低延迟至毫秒级
2. **增量更新**: 仅更新变化的部分
3. **并行采集**: 多个 goroutine 并行采集不同指标
4. **自适应周期**: 根据集群规模自动调整更新频率

## 贡献

如有问题或建议，请提交 Issue 或 PR。

