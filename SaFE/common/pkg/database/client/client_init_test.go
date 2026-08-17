/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package client

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/utils"
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
