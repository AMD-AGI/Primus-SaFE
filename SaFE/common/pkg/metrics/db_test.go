/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package metrics

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/lib/pq"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	dto "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestClassifyDBError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want string
	}{
		{"nil is not an error", nil, ""},
		{"no rows from sqlx", sql.ErrNoRows, ErrKindNoRows},
		{"no rows from gorm", gorm.ErrRecordNotFound, ErrKindNoRows},
		{"deadline exceeded", context.DeadlineExceeded, ErrKindTimeout},
		{"canceled", context.Canceled, ErrKindCanceled},
		{"bad connection", driver.ErrBadConn, ErrKindConnection},
		{"unexpected eof", io.ErrUnexpectedEOF, ErrKindConnection},
		// 25006 is what a PostgreSQL replica returns for a write while it is
		// being promoted, which is the signal an operator needs to page on.
		{"read only transaction", &pq.Error{Code: "25006"}, ErrKindReadOnlyTransaction},
		{"other invalid tx state", &pq.Error{Code: "25001"}, ErrKindInvalidTxState},
		{"too many connections", &pq.Error{Code: "53300"}, ErrKindTooManyConnections},
		{"other insufficient resources", &pq.Error{Code: "53200"}, ErrKindInsufficientResources},
		{"cannot connect now", &pq.Error{Code: "57P03"}, ErrKindUnavailable},
		{"admin shutdown", &pq.Error{Code: "57P01"}, ErrKindUnavailable},
		{"connection exception", &pq.Error{Code: "08006"}, ErrKindConnection},
		{"unique violation", &pq.Error{Code: "23505"}, ErrKindConstraint},
		{"deadlock", &pq.Error{Code: "40P01"}, ErrKindSerialization},
		{"syntax error", &pq.Error{Code: "42601"}, ErrKindSyntaxOrAccess},
		{"system error", &pq.Error{Code: "58030"}, ErrKindSystemError},
		{"unknown pq class", &pq.Error{Code: "P0001"}, ErrKindOther},
		{"wrapped pq error", fmt.Errorf("upsert failed: %w", &pq.Error{Code: "25006"}), ErrKindReadOnlyTransaction},
		{"connection reset", &net.OpError{Op: "read", Err: syscall.ECONNRESET}, ErrKindConnection},
		{"plain error", errors.New("boom"), ErrKindOther},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, ClassifyDBError(tt.err))
		})
	}
}

func TestClassifyDBErrorNetworkTimeout(t *testing.T) {
	// A dial timeout reports Timeout() true and must not be lumped in with
	// connection failures, because the two have different remediations.
	err := &net.OpError{Op: "dial", Err: &timeoutError{}}
	assert.Equal(t, ErrKindTimeout, ClassifyDBError(err))
}

// timeoutError is a net.Error that reports itself as a timeout.
type timeoutError struct{}

func (e *timeoutError) Error() string   { return "i/o timeout" }
func (e *timeoutError) Timeout() bool   { return true }
func (e *timeoutError) Temporary() bool { return true }

func TestSQLOperation(t *testing.T) {
	tests := []struct {
		name  string
		query string
		want  string
	}{
		{"select", "SELECT * FROM workload WHERE id = $1", OpSelect},
		{"lowercase select", "select 1", OpSelect},
		{"insert with leading newline", "\nINSERT INTO user_token (a)\nVALUES (:a)\n", OpInsert},
		{"verb followed by newline", "SELECT\n*\nFROM workload", OpSelect},
		{"update with leading spaces", "   UPDATE workload SET x = 1", OpUpdate},
		{"delete", "DELETE FROM workload", OpDelete},
		{"cte", "WITH ranked AS (SELECT 1) SELECT * FROM ranked", OpWith},
		{"parenthesised select", "(SELECT 1)", OpSelect},
		{"unknown verb", "EXPLAIN ANALYZE SELECT 1", OpOther},
		{"empty", "", OpOther},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, SQLOperation(tt.query))
		})
	}
}

func TestObserveDBOperation(t *testing.T) {
	// Use a driver label unique to this test so the series stay isolated from
	// other tests sharing the default registry.
	const testDriver = "test_observe"

	ObserveDBOperation(testDriver, OpSelect, time.Now(), nil)
	assert.Equal(t, uint64(1), histogramCount(t, testDriver, OpSelect, statusSuccess))

	// An empty result set is a normal outcome and must not count as an error.
	ObserveDBOperation(testDriver, OpSelect, time.Now(), sql.ErrNoRows)
	assert.Equal(t, uint64(2), histogramCount(t, testDriver, OpSelect, statusSuccess))
	assert.Equal(t, 0.0, testutil.ToFloat64(dbErrors.WithLabelValues(testDriver, OpSelect, ErrKindNoRows)))

	ObserveDBOperation(testDriver, OpInsert, time.Now(), &pq.Error{Code: "25006"})
	assert.Equal(t, 1.0, testutil.ToFloat64(dbErrors.WithLabelValues(testDriver, OpInsert, ErrKindReadOnlyTransaction)))
	assert.Equal(t, uint64(1), histogramCount(t, testDriver, OpInsert, statusError))
}

// histogramCount reports how many observations landed on one series of
// dbOperationDuration.
func histogramCount(t *testing.T, labels ...string) uint64 {
	t.Helper()
	observer, err := dbOperationDuration.GetMetricWithLabelValues(labels...)
	require.NoError(t, err)
	var collected dto.Metric
	require.NoError(t, observer.(prometheus.Metric).Write(&collected))
	return collected.GetHistogram().GetSampleCount()
}

// The sqlx and GORM pools of one database are distinct connection pools and
// must be reported separately rather than one silently shadowing the other.
func TestRegisterDBPoolSeparatesDrivers(t *testing.T) {
	sqlxKey := PoolKey{Pool: "db-a:5432/safe", Driver: DriverSqlx}
	gormKey := PoolKey{Pool: "db-a:5432/safe", Driver: DriverGorm}
	t.Cleanup(func() {
		UnregisterDBPool(sqlxKey)
		UnregisterDBPool(gormKey)
	})

	RegisterDBPool(sqlxKey, func() sql.DBStats {
		return sql.DBStats{MaxOpenConnections: 100, OpenConnections: 7, InUse: 3, Idle: 4}
	})
	RegisterDBPool(gormKey, func() sql.DBStats {
		return sql.DBStats{OpenConnections: 5, InUse: 1, Idle: 4}
	})

	expected := `
# HELP safe_db_pool_connections Number of database connections in the pool, by state.
# TYPE safe_db_pool_connections gauge
safe_db_pool_connections{driver="gorm",pool="db-a:5432/safe",state="idle"} 4
safe_db_pool_connections{driver="gorm",pool="db-a:5432/safe",state="in_use"} 1
safe_db_pool_connections{driver="gorm",pool="db-a:5432/safe",state="open"} 5
safe_db_pool_connections{driver="sqlx",pool="db-a:5432/safe",state="idle"} 4
safe_db_pool_connections{driver="sqlx",pool="db-a:5432/safe",state="in_use"} 3
safe_db_pool_connections{driver="sqlx",pool="db-a:5432/safe",state="open"} 7
`
	assert.NoError(t, testutil.CollectAndCompare(poolCollector, strings.NewReader(expected), "safe_db_pool_connections"))
}

// Two clients against the same database name on different servers must not
// shadow each other, which is why the key carries the host and port.
func TestRegisterDBPoolDistinguishesHosts(t *testing.T) {
	first := PoolKey{Pool: "db-a:5432/safe", Driver: DriverSqlx}
	second := PoolKey{Pool: "db-b:5432/safe", Driver: DriverSqlx}
	t.Cleanup(func() {
		UnregisterDBPool(first)
		UnregisterDBPool(second)
	})

	RegisterDBPool(first, func() sql.DBStats { return sql.DBStats{OpenConnections: 1} })
	RegisterDBPool(second, func() sql.DBStats { return sql.DBStats{OpenConnections: 2} })

	assert.Equal(t, 6, testutil.CollectAndCount(poolCollector, "safe_db_pool_connections"))
}

// A closed pool otherwise keeps reporting a frozen set of zeroes forever.
func TestUnregisterDBPool(t *testing.T) {
	key := PoolKey{Pool: "db-gone:5432/safe", Driver: DriverSqlx}
	RegisterDBPool(key, func() sql.DBStats { return sql.DBStats{OpenConnections: 3} })
	require.Equal(t, 3, testutil.CollectAndCount(poolCollector, "safe_db_pool_connections"))

	UnregisterDBPool(key)
	assert.Equal(t, 0, testutil.CollectAndCount(poolCollector, "safe_db_pool_connections"))
}

// RegisterDBPool must not panic when handed a nil source.
func TestRegisterDBPoolNil(t *testing.T) {
	assert.NotPanics(t, func() {
		RegisterDBPool(PoolKey{Pool: "nil-pool", Driver: DriverSqlx}, nil)
	})
}
