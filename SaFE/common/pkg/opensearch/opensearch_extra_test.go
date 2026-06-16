/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package opensearch

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/robustclient"
)

func newSearchClientTo(t *testing.T, handler http.HandlerFunc) (*SearchClient, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(handler)
	rc := robustclient.NewClient(robustclient.DefaultClientConfig())
	rc.RegisterCluster("c1", srv.URL)
	return NewClient(SearchClientConfig{DefaultIndex: "node-"}, rc.ForCluster("c1")), srv
}

func TestRequestNotInitialized(t *testing.T) {
	sc := NewClient(SearchClientConfig{}, nil)
	_, err := sc.Request("/x", "GET", nil)
	assert.Error(t, err)
}

func TestRequestSuccessAndErrors(t *testing.T) {
	// success envelope
	sc, srv := newSearchClientTo(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(`{"meta":{"code":2000},"data":{"status_code":200,"body":{"hits":{"hits":[]}}}}`))
	})
	defer srv.Close()
	out, err := sc.Request("api", "GET", []byte(`{}`))
	assert.NoError(t, err)
	assert.Contains(t, string(out), "hits")

	// business error code
	sc2, srv2 := newSearchClientTo(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(`{"meta":{"code":4001,"message":"bad"},"data":{}}`))
	})
	defer srv2.Close()
	_, err = sc2.Request("/api", "GET", nil)
	assert.Error(t, err)

	// opensearch status >= 400
	sc3, srv3 := newSearchClientTo(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(`{"meta":{"code":2000},"data":{"status_code":404,"body":{}}}`))
	})
	defer srv3.Close()
	_, err = sc3.Request("/api", "GET", nil)
	assert.Error(t, err)

	// malformed json
	sc4, srv4 := newSearchClientTo(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(`not-json`))
	})
	defer srv4.Close()
	_, err = sc4.Request("/api", "GET", nil)
	assert.Error(t, err)
}

func TestSearchByTimeRange(t *testing.T) {
	sc, srv := newSearchClientTo(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(`{"meta":{"code":2000},"data":{"status_code":200,"body":{"ok":true}}}`))
	})
	defer srv.Close()
	now := time.Now().UTC()
	out, err := sc.SearchByTimeRange(now, now, "", "_search", []byte(`{}`))
	assert.NoError(t, err)
	assert.Contains(t, string(out), "ok")

	// testhook searchFunc override
	hooked := NewTestSearchClient(func(_, _ time.Time, _, _ string, _ []byte) ([]byte, error) {
		return []byte(`hooked`), nil
	})
	b, err := hooked.SearchByTimeRange(now, now, "i", "u", nil)
	assert.NoError(t, err)
	assert.Equal(t, "hooked", string(b))
}

func TestGenerateIndexPattern(t *testing.T) {
	sc := NewClient(SearchClientConfig{}, nil)
	base := time.Date(2026, 4, 15, 0, 0, 0, 0, time.UTC)

	// equal times -> single index
	p, err := sc.generateIndexPattern("node-", base, base)
	assert.NoError(t, err)
	assert.Equal(t, "node-2026.04.15", p)

	// multi-day (<30) -> comma list
	p, err = sc.generateIndexPattern("node-", base, base.AddDate(0, 0, 2))
	assert.NoError(t, err)
	assert.Contains(t, p, ",")

	// >= 30 days -> wildcard
	p, err = sc.generateIndexPattern("node-", base, base.AddDate(0, 0, 40))
	assert.NoError(t, err)
	assert.Equal(t, "node-*", p)
}

func TestTruncateForLogAndFallbackHelpers(t *testing.T) {
	assert.Equal(t, "abc", truncateForLog([]byte("abc"), 5))
	assert.Contains(t, truncateForLog([]byte("abcdefgh"), 3), "truncated")

	assert.True(t, isEmptyJSONString(json.RawMessage(``)))
	assert.True(t, isEmptyJSONString(json.RawMessage(`null`)))
	assert.True(t, isEmptyJSONString(json.RawMessage(`""`)))
	assert.False(t, isEmptyJSONString(json.RawMessage(`"x"`)))

	assert.True(t, sourceNeedsFallback(map[string]json.RawMessage{}))
	assert.True(t, sourceNeedsFallback(map[string]json.RawMessage{"message": json.RawMessage(`""`)}))
	assert.False(t, sourceNeedsFallback(map[string]json.RawMessage{"message": json.RawMessage(`"hi"`)}))
}

func TestDiscovery(t *testing.T) {
	assert.NoError(t, StartDiscover(nil))

	rc := robustclient.NewClient(robustclient.DefaultClientConfig())
	rc.RegisterCluster("c1", "http://x")
	InitRobustClient(rc)

	sc := GetOpensearchClient("c1")
	assert.NotNil(t, sc)
	// cached path second time
	assert.NotNil(t, GetOpensearchClient("c1"))
	// unknown cluster
	assert.Nil(t, GetOpensearchClient("none"))

	any := GetAnyOpensearchClient()
	assert.NotNil(t, any)
}
