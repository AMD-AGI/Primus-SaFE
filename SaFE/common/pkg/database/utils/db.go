/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package utils

import (
	"database/sql"
	"fmt"
	"time"

	sqrl "github.com/Masterminds/squirrel"
	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/schema"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"

	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/metrics"
	jsonutils "github.com/AMD-AIG-AIMA/SAFE/utils/pkg/json"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/timeutil"
)

// DBDriver represents the type of database driver to use
type DBDriver string

const (
	// PgDriver represents the PostgreSQL database driver
	PgDriver DBDriver = "postgres"
)

// Connect establishes a connection to the database using the provided configuration and driver.
// It creates a sqlx.DB connection pool with configurable connection limits and lifetimes.
// Parameters:
//   - cfg: Database configuration containing connection details
//   - driverName: Database driver to use (e.g., postgres)
//
// Returns:
//   - *sqlx.DB: Database connection pool
//
// - error: Connection error if any.
func Connect(cfg *DBConfig, driverName DBDriver) (*sqlx.DB, error) {
	dataSource := cfg.SourceName()
	db, err := sqlx.Connect(string(driverName), dataSource)
	if err != nil {
		return nil, fmt.Errorf("failed to connect db %s, err: %v", cfg.DBName, err)
	}
	applyPoolLimits(db.DB, cfg)
	metrics.RegisterDBPool(SqlxPoolKey(cfg), db.Stats)
	return db, nil
}

// poolName identifies the server and database a configuration points at, for
// use as a metric label. It carries no credentials.
func poolName(cfg *DBConfig) string {
	return fmt.Sprintf("%s:%d/%s", cfg.Host, cfg.Port, cfg.DBName)
}

// SqlxPoolKey identifies the sqlx pool of a configuration in the pool metrics.
func SqlxPoolKey(cfg *DBConfig) metrics.PoolKey {
	return metrics.PoolKey{Pool: poolName(cfg), Driver: metrics.DriverSqlx}
}

// GormPoolKey identifies the GORM pool of a configuration in the pool metrics.
// GORM opens its own pool, separate from the sqlx one.
func GormPoolKey(cfg *DBConfig) metrics.PoolKey {
	return metrics.PoolKey{Pool: poolName(cfg), Driver: metrics.DriverGorm}
}

// ConnectGorm establishes a connection to the database using GORM ORM.
// It configures PostgreSQL connection with specific GORM settings including naming strategy
// and various ORM features configuration.
// Parameters:
//   - cfg: Database configuration containing connection details
//
// Returns:
//   - *gorm.DB: GORM database instance
//   - error: Connection error if any
func ConnectGorm(cfg *DBConfig) (*gorm.DB, error) {
	// init gorm
	dsn := fmt.Sprintf("host=%s port=%v user=%s dbname=%s password=%s sslmode=%s%s",
		cfg.Host, cfg.Port, cfg.Username, cfg.DBName, cfg.Password, cfg.SSLMode,
		targetSessionAttrsParam(cfg.TargetSessionAttrs))
	dialector := postgres.Dialector{
		Config: &postgres.Config{
			DSN: dsn,
		},
	}
	gormDB, err := gorm.Open(dialector, &gorm.Config{
		NamingStrategy: schema.NamingStrategy{
			SingularTable: true,
		},
		FullSaveAssociations:                     false,
		PrepareStmt:                              false,
		DisableAutomaticPing:                     false,
		DisableForeignKeyConstraintWhenMigrating: false,
		DisableNestedTransaction:                 false,
		AllowGlobalUpdate:                        false,
		QueryFields:                              false,
		Plugins:                                  nil,
	})
	if err != nil {
		return nil, err
	}
	if err = gormDB.Use(gormMetricsPlugin{}); err != nil {
		return nil, err
	}
	// GORM opens a pool of its own rather than sharing the sqlx one, so it needs
	// both the same limits and its own metrics.
	sqlDB, err := gormDB.DB()
	if err != nil {
		return nil, err
	}
	applyPoolLimits(sqlDB, cfg)
	metrics.RegisterDBPool(GormPoolKey(cfg), sqlDB.Stats)
	return gormDB, nil
}

// applyPoolLimits bounds how long a pooled connection may be reused.
//
// The lifetime is the part that matters beyond tuning. A connection is bound to
// the backend it was dialled to, and on a Postgres cluster that backend can stop
// being the primary while the connection stays open: the address the pool
// resolves now points elsewhere, but an established connection does not move.
// Reusing one then fails every write with SQLSTATE 25006, "cannot execute INSERT
// in a read-only transaction", on a pool that reports itself healthy.
//
// database/sql leaves both timers at zero, meaning a connection is reusable for
// as long as the process lives. That is how a pool kept serving a demoted
// replica for three days after a failover, while the sqlx pool in the same
// process -- which has always set these -- rotated onto the new primary within
// its lifetime. Applied here rather than at each call site so a second pool
// cannot be opened without them.
func applyPoolLimits(sqlDB *sql.DB, cfg *DBConfig) {
	if cfg.MaxOpenConns > 0 {
		sqlDB.SetMaxOpenConns(cfg.MaxOpenConns)
	}
	if cfg.MaxIdleConns > 0 {
		sqlDB.SetMaxIdleConns(cfg.MaxIdleConns)
	}
	sqlDB.SetConnMaxIdleTime(cfg.MaxIdleTime)
	sqlDB.SetConnMaxLifetime(cfg.MaxLifetime)
}

// ParseNullString parses the input data.
func ParseNullString(str sql.NullString) string {
	if str.Valid {
		return str.String
	}
	return ""
}

// ParseNullTimeToString parses the input data.
func ParseNullTimeToString(t pq.NullTime) string {
	if t.Valid && !t.Time.IsZero() {
		return timeutil.FormatRFC3339(t.Time)
	}
	return ""
}

// ParseNullTime parses the input data.
func ParseNullTime(t pq.NullTime) time.Time {
	if t.Valid {
		return t.Time
	}
	return time.Time{}
}

// NullString converts a string to sql.NullString.
func NullString(str string) sql.NullString {
	if str == "" {
		return sql.NullString{
			Valid: false,
		}
	}
	return sql.NullString{
		String: str,
		Valid:  true,
	}
}

// NullTime converts a time.Time to pq.NullTime.
func NullTime(t time.Time) pq.NullTime {
	if t.IsZero() {
		return pq.NullTime{
			Valid: false,
		}
	}
	return pq.NullTime{
		Time:  t,
		Valid: true,
	}
}

// NullMetaV1Time converts a metav1.Time pointer to pq.NullTime.
func NullMetaV1Time(t *metav1.Time) pq.NullTime {
	if t.IsZero() {
		return pq.NullTime{
			Valid: false,
		}
	}
	return pq.NullTime{
		Time:  t.Time,
		Valid: true,
	}
}

// CvtToSqlStr converts data to the target format.
func CvtToSqlStr(sql sqrl.Sqlizer) string {
	sqlStr, args, err := sql.ToSql()
	if err != nil {
		klog.Errorf("failed to convert sql, err: %v", err)
		return ""
	}
	return sqlStr + " " + string(jsonutils.MarshalSilently(args))
}
