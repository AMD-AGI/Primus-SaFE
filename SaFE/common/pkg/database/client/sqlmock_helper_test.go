/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package client

import (
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/utils"
)

// newMockClient builds a Client whose sqlx DB is backed by go-sqlmock. Because
// getDB() returns db.Unsafe(), struct scans tolerate column mismatches, so mock
// rows only need the columns the test cares about.
func newMockClient(t *testing.T) (*Client, sqlmock.Sqlmock) {
	t.Helper()
	sqldb, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqldb.Close() })
	sqlxDB := sqlx.NewDb(sqldb, "postgres")

	// Some CRUD files use gorm instead of sqlx. Build a gorm DB over the same
	// mock connection. SkipDefaultTransaction avoids BEGIN/COMMIT expectations.
	gormDB, err := gorm.Open(postgres.New(postgres.Config{
		Conn:                 sqldb,
		PreferSimpleProtocol: true,
	}), &gorm.Config{SkipDefaultTransaction: true})
	require.NoError(t, err)

	c := &Client{db: sqlxDB, gorm: gormDB, DBConfig: &utils.DBConfig{}}
	return c, mock
}

// idRows is a convenience for a single-column "id" result set.
func idRows(id int64) *sqlmock.Rows {
	return sqlmock.NewRows([]string{"id"}).AddRow(id)
}

// newLooseMockClient builds a Client whose mock accepts queries/execs in any
// order and pre-arms a pool of generic query+exec expectations. Combined with
// db.Unsafe() (lenient struct scans) this lets a test just invoke a CRUD method
// and have its DB op satisfied regardless of whether it is a Query or Exec.
// Tests ignore the returned error because the goal is to exercise the method
// body, not to assert on a real database result.
func newLooseMockClient(t *testing.T) (*Client, sqlmock.Sqlmock) {
	c, mock := newMockClient(t)
	mock.MatchExpectationsInOrder(false)
	return c, mock
}

// arm registers n generic query and n generic exec expectations that match any
// statement, each usable once.
func arm(mock sqlmock.Sqlmock, n int) {
	for i := 0; i < n; i++ {
		mock.ExpectQuery(".").WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(1)))
		mock.ExpectExec(".").WillReturnResult(sqlmock.NewResult(1, 1))
	}
	// A few transaction expectations for methods that use advisory locks or
	// explicit transactions.
	for i := 0; i < 4; i++ {
		mock.ExpectBegin()
		mock.ExpectCommit()
		mock.ExpectRollback()
	}
}
