/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package client

import (
	"context"
	"database/sql"
	"time"

	"github.com/jmoiron/sqlx"

	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/metrics"
)

// instrumentedDB wraps a sqlx handle and records Prometheus metrics for the
// query methods the client package uses. Everything else is promoted from the
// embedded *sqlx.DB unchanged.
//
// Statements issued inside a transaction obtained from BeginTxx are not
// measured individually; the GORM path and the direct query methods below cover
// every other database call in this package.
type instrumentedDB struct {
	*sqlx.DB
}

func (db *instrumentedDB) SelectContext(ctx context.Context, dest interface{}, query string, args ...interface{}) error {
	start := time.Now()
	err := db.DB.SelectContext(ctx, dest, query, args...)
	metrics.ObserveDBOperation(metrics.DriverSqlx, metrics.SQLOperation(query), start, err)
	return err
}

func (db *instrumentedDB) GetContext(ctx context.Context, dest interface{}, query string, args ...interface{}) error {
	start := time.Now()
	err := db.DB.GetContext(ctx, dest, query, args...)
	metrics.ObserveDBOperation(metrics.DriverSqlx, metrics.SQLOperation(query), start, err)
	return err
}

func (db *instrumentedDB) ExecContext(ctx context.Context, query string, args ...interface{}) (sql.Result, error) {
	start := time.Now()
	res, err := db.DB.ExecContext(ctx, query, args...)
	metrics.ObserveDBOperation(metrics.DriverSqlx, metrics.SQLOperation(query), start, err)
	return res, err
}

func (db *instrumentedDB) NamedExecContext(ctx context.Context, query string, arg interface{}) (sql.Result, error) {
	start := time.Now()
	res, err := db.DB.NamedExecContext(ctx, query, arg)
	metrics.ObserveDBOperation(metrics.DriverSqlx, metrics.SQLOperation(query), start, err)
	return res, err
}

func (db *instrumentedDB) NamedQueryContext(ctx context.Context, query string, arg interface{}) (*sqlx.Rows, error) {
	start := time.Now()
	rows, err := db.DB.NamedQueryContext(ctx, query, arg)
	metrics.ObserveDBOperation(metrics.DriverSqlx, metrics.SQLOperation(query), start, err)
	return rows, err
}
