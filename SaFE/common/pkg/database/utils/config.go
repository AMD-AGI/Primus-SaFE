/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package utils

import (
	"fmt"
	"time"
)

// DBConfig represents the database configuration parameters for connecting to a database.
// It contains all necessary connection details and connection pool settings.
type DBConfig struct {
	DBName   string // Database name to connect to
	Username string // Database username
	Password string // Database user password
	Host     string // Database host address
	SSLMode  string // SSL mode for the connection
	// TargetSessionAttrs constrains the session the driver will accept, e.g.
	// "read-write" to refuse one that cannot write. Empty accepts any.
	TargetSessionAttrs string
	Port               int           // Database port
	MaxIdleConns       int           // Maximum number of idle connections in the pool
	MaxOpenConns       int           // Maximum number of open connections to the database
	MaxIdleTime        time.Duration // Maximum amount of time a connection may be idle
	MaxLifetime        time.Duration // Maximum amount of time a connection may be reused
	ConnectTimeout     int           // Connection timeout in seconds
	RequestTimeout     time.Duration // Request timeout for database operations
}

// SourceName generates a PostgreSQL connection string based on the database configuration.
// It formats the configuration parameters into a libpq-compatible connection string.
// Returns:
//   - string: Formatted connection string containing user, password, dbname, host, port, sslmode and connect_timeout
func (c *DBConfig) SourceName() string {
	return fmt.Sprintf("user=%s password=%s dbname=%s host=%s port=%d sslmode=%s connect_timeout=%d%s",
		c.Username, c.Password, c.DBName, c.Host, c.Port, c.SSLMode, c.ConnectTimeout,
		targetSessionAttrsParam(c.TargetSessionAttrs))
}

// GormSourceName generates the connection string GORM's postgres dialector is
// opened with.
//
// Beside SourceName rather than inline at the call site, and for the reason this
// file's own history gives: two things built from one config, in two places, is
// how the pool limits came to differ between the two pools. The keyword order
// differs from SourceName's because that is what each was written with and the
// drivers do not care; what has to agree is which parameters appear.
func (c *DBConfig) GormSourceName() string {
	return fmt.Sprintf("host=%s port=%v user=%s dbname=%s password=%s sslmode=%s%s",
		c.Host, c.Port, c.Username, c.DBName, c.Password, c.SSLMode,
		targetSessionAttrsParam(c.TargetSessionAttrs))
}

// targetSessionAttrsParam renders the parameter, or nothing when unset.
//
// Omitted rather than passed empty: both drivers read an explicit empty value as
// a setting, and libpq rejects it, so an unset field has to leave the keyword out
// of the string entirely.
func targetSessionAttrsParam(attrs string) string {
	if attrs == "" {
		return ""
	}
	return " target_session_attrs=" + attrs
}
