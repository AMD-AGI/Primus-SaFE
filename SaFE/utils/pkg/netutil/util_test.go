/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
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

func TestGetLocalIp(t *testing.T) {
	ip, err := GetLocalIp()
	// Result depends on the host network; only validate consistency.
	if err == nil {
		assert.Assert(t, ip != "")
	}
}

func TestConvertIpToInt(t *testing.T) {
	assert.Equal(t, ConvertIpToInt("0.0.0.1"), 1)
	assert.Equal(t, ConvertIpToInt("1.0.0.0"), 1<<24)
	assert.Equal(t, ConvertIpToInt("invalid"), 0)
}

func TestGetSecondLevelDomainSpecial(t *testing.T) {
	// hostname falls back to the raw uri when it cannot be parsed as a URL
	assert.Equal(t, GetSecondLevelDomain("localhost"), "localhost")
	assert.Equal(t, GetSecondLevelDomain("127.0.0.1"), "127.0.0.1")
	// two-part domains are returned as-is
	assert.Equal(t, GetSecondLevelDomain("example.com"), "example.com")
}

func TestGetHostnameInvalid(t *testing.T) {
	// an unparseable URL yields an empty hostname
	assert.Equal(t, GetHostname("http://[::1"), "")
}

func TestGetSchemeHostInvalid(t *testing.T) {
	// missing scheme and unparseable URL both yield an empty result
	assert.Equal(t, GetSchemeHost("not-a-url"), "")
	assert.Equal(t, GetSchemeHost("http://[::1"), "")
}
