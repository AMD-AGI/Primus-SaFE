/*
 * Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package utils

import (
	"fmt"
	"time"
)

type DBConfig struct {
	DBName         string
	Username       string
	Password       string
	Host           string
	SSLMode        string
	Port           int
	MaxIdleConns   int
	MaxOpenConns   int
	MaxIdleTime    time.Duration
	MaxLifetime    time.Duration
	ConnectTimeout int
}

func (c *DBConfig) SourceName() string {
	return fmt.Sprintf("user=%s password=%s dbname=%s host=%s port=%d sslmode=%s connect_timeout=%d",
		c.Username, c.Password, c.DBName, c.Host, c.Port, c.SSLMode, c.ConnectTimeout)
}
