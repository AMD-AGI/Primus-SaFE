/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package utils

import (
	"database/sql"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// retiredByLifetime waits for the pool to close a connection because its
// lifetime elapsed, and reports whether it did.
//
// Polled rather than asserted once: database/sql retires an expired connection
// either when the next caller takes it out of the free list or when the cleaner
// goroutine comes round, so which of the two gets there first is not something a
// test should depend on.
//
// The ping's own error is discarded on purpose. It is what asks for a connection
// and so provokes the check, but the mock driver hands out one connection only --
// once the expired one is gone the next ping has nothing to take, and failing on
// that would fail the test for the very thing it is here to observe.
func retiredByLifetime(t *testing.T, db *sql.DB) bool {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if db.Stats().MaxLifetimeClosed > 0 {
			return true
		}
		_ = db.Ping()
		time.Sleep(10 * time.Millisecond)
	}
	return false
}

// TestPoolLimitsRetireAConnectionOnItsLifetime is the regression this file
// exists for.
//
// A connection is bound to the backend it was dialled to, and that backend can
// stop being the primary while the connection stays open. Reusing one then fails
// every write with SQLSTATE 25006 on a pool that reports itself healthy, and
// with no lifetime there is nothing that ever ends it: one such connection
// served a demoted replica for three days.
//
// Both directions are checked, because a test that only sets a lifetime would
// pass against a pool that ignores it.
func TestPoolLimitsRetireAConnectionOnItsLifetime(t *testing.T) {
	t.Run("a configured lifetime ends a connection", func(t *testing.T) {
		db, _, err := sqlmock.New()
		require.NoError(t, err)
		t.Cleanup(func() { _ = db.Close() })

		applyPoolLimits(db, &DBConfig{MaxLifetime: time.Millisecond, MaxIdleTime: time.Minute})
		require.NoError(t, db.Ping())

		assert.True(t, retiredByLifetime(t, db),
			"a pooled connection outlived MaxLifetime, so a failover leaves it serving the old backend")
	})

	t.Run("no lifetime keeps it forever, which is what was wrong", func(t *testing.T) {
		db, _, err := sqlmock.New()
		require.NoError(t, err)
		t.Cleanup(func() { _ = db.Close() })

		// The database/sql default, and what the GORM pool was left on.
		applyPoolLimits(db, &DBConfig{})
		require.NoError(t, db.Ping())
		time.Sleep(50 * time.Millisecond)
		require.NoError(t, db.Ping())

		assert.Zero(t, db.Stats().MaxLifetimeClosed,
			"nothing should retire a connection when no lifetime is configured")
	})
}

// TestPoolLimitsCarryTheConfiguredCeilings pins that the ceilings reach the
// pool, and that a zero is read as "leave the default" rather than as a limit of
// none -- MaxOpenConns of 0 means unlimited to database/sql, so passing it
// through would uncap a pool the caller meant to leave alone.
func TestPoolLimitsCarryTheConfiguredCeilings(t *testing.T) {
	db, _, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	applyPoolLimits(db, &DBConfig{MaxOpenConns: 7, MaxIdleConns: 3})
	assert.Equal(t, 7, db.Stats().MaxOpenConnections)

	applyPoolLimits(db, &DBConfig{})
	assert.Equal(t, 7, db.Stats().MaxOpenConnections,
		"a zero must not be forwarded as a limit")
}
