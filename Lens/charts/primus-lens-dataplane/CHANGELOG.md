# Primus Lens Helm Chart - 变更日志

## [Unreleased] - 当前开发版本

### 🎯 重大变更

#### Storage Config Secret 动态创建 (2024-12-04)

**目标**: 创建统一的 Storage Config Secret，自动从 Operator 管理的 Secret 中读取凭据。

**实现方式**:
- ✅ 通过 Kubernetes Job 动态创建 `primus-lens-storage-config` Secret
- ✅ 从 PGO 生成的 Secret (`{release}-pguser-primus-lens`) 读取 PostgreSQL 凭据
  - 字段: user, password, dbname, host, port
- ✅ 从 OpenSearch Operator 生成的 Secret (`{clusterName}-admin-password`) 读取凭据
  - 字段: username, password
- ✅ 自动等待这些 Secret 就绪（最多重试 60 次，每次 5 秒）

**Secret 结构** (符合 `secret_template.go`):
- `opensearch`: OpenSearch 连接配置 (JSON)
- `prometheus`: VictoriaMetrics 连接配置 (JSON)
- `postgres`: PostgreSQL 连接配置 (JSON)

**部署顺序**:
1. Phase 3: 基础设施 CR 部署（Operators 创建密码 Secret）
2. Phase 4: 等待基础设施就绪
3. Phase 5: PostgreSQL 初始化 (weight 10)
4. **Phase 5+: Storage Config Creator Job (weight 16)** ← 新增
   - ServiceAccount + RBAC (weight 15)
   - Job 读取 Operator Secret 并创建统一配置 (weight 16)
5. Phase 6: 应用组件部署（使用 storage-config Secret）

**新增文件**:
- ✅ `templates/05-postgres-init/storage-config-secret.yaml`
  - ServiceAccount: `{release}-storage-config-creator`
  - Role + RoleBinding: 读取和创建 Secret 权限
  - Job: 动态创建 storage-config Secret

**values.yaml 简化**:
- ❌ 移除 `database.password` (由 PGO 自动生成)
- ❌ 移除 `opensearch.adminPassword` (由 OpenSearch Operator 管理)
- ✅ 密码完全由 Operator 管理，不需要手动配置

**安全优势**:
- ✅ 密码由 Operator 自动生成，更安全
- ✅ 不会在 Git 或 values.yaml 中暴露密码
- ✅ 使用专用 ServiceAccount，权限最小化
- ✅ Job 执行完成后自动清理 (ttlSecondsAfterFinished: 300)

**使用方式** (应用代码):
```go
// 读取统一的 storage-config Secret
cfg := &clientsets.PrimusLensClientConfig{}
cfg.LoadFromSecret(secret.Data)

// 或使用 ClusterManager
storageClients := clientsets.GetClusterManager().
    GetCurrentClusterClients().StorageClientSet
```

#### 部署顺序优化 (2024-12)

**问题**: 原设计中 FluentBit 和 VMAgent 与应用组件同时部署，但它们依赖 telemetry-processor 服务。

**解决方案**: 调整部署顺序，确保正确的依赖关系：

1. **Phase 3**: 基础设施 CR 部署（PostgreSQL, OpenSearch, VictoriaMetrics）
   - 作为**正常资源**部署，不使用 hooks
   - 让 Operators 有时间创建对应的 Pods

2. **Phase 4**: 等待基础设施就绪（新增）
   - **新增 Job**: `wait-for-infrastructure`
   - Hook weight: `5`
   - 等待 PostgreSQL, OpenSearch, VictoriaMetrics Pods Running
   - 最多重试 60 次（约 30 分钟）

3. **Phase 5**: 数据库初始化
   - Job: `postgres-init`
   - Hook weight: `10`
   - 依赖 Phase 4 完成

4. **Phase 6**: 应用组件部署
   - 作为**正常资源**部署
   - 包括 telemetry-processor, API, Web 等

5. **Phase 7**: 监控组件部署（调整）
   - FluentBit 和 VMAgent 作为 **post-install hooks**
   - Hook weight: `100`
   - 确保在 telemetry-processor 启动后部署

**影响的文件**:
- ✅ 新增: `templates/02-init-jobs/wait-for-infrastructure-job.yaml`
- ✅ 新增: `templates/04-monitoring/fluentbit-config.yaml`
- ✅ 修改: `templates/04-monitoring/vmagent.yaml` (添加 hook annotations)
- ✅ 修改: `templates/_helpers.tpl` (更新 hook weight 定义)
- ✅ 新增: `DEPLOYMENT_ORDER.md` (详细部署流程文档)

**优势**:
- ✅ 避免竞态条件
- ✅ 确保依赖服务就绪后再部署监控
- ✅ 更可靠的部署流程
- ✅ 更清晰的错误提示

### 新增功能

- ✅ 完整的 Helm Chart 实现
- ✅ 支持 3 种 Profile (minimal/normal/large)
- ✅ 支持 2 种访问方式 (ssh-tunnel/ingress)
- ✅ 自动等待 Operators 就绪
- ✅ 自动等待基础设施就绪
- ✅ 自动初始化数据库
- ✅ 智能部署编排（Helm Hooks）
- ✅ 多环境配置支持 (dev/prod)

### 文档

- ✅ README.md - 完整用户文档
- ✅ QUICKSTART.md - 快速开始指南
- ✅ DEPLOYMENT_SUMMARY.md - 部署总结
- ✅ DEPLOYMENT_ORDER.md - 详细部署流程
- ✅ STRUCTURE.md - 目录结构说明
- ✅ Makefile - 30+ 便捷命令

## [1.0.0] - 初始版本

### 初始实现

基于 [HELM_REFACTOR_DESIGN.md](../bootstrap/HELM_REFACTOR_DESIGN.md) 的架构设计实现。

**核心组件**:
- Chart.yaml: 定义 6 个子 Chart 依赖
- values.yaml: 默认配置和 3 个 Profile
- templates/: 完整的 Kubernetes 资源模板

**子 Chart 依赖**:
- victoria-metrics-operator (v0.35.2)
- fluent-operator (v3.1.0)
- opensearch-operator (v2.6.0)
- pgo (v5.7.0)
- grafana-operator (v5.15.0)
- kube-state-metrics (v5.27.0)

**应用组件**:
- API Service
- Web Console
- Telemetry Collector
- Node Exporter
- GPU Resource Exporter
- System Tuner

**基础设施**:
- PostgreSQL (Crunchy Operator)
- OpenSearch
- VictoriaMetrics
- Grafana

---

## 开发计划

### 短期 (1-2 周)

- [ ] 补充更多应用组件模板 (telemetry-collector, jobs, gpu-exporter)
- [ ] 完善 Grafana Dashboard 配置（从 JSON 转换为 YAML）
- [ ] 添加 OpenSearch 初始化 Job（索引模板）
- [ ] 在测试集群验证完整部署流程

### 中期 (1-2 月)

- [ ] 添加 Helm test 用例
- [ ] 添加 CI/CD 集成（GitHub Actions）
- [ ] 支持更多 Profile（custom）
- [ ] 支持更多 Ingress Controller（Higress, Traefik）
- [ ] 集成 External Secrets Operator
- [ ] 添加升级路径测试

### 长期 (3+ 月)

- [ ] 支持多集群部署
- [ ] 添加高级监控和告警规则
- [ ] 性能优化和资源调优
- [ ] 安全加固（OPA/Gatekeeper 策略）
- [ ] 完整的 Disaster Recovery 方案

---

## 贡献指南

欢迎提交 Issue 和 Pull Request！

**报告 Bug**:
1. 提供详细的错误信息
2. 包含 `helm status` 和 `kubectl get pods` 输出
3. 附上相关 Job 日志

**功能请求**:
1. 描述使用场景
2. 说明期望行为
3. 提供配置示例

**提交 PR**:
1. Fork 项目
2. 创建功能分支
3. 添加测试（如适用）
4. 更新文档
5. 提交 PR

---

## 支持

- GitHub Issues: https://github.com/AMD-AGI/Primus-SaFE/issues
- 文档: [README.md](README.md), [QUICKSTART.md](QUICKSTART.md)
- 架构设计: [HELM_REFACTOR_DESIGN.md](../bootstrap/HELM_REFACTOR_DESIGN.md)

