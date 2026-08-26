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

// BeginTxx returns an instrumented transaction so that batched writes are
// measured too. They are the first thing to fail when the server turns
// read-only, so they must not be a blind spot.
func (db *instrumentedDB) BeginTxx(ctx context.Context, opts *sql.TxOptions) (*instrumentedTx, error) {
	tx, err := db.DB.BeginTxx(ctx, opts)
	if err != nil {
		return nil, err
	}
	return &instrumentedTx{Tx: tx}, nil
}

// instrumentedTx wraps a sqlx transaction and reports the statements issued
// inside it, plus the commit. Rollback is deliberately left uninstrumented:
// callers defer it unconditionally, so it reports sql.ErrTxDone on every
// transaction that already committed.
type instrumentedTx struct {
	*sqlx.Tx
}

func (tx *instrumentedTx) SelectContext(ctx context.Context, dest interface{}, query string, args ...interface{}) error {
	start := time.Now()
	err := tx.Tx.SelectContext(ctx, dest, query, args...)
	metrics.ObserveDBOperation(metrics.DriverSqlx, metrics.SQLOperation(query), start, err)
	return err
}

func (tx *instrumentedTx) GetContext(ctx context.Context, dest interface{}, query string, args ...interface{}) error {
	start := time.Now()
	err := tx.Tx.GetContext(ctx, dest, query, args...)
	metrics.ObserveDBOperation(metrics.DriverSqlx, metrics.SQLOperation(query), start, err)
	return err
}

func (tx *instrumentedTx) ExecContext(ctx context.Context, query string, args ...interface{}) (sql.Result, error) {
	start := time.Now()
	res, err := tx.Tx.ExecContext(ctx, query, args...)
	metrics.ObserveDBOperation(metrics.DriverSqlx, metrics.SQLOperation(query), start, err)
	return res, err
}

func (tx *instrumentedTx) NamedExecContext(ctx context.Context, query string, arg interface{}) (sql.Result, error) {
	start := time.Now()
	res, err := tx.Tx.NamedExecContext(ctx, query, arg)
	metrics.ObserveDBOperation(metrics.DriverSqlx, metrics.SQLOperation(query), start, err)
	return res, err
}

func (tx *instrumentedTx) Commit() error {
	start := time.Now()
	err := tx.Tx.Commit()
	metrics.ObserveDBOperation(metrics.DriverSqlx, metrics.OpCommit, start, err)
	return err
}
