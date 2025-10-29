/*
 * Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package utils

import (
	"database/sql"
	"fmt"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/schema"

	sqrl "github.com/Masterminds/squirrel"
	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"

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
//
// Parameters:
//   - cfg: Database configuration containing connection details
//   - driverName: Database driver to use (e.g., postgres)
//
// Returns:
//   - *sqlx.DB: Database connection pool
//   - error: Connection error if any
func Connect(cfg *DBConfig, driverName DBDriver) (*sqlx.DB, error) {
	dataSource := cfg.SourceName()
	db, err := sqlx.Connect(string(driverName), dataSource)
	if err != nil {
		return nil, fmt.Errorf("failed to connect db %s, err: %v", cfg.DBName, err)
	}
	if cfg.MaxOpenConns > 0 {
		db.SetMaxOpenConns(cfg.MaxOpenConns)
	}
	if cfg.MaxIdleConns > 0 {
		db.SetMaxIdleConns(cfg.MaxIdleConns)
	}
	db.SetConnMaxIdleTime(cfg.MaxIdleTime)
	db.SetConnMaxLifetime(cfg.MaxLifetime)
	return db, nil
}

// ConnectGorm establishes a connection to the database using GORM ORM.
// It configures PostgreSQL connection with specific GORM settings including naming strategy
// and various ORM features configuration.
//
// Parameters:
//   - cfg: Database configuration containing connection details
//
// Returns:
//   - *gorm.DB: GORM database instance
//   - error: Connection error if any
func ConnectGorm(cfg *DBConfig) (*gorm.DB, error) {
	// init gorm
	dsn := fmt.Sprintf("host=%s port=%v user=%s dbname=%s password=%s sslmode=%s",
		cfg.Host, cfg.Port, cfg.Username, cfg.DBName, cfg.Password, cfg.SSLMode)
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
	return gormDB, nil
}

// ParseNullString converts a sql.NullString to a regular string.
// If the NullString is valid, it returns the string value, otherwise returns an empty string.
//
// Parameters:
//   - str: sql.NullString to parse
//
// Returns:
//   - string: String value or empty string if null
func ParseNullString(str sql.NullString) string {
	if str.Valid {
		return str.String
	}
	return ""
}

// ParseNullTimeToString converts a pq.NullTime to a formatted time string.
// If the NullTime is valid and not zero, it returns RFC3339 formatted time string, otherwise returns empty string.
//
// Parameters:
//   - t: pq.NullTime to parse
//
// Returns:
//   - string: RFC3339 formatted time string or empty strin
func ParseNullTimeToString(t pq.NullTime) string {
	if t.Valid && !t.Time.IsZero() {
		return timeutil.FormatRFC3339(t.Time)
	}
	return ""
}

// ParseNullTime converts a pq.NullTime to a time.Time.
// If the NullTime is valid, it returns the time value, otherwise returns zero time.
//
// Parameters:
//   - t: pq.NullTime to parse
//
// Returns:
//   - time.Time: Time value or zero time if null
func ParseNullTime(t pq.NullTime) time.Time {
	if t.Valid {
		return t.Time
	}
	return time.Time{}
}

// NullString converts a string to sql.NullString.
// If the string is empty, it returns an invalid NullString, otherwise returns a valid NullString with the value.
//
// Parameters:
//   - str: String to convert
//
// Returns:
//   - sql.NullString: NullString representation of the inpu
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
// If the time is zero, it returns an invalid NullTime, otherwise returns a valid NullTime with the value.
//
// Parameters:
//   - t: Time to convert
//
// Returns:
//   - pq.NullTime: NullTime representation of the input
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
// If the time is zero or nil, it returns an invalid NullTime, otherwise returns a valid NullTime with the value.
//
// Parameters:
//   - t: metav1.Time pointer to convert
//
// Returns:
//   - pq.NullTime: NullTime representation of the input
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

// CvtToSqlStr converts a Squirrel SQL query to a string representation including the SQL and arguments.
// Useful for debugging SQL queries.
//
// Parameters:
//   - sql: Squirrel SQL query to convert
//
// Returns:
//   - string: String representation of the SQL query and its argument
func CvtToSqlStr(sql sqrl.Sqlizer) string {
	sqlStr, args, err := sql.ToSql()
	if err != nil {
		klog.Errorf("failed to convert sql, err: %v", err)
		return ""
	}
	return sqlStr + " " + string(jsonutils.MarshalSilently(args))
}
