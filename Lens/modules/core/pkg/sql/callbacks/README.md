# 数据库连接自动重连机制

## 概述

本模块提供了一个零侵入的数据库连接自动重连机制，专门用于处理 PostgreSQL 主从切换场景。当数据库发生主从切换时，应用程序的连接池可能仍然连接到旧的主节点（现已降级为只读副本），导致写操作失败。

## 问题场景

### 典型错误

```
ERROR: cannot execute INSERT in a read-only transaction (SQLSTATE 25006)
```

### 发生原因

1. **初始状态**：应用连接到 PostgreSQL 主节点（可读写）
2. **主从切换**：主节点故障，从节点提升为新主节点
3. **Service DNS 更新**：Kubernetes Service endpoint 指向新主节点
4. **连接池未更新**：应用的连接池中的旧连接仍然指向旧主节点（现在是只读副本）
5. **写操作失败**：尝试执行 INSERT/UPDATE/DELETE 时遇到只读错误

## 解决方案

### 多层防护机制

#### 1. 连接池生命周期管理（被动防护）

在 `conn.go` 中配置：

```go
sqlDB.SetConnMaxLifetime(5 * time.Minute)    // 连接最大生命周期：5分钟
sqlDB.SetConnMaxIdleTime(2 * time.Minute)    // 空闲连接最大生存时间：2分钟
```

**作用**：
- 确保连接定期刷新，最多5分钟后会建立新连接
- 空闲连接2分钟后自动清理
- 自动防止长期连接到过期的节点

#### 2. 主动健康检查（主动防护）

在写操作前检查数据库状态：

```go
// 使用 PostgreSQL 内置函数检查是否为只读副本
SELECT pg_is_in_recovery()
```

**特点**：
- 每10秒最多检查一次（缓存机制，避免性能影响）
- 检测到只读副本时自动触发重连
- 对业务代码完全透明

#### 3. 错误后自动重连（事后补救）

当检测到只读事务错误时：

1. 识别特征错误信息
2. 关闭所有现有连接
3. 重新建立连接池
4. 验证新连接可写性
5. 最多重试3次，使用指数退避策略

## 使用方法

### 自动启用（推荐）

在 `storage.go` 中初始化数据库时已自动启用：

```go
gormDb, err := sql.InitGormDB(clusterName, sqlConfig,
    sql.WithTracingCallback(),
    sql.WithErrorStackCallback(),
    sql.WithReconnectCallback(),  // 自动重连机制
)
```

### 无需修改业务代码

所有现有的数据库操作代码**无需任何修改**，例如：

```go
// 业务代码保持不变
err := database.GetFacade().GetNode().UpdateNode(ctx, node)
if err != nil {
    // 如果是只读错误，回调会自动处理重连
    // 应用层可以选择重试
    return err
}
```

## 工作流程

```
┌─────────────────┐
│  业务发起写操作  │
└────────┬────────┘
         │
         ▼
┌─────────────────────────┐
│ Before Hook:            │
│ 健康检查（缓存10秒）     │
└────────┬────────────────┘
         │
         ├─ 健康 ──────────┐
         │                 │
         └─ 不健康 ─────┐  │
                        │  │
         ┌──────────────▼──▼──┐
         │  执行数据库操作      │
         └──────────┬──────────┘
                    │
         ┌──────────▼──────────┐
         │  成功？              │
         └──────────┬──────────┘
                    │
         ┌──────────▼──────────────┐
         │ After Hook:             │
         │ 检测到只读错误？         │
         └──────────┬──────────────┘
                    │
         ┌──────────▼──────────────┐
         │ 关闭所有连接             │
         │ 重新建立连接池           │
         │ 验证可写性               │
         │ 最多重试3次              │
         └──────────┬──────────────┘
                    │
         ┌──────────▼──────────────┐
         │ 应用层收到错误           │
         │ 可选择重试业务逻辑       │
         └─────────────────────────┘
```

## 性能影响

1. **健康检查缓存**：每10秒最多检查一次，对性能影响极小
2. **只在写操作前检查**：读操作不受影响
3. **轻量级检查**：使用 `pg_is_in_recovery()` 函数，响应时间 < 1ms
4. **仅在错误时重连**：正常情况下无额外开销

## 配置参数

可在 `reconnect.go` 中调整：

```go
const (
    reconnectMaxRetries = 3                      // 最大重试次数
    reconnectInterval   = 500 * time.Millisecond // 重试间隔
)

// 健康检查缓存间隔
checkInterval: 10 * time.Second
```

## 日志输出

### 正常情况
```
INFO: Configured connection pool for 'cluster-name': MaxIdleConn=10, MaxOpenConn=40, ConnMaxLifetime=5m, ConnMaxIdleTime=2m
INFO: Registered database reconnection callbacks successfully
```

### 检测到问题
```
WARN: Detected read-only transaction error: ERROR: cannot execute INSERT in a read-only transaction (SQLSTATE 25006)
INFO: Attempting to reconnect and retry (attempt 1/3)...
INFO: Closing all existing database connections...
INFO: Successfully reconnected to database
```

### 健康检查
```
WARN: Health check: database not writable: database is in recovery mode (read-only replica)
INFO: Successfully reconnected to database
```

## 故障恢复时间

- **被动模式（仅连接池）**：最多5分钟（ConnMaxLifetime）
- **主动模式（健康检查）**：最多10秒（checkInterval）
- **响应模式（错误重连）**：立即（< 1秒）

## 注意事项

1. **应用层重试**：当前实现不会自动重试业务逻辑，应用层收到错误后可以选择重试
2. **事务处理**：如果在事务中发生错误，整个事务会回滚，需要应用层重新开始事务
3. **并发安全**：所有操作都是并发安全的，使用了互斥锁保护共享状态

## 扩展性

如果需要自动重试业务逻辑，可以在应用层添加重试装饰器：

```go
func withRetry(fn func() error, maxRetries int) error {
    for i := 0; i < maxRetries; i++ {
        err := fn()
        if err == nil {
            return nil
        }
        if isReadOnlyError(err) && i < maxRetries-1 {
            time.Sleep(time.Second)
            continue
        }
        return err
    }
    return fmt.Errorf("max retries exceeded")
}

// 使用
err := withRetry(func() error {
    return database.GetFacade().GetNode().UpdateNode(ctx, node)
}, 3)
```

## 测试

可以通过以下方式测试：

1. **模拟主从切换**：手动将数据库切换到只读模式
   ```sql
   -- 在 PostgreSQL 中
   ALTER SYSTEM SET default_transaction_read_only = on;
   SELECT pg_reload_conf();
   ```

2. **观察日志**：查看是否有重连日志输出

3. **验证恢复**：恢复数据库为可写模式后，观察连接是否自动恢复

## 相关文件

- `reconnect.go` - 核心重连逻辑
- `opts.go` - GORM 回调注册
- `conn.go` - 连接池配置
- `storage.go` - 初始化调用

