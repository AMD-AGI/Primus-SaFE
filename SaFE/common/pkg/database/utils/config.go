/*
 * Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
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
	DBName         string        // Database name to connect to
	Username       string        // Database user name
	Password       string        // Database user password
	Host           string        // Database host address
	SSLMode        string        // SSL mode for the connection
	Port           int           // Database port
	MaxIdleConns   int           // Maximum number of idle connections in the pool
	MaxOpenConns   int           // Maximum number of open connections to the database
	MaxIdleTime    time.Duration // Maximum amount of time a connection may be idle
	MaxLifetime    time.Duration // Maximum amount of time a connection may be reused
	ConnectTimeout int           // Connection timeout in seconds
	RequestTimeout time.Duration // Request timeout for database operations
}

// SourceName generates a PostgreSQL connection string based on the database configuration.
// It formats the configuration parameters into a libpq-compatible connection string.
//
// Returns:
//   - string: Formatted connection string containing user, password, dbname, host, port, sslmode and connect_timeout
func (c *DBConfig) SourceName() string {
	return fmt.Sprintf("user=%s password=%s dbname=%s host=%s port=%d sslmode=%s connect_timeout=%d",
		c.Username, c.Password, c.DBName, c.Host, c.Port, c.SSLMode, c.ConnectTimeout)
}
