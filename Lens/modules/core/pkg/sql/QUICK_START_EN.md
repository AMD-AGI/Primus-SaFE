# Quick Start: Zero-Intrusion Database Auto-Reconnection

## 🎯 Your Problem

```
ERROR: cannot execute INSERT in a read-only transaction (SQLSTATE 25006)
```

**Reason**: After PostgreSQL master-slave switchover, application connection pool is still connected to old read-only replica

## ✨ Solution

**Good news**: Business code **requires absolutely no modifications**! Framework layer handles it automatically.

## 📦 Included Features

### 1️⃣ Automatic Protection (Already Enabled)

All write operations (Create/Update/Delete) are automatically protected:

```go
// Your code remains unchanged
err := database.GetFacade().GetNode().UpdateNode(ctx, node)
// Framework automatically:
// ✅ Periodically refreshes connections (5 minutes)
// ✅ Actively checks database status (10 second cache)
// ✅ Immediately reconnects after detecting errors
```

### 2️⃣ Optional Enhancement (Use When Needed)

If you want **faster recovery**, add application-layer retry:

```go
// Original code
err := database.GetFacade().GetNode().UpdateNode(ctx, node)

// Change to (optional)
err := database.WithRetry(ctx, func() error {
    return database.GetFacade().GetNode().UpdateNode(ctx, node)
})
```

## 🚀 Use Cases

### Scenario 1: Controller/Reconciler (Recommended Enhancement)

```go
func (r *NodeReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    node := &corev1.Node{}
    // ... fetch node information ...
    
    dbNode := convertToDBNode(node)
    
    // Recommended: Add retry to reduce reconcile loops
    err := database.WithRetry(ctx, func() error {
        return database.GetFacade().GetNode().UpdateNode(ctx, dbNode)
    })
    
    if err != nil {
        return ctrl.Result{}, err
    }
    
    return ctrl.Result{}, nil
}
```

### Scenario 2: API Handlers (Optional Use)

```go
func (h *Handler) UpdateNode(w http.ResponseWriter, r *http.Request) {
    // ... parse request ...
    
    // Optional: API endpoints can use retry, but consider response time
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

### Scenario 3: Batch Operations (Use Carefully)

```go
func BatchUpdateNodes(ctx context.Context, nodes []*model.Node) error {
    facade := database.GetFacade().GetNode()
    
    for _, node := range nodes {
        // Retry each node separately
        err := database.WithRetry(ctx, func() error {
            return facade.UpdateNode(ctx, node)
        })
        if err != nil {
            return err // Or continue processing, depending on business needs
        }
    }
    
    return nil
}
```

### Scenario 4: Read Operations (No Retry Needed)

```go
func ListNodes(ctx context.Context) ([]*model.Node, error) {
    // Read operations not affected by master-slave switchover, no special handling needed
    return database.GetFacade().GetNode().ListGpuNodes(ctx)
}
```

## 📊 Recovery Time Comparison

| Usage Method | Recovery Time | Use Case |
|-------------|---------------|----------|
| Framework layer auto-protection only | < 10 seconds | Sufficient for most scenarios |
| Framework layer + application layer retry | < 1 second | Critical business paths |

## 🔧 Custom Configuration (Optional)

If default configuration doesn't meet requirements, customize:

```go
customConfig := database.RetryConfig{
    MaxRetries:    5,                      // Max 5 retries (default 3)
    InitialDelay:  1 * time.Second,        // Initial delay 1 second (default 500ms)
    MaxDelay:      10 * time.Second,       // Max delay 10 seconds (default 5 seconds)
    DelayMultiple: 2.0,                    // Exponential backoff factor (default 2.0)
}

err := database.WithRetryConfig(ctx, customConfig, func() error {
    return database.GetFacade().GetNode().UpdateNode(ctx, node)
})
```

## 📝 Log Observation

### Normal Case (Startup)
```
INFO: Configured connection pool: ConnMaxLifetime=5m, ConnMaxIdleTime=2m
INFO: Registered database reconnection callbacks successfully
```

### Problem Detected
```
WARN: Detected read-only transaction error: SQLSTATE 25006
INFO: Attempting to reconnect (attempt 1/3)...
INFO: Successfully reconnected to database
```

### Application Layer Retry
```
WARN: Retriable error encountered (attempt 1/3): read-only transaction, retrying in 500ms...
INFO: Operation succeeded after 1 retries
```

## ⚡ Best Practices

### ✅ Recommended

1. **Controller/Reconciler**: Use `WithRetry()` to wrap write operations
2. **Critical Business**: Use `WithRetry()` to improve reliability
3. **Async Tasks**: Use `WithRetry()` to reduce failures

### ⚠️ Cautions

1. **User APIs**: Use retry carefully to avoid excessive response time
2. **Large Batch Operations**: Consider setting timeouts or batch processing
3. **Read Operations**: No need to use retry

### ❌ Avoid

1. **Don't wrap retry outside transactions**: Errors within transactions should let transaction rollback
2. **Don't nest retries**: Avoid nested retry logic

## 🐛 Troubleshooting

### Issue: Still seeing read-only errors

**Possible Causes**:
1. Many active connections in connection pool, not yet expired
2. Health check cache not yet invalidated

**Solutions**:
1. Wait 10 seconds (health check cache interval)
2. Or restart application (immediately clear connection pool)

### Issue: Reconnection fails

**Possible Causes**:
1. New master node not fully ready
2. DNS not yet updated

**Solutions**:
1. Check database cluster status
2. Check Kubernetes Service status
3. View application logs for detailed errors

### Issue: Performance degradation

**Possible Causes**:
1. Frequent reconnections
2. Database itself has issues

**Solutions**:
1. Check reconnection log frequency
2. Check database performance metrics
3. Consider adjusting `checkInterval` parameter

## 📚 More Documentation

- 📖 [Detailed Technical Documentation](./callbacks/README_EN.md) - Understand implementation principles
- 📖 [Complete Solution Documentation](./AUTO_RECONNECT_EN.md) - Architecture and configuration
- 📖 [Usage Examples](../database/retry_example.go) - More code examples

## 🎉 Summary

**For most scenarios**, you **don't need to do anything**, the framework has already handled it automatically!

**If you want faster recovery**, simply wrap critical paths with `database.WithRetry()`.

It's that simple! 🚀

