# Primus Lens - Quick Start Guide

快速开始指南，5 分钟内完成 Primus Lens 部署。

## 前置条件

1. **Kubernetes 集群**: 版本 1.24+
2. **Helm**: 版本 3.8+
3. **kubectl**: 已配置并能访问集群
4. **StorageClass**: 集群中至少有一个可用的 StorageClass

验证前置条件：

```bash
# 检查 Kubernetes 版本
kubectl version --short

# 检查 Helm 版本
helm version --short

# 检查 StorageClass
kubectl get storageclass

# 检查集群节点
kubectl get nodes
```

## 步骤 1: 下载依赖

```bash
cd Lens/charts
helm dependency update
```

这将下载所有必需的 Operator Charts：
- VictoriaMetrics Operator
- Fluent Operator
- OpenSearch Operator
- PostgreSQL Operator (PGO)
- Grafana Operator
- Kube State Metrics

## 步骤 2: 准备配置（可选）

### 最小化配置

创建 `my-values.yaml`:

```yaml
global:
  clusterName: "my-cluster"
  storageClass: "local-path"  # 使用你的 StorageClass
  accessType: "ssh-tunnel"

profile: "minimal"  # 适合测试环境
```

### 完整配置

```yaml
global:
  clusterName: "prod-cluster"
  storageClass: "fast-ssd"
  accessMode: "ReadWriteMany"  # 如果支持 RWX
  imageRegistry: "docker.io"
  accessType: "ingress"
  domain: "example.com"
  
  imagePullSecrets:
    - name: primus-lens-image
      credentials:
        registry: "docker.io"
        username: "myuser"
        password: "mypass"

profile: "normal"  # 或 "large"

grafana:
  adminPassword: "change-me-in-production"
```

## 步骤 3: 安装

### 方式 A: 使用默认配置

```bash
helm install primus-lens . \
  --namespace primus-lens \
  --create-namespace \
  --timeout 30m \
  --wait
```

### 方式 B: 使用自定义配置

```bash
helm install primus-lens . \
  -f my-values.yaml \
  --namespace primus-lens \
  --create-namespace \
  --timeout 30m \
  --wait
```

### 方式 C: 通过命令行参数

```bash
helm install primus-lens . \
  --set global.clusterName=my-cluster \
  --set global.storageClass=local-path \
  --set profile=minimal \
  --namespace primus-lens \
  --create-namespace \
  --timeout 30m \
  --wait
```

## 步骤 4: 验证部署

```bash
# 检查 Release 状态
helm status primus-lens -n primus-lens

# 检查所有 Pods
kubectl get pods -n primus-lens

# 等待所有 Pods 运行
kubectl wait --for=condition=ready pod \
  --all \
  -n primus-lens \
  --timeout=600s
```

## 步骤 5: 访问服务

### SSH Tunnel 方式（默认）

#### Web Console

```bash
kubectl port-forward -n primus-lens svc/primus-lens-web 30180:80
```

然后打开浏览器访问: http://localhost:30180

#### Grafana

```bash
kubectl port-forward -n primus-lens svc/grafana-service 30182:3000
```

然后打开浏览器访问: http://localhost:30182/grafana
- 默认用户名: `admin`
- 默认密码: `admin`

### Ingress 方式

如果使用 `accessType: ingress`，直接访问：
- Web Console: https://my-cluster.example.com
- Grafana: https://my-cluster.example.com/grafana

## 常见问题

### Q1: 如何查看安装进度？

```bash
# 实时查看所有 pods 状态
watch kubectl get pods -n primus-lens

# 查看特定 Operator 状态
kubectl get pods -n primus-lens | grep operator

# 查看初始化 Jobs
kubectl get jobs -n primus-lens
```

### Q2: 初始化 Job 失败怎么办？

```bash
# 查看 Job 日志
kubectl logs -n primus-lens job/primus-lens-wait-operators
kubectl logs -n primus-lens job/primus-lens-postgres-init

# Job 会自动重试，最多 30 次
# 如果一直失败，检查：
# 1. Operator Pods 是否正常运行
# 2. 存储是否可用
# 3. 镜像拉取是否成功
```

### Q3: 如何更新配置？

```bash
# 修改 values 文件后执行
helm upgrade primus-lens . \
  -f my-values.yaml \
  -n primus-lens

# 或直接修改参数
helm upgrade primus-lens . \
  --set apps.api.replicas=5 \
  -n primus-lens
```

### Q4: 如何卸载？

```bash
# 卸载 Release
helm uninstall primus-lens -n primus-lens

# 删除命名空间（会删除所有数据！）
kubectl delete namespace primus-lens

# 如果需要保留数据，只删除 Release
helm uninstall primus-lens -n primus-lens
# 手动删除不需要的 PVC
kubectl delete pvc <pvc-name> -n primus-lens
```

## 生产环境建议

### 1. 使用高性能存储

```yaml
global:
  storageClass: "fast-ssd"  # 或 "ceph-rbd"
  accessMode: "ReadWriteMany"  # 如果支持
```

### 2. 选择合适的 Profile

```yaml
profile: "large"  # 大规模生产环境
```

### 3. 配置 Ingress 和 TLS

```yaml
global:
  accessType: "ingress"
  domain: "prod.example.com"

ingress:
  enabled: true
  className: "nginx"
  tls:
    enabled: true
    secretName: "primus-lens-tls"
  annotations:
    cert-manager.io/cluster-issuer: "letsencrypt-prod"
```

### 4. 使用外部密钥管理

```bash
# 不要在 values 文件中存储密码
# 使用命令行传递或外部密钥管理
helm install primus-lens . \
  -f values-prod.yaml \
  --set grafana.adminPassword=$GRAFANA_PASS \
  --set global.imagePullSecrets[0].credentials.password=$DOCKER_PASS \
  -n primus-lens
```

### 5. 启用监控和告警

```yaml
monitoring:
  kubeStateMetrics:
    enabled: true

grafana:
  dashboards:
    enabled: true
```

## 下一步

- 📖 阅读完整文档: [README.md](README.md)
- 🏗️ 查看架构设计: [HELM_REFACTOR_DESIGN.md](../bootstrap/HELM_REFACTOR_DESIGN.md)
- 🐛 报告问题: https://github.com/AMD-AGI/Primus-SaFE/issues

## 获取帮助

```bash
# 查看所有配置参数
helm show values . > all-values.yaml

# 渲染模板（不安装）
helm template primus-lens . -f my-values.yaml

# 调试模式
helm install primus-lens . \
  -f my-values.yaml \
  --debug \
  --dry-run \
  -n primus-lens
```

祝你使用愉快！🚀

