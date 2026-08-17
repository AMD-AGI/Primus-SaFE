/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package client

import (
	"net"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/jmoiron/sqlx"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	commonconfig "github.com/AMD-AIG-AIMA/SAFE/common/pkg/config"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/utils"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/metrics"
)

// TestBuildClientReportsWhyItCouldNotBuild covers the failure path a caller acts
// on. A nil client is a contract here -- the audit middleware degrades to a
// passthrough on one, the email relay reports it -- so what matters is that the
// reason reaches the log rather than being swallowed on the way out.
func TestBuildClientReportsWhyItCouldNotBuild(t *testing.T) {
	client, err := buildClient(&utils.DBConfig{})

	assert.Nil(t, client)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "check db params")
	for _, missing := range []string{"dbname", "username", "host"} {
		assert.Contains(t, err.Error(), missing,
			"the error has to name what is missing, or a nil client explains nothing")
	}
}

// TestNewClientRetriesAfterAFailure is the regression this rewrite exists for.
//
// The singleton was built with sync.Once, which is spent whether the body
// succeeded or not: one failed attempt left the instance nil for the life of the
// process, so a blip while the cluster elected a primary cost a deployment its
// database client until somebody restarted the pod. A second call has to be a
// second attempt.
//
// Asserted through the failure path because the success path needs a database.
// Reaching buildClient twice is the property under test; that both attempts fail
// here is incidental.
func TestNewClientRetriesAfterAFailure(t *testing.T) {
	instanceMu.Lock()
	instance = nil
	instanceMu.Unlock()

	attempts := 0
	t.Cleanup(func() {
		instanceMu.Lock()
		instance = nil
		instanceMu.Unlock()
	})

	// Stand in for the config reader so the test does not depend on viper state,
	// and count how many times a call reaches it.
	restore := configuredDB
	t.Cleanup(func() { configuredDB = restore })
	configuredDB = func() *utils.DBConfig {
		attempts++
		return &utils.DBConfig{}
	}

	assert.Nil(t, NewClient())
	assert.Nil(t, NewClient())
	assert.Equal(t, 2, attempts,
		"a failed init must not settle the question: the next call has to try again")
}

// TestNewClientCachesASuccess keeps the other half of the contract. Retrying is
// only correct while there is nothing to return; once a client exists it is the
// singleton, and rebuilding it would open a second pair of pools.
func TestNewClientCachesASuccess(t *testing.T) {
	instanceMu.Lock()
	instance = &Client{DBConfig: &utils.DBConfig{DBName: "already-built"}}
	instanceMu.Unlock()
	t.Cleanup(func() {
		instanceMu.Lock()
		instance = nil
		instanceMu.Unlock()
	})

	calls := 0
	restore := configuredDB
	t.Cleanup(func() { configuredDB = restore })
	configuredDB = func() *utils.DBConfig {
		calls++
		return &utils.DBConfig{}
	}

	require.NotNil(t, NewClient())
	assert.Equal(t, "already-built", NewClient().DBName)
	assert.Zero(t, calls, "an existing client must be returned, not rebuilt")
}

// TestDiscardPoolStopsItReporting covers the half of a failed init that is
// invisible at runtime.
//
// Connect registers a metrics collector against the pool it opened. Closing the
// pool without unregistering leaves the collector reading a closed handle, which
// publishes a frozen set of zeroes for the life of the process -- on a dashboard
// that is indistinguishable from a pool that is simply idle, so the one signal
// that would say "this client never came up" reads as healthy.
func TestDiscardPoolStopsItReporting(t *testing.T) {
	sqldb, _, err := sqlmock.New()
	require.NoError(t, err)
	db := sqlx.NewDb(sqldb, "postgres")

	cfg := &utils.DBConfig{Host: "discard-test", Port: 5432, DBName: "safe"}
	labels := map[string]string{"pool": "discard-test:5432/safe", "driver": "sqlx", "state": "open"}
	metrics.RegisterDBPool(utils.SqlxPoolKey(cfg), db.Stats)
	t.Cleanup(func() { metrics.UnregisterDBPool(utils.SqlxPoolKey(cfg)) })
	require.NotNil(t, findMetric(t, "safe_db_pool_connections", labels),
		"precondition: the pool should be reporting before it is discarded")

	discardPool(db, cfg)

	assert.Nil(t, findMetric(t, "safe_db_pool_connections", labels),
		"a discarded pool must stop reporting, or its zeroes look like an idle pool")
	assert.Error(t, db.Ping(), "the pool should be closed as well as unregistered")
}

// TestConfiguredDBReadsEachSettingIntoItsOwnField guards a mapping that nothing
// else would catch. Thirteen settings are copied into one struct, and the two
// durations are the pair worth pinning: swapping the lifetime and the idle time
// would compile, pass every other test, and quietly restore the unbounded reuse
// this branch exists to end.
func TestConfiguredDBReadsEachSettingIntoItsOwnField(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)
	commonconfig.SetValue("db.max_life_time_second", "111")
	commonconfig.SetValue("db.max_idle_time_second", "222")
	commonconfig.SetValue("db.ssl_mode", "disable")
	commonconfig.SetValue("db.target_session_attrs", "")

	cfg := configuredDB()

	assert.Equal(t, 111*time.Second, cfg.MaxLifetime)
	assert.Equal(t, 222*time.Second, cfg.MaxIdleTime)
	assert.Equal(t, "disable", cfg.SSLMode)
	assert.Equal(t, "", cfg.TargetSessionAttrs, "an explicit empty must reach the DSN builder")
}

// TestBuildClientReportsAnUnreachableServer covers the connect path without a
// server: a config that passes validation but points at a closed port fails in
// utils.Connect, which is the branch a database that is down takes.
func TestBuildClientReportsAnUnreachableServer(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	port := listener.Addr().(*net.TCPAddr).Port
	require.NoError(t, listener.Close()) // nothing is listening there now

	client, err := buildClient(&utils.DBConfig{
		DBName: "safe", Username: "u", Password: "p", Host: "127.0.0.1",
		Port: port, SSLMode: "disable", ConnectTimeout: 1,
	})

	assert.Nil(t, client)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "connect db")
}

// TestNewClientWithConfigRefusesAConfigItCannotUse keeps the non-singleton
// constructor's contract: it reports rather than exits, and it reports the same
// reason the singleton logs, because both now take one build path.
func TestNewClientWithConfigRefusesAConfigItCannotUse(t *testing.T) {
	client, err := NewClientWithConfig(nil)
	assert.Nil(t, client)
	require.Error(t, err)

	client, err = NewClientWithConfig(&utils.DBConfig{})
	assert.Nil(t, client)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "check db params")
}
