# 端到端集成测试

## 概述

本目录包含框架检测系统的端到端（E2E）集成测试。这些测试验证完整的数据流，从 Workload 创建到最终的框架检测结果。

## 测试场景

### 场景 1: 完整的复用流程（`TestE2E_ReuseFlow`）

验证元数据复用机制：
1. 创建第一个 Workload（Workload-A）并检测框架
2. 创建相似的 Workload（Workload-B）
3. 验证 Workload-B 成功复用 Workload-A 的检测结果
4. 验证复用后的置信度提升

**预期结果**：
- Workload-A 被正确检测（组件判断）
- Workload-B 成功复用 Workload-A 的结果
- 复用信息被正确记录

### 场景 3: 冲突解决（`TestE2E_ConflictResolution`）

验证多源检测冲突的解决：
1. 组件判断识别为 Primus（置信度 0.8）
2. 日志识别为 DeepSpeed（置信度 0.7）
3. 验证冲突解决机制（优先级规则）
4. 用户手动标注覆盖自动检测

**预期结果**：
- 组件判断（更高优先级）被选择
- 冲突被正确记录
- 用户标注成功覆盖自动检测

### 场景 4: WandB 数据源（`TestE2E_WandBDetection`）

验证 WandB Exporter 的 API 上报：
1. 创建 Workload
2. WandB Exporter 上报检测数据（包含环境变量、配置等证据）
3. 验证框架检测结果
4. 验证证据信息完整性

**预期结果**：
- WandB API 调用成功
- 框架被正确检测（基于环境变量）
- WandB 被记录为检测源
- 证据信息完整

### 场景 5: WandB 日志和指标上报（`TestE2E_WandBLogsAndMetrics`）

验证 WandB 日志和指标处理：
1. 上报框架检测数据
2. 上报训练指标（loss, accuracy）
3. 上报训练日志（包含框架特征）
4. 验证多源检测融合

**预期结果**：
- 所有 API 调用成功
- 指标被正确存储
- 日志触发框架检测
- 多个检测源被融合

### 场景 6: WandB 批量上报（`TestE2E_WandBBatchReport`）

验证批量 API 的处理：
1. 一次性上报检测、指标、日志数据
2. 验证批量处理结果
3. 验证所有数据被正确处理

**预期结果**：
- 批量 API 返回成功
- 所有数据类型都被处理
- 框架被正确检测

## 运行测试

### 前提条件

1. **数据库**：测试需要访问 PostgreSQL 数据库
   ```bash
   # 设置测试数据库环境变量
   export DB_HOST=localhost
   export DB_PORT=5432
   export DB_NAME=primus_lens_test
   export DB_USER=postgres
   export DB_PASSWORD=password
   ```

2. **初始化数据库**：
   ```bash
   # 创建测试数据库
   createdb primus_lens_test
   
   # 执行数据库迁移
   psql -d primus_lens_test -f ../../bootstrap/manifests/setup_primus_lens.sql
   ```

3. **依赖服务**：
   - telemetry-processor 服务（测试会启动内嵌的 HTTP 服务器）
   - 不需要外部服务

### 运行全部测试

```bash
cd Lens/test/e2e
go test -v ./...
```

### 运行单个测试

```bash
# 运行复用流程测试
go test -v -run TestE2E_ReuseFlow

# 运行冲突解决测试
go test -v -run TestE2E_ConflictResolution

# 运行 WandB 检测测试
go test -v -run TestE2E_WandBDetection
```

### 跳过 E2E 测试（短测试模式）

```bash
# E2E 测试耗时较长，可以跳过
go test -v -short ./...
```

### 并行测试

```bash
# 注意：E2E 测试可能共享数据库，建议串行执行
go test -v -p 1 ./...
```

## 测试配置

### 环境变量

| 变量名 | 默认值 | 说明 |
|--------|--------|------|
| `DB_HOST` | `localhost` | 数据库主机 |
| `DB_PORT` | `5432` | 数据库端口 |
| `DB_NAME` | `primus_lens_test` | 测试数据库名 |
| `DB_USER` | `postgres` | 数据库用户 |
| `DB_PASSWORD` | `password` | 数据库密码 |
| `E2E_TIMEOUT` | `5m` | 测试超时时间 |

### 测试数据清理

每个测试套件结束后会自动清理测试数据。如果需要手动清理：

```bash
# 清理测试数据
psql -d primus_lens_test -c "TRUNCATE TABLE ai_workload_metadata CASCADE;"
```

## 测试结构

```
test/e2e/
├── README.md                           # 本文档
├── framework_detection_e2e_test.go     # E2E 测试主文件
└── testdata/                           # 测试数据（如果需要）
```

## 测试辅助函数

### SetupE2ETest

初始化测试环境：
- 创建数据库连接
- 初始化 DetectionManager
- 初始化 ReuseEngine
- 启动测试 API 服务器

### TearDown

清理测试环境：
- 清理测试数据
- 关闭数据库连接

### 辅助方法

- `createTestWorkload(uid, image)` - 创建测试 Workload
- `getDetection(uid)` - 查询检测结果
- `reportDetection(uid, source, framework, confidence)` - 上报检测
- `callWandBDetectionAPI(req)` - 调用 WandB Detection API
- `callWandBMetricsAPI(req)` - 调用 WandB Metrics API
- `callWandBLogsAPI(req)` - 调用 WandB Logs API
- `callWandBBatchAPI(req)` - 调用 WandB Batch API

## 故障排查

### 测试失败：数据库连接错误

**问题**：`Failed to connect to database`

**解决**：
1. 检查数据库是否运行：`pg_isready`
2. 检查环境变量是否正确设置
3. 检查数据库用户权限

### 测试失败：API 调用超时

**问题**：`context deadline exceeded`

**解决**：
1. 增加 `E2E_TIMEOUT` 环境变量
2. 检查服务器是否正常启动
3. 检查网络连接

### 测试失败：检测结果不符合预期

**问题**：检测到的框架不正确

**解决**：
1. 检查检测规则配置
2. 查看详细日志（使用 `-v` 参数）
3. 验证测试数据是否正确

## 性能基准

| 测试场景 | 预期时间 | 说明 |
|---------|---------|------|
| ReuseFlow | < 5s | 包含等待时间 |
| ConflictResolution | < 3s | 快速检测 |
| WandBDetection | < 2s | 单次 API 调用 |
| WandBLogsAndMetrics | < 3s | 多次 API 调用 |
| WandBBatchReport | < 2s | 批量处理 |

**总测试时间**：< 20s

## 持续集成

### GitHub Actions

```yaml
name: E2E Tests

on: [push, pull_request]

jobs:
  e2e:
    runs-on: ubuntu-latest
    services:
      postgres:
        image: postgres:14
        env:
          POSTGRES_DB: primus_lens_test
          POSTGRES_PASSWORD: password
        options: >-
          --health-cmd pg_isready
          --health-interval 10s
          --health-timeout 5s
          --health-retries 5
    
    steps:
      - uses: actions/checkout@v3
      
      - name: Set up Go
        uses: actions/setup-go@v4
        with:
          go-version: '1.21'
      
      - name: Run E2E Tests
        env:
          DB_HOST: postgres
          DB_PORT: 5432
          DB_NAME: primus_lens_test
          DB_USER: postgres
          DB_PASSWORD: password
        run: |
          cd Lens/test/e2e
          go test -v ./...
```

## 扩展测试

如需添加新的 E2E 测试场景：

1. 在 `framework_detection_e2e_test.go` 中添加新的测试函数
2. 遵循命名约定：`TestE2E_<ScenarioName>`
3. 使用 `suite.Setup()` 和 `suite.TearDown()`
4. 添加详细的日志输出
5. 更新本 README 文档

## 相关文档

- [Phase 5 任务计划](../../docs/local/phase5-remaining-tasks.md)
- [多源框架检测设计](../../docs/local/multi-source-framework-detection-design.md)
- [Task 4 实现总结](../../docs/local/task4-e2e-testing-summary.md)

