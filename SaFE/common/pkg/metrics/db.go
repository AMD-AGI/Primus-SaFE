/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

// Package metrics defines the PostgreSQL access metrics shared by every SaFE
// service that talks to the database. They are registered on the
// controller-runtime registry so they are exposed on the existing /metrics
// endpoint and collected via pull.
package metrics

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"io"
	"net"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/lib/pq"
	"github.com/prometheus/client_golang/prometheus"
	"gorm.io/gorm"
	ctrlmetrics "sigs.k8s.io/controller-runtime/pkg/metrics"
)

// Driver names used as the "driver" label on the database metrics.
const (
	DriverSqlx = "sqlx"
	DriverGorm = "gorm"
)

// Error kinds reported on the "kind" label of safe_db_errors_total. They are
// deliberately coarse: each one maps to a distinct operational response.
const (
	// ErrKindNoRows is an empty result set. Reported so that it can be
	// excluded from alerting, since it is a normal outcome for many queries.
	ErrKindNoRows = "no_rows"
	// ErrKindReadOnlyTransaction means PostgreSQL refused a write because the
	// session is read-only, which is what a replica reports while it is being
	// promoted. A sustained rate of these means writes are failing cluster-wide.
	ErrKindReadOnlyTransaction = "read_only_transaction"
	// ErrKindUnavailable covers administrator shutdown and "cannot connect now",
	// i.e. the server is restarting or failing over.
	ErrKindUnavailable = "unavailable"
	// ErrKindTooManyConnections means the server rejected the connection because
	// its connection slots are exhausted.
	ErrKindTooManyConnections = "too_many_connections"
	// ErrKindInsufficientResources covers the remaining out-of-resource errors.
	ErrKindInsufficientResources = "insufficient_resources"
	// ErrKindConnection is a transport level failure: refused, reset, bad conn.
	ErrKindConnection = "connection"
	// ErrKindTimeout is a deadline exceeded on the caller side.
	ErrKindTimeout = "timeout"
	// ErrKindCanceled means the caller's context was canceled, usually because
	// the HTTP client that triggered the query went away.
	ErrKindCanceled = "canceled"
	// ErrKindConstraint is an integrity constraint violation (unique, fk, ...).
	ErrKindConstraint = "constraint"
	// ErrKindSerialization covers deadlocks and serialization failures.
	ErrKindSerialization = "serialization"
	// ErrKindInvalidTxState is an invalid transaction state other than read-only.
	ErrKindInvalidTxState = "invalid_transaction_state"
	// ErrKindSyntaxOrAccess is a malformed statement or a permission problem,
	// which almost always means a code or migration bug rather than an outage.
	ErrKindSyntaxOrAccess = "syntax_or_access"
	// ErrKindSystemError is a server side I/O error.
	ErrKindSystemError = "system_error"
	// ErrKindOther is anything not recognised above.
	ErrKindOther = "other"
)

// Operation values for the "operation" label. The SQL verb is used rather than
// the calling method so that the label stays low cardinality across the ~150
// database helpers in the client package.
const (
	OpSelect = "select"
	OpInsert = "insert"
	OpUpdate = "update"
	OpDelete = "delete"
	OpWith   = "with"
	OpOther  = "other"
)

const (
	statusSuccess = "success"
	statusError   = "error"
)

var (
	dbOperationDuration = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name: "safe_db_operation_duration_seconds",
		Help: "Latency of database operations, by driver, SQL verb and outcome.",
		// Reaches 30s because the client-side request timeout defaults to 20s
		// and connection acquisition can stall for longer during a failover.
		Buckets: []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10, 30},
	}, []string{"driver", "operation", "status"})

	dbErrors = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "safe_db_errors_total",
		Help: "Database operation failures, by driver, SQL verb and error kind.",
	}, []string{"driver", "operation", "kind"})
)

// ObserveDBOperation records the latency and outcome of a single database
// operation. A nil err counts as a success; sql.ErrNoRows is treated as a
// success as well because it is a normal result rather than a fault.
func ObserveDBOperation(driverName, operation string, start time.Time, err error) {
	kind := ClassifyDBError(err)
	status := statusSuccess
	if err != nil && kind != ErrKindNoRows {
		status = statusError
		dbErrors.WithLabelValues(driverName, operation, kind).Inc()
	}
	dbOperationDuration.WithLabelValues(driverName, operation, status).Observe(time.Since(start).Seconds())
}

// SQLOperation extracts the leading verb of a SQL statement so it can be used
// as a metric label. Unrecognised statements collapse to OpOther to keep the
// label bounded.
func SQLOperation(query string) string {
	statement := strings.TrimLeft(query, " \t\r\n(")
	end := strings.IndexFunc(statement, unicode.IsSpace)
	if end < 0 {
		end = len(statement)
	}
	switch strings.ToLower(statement[:end]) {
	case "select":
		return OpSelect
	case "insert":
		return OpInsert
	case "update":
		return OpUpdate
	case "delete":
		return OpDelete
	case "with":
		return OpWith
	default:
		return OpOther
	}
}

// ClassifyDBError maps an error returned by the database layer onto one of the
// ErrKind constants. It returns an empty string for a nil error.
func ClassifyDBError(err error) string {
	if err == nil {
		return ""
	}
	switch {
	case errors.Is(err, sql.ErrNoRows), errors.Is(err, gorm.ErrRecordNotFound):
		return ErrKindNoRows
	case errors.Is(err, context.DeadlineExceeded):
		return ErrKindTimeout
	case errors.Is(err, context.Canceled):
		return ErrKindCanceled
	case errors.Is(err, driver.ErrBadConn), errors.Is(err, io.EOF), errors.Is(err, io.ErrUnexpectedEOF):
		return ErrKindConnection
	}

	var pqErr *pq.Error
	if errors.As(err, &pqErr) {
		return classifyPQCode(pqErr.Code)
	}

	var netErr net.Error
	if errors.As(err, &netErr) {
		if netErr.Timeout() {
			return ErrKindTimeout
		}
		return ErrKindConnection
	}
	return ErrKindOther
}

// classifyPQCode maps a PostgreSQL SQLSTATE onto an error kind. Specific codes
// are checked before falling back to the two character class.
func classifyPQCode(code pq.ErrorCode) string {
	switch code {
	case "25006": // read_only_sql_transaction
		return ErrKindReadOnlyTransaction
	case "53300": // too_many_connections
		return ErrKindTooManyConnections
	case "57P01", "57P02", "57P03": // admin_shutdown, crash_shutdown, cannot_connect_now
		return ErrKindUnavailable
	}
	switch code.Class() {
	case "08": // connection_exception
		return ErrKindConnection
	case "23": // integrity_constraint_violation
		return ErrKindConstraint
	case "25": // invalid_transaction_state
		return ErrKindInvalidTxState
	case "40": // transaction_rollback, includes deadlock_detected
		return ErrKindSerialization
	case "42": // syntax_error_or_access_rule_violation
		return ErrKindSyntaxOrAccess
	case "53": // insufficient_resources
		return ErrKindInsufficientResources
	case "57": // operator_intervention
		return ErrKindUnavailable
	case "58": // system_error
		return ErrKindSystemError
	}
	return ErrKindOther
}

// PoolStatsFunc reports the current statistics of a connection pool.
type PoolStatsFunc func() sql.DBStats

// dbPoolCollector publishes sql.DBStats for every registered pool. A single
// collector serves all pools so that additional pools can be registered after
// the collector is already bound to the default registry.
type dbPoolCollector struct {
	mu    sync.RWMutex
	pools map[string]PoolStatsFunc

	connections     *prometheus.Desc
	maxOpen         *prometheus.Desc
	waitCount       *prometheus.Desc
	waitDuration    *prometheus.Desc
	closedMaxIdle   *prometheus.Desc
	closedMaxLifeTm *prometheus.Desc
}

var poolCollector = newDBPoolCollector()

func newDBPoolCollector() *dbPoolCollector {
	return &dbPoolCollector{
		pools: map[string]PoolStatsFunc{},
		connections: prometheus.NewDesc(
			"safe_db_pool_connections",
			"Number of database connections in the pool, by state.",
			[]string{"pool", "state"}, nil),
		maxOpen: prometheus.NewDesc(
			"safe_db_pool_max_open_connections",
			"Configured upper bound on open connections.",
			[]string{"pool"}, nil),
		waitCount: prometheus.NewDesc(
			"safe_db_pool_wait_count_total",
			"Total number of times a caller had to wait for a connection.",
			[]string{"pool"}, nil),
		waitDuration: prometheus.NewDesc(
			"safe_db_pool_wait_duration_seconds_total",
			"Total time callers spent waiting for a connection.",
			[]string{"pool"}, nil),
		closedMaxIdle: prometheus.NewDesc(
			"safe_db_pool_closed_max_idle_total",
			"Total connections closed because the idle limit was reached.",
			[]string{"pool"}, nil),
		closedMaxLifeTm: prometheus.NewDesc(
			"safe_db_pool_closed_max_lifetime_total",
			"Total connections closed because the lifetime limit was reached.",
			[]string{"pool"}, nil),
	}
}

func init() {
	ctrlmetrics.Registry.MustRegister(
		dbOperationDuration,
		dbErrors,
		poolCollector,
	)
}

// RegisterDBPool starts publishing pool statistics under the given pool name.
// Registering the same name twice replaces the previous source, which keeps the
// call idempotent for singleton clients.
func RegisterDBPool(pool string, stats PoolStatsFunc) {
	if stats == nil {
		return
	}
	poolCollector.mu.Lock()
	defer poolCollector.mu.Unlock()
	poolCollector.pools[pool] = stats
}

func (c *dbPoolCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.connections
	ch <- c.maxOpen
	ch <- c.waitCount
	ch <- c.waitDuration
	ch <- c.closedMaxIdle
	ch <- c.closedMaxLifeTm
}

func (c *dbPoolCollector) Collect(ch chan<- prometheus.Metric) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	for pool, stats := range c.pools {
		s := stats()
		ch <- prometheus.MustNewConstMetric(c.connections, prometheus.GaugeValue, float64(s.OpenConnections), pool, "open")
		ch <- prometheus.MustNewConstMetric(c.connections, prometheus.GaugeValue, float64(s.InUse), pool, "in_use")
		ch <- prometheus.MustNewConstMetric(c.connections, prometheus.GaugeValue, float64(s.Idle), pool, "idle")
		ch <- prometheus.MustNewConstMetric(c.maxOpen, prometheus.GaugeValue, float64(s.MaxOpenConnections), pool)
		ch <- prometheus.MustNewConstMetric(c.waitCount, prometheus.CounterValue, float64(s.WaitCount), pool)
		ch <- prometheus.MustNewConstMetric(c.waitDuration, prometheus.CounterValue, s.WaitDuration.Seconds(), pool)
		ch <- prometheus.MustNewConstMetric(c.closedMaxIdle, prometheus.CounterValue, float64(s.MaxIdleClosed), pool)
		ch <- prometheus.MustNewConstMetric(c.closedMaxLifeTm, prometheus.CounterValue, float64(s.MaxLifetimeClosed), pool)
	}
}
