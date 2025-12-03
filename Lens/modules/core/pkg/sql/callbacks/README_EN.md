# Database Connection Auto-Reconnection Mechanism

## Overview

This module provides a zero-intrusion automatic reconnection mechanism for database connections, specifically designed to handle PostgreSQL master-slave switchover scenarios. When a database master-slave switchover occurs, the application's connection pool may still be connected to the old master node (now downgraded to a read-only replica), causing write operations to fail.

## Problem Scenario

### Typical Error

```
ERROR: cannot execute INSERT in a read-only transaction (SQLSTATE 25006)
```

### Root Cause

1. **Initial State**: Application connects to PostgreSQL master node (read-write)
2. **Master-Slave Switchover**: Master node fails, slave node is promoted to new master
3. **Service DNS Update**: Kubernetes Service endpoint points to new master node
4. **Connection Pool Not Updated**: Old connections in application's connection pool still point to old master (now read-only replica)
5. **Write Operations Fail**: Attempts to execute INSERT/UPDATE/DELETE encounter read-only errors

## Solution

### Multi-Layer Protection Mechanism

#### 1. Connection Pool Lifecycle Management (Passive Protection)

Configured in `conn.go`:

```go
sqlDB.SetConnMaxLifetime(5 * time.Minute)    // Connection max lifetime: 5 minutes
sqlDB.SetConnMaxIdleTime(2 * time.Minute)    // Idle connection max lifetime: 2 minutes
```

**Effects**:
- Ensures connections are refreshed periodically, establishing new connections after max 5 minutes
- Idle connections are automatically cleaned up after 2 minutes
- Automatically prevents long-term connections to expired nodes

#### 2. Active Health Check (Proactive Protection)

Check database status before write operations:

```go
// Use PostgreSQL built-in function to check if it's a read-only replica
SELECT pg_is_in_recovery()
```

**Features**:
- Checks at most once every 10 seconds (caching mechanism to avoid performance impact)
- Automatically triggers reconnection when read-only replica is detected
- Completely transparent to business code

#### 3. Auto-Reconnect After Errors (Remediation)

When read-only transaction errors are detected:

1. Identify characteristic error messages
2. Close all existing connections
3. Reestablish connection pool
4. Verify writability of new connections
5. Retry up to 3 times with exponential backoff strategy

## Usage

### Automatic Activation (Recommended)

Already automatically enabled during database initialization in `storage.go`:

```go
gormDb, err := sql.InitGormDB(clusterName, sqlConfig,
    sql.WithTracingCallback(),
    sql.WithErrorStackCallback(),
    sql.WithReconnectCallback(),  // Auto-reconnection mechanism
)
```

### No Business Code Modification Required

All existing database operation code **requires no modifications**, for example:

```go
// Business code remains unchanged
err := database.GetFacade().GetNode().UpdateNode(ctx, node)
if err != nil {
    // If it's a read-only error, the callback will automatically handle reconnection
    // Application layer can choose to retry
    return err
}
```

## Workflow

```
┌─────────────────┐
│ Business Write  │
│   Operation     │
└────────┬────────┘
         │
         ▼
┌─────────────────────────┐
│ Before Hook:            │
│ Health Check (10s cache)│
└────────┬────────────────┘
         │
         ├─ Healthy ───────┐
         │                 │
         └─ Unhealthy ──┐  │
                        │  │
         ┌──────────────▼──▼──┐
         │ Execute DB Operation│
         └──────────┬──────────┘
                    │
         ┌──────────▼──────────┐
         │  Success?            │
         └──────────┬──────────┘
                    │
         ┌──────────▼──────────────┐
         │ After Hook:             │
         │ Detected Read-only Error│
         └──────────┬──────────────┘
                    │
         ┌──────────▼──────────────┐
         │ Close All Connections   │
         │ Reestablish Pool        │
         │ Verify Writability      │
         │ Retry up to 3 times     │
         └──────────┬──────────────┘
                    │
         ┌──────────▼──────────────┐
         │ Application Layer       │
         │ Receives Error          │
         │ Can Retry Business Logic│
         └─────────────────────────┘
```

## Performance Impact

1. **Health Check Cache**: Checks at most once every 10 seconds, minimal performance impact
2. **Only Before Write Operations**: Read operations are not affected
3. **Lightweight Check**: Uses `pg_is_in_recovery()` function, response time < 1ms
4. **Only Reconnects on Errors**: No extra overhead under normal circumstances

## Configuration Parameters

Can be adjusted in `reconnect.go`:

```go
const (
    reconnectMaxRetries = 3                      // Maximum retry attempts
    reconnectInterval   = 500 * time.Millisecond // Retry interval
)

// Health check cache interval
checkInterval: 10 * time.Second
```

## Log Output

### Normal Case
```
INFO: Configured connection pool for 'cluster-name': MaxIdleConn=10, MaxOpenConn=40, ConnMaxLifetime=5m, ConnMaxIdleTime=2m
INFO: Registered database reconnection callbacks successfully
```

### Problem Detected
```
WARN: Detected read-only transaction error: ERROR: cannot execute INSERT in a read-only transaction (SQLSTATE 25006)
INFO: Attempting to reconnect and retry (attempt 1/3)...
INFO: Closing all existing database connections...
INFO: Successfully reconnected to database
```

### Health Check
```
WARN: Health check: database not writable: database is in recovery mode (read-only replica)
INFO: Successfully reconnected to database
```

## Failure Recovery Time

- **Passive Mode (Connection Pool Only)**: Max 5 minutes (ConnMaxLifetime)
- **Active Mode (Health Check)**: Max 10 seconds (checkInterval)
- **Reactive Mode (Error Reconnect)**: Immediate (< 1 second)

## Notes

1. **Application Layer Retry**: Current implementation does not automatically retry business logic; application layer can choose to retry after receiving errors
2. **Transaction Handling**: If error occurs within a transaction, entire transaction will be rolled back; application layer needs to restart the transaction
3. **Concurrency Safety**: All operations are concurrency-safe, using mutexes to protect shared state

## Extensibility

If automatic business logic retry is needed, add a retry decorator at application layer:

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

// Usage
err := withRetry(func() error {
    return database.GetFacade().GetNode().UpdateNode(ctx, node)
}, 3)
```

## Testing

Can test with the following methods:

1. **Simulate Master-Slave Switchover**: Manually switch database to read-only mode
   ```sql
   -- In PostgreSQL
   ALTER SYSTEM SET default_transaction_read_only = on;
   SELECT pg_reload_conf();
   ```

2. **Observe Logs**: Check for reconnection log output

3. **Verify Recovery**: After restoring database to writable mode, observe if connection automatically recovers

## Related Files

- `reconnect.go` - Core reconnection logic
- `opts.go` - GORM callback registration
- `conn.go` - Connection pool configuration
- `storage.go` - Initialization invocation

