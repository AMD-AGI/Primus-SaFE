# Kubernetes 集群网络故障排查记录

**日期**: 2025-12-10  
**环境**: Kubernetes v1.32.5 + Cilium 1.17.3 + IPVS 模式  
**影响范围**: 全集群 Pod 网络  

---

## 问题现象

在尝试安装 Primus Lens Operators 时，PGO (PostgreSQL Operator) Pod 启动失败，日志显示：

```
panic: Get "https://10.254.0.1:443/api?timeout=32s": dial tcp 10.254.0.1:443: i/o timeout
```

Pod 无法连接到 Kubernetes API Server 的 ClusterIP (10.254.0.1)。

---

## 排查过程

### 1. 初步检查

首先怀疑是 kube-proxy 问题，检查了以下内容：

```bash
# 检查 kubernetes service
kubectl get svc kubernetes -n default
# 结果: ClusterIP 10.254.0.1 存在

# 检查 kube-proxy pods
kubectl get pods -n kube-system -l k8s-app=kube-proxy
# 结果: 所有节点的 kube-proxy 都在运行

# 检查 Cilium pods
kubectl get pods -n kube-system -l k8s-app=cilium
# 结果: 所有节点的 Cilium 都在运行
```

### 2. DNS 问题发现

创建测试 Job 发现 DNS 解析超时：

```bash
# 测试 DNS 解析
nslookup kubernetes.default.svc
# 结果: connection timed out; no servers could be reached
```

检查 kubelet DNS 配置：

```bash
kubectl get configmap -n kube-system kubelet-config -o yaml | grep -A3 "clusterDNS"
# 结果: clusterDNS: 169.254.25.10 (NodeLocal DNSCache)
```

### 3. NodeLocal DNSCache 检查

```bash
kubectl logs nodelocaldns-xxx -n kube-system
# 结果: 大量 i/o timeout 错误，无法连接上游 DNS
```

### 4. CoreDNS 问题发现

```bash
kubectl logs -n kube-system -l k8s-app=kube-dns
# 关键错误:
# Failed to watch *v1.Service: dial tcp 10.254.0.1:443: connection refused
# Failed to watch *v1.Namespace: Unauthorized
```

CoreDNS 也无法连接 API Server！

### 5. 网络连通性测试

#### hostNetwork 模式测试

```bash
# 从 hostNetwork pod 测试
kubectl exec debug-net -- curl -k https://10.254.0.1:443/healthz
# 结果: ok ✅
```

#### Pod 网络模式测试

```bash
# 从普通 pod 测试
kubectl exec debug-pod-net -- curl -k https://10.254.0.1:443/healthz
# 结果: Connection timed out ❌

# 甚至无法 ping 网关
kubectl exec debug-pod-net -- ping 10.0.1.202
# 结果: 100% packet loss ❌
```

**结论**: hostNetwork 正常，Pod 网络完全不通。

### 6. IPVS 规则检查

```bash
ipvsadm -Ln | grep -A10 "10.254.0.1"
# 结果: IPVS 规则正确
# TCP  10.254.0.1:443 rr
#   -> 10.235.192.47:6443   Masq
#   -> 10.235.192.66:6443   Masq
#   -> 10.235.192.68:6443   Masq
```

### 7. Cilium Endpoint 检查

```bash
cilium endpoint list | grep debug-pod
# 结果:
# 3098  Enabled  Enabled  5  reserved:init  10.0.1.119  ready
```

**关键发现**: Pod 的 Identity 是 `5 (reserved:init)`，表示 Cilium 没有正确识别 Pod 身份！

### 8. Cilium 日志分析

```bash
kubectl logs cilium-xxx -n kube-system | grep -i error
# 关键错误:
# services is forbidden: User "system:serviceaccount:kube-system:cilium" cannot list resource "services"
# ciliumendpoints.cilium.io is forbidden: User "system:serviceaccount:kube-system:cilium" cannot list resource "ciliumendpoints"
# networkpolicies.networking.k8s.io is forbidden: User "system:serviceaccount:kube-system:cilium" cannot list resource "networkpolicies"
```

### 9. RBAC 检查

```bash
kubectl get clusterrole cilium
# 结果: Error from server (NotFound): clusterroles.rbac.authorization.k8s.io "cilium" not found
```

**根因确认**: Cilium 的 ClusterRole 被删除了！

---

## 根本原因

在之前清理 Helm 资源时，执行了以下命令：

```bash
kubectl delete clusterrole,clusterrolebinding -l app.kubernetes.io/managed-by=Helm
```

这个命令意外删除了带有 Helm managed-by 标签的 **Cilium ClusterRole 和 ClusterRoleBinding**。

### 影响链路

```
Cilium ClusterRole 被删除
    ↓
Cilium 无法访问 Kubernetes API (权限不足)
    ↓
Cilium 无法获取 Pod 标签
    ↓
Pod 被标记为 reserved:init 身份 (Identity=5)
    ↓
默认网络策略阻止了 init 状态 Pod 的所有出站流量
    ↓
Pod 无法访问任何 ClusterIP 服务
    ↓
DNS 解析失败 + API Server 不可达
```

---

## 解决方案

### 1. 重新创建 Cilium ClusterRole

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: cilium
  labels:
    app.kubernetes.io/name: cilium
    app.kubernetes.io/part-of: cilium
rules:
- apiGroups: [""]
  resources: [pods, nodes, namespaces, endpoints, services]
  verbs: [get, list, watch]
- apiGroups: [""]
  resources: [nodes/status]
  verbs: [patch]
- apiGroups: [networking.k8s.io]
  resources: [networkpolicies]
  verbs: [get, list, watch]
- apiGroups: [discovery.k8s.io]
  resources: [endpointslices]
  verbs: [get, list, watch]
- apiGroups: [cilium.io]
  resources: ["*"]
  verbs: ["*"]
- apiGroups: [apiextensions.k8s.io]
  resources: [customresourcedefinitions]
  verbs: [get, list, watch]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: cilium
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: cilium
subjects:
- kind: ServiceAccount
  name: cilium
  namespace: kube-system
```

### 2. 重启 Cilium

```bash
kubectl rollout restart daemonset cilium -n kube-system
kubectl rollout status daemonset cilium -n kube-system
```

### 3. 重启受影响的 Pod

```bash
# 重启 CoreDNS
kubectl delete pods -n kube-system -l k8s-app=kube-dns

# 重启其他受影响的 Pod
kubectl delete pods -n primus-lens --all
```

---

## 验证修复

```bash
# 测试 ClusterIP 访问
kubectl run test --image=curlimages/curl --rm -i --restart=Never \
  -- curl -k -s https://10.254.0.1:443/healthz
# 期望结果: ok

# 测试 DNS 解析
kubectl run test --image=busybox --rm -i --restart=Never \
  -- nslookup kubernetes.default.svc.cluster.local
# 期望结果: Address: 10.254.0.1
```

---

## 经验教训

### 1. 谨慎使用标签选择器删除资源

```bash
# 危险操作：可能删除意外的集群级资源
kubectl delete clusterrole,clusterrolebinding -l app.kubernetes.io/managed-by=Helm

# 安全做法：先列出要删除的资源
kubectl get clusterrole,clusterrolebinding -l app.kubernetes.io/managed-by=Helm
```

### 2. 集群级资源需要特别注意

ClusterRole、ClusterRoleBinding、CRD 等集群级资源可能被多个组件共享。删除前需要确认不会影响核心组件（如 CNI、DNS）。

### 3. 排查网络问题的检查清单

1. **验证基础连通性**
   - hostNetwork pod 能否访问目标？
   - 普通 pod 能否 ping 网关？

2. **检查 CNI 状态**
   - Cilium endpoint 状态是否正常？
   - Pod Identity 是否正确（不应该是 reserved:init）？

3. **检查 RBAC**
   - CNI 组件的 ServiceAccount 是否有足够权限？
   - ClusterRole/ClusterRoleBinding 是否存在？

4. **检查 kube-proxy/IPVS**
   - IPVS 规则是否正确？
   - kube-ipvs0 接口是否存在？

---

## 相关命令参考

```bash
# 检查 Cilium endpoint
kubectl exec -n kube-system <cilium-pod> -- cilium endpoint list

# 检查 Cilium 状态
kubectl exec -n kube-system <cilium-pod> -- cilium status

# 检查 IPVS 规则
ipvsadm -Ln | grep -A10 "<ClusterIP>"

# 检查 ClusterRole
kubectl get clusterrole <name> -o yaml

# 创建 debug pod (hostNetwork)
kubectl run debug --image=nicolaka/netshoot --rm -it \
  --overrides='{"spec":{"hostNetwork":true}}' -- /bin/bash

# 创建 debug pod (pod network)
kubectl run debug --image=nicolaka/netshoot --rm -it -- /bin/bash
```

---

## 附录：恢复其他被删除的 ClusterRole

如果其他组件的 ClusterRole 也被删除，可以通过 Helm upgrade 恢复：

```bash
# 方法1: Helm upgrade (推荐)
helm upgrade <release-name> <chart> -n <namespace> --reuse-values

# 方法2: 从 Helm template 提取并应用
helm template <release-name> <chart> | kubectl apply -f -

# 方法3: 手动创建 (参考官方文档)
kubectl apply -f <clusterrole.yaml>
```

