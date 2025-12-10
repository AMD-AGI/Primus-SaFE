# Templates 目录重构总结

## 🎯 重构目标

将 `templates/` 目录结构调整为与实际部署 Phase 一一对应，使目录名称直接反映部署顺序和阶段。

## 📊 变更对比

### 旧结构 (基于资源类型)

```
templates/
├── 00-namespace/          # ✅ 正确
├── 01-secrets/            # ✅ 正确
├── 02-init-jobs/          # ❌ 混合了多个 Phase
├── 03-apps/               # ❌ Phase 顺序不对
├── 04-monitoring/         # ❌ 混合了 Phase 3 和 Phase 7
├── 05-database/           # ❌ 应该在 Phase 3
├── 06-storage/            # ❌ 应该在 Phase 3
├── 07-grafana/            # ❌ 应该在 Phase 8
└── 08-ingress/            # ❌ 应该在 Phase 8
```

### 新结构 (基于部署 Phase)

```
templates/
├── 00-namespace/          # Phase 0 ✅
├── 01-secrets/            # Phase 0 ✅
├── 02-wait-operators/     # Phase 2 ✅
├── 03-infrastructure/     # Phase 3 ✅
├── 04-wait-infrastructure/# Phase 4 ✅
├── 05-postgres-init/      # Phase 5 ✅
├── 06-apps/               # Phase 6 ✅
├── 07-monitoring/         # Phase 7 ✅
└── 08-grafana/            # Phase 8 ✅
```

## 📁 文件移动清单

| 原路径 | 新路径 | 说明 |
|--------|--------|------|
| `02-init-jobs/wait-for-operators-job.yaml` | `02-wait-operators/wait-for-operators-job.yaml` | Phase 2 |
| `05-database/pg-cr.yaml` | `03-infrastructure/pg-cr.yaml` | Phase 3 |
| `06-storage/opensearch-cr.yaml` | `03-infrastructure/opensearch-cr.yaml` | Phase 3 |
| `04-monitoring/vmcluster.yaml` | `03-infrastructure/vmcluster.yaml` | Phase 3 |
| `02-init-jobs/wait-for-infrastructure-job.yaml` | `04-wait-infrastructure/wait-for-infrastructure-job.yaml` | Phase 4 |
| `02-init-jobs/postgres-init-configmap.yaml` | `05-postgres-init/postgres-init-configmap.yaml` | Phase 5 |
| `02-init-jobs/postgres-init-job.yaml` | `05-postgres-init/postgres-init-job.yaml` | Phase 5 |
| `03-apps/app-api.yaml` | `06-apps/app-api.yaml` | Phase 6 |
| `03-apps/app-web.yaml` | `06-apps/app-web.yaml` | Phase 6 |
| `03-apps/app-node-exporter.yaml` | `06-apps/app-node-exporter.yaml` | Phase 6 |
| `04-monitoring/fluentbit-config.yaml` | `07-monitoring/fluentbit-config.yaml` | Phase 7 |
| `04-monitoring/vmagent.yaml` | `07-monitoring/vmagent.yaml` | Phase 7 |
| `07-grafana/grafana-cr.yaml` | `08-grafana/grafana-cr.yaml` | Phase 8 |
| `07-grafana/datasource.yaml` | `08-grafana/datasource.yaml` | Phase 8 |
| `07-grafana/folders.yaml` | `08-grafana/folders.yaml` | Phase 8 |
| `08-ingress/nginx-ingress.yaml` | `08-grafana/nginx-ingress.yaml` | Phase 8 |

## 🎯 新目录结构说明

### 00-namespace/ (Phase 0)
- **Hook**: pre-install, weight: -100
- **内容**: namespace.yaml
- **说明**: 创建命名空间

### 01-secrets/ (Phase 0)
- **Hook**: pre-install, weight: -90
- **内容**: image-pull-secret, tls-cert-secret, service-account
- **说明**: 创建密钥和 RBAC

### 02-wait-operators/ (Phase 2)
- **Hook**: pre-install, weight: 0
- **内容**: wait-for-operators-job.yaml
- **说明**: 等待所有 Operators Ready

### 03-infrastructure/ (Phase 3)
- **Hook**: 无（正常资源）
- **内容**: pg-cr, opensearch-cr, vmcluster
- **说明**: 部署基础设施 Custom Resources

### 04-wait-infrastructure/ (Phase 4)
- **Hook**: post-install, weight: 5
- **内容**: wait-for-infrastructure-job.yaml
- **说明**: 等待基础设施 Pods Running

### 05-postgres-init/ (Phase 5)
- **Hook**: post-install, weight: 10
- **内容**: postgres-init-configmap, postgres-init-job
- **说明**: 初始化数据库模式

### 06-apps/ (Phase 6)
- **Hook**: 无（正常资源）
- **内容**: app-api, app-web, app-node-exporter
- **说明**: 部署应用组件

### 07-monitoring/ (Phase 7)
- **Hook**: post-install, weight: 100
- **内容**: fluentbit-config, vmagent
- **说明**: 部署监控组件（依赖 telemetry-processor）

### 08-grafana/ (Phase 8)
- **Hook**: 无（正常资源）
- **内容**: grafana-cr, datasource, folders, nginx-ingress
- **说明**: 部署 Grafana 和 Ingress

## ✅ 优势

### 1. 清晰的部署顺序
目录名称直接反映部署 Phase，一目了然。

```
00 → 01 → 02 → 03 → 04 → 05 → 06 → 07 → 08
```

### 2. 易于理解和维护
- 新成员可以快速理解部署流程
- 不需要查看 annotations 就知道资源的部署阶段

### 3. 便于扩展
添加新资源时，根据依赖关系选择合适的 Phase 目录即可。

### 4. 与文档一致
目录结构与 `DEPLOYMENT_ORDER.md` 文档完全对应。

## 📝 更新的文档

1. ✅ `STRUCTURE.md` - 更新目录结构说明
2. ✅ `templates/README.md` - 新增 templates 目录说明文档
3. ✅ `DIRECTORY_RESTRUCTURE_SUMMARY.md` - 本文档

## 🔄 迁移影响

### Helm 模板渲染
✅ **无影响** - Helm 会遍历所有 templates 子目录，目录名称不影响功能。

### Hook 执行顺序
✅ **无影响** - Hook 顺序由 `helm.sh/hook-weight` annotation 决定，与目录名称无关。

### CI/CD 流程
✅ **无影响** - 部署命令保持不变：`helm install primus-lens .`

### 开发体验
✅ **改善** - 开发者可以更快找到需要修改的文件。

## 🎓 最佳实践

### 添加新资源时的步骤

1. **确定 Phase**: 根据资源的依赖关系确定应该在哪个 Phase 部署
2. **选择目录**: 将资源放入对应的 Phase 目录
3. **添加 Hook**: 如果需要，添加适当的 Hook annotations
4. **设置 Weight**: 在同一 Phase 内，使用 weight 控制顺序
5. **更新文档**: 在 `templates/README.md` 中记录新资源

### 命名规范

- **目录**: `XX-{phase-name}/` (XX 是两位数字)
- **文件**: `{resource-type}-{name}.yaml`
- **Job**: `{action}-job.yaml`
- **CR**: `{resource-name}-cr.yaml`
- **ConfigMap**: `{name}-config.yaml`

## 🚀 后续工作

- [ ] 添加更多应用组件到 `06-apps/`
- [ ] 完善 Grafana Dashboard 配置
- [ ] 添加更多监控组件到 `07-monitoring/`
- [ ] 支持更多 Ingress Controller

## 📚 相关文档

- [DEPLOYMENT_ORDER.md](DEPLOYMENT_ORDER.md) - 详细部署流程
- [STRUCTURE.md](STRUCTURE.md) - 完整目录结构
- [templates/README.md](templates/README.md) - Templates 目录说明
- [README.md](README.md) - 用户文档

---

通过这次重构，目录结构与部署流程完美对应，大大提升了可维护性！🎉

