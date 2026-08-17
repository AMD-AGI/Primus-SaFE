/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package utils

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func dsnConfig(attrs string) *DBConfig {
	return &DBConfig{
		Username: "u", Password: "p", DBName: "d", Host: "h", Port: 5432,
		SSLMode: "require", ConnectTimeout: 5, TargetSessionAttrs: attrs,
	}
}

// TestSourceNameAsksForAWritableSession pins the parameter that keeps a
// connection off a demoted replica.
//
// The address in the DB secret follows the primary, so a new connection normally
// reaches it -- but "normally" is doing the work there. Between a failover and
// the address catching up, a connection lands on a node that answers, accepts the
// session, and then refuses every write with SQLSTATE 25006. Asking for a
// writable session moves that from a 500 the caller sees to a connect the driver
// declines and the pool retries.
func TestSourceNameAsksForAWritableSession(t *testing.T) {
	assert.Contains(t, dsnConfig("read-write").SourceName(), "target_session_attrs=read-write")
}

// TestBothDSNsCarryTheSameSessionAttr is the one that guards against a repeat of
// the bug this branch fixes.
//
// That bug was two things built from one config, in two places, which came to
// disagree -- the sqlx pool had a connection lifetime and the GORM pool did not.
// The two DSNs are the same shape of risk, so the parameter is asserted on both
// rather than on the one that happens to be a method.
func TestBothDSNsCarryTheSameSessionAttr(t *testing.T) {
	cfg := dsnConfig("read-write")
	assert.Contains(t, cfg.SourceName(), "target_session_attrs=read-write")
	assert.Contains(t, cfg.GormSourceName(), "target_session_attrs=read-write")

	unset := dsnConfig("")
	assert.NotContains(t, unset.SourceName(), "target_session_attrs")
	assert.NotContains(t, unset.GormSourceName(), "target_session_attrs")
}

// TestGormSourceNameCarriesTheConnectionItNeeds pins the parameters, not their
// order: the two DSNs were written with different keyword orders and the drivers
// do not care, but a parameter going missing from one of them is the asymmetry
// that has already cost a three-day outage once.
func TestGormSourceNameCarriesTheConnectionItNeeds(t *testing.T) {
	dsn := dsnConfig("read-write").GormSourceName()
	for _, want := range []string{
		"host=h", "port=5432", "user=u", "dbname=d", "password=p", "sslmode=require",
	} {
		assert.Contains(t, dsn, want)
	}
}

// TestSourceNameOmitsAnUnsetSessionAttr is the half that matters for the escape
// hatch. Both drivers read an explicit empty value as a setting and libpq
// rejects it, so clearing the config has to leave the keyword out rather than
// pass nothing to it -- otherwise the way out of this constraint is itself a
// connection failure.
func TestSourceNameOmitsAnUnsetSessionAttr(t *testing.T) {
	dsn := dsnConfig("").SourceName()
	assert.NotContains(t, dsn, "target_session_attrs")
	assert.Equal(t,
		"user=u password=p dbname=d host=h port=5432 sslmode=require connect_timeout=5",
		dsn, "an unset attr must leave the string exactly as it was before")
}
