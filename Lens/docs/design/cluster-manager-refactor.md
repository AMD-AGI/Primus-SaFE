# ClusterManager 重构设计

## 背景

当前 `cluster_manager.go` 存在以下问题：

1. 初始化逻辑混杂，通过 `multiCluster` 布尔值区分，职责不清晰
2. 控制面和数据面的初始化路径耦合在一起
3. 组件类型和所需 client 是代码层面确定的，但目前通过运行时参数传递

## 设计方案

### 1. 组件声明式注册

每个组件在代码中硬编码声明自己的类型和依赖：

```go
// 组件声明
type ComponentDeclaration struct {
    Type          ComponentType   // 组件类型
    RequireK8S    bool            // 是否需要 K8S client
    RequireStorage bool           // 是否需要 Storage client
}

type ComponentType int

const (
    ComponentTypeControlPlane ComponentType = iota  // 控制面
    ComponentTypeDataPlane                          // 数据面
)
```

### 2. 组件声明示例

```go
// api-server/main.go - 控制面组件
var declaration = clientsets.ComponentDeclaration{
    Type:           clientsets.ComponentTypeControlPlane,
    RequireK8S:     true,
    RequireStorage: true,
}

// gpu-exporter/main.go - 数据面组件  
var declaration = clientsets.ComponentDeclaration{
    Type:           clientsets.ComponentTypeDataPlane,
    RequireK8S:     true,
    RequireStorage: false,
}
```

### 3. 初始化入口

```go
func InitClusterManager(ctx context.Context, decl ComponentDeclaration) error
```

ClusterManager 根据声明自动选择初始化路径：

```
ComponentTypeControlPlane
    │
    ├─► initControlPlaneClient()     // 初始化本集群 client
    │
    └─► initMultiClusterClients()    // 加载所有集群配置 + 周期性同步

ComponentTypeDataPlane
    │
    └─► initDataPlaneClient()        // 仅初始化当前集群 client
```

### 4. 文件拆分

```
clientsets/
├── cluster_manager.go          // 统一入口、ClusterManager 结构
├── declaration.go              // ComponentDeclaration 定义
├── controlplane_init.go        // 控制面初始化逻辑
├── multicluster_init.go        // 多集群初始化逻辑
└── dataplane_init.go           // 数据面初始化逻辑
```

### 5. 接口分离

```go
// 控制面使用
type ControlPlaneManager interface {
    GetClientSetByClusterName(name string) (*ClusterClientSet, error)
    ListAllClientSets() map[string]*ClusterClientSet
}

// 数据面使用
type DataPlaneManager interface {
    GetCurrentClusterClients() *ClusterClientSet
}
```

## 现有接口引用分析

| 接口 | 引用次数 | 使用场景 | 重构策略 |
|-----|---------|---------|---------|
| `GetClusterManager()` | 303 | 全局入口 | 保留，返回类型不变 |
| `GetClusterClientsOrDefault()` | 158 | API 层按请求参数获取集群 | 仅控制面可用 |
| `GetCurrentClusterName()` | 69 | 获取当前集群名 | 两种类型都可用 |
| `GetCurrentClusterClients()` | 54 | 数据面组件获取本集群 client | 两种类型都可用 |
| `GetClientSetByClusterName()` | 40 | 控制面按名称获取 client | 仅控制面可用 |
| `ListAllClientSets()` | 10 | 多集群同步 | 仅控制面可用 |
| `InitClusterManager()` | 3 | 初始化入口 | 签名变更 |
| `IsMultiCluster()` | 3 | 测试用 | 改为判断 ComponentType |

### 使用模式

**数据面组件** (gpu-exporter, node-exporter, jobs 等)：
```go
// 只用这两个接口
clientsets.GetClusterManager().GetCurrentClusterClients()
clientsets.GetClusterManager().GetCurrentClusterName()
```

**控制面组件** (api-server, multi-cluster-config-exporter 等)：
```go
// 需要多集群能力
cm.GetClusterClientsOrDefault(clusterName)
cm.GetClientSetByClusterName(clusterName)
cm.ListAllClientSets()
```

### 兼容性策略

1. **保持接口签名不变**：`GetCurrentClusterClients()`, `GetCurrentClusterName()` 等接口签名保持不变
2. **运行时检查**：数据面组件调用 `GetClientSetByClusterName()` 等控制面接口时返回错误
3. **编译时无感知**：现有代码无需修改，仅在运行时根据组件类型决定是否可用

### 重构步骤

1. 新增 `declaration.go`，定义 `ComponentDeclaration`
2. 拆分初始化逻辑到 `controlplane_init.go`, `dataplane_init.go`, `multicluster_init.go`
3. 修改 `InitClusterManager()` 签名，接收 `ComponentDeclaration`
4. 为控制面专用接口添加组件类型检查
5. 逐个组件添加声明，验证行为正确

## 重构计划

> **实施说明**：由于所有组件都使用统一的 `server.InitServer` 入口，Phase 3-4 通过修改 `server.go` 一次完成。

### Phase 1: 基础设施 ✅ 已完成

**目标**：添加声明式框架，不改变现有行为

1. 新建 `clientsets/declaration.go`
   - 定义 `ComponentType` 枚举
   - 定义 `ComponentDeclaration` 结构体

2. 新建 `clientsets/controlplane_init.go`
   - 从 `cluster_manager.go` 提取控制面初始化逻辑
   - `initControlPlaneClients()` - 初始化本集群 K8S + Storage

3. 新建 `clientsets/multicluster_init.go`
   - 从 `cluster_manager.go` 提取多集群初始化逻辑
   - `initMultiClusterClients()` - 从 cluster_config 表加载所有集群
   - `startMultiClusterSync()` - 周期性同步

4. 新建 `clientsets/dataplane_init.go`
   - 从 `cluster_manager.go` 提取数据面初始化逻辑
   - `initDataPlaneClients()` - 仅初始化当前集群

5. 修改 `cluster_manager.go`
   - 新增 `InitClusterManagerV2(ctx, decl)` 入口
   - 保留旧 `InitClusterManager()` 作为兼容层，内部调用 V2
   - 添加 `componentType` 字段到 `ClusterManager`

**验证**：现有测试全部通过，行为无变化

---

### Phase 2: 接口保护 ✅ 已完成

**目标**：为控制面专用接口添加运行时检查

1. 修改 `GetClientSetByClusterName()`
   - 数据面组件调用时返回 error

2. 修改 `ListAllClientSets()`
   - 数据面组件调用时返回空 map + warning log

3. 修改 `GetClusterClientsOrDefault()`
   - 数据面组件调用时，忽略 clusterName 参数，直接返回当前集群

**验证**：添加单元测试验证组件类型检查逻辑

---

### Phase 3-4: 组件迁移 ✅ 已完成

**目标**：迁移所有组件使用新声明式 API

**实际实现**：由于所有组件都使用 `server.InitServer` 或 `server.InitServerWithPreInitFunc`，只需修改 `core/pkg/server/server.go` 即可完成全部迁移：

```go
// server.go 中根据 cfg.IsControlPlane 自动选择组件类型
decl := clientsets.ComponentDeclaration{
    RequireK8S:     cfg.LoadK8SClient,
    RequireStorage: cfg.LoadStorageClient,
}

if cfg.IsControlPlane {
    decl.Type = clientsets.ComponentTypeControlPlane
} else {
    decl.Type = clientsets.ComponentTypeDataPlane
}

err = clientsets.InitClusterManagerV2(ctx, decl)
```

**涉及组件**（全部自动迁移）：
- 控制面：api-server, multi-cluster-config-exporter
- 数据面：gpu-resource-exporter, node-exporter, storage-exporter, network-exporter, gateway-exporter, github-runners-exporter, telemetry-processor, ai-advisor, primus-safe-adapter, jobs

---

### Phase 5: 清理 ✅ 已完成

**目标**：移除兼容代码

已完成：
1. ✅ 移除旧 `InitClusterManager(ctx, multiCluster, loadK8S, loadStorage)` 函数
2. ✅ 将 `InitClusterManagerV2()` 重命名为 `InitClusterManager(ctx, decl)`
3. ✅ 移除 `ClusterManager` 结构体中的 `multiCluster` 字段
4. ✅ 移除旧的 `initializeK8SClients()` 和 `initializeStorageClients()` 方法
5. ✅ 更新 `IsMultiCluster()` 使用 `componentType.IsControlPlane()`
6. ✅ 更新所有测试

保留（向后兼容）：
- `InitClientSets()` 函数（标记为 Deprecated）
- `cfg.MultiCluster` 配置项（可后续移除）

---

### 组件分类参考

| 组件 | 类型 | K8S | Storage |
|-----|------|-----|---------|
| api-server | ControlPlane | Y | Y |
| multi-cluster-config-exporter | ControlPlane | Y | Y |
| gpu-resource-exporter | DataPlane | Y | Y |
| node-exporter | DataPlane | Y | N |
| storage-exporter | DataPlane | Y | N |
| network-exporter | DataPlane | Y | N |
| gateway-exporter | DataPlane | Y | N |
| github-runners-exporter | DataPlane | Y | Y |
| telemetry-processor | DataPlane | Y | Y |
| ai-advisor | DataPlane | Y | Y |
| primus-safe-adapter | DataPlane | Y | Y |
| jobs (dataplane jobs) | DataPlane | Y | Y |

## 收益

1. **声明式**：组件类型和依赖在代码中显式声明，一目了然
2. **编译时确定**：不依赖运行时配置，减少出错可能
3. **职责分离**：控制面/数据面初始化逻辑独立，便于维护
4. **按需加载**：数据面不加载多集群配置，控制面不遗漏
