# GPU 聚合数据查询 API - 实现总结

## 概述

为 `gpu_aggregation_job.go` 生成的聚合数据创建了完整的查询 API，使前端或其他服务能够方便地查询集群、namespace、label 等多维度的 GPU 使用统计数据。

## 已实现的功能

### 1. API 端点

创建了 5 个主要的 API 端点来查询不同类型的聚合数据：

#### 1.1 集群级别小时统计
- **端点**: `GET /api/v1/gpu-aggregation/cluster/hourly-stats`
- **功能**: 查询集群维度的 GPU 分配率和使用率统计
- **数据来源**: `cluster_gpu_hourly_stats` 表
- **返回数据**:
  - GPU 总容量
  - 已分配 GPU 数量
  - 分配率 (0-100%)
  - 平均/最大/最小使用率
  - P50/P95 使用率
  - 采样次数

#### 1.2 Namespace 级别小时统计
- **端点**: `GET /api/v1/gpu-aggregation/namespaces/hourly-stats`
- **功能**: 查询各个 namespace 的 GPU 使用情况
- **数据来源**: `namespace_gpu_hourly_stats` 表
- **支持**:
  - 查询所有 namespace（`namespace` 参数为空）
  - 查询特定 namespace
- **返回数据**:
  - Namespace 名称
  - 已分配 GPU 数量
  - 平均/最大/最小使用率
  - 活跃 workload 数量

#### 1.3 Label/Annotation 级别小时统计
- **端点**: `GET /api/v1/gpu-aggregation/labels/hourly-stats`
- **功能**: 查询按 label 或 annotation 分组的 GPU 使用统计
- **数据来源**: `label_gpu_hourly_stats` 表
- **支持**:
  - Label 和 Annotation 两种维度类型
  - 查询指定 key 的所有 value
  - 查询指定 key-value 组合
- **返回数据**:
  - 维度类型/key/value
  - 已分配 GPU 数量
  - 平均/最大/最小使用率
  - 活跃 workload 数量

#### 1.4 最新快照查询
- **端点**: `GET /api/v1/gpu-aggregation/snapshots/latest`
- **功能**: 获取最新一次采样的详细快照
- **数据来源**: `gpu_allocation_snapshots` 表
- **返回数据**:
  - 快照时间
  - GPU 容量和分配情况
  - 详细的 namespace 和 workload 信息（JSON 格式）

#### 1.5 历史快照列表
- **端点**: `GET /api/v1/gpu-aggregation/snapshots`
- **功能**: 查询指定时间范围内的历史快照
- **数据来源**: `gpu_allocation_snapshots` 表
- **支持**:
  - 自定义时间范围
  - 默认查询最近 24 小时

### 2. 文件结构

```
Lens/modules/api/
├── pkg/api/
│   ├── gpu_aggregation.go     # 新增：GPU 聚合数据 API 实现
│   └── router.go               # 修改：添加路由注册
└── docs/
    ├── gpu_aggregation_api.md           # 新增：详细 API 文档
    └── gpu_aggregation_api_summary_zh.md # 新增：实现总结
```

## 技术实现细节

### 1. 请求参数验证

所有 API 都实现了完善的参数验证：
- 使用 Gin 的 `ShouldBindQuery` 进行参数绑定
- 时间格式验证（RFC3339 格式）
- 枚举值验证（如 `dimension_type` 只能是 `label` 或 `annotation`）

### 2. 时间处理

- 统一使用 RFC3339 格式（如：`2025-11-05T14:00:00Z`）
- 支持 UTC 时区
- 提供默认时间范围（如快照列表默认查询最近 24 小时）

### 3. 多集群支持

- 通过 `cluster` 查询参数支持多集群
- 使用 `GetClusterClientsOrDefault` 获取集群客户端
- 未指定集群时使用默认集群

### 4. 错误处理

- 统一的错误响应格式
- 使用 `errors.WrapError` 包装错误
- 区分不同的错误类型（参数错误、数据不存在、数据库错误等）

### 5. 数据库查询优化

- 使用 `GetFacadeForCluster` 获取特定集群的数据库 facade
- 利用已有的 facade 接口方法，无需编写原始 SQL
- 支持时间范围查询和排序

## 使用示例

### 1. 查询集群 GPU 利用率趋势（最近 7 天）

```bash
curl "http://localhost:8080/api/v1/gpu-aggregation/cluster/hourly-stats?start_time=2025-10-29T00:00:00Z&end_time=2025-11-05T23:59:59Z"
```

### 2. 查看所有 Namespace 的 GPU 使用情况（今天）

```bash
curl "http://localhost:8080/api/v1/gpu-aggregation/namespaces/hourly-stats?start_time=2025-11-05T00:00:00Z&end_time=2025-11-05T23:59:59Z"
```

### 3. 按团队查看 GPU 使用（Label 维度）

```bash
curl "http://localhost:8080/api/v1/gpu-aggregation/labels/hourly-stats?dimension_type=label&dimension_key=team&start_time=2025-11-05T00:00:00Z&end_time=2025-11-05T23:59:59Z"
```

### 4. 获取当前 GPU 分配快照

```bash
curl "http://localhost:8080/api/v1/gpu-aggregation/snapshots/latest"
```

### 5. 查看最近 24 小时的采样快照

```bash
curl "http://localhost:8080/api/v1/gpu-aggregation/snapshots"
```

## 前端集成建议

### 1. 可视化场景

基于这些 API，前端可以实现以下可视化功能：

#### 集群总览 Dashboard
- **时序图**: 显示集群 GPU 分配率和使用率趋势
- **数据源**: `/cluster/hourly-stats`
- **更新频率**: 每小时

#### Namespace GPU 使用排行
- **柱状图/饼图**: 显示各 namespace 的 GPU 分配情况
- **数据源**: `/namespaces/hourly-stats`
- **过滤**: 可按时间范围筛选

#### 团队 GPU 使用对比
- **分组柱状图**: 对比不同团队的 GPU 使用效率
- **数据源**: `/labels/hourly-stats?dimension_type=label&dimension_key=team`
- **指标**: 分配量、平均使用率

#### 实时 GPU 分配视图
- **树状图/热力图**: 显示当前各 namespace 和 workload 的 GPU 分配
- **数据源**: `/snapshots/latest`
- **更新频率**: 每 5 分钟（配合快照采样频率）

#### 历史趋势分析
- **多维度时序图**: 支持切换查看集群/namespace/label 维度
- **数据源**: 根据选择的维度调用对应 API
- **时间范围**: 支持日/周/月/自定义

### 2. 前端请求示例（JavaScript）

```javascript
// 使用 axios
import axios from 'axios';

// 1. 获取集群统计
async function getClusterStats(startTime, endTime) {
  const response = await axios.get('/api/v1/gpu-aggregation/cluster/hourly-stats', {
    params: {
      start_time: startTime.toISOString(),
      end_time: endTime.toISOString()
    }
  });
  return response.data.data;
}

// 2. 获取所有 namespace 统计
async function getNamespaceStats(startTime, endTime) {
  const response = await axios.get('/api/v1/gpu-aggregation/namespaces/hourly-stats', {
    params: {
      start_time: startTime.toISOString(),
      end_time: endTime.toISOString()
    }
  });
  return response.data.data;
}

// 3. 获取最新快照
async function getLatestSnapshot() {
  const response = await axios.get('/api/v1/gpu-aggregation/snapshots/latest');
  return response.data.data;
}

// 4. 获取团队维度统计
async function getTeamStats(startTime, endTime) {
  const response = await axios.get('/api/v1/gpu-aggregation/labels/hourly-stats', {
    params: {
      dimension_type: 'label',
      dimension_key: 'team',
      start_time: startTime.toISOString(),
      end_time: endTime.toISOString()
    }
  });
  return response.data.data;
}
```

### 3. 数据刷新策略

| 数据类型 | 刷新频率 | 说明 |
|---------|---------|------|
| 小时统计 | 每小时 | 在整点后 1-2 分钟获取上一小时的数据 |
| 最新快照 | 5 分钟 | 配合 Job 的采样频率 |
| 历史数据 | 按需加载 | 用户切换时间范围时加载 |

## API 性能考虑

### 1. 查询优化

- 时间范围查询使用索引（`stat_hour` 字段已索引）
- 避免全表扫描，始终指定时间范围
- 集群名称过滤使用索引

### 2. 建议的查询限制

- **单次查询时间范围**: 建议不超过 30 天
- **快照列表**: 默认只查询 24 小时，避免返回过多数据
- **分页**: 如果需要大量历史数据，考虑实现分页

### 3. 缓存策略

- 对于已完成的小时统计数据，可在前端缓存
- 最新快照可以短时间缓存（5 分钟）
- 考虑使用浏览器的 LocalStorage 或 SessionStorage

## 未来扩展

### 1. 聚合视图 API

可以考虑添加更多聚合视图：

```
GET /api/v1/gpu-aggregation/summary/daily      # 日维度汇总
GET /api/v1/gpu-aggregation/summary/weekly     # 周维度汇总
GET /api/v1/gpu-aggregation/summary/monthly    # 月维度汇总
GET /api/v1/gpu-aggregation/top/namespaces     # Top N namespace
GET /api/v1/gpu-aggregation/top/labels         # Top N label values
```

### 2. 导出功能

```
GET /api/v1/gpu-aggregation/export/csv         # 导出为 CSV
GET /api/v1/gpu-aggregation/export/excel       # 导出为 Excel
```

### 3. 对比分析

```
GET /api/v1/gpu-aggregation/compare/periods    # 时间段对比
GET /api/v1/gpu-aggregation/compare/clusters   # 集群间对比
```

### 4. 告警阈值查询

```
GET /api/v1/gpu-aggregation/alerts/utilization # 使用率告警
GET /api/v1/gpu-aggregation/alerts/allocation  # 分配率告警
```

## 测试建议

### 1. 单元测试

为每个 API 端点编写单元测试：
- 参数验证测试
- 正常响应测试
- 错误处理测试

### 2. 集成测试

测试完整的查询流程：
1. Job 采样并写入数据
2. 小时聚合生成统计
3. API 查询并验证数据正确性

### 3. 性能测试

- 大时间范围查询的响应时间
- 并发请求的处理能力
- 数据库查询性能

## 总结

本次实现完成了完整的 GPU 聚合数据查询 API，具有以下特点：

✅ **功能完整**: 覆盖集群、namespace、label 三个维度的统计查询  
✅ **设计合理**: RESTful 风格，参数验证完善  
✅ **易于使用**: 提供详细的 API 文档和使用示例  
✅ **扩展性好**: 易于添加新的查询维度和聚合视图  
✅ **性能优化**: 使用索引，支持时间范围查询  
✅ **多集群支持**: 可查询不同集群的数据  

这些 API 可以直接用于构建 GPU 资源管理和监控的前端界面。

