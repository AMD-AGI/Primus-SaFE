# 快速开始：零侵入数据库自动重连

## 🎯 你的问题

```
ERROR: cannot execute INSERT in a read-only transaction (SQLSTATE 25006)
```

**原因**：PostgreSQL 主从切换后，应用连接池仍连接到旧的只读副本

## ✨ 解决方案

**好消息**：业务代码**完全不需要修改**！框架层已自动处理。

## 📦 已包含的功能

### 1️⃣ 自动防护（已启用）

所有写操作（Create/Update/Delete）都自动受保护：

```go
// 你的代码保持不变
err := database.GetFacade().GetNode().UpdateNode(ctx, node)
// 框架会自动：
// ✅ 定期刷新连接（5分钟）
// ✅ 主动检查数据库状态（10秒缓存）
// ✅ 检测错误后立即重连
```

### 2️⃣ 可选增强（需要时使用）

如果你想要**更快的恢复**，可以添加应用层重试：

```go
// 原始代码
err := database.GetFacade().GetNode().UpdateNode(ctx, node)

// 改为（可选）
err := database.WithRetry(ctx, func() error {
    return database.GetFacade().GetNode().UpdateNode(ctx, node)
})
```

## 🚀 使用场景

### 场景 1：Controller/Reconciler（推荐增强）

```go
func (r *NodeReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    node := &corev1.Node{}
    // ... 获取节点信息 ...
    
    dbNode := convertToDBNode(node)
    
    // 推荐：添加重试，减少 reconcile 循环
    err := database.WithRetry(ctx, func() error {
        return database.GetFacade().GetNode().UpdateNode(ctx, dbNode)
    })
    
    if err != nil {
        return ctrl.Result{}, err
    }
    
    return ctrl.Result{}, nil
}
```

### 场景 2：API 处理器（可选使用）

```go
func (h *Handler) UpdateNode(w http.ResponseWriter, r *http.Request) {
    // ... 解析请求 ...
    
    // 可选：API 端点可以使用重试，但要考虑响应时间
    err := database.WithRetry(r.Context(), func() error {
        return database.GetFacade().GetNode().UpdateNode(r.Context(), node)
    })
    
    if err != nil {
        http.Error(w, err.Error(), http.StatusInternalServerError)
        return
    }
    
    w.WriteHeader(http.StatusOK)
}
```

### 场景 3：批量操作（谨慎使用）

```go
func BatchUpdateNodes(ctx context.Context, nodes []*model.Node) error {
    facade := database.GetFacade().GetNode()
    
    for _, node := range nodes {
        // 为每个节点单独重试
        err := database.WithRetry(ctx, func() error {
            return facade.UpdateNode(ctx, node)
        })
        if err != nil {
            return err // 或继续处理，取决于业务需求
        }
    }
    
    return nil
}
```

### 场景 4：只读操作（不需要重试）

```go
func ListNodes(ctx context.Context) ([]*model.Node, error) {
    // 读操作不受主从切换影响，不需要特殊处理
    return database.GetFacade().GetNode().ListGpuNodes(ctx)
}
```

## 📊 恢复时间对比

| 使用方式 | 恢复时间 | 适用场景 |
|---------|---------|---------|
| 仅框架层自动防护 | < 10秒 | 大部分场景已足够 |
| 框架层 + 应用层重试 | < 1秒 | 关键业务路径 |

## 🔧 自定义配置（可选）

如果默认配置不满足需求，可以自定义：

```go
customConfig := database.RetryConfig{
    MaxRetries:    5,                      // 最多重试5次（默认3次）
    InitialDelay:  1 * time.Second,        // 初始延迟1秒（默认500ms）
    MaxDelay:      10 * time.Second,       // 最大延迟10秒（默认5秒）
    DelayMultiple: 2.0,                    // 指数退避系数（默认2.0）
}

err := database.WithRetryConfig(ctx, customConfig, func() error {
    return database.GetFacade().GetNode().UpdateNode(ctx, node)
})
```

## 📝 日志观察

### 正常情况（启动时）
```
INFO: Configured connection pool: ConnMaxLifetime=5m, ConnMaxIdleTime=2m
INFO: Registered database reconnection callbacks successfully
```

### 检测到问题时
```
WARN: Detected read-only transaction error: SQLSTATE 25006
INFO: Attempting to reconnect (attempt 1/3)...
INFO: Successfully reconnected to database
```

### 应用层重试时
```
WARN: Retriable error encountered (attempt 1/3): read-only transaction, retrying in 500ms...
INFO: Operation succeeded after 1 retries
```

## ⚡ 最佳实践

### ✅ 推荐

1. **Controller/Reconciler**: 使用 `WithRetry()` 包装写操作
2. **关键业务**: 使用 `WithRetry()` 提高可靠性
3. **异步任务**: 使用 `WithRetry()` 减少失败

### ⚠️ 注意

1. **用户 API**: 谨慎使用重试，避免响应时间过长
2. **大批量操作**: 考虑设置超时或分批处理
3. **只读操作**: 不需要使用重试

### ❌ 避免

1. **不要在事务外层包装重试**: 事务内部的错误应该让事务回滚
2. **不要嵌套重试**: 避免重试逻辑嵌套使用

## 🐛 问题排查

### 问题：仍然看到只读错误

**可能原因**：
1. 连接池中有大量活跃连接，还未到过期时间
2. 健康检查缓存还未失效

**解决方法**：
1. 等待10秒（健康检查缓存间隔）
2. 或重启应用（立即清空连接池）

### 问题：重连失败

**可能原因**：
1. 新的主节点还未完全就绪
2. DNS 还未更新

**解决方法**：
1. 检查数据库集群状态
2. 检查 Kubernetes Service 状态
3. 查看应用日志了解详细错误

### 问题：性能下降

**可能原因**：
1. 频繁重连导致
2. 数据库本身有问题

**解决方法**：
1. 检查重连日志频率
2. 检查数据库性能指标
3. 考虑调整 `checkInterval` 参数

## 📚 更多文档

- 📖 [详细技术文档](./callbacks/README.md) - 了解实现原理
- 📖 [完整方案说明](./AUTO_RECONNECT.md) - 架构和配置
- 📖 [使用示例](../database/retry_example.go) - 更多代码示例

## 🎉 总结

**对于大部分场景**，你**什么都不需要做**，框架已经自动处理了！

**如果你想要更快的恢复**，只需在关键路径添加 `database.WithRetry()` 包装即可。

就这么简单！🚀

