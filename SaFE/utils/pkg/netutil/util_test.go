/*
 * Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package netutil

import (
	"testing"

	"gotest.tools/assert"
)

func TestGetSecondLevelDomain(t *testing.T) {
	host := "https://primus-safe.amd.primus.ai"
	domain := GetSecondLevelDomain(host)
	assert.Equal(t, domain, "primus.ai")

	host = "primus-safe.amd.primus.ai"
	domain = GetSecondLevelDomain(host)
	assert.Equal(t, domain, "primus.ai")

	host = "apiserver.tas.primus.ai"
	domain = GetSecondLevelDomain(host)
	assert.Equal(t, domain, "primus.ai")
}

func TestGetHostname(t *testing.T) {
	uri := "http://primus-safe.amd.primus.ai/login"
	homepage := GetHostname(uri)
	assert.Equal(t, homepage, "primus-safe.amd.primus.ai")

	uri = "http://localhost:5173/login"
	homepage = GetHostname(uri)
	assert.Equal(t, homepage, "localhost")
}

func TestGetSchemeHost(t *testing.T) {
	uri := "https://primus-safe.amd.primus.ai/login"
	homepage := GetSchemeHost(uri)
	assert.Equal(t, homepage, "https://primus-safe.amd.primus.ai")

	uri = "http://localhost:5173/login"
	homepage = GetSchemeHost(uri)
	assert.Equal(t, homepage, "http://localhost:5173")
}
