/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package opensearch

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/observability"
)

func newSearchClientTo(t *testing.T, handler http.HandlerFunc) (*SearchClient, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(handler)
	lc := observability.NewLogsClient(observability.LogsClientConfig{BaseURL: srv.URL})
	return NewClient(SearchClientConfig{DefaultIndex: "node-"}, lc), srv
}

func TestRequestNotInitialized(t *testing.T) {
	sc := NewClient(SearchClientConfig{}, nil)
	_, err := sc.Request("/x", "GET", nil)
	assert.Error(t, err)
}

func TestRequestSuccessAndErrors(t *testing.T) {
	// success: raw OpenSearch body returned verbatim
	sc, srv := newSearchClientTo(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(`{"hits":{"hits":[]}}`))
	})
	defer srv.Close()
	out, err := sc.Request("api", "GET", []byte(`{}`))
	assert.NoError(t, err)
	assert.Contains(t, string(out), "hits")

	// non-2xx HTTP -> error
	sc2, srv2 := newSearchClientTo(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"error":"not found"}`))
	})
	defer srv2.Close()
	_, err = sc2.Request("/api", "GET", nil)
	assert.Error(t, err)

	// server error -> error
	sc3, srv3 := newSearchClientTo(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})
	defer srv3.Close()
	_, err = sc3.Request("/api", "GET", nil)
	assert.Error(t, err)
}

func TestSearchByTimeRange(t *testing.T) {
	sc, srv := newSearchClientTo(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(`{"ok":true}`))
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

func TestFallbackHelpers(t *testing.T) {
	assert.True(t, isEmptyJSONString(json.RawMessage(``)))
	assert.True(t, isEmptyJSONString(json.RawMessage(`null`)))
	assert.True(t, isEmptyJSONString(json.RawMessage(`""`)))
	assert.False(t, isEmptyJSONString(json.RawMessage(`"x"`)))

	assert.True(t, sourceNeedsFallback(map[string]json.RawMessage{}))
	assert.True(t, sourceNeedsFallback(map[string]json.RawMessage{"message": json.RawMessage(`""`)}))
	assert.False(t, sourceNeedsFallback(map[string]json.RawMessage{"message": json.RawMessage(`"hi"`)}))
}

func TestInitDirect(t *testing.T) {
	scheme := runtime.NewScheme()
	assert.NoError(t, v1.AddToScheme(scheme))
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).Build()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	InitDirect(ctx, k8sClient)
	defer SetLogsRegistryForTest(nil)

	// No clusters discovered -> no client for any name.
	assert.Nil(t, GetOpensearchClient("nonexistent"))
	// With an active (empty) registry, GetAnyOpensearchClient finds nothing.
	assert.Nil(t, GetAnyOpensearchClient())
}

func TestGetAnyOpensearchClientEmptyRegistry(t *testing.T) {
	// An installed but empty registry (no cached clients) yields nil.
	reg := observability.NewLogsRegistry(observability.LogsClientConfig{})
	SetLogsRegistryForTest(reg)
	defer SetLogsRegistryForTest(nil)

	assert.Nil(t, GetAnyOpensearchClient())
}

func TestDiscovery(t *testing.T) {
	assert.NoError(t, StartDiscover(nil))

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	reg := observability.NewLogsRegistry(observability.LogsClientConfig{})
	reg.RegisterCluster("c1", srv.URL)
	SetLogsRegistryForTest(reg)

	sc := GetOpensearchClient("c1")
	assert.NotNil(t, sc)
	// cached path second time
	assert.NotNil(t, GetOpensearchClient("c1"))
	// unknown cluster
	assert.Nil(t, GetOpensearchClient("none"))

	any := GetAnyOpensearchClient()
	assert.NotNil(t, any)
}
