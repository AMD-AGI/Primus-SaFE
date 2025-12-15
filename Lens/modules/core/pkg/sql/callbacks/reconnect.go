package callbacks

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	"gorm.io/gorm"
)

const (
	reconnectMaxRetries = 3
	reconnectInterval   = 500 * time.Millisecond
)

// Read-only transaction error codes and message patterns
var readOnlyErrors = []string{
	"cannot execute INSERT in a read-only transaction",
	"cannot execute UPDATE in a read-only transaction",
	"cannot execute DELETE in a read-only transaction",
	"SQLSTATE 25006", // PostgreSQL read-only transaction error code
}

// healthCheckCache stores health check results to avoid frequent checks
type healthCheckCache struct {
	mu            sync.RWMutex
	lastCheck     time.Time
	isHealthy     bool
	checkInterval time.Duration
}

var (
	healthCache = &healthCheckCache{
		checkInterval: 10 * time.Second, // Check at most once every 10 seconds
	}
)

// isReadOnlyError checks if the error is a read-only transaction error
func isReadOnlyError(err error) bool {
	if err == nil {
		return false
	}
	errMsg := err.Error()
	for _, pattern := range readOnlyErrors {
		if strings.Contains(errMsg, pattern) {
			return true
		}
	}
	return false
}

// checkDBWritable checks if the database is writable
func checkDBWritable(ctx context.Context, sqlDB *sql.DB) error {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	// Perform a lightweight write capability check
	// Use pg_is_in_recovery() to check if this is a read-only replica
	var isRecovery bool
	err := sqlDB.QueryRowContext(ctx, "SELECT pg_is_in_recovery()").Scan(&isRecovery)
	if err != nil {
		return fmt.Errorf("failed to check database status: %w", err)
	}

	if isRecovery {
		return fmt.Errorf("database is in recovery mode (read-only replica)")
	}

	return nil
}

// reconnectDB re-establishes the database connection by forcing connection pool refresh
// NOTE: Do NOT call sqlDB.Close() as it permanently closes the connection pool
// and sql.DB does not automatically reopen after Close() is called.
func reconnectDB(db *gorm.DB) error {
	sqlDB, err := db.DB()
	if err != nil {
		return fmt.Errorf("failed to get sql.DB: %w", err)
	}

	log.Infof("Forcing database connection pool refresh...")

	// Save current pool settings
	stats := sqlDB.Stats()
	maxOpenConns := stats.MaxOpenConnections

	// Force all existing connections to expire immediately
	// This will cause the pool to create new connections on next use
	sqlDB.SetConnMaxLifetime(1 * time.Nanosecond)
	sqlDB.SetConnMaxIdleTime(1 * time.Nanosecond)

	// Wait briefly for idle connections to be cleaned up
	time.Sleep(100 * time.Millisecond)

	// Restore reasonable connection pool settings
	sqlDB.SetConnMaxLifetime(5 * time.Minute)
	sqlDB.SetConnMaxIdleTime(2 * time.Minute)

	// Verify the connection works by pinging
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := sqlDB.PingContext(ctx); err != nil {
		return fmt.Errorf("failed to ping database after connection refresh: %w", err)
	}

	log.Infof("Successfully refreshed database connection pool (maxOpenConns=%d)", maxOpenConns)
	return nil
}

// beforeOperationHealthCheck performs health check before operations
func beforeOperationHealthCheck(db *gorm.DB) {
	// Quick cache check for write operations (Create/Update/Delete)
	healthCache.mu.RLock()
	shouldCheck := time.Since(healthCache.lastCheck) > healthCache.checkInterval || !healthCache.isHealthy
	healthCache.mu.RUnlock()

	if !shouldCheck {
		return
	}

	// Perform health check
	sqlDB, err := db.DB()
	if err != nil {
		log.Warnf("Health check: failed to get sql.DB: %v", err)
		return
	}

	ctx := db.Statement.Context
	if ctx == nil {
		ctx = context.Background()
	}

	err = checkDBWritable(ctx, sqlDB)

	healthCache.mu.Lock()
	healthCache.lastCheck = time.Now()
	healthCache.isHealthy = (err == nil)
	healthCache.mu.Unlock()

	if err != nil {
		log.Warnf("Health check: database not writable: %v", err)
		// Attempt to reconnect
		if reconnectErr := reconnectDB(db); reconnectErr != nil {
			log.Errorf("Health check: failed to reconnect: %v", reconnectErr)
		} else {
			healthCache.mu.Lock()
			healthCache.isHealthy = true
			healthCache.mu.Unlock()
		}
	}
}

// afterOperationErrorHandler handles errors after operations and retries automatically
func afterOperationErrorHandler(db *gorm.DB) {
	if db.Error == nil {
		return
	}

	// Check if this is a read-only transaction error
	if !isReadOnlyError(db.Error) {
		return
	}

	log.Warnf("Detected read-only transaction error: %v", db.Error)

	// Mark as unhealthy
	healthCache.mu.Lock()
	healthCache.isHealthy = false
	healthCache.mu.Unlock()

	// Attempt to reconnect and retry
	for i := 0; i < reconnectMaxRetries; i++ {
		log.Infof("Attempting to reconnect and retry (attempt %d/%d)...", i+1, reconnectMaxRetries)

		if err := reconnectDB(db); err != nil {
			log.Errorf("Reconnect attempt %d failed: %v", i+1, err)
			time.Sleep(reconnectInterval * time.Duration(i+1)) // Exponential backoff
			continue
		}

		// Verify database is writable
		sqlDB, err := db.DB()
		if err != nil {
			log.Errorf("Failed to get sql.DB after reconnect: %v", err)
			continue
		}

		ctx := db.Statement.Context
		if ctx == nil {
			ctx = context.Background()
		}

		if err := checkDBWritable(ctx, sqlDB); err != nil {
			log.Errorf("Database still not writable after reconnect: %v", err)
			time.Sleep(reconnectInterval * time.Duration(i+1))
			continue
		}

		// Reconnection successful
		log.Infof("Reconnected successfully, operation will be retried by application")
		healthCache.mu.Lock()
		healthCache.isHealthy = true
		healthCache.mu.Unlock()

		// Note: We don't clear db.Error here because we want the application layer to know an error occurred
		// The application layer can choose to retry. Automatic retry would require more complex logic
		break
	}
}

// RegisterReconnectCallbacks registers auto-reconnect callbacks
func RegisterReconnectCallbacks(db *gorm.DB) error {
	// Register health checks before write operations
	if err := db.Callback().Create().Before("gorm:create").Register("reconnect:health_check_create", beforeOperationHealthCheck); err != nil {
		return fmt.Errorf("failed to register reconnect:health_check_create: %w", err)
	}
	if err := db.Callback().Update().Before("gorm:update").Register("reconnect:health_check_update", beforeOperationHealthCheck); err != nil {
		return fmt.Errorf("failed to register reconnect:health_check_update: %w", err)
	}
	if err := db.Callback().Delete().Before("gorm:delete").Register("reconnect:health_check_delete", beforeOperationHealthCheck); err != nil {
		return fmt.Errorf("failed to register reconnect:health_check_delete: %w", err)
	}

	// Register error handlers after operations
	if err := db.Callback().Create().After("gorm:after_create").Register("reconnect:error_handler_create", afterOperationErrorHandler); err != nil {
		return fmt.Errorf("failed to register reconnect:error_handler_create: %w", err)
	}
	if err := db.Callback().Update().After("gorm:after_update").Register("reconnect:error_handler_update", afterOperationErrorHandler); err != nil {
		return fmt.Errorf("failed to register reconnect:error_handler_update: %w", err)
	}
	if err := db.Callback().Delete().After("gorm:after_delete").Register("reconnect:error_handler_delete", afterOperationErrorHandler); err != nil {
		return fmt.Errorf("failed to register reconnect:error_handler_delete: %w", err)
	}

	log.Infof("Registered database reconnection callbacks successfully")
	return nil
}
