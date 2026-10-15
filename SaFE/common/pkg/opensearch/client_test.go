/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package opensearch

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	gassert "gotest.tools/assert"

	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/robustclient"
)

func TestQueryIndex(t *testing.T) {
	endTime, err := time.Parse("2006-01-02T15:04:05", "2025-01-01T10:00:00")
	gassert.NilError(t, err)

	client := NewClient(SearchClientConfig{DefaultIndex: "node-"}, nil)
	tests := []struct {
		name      string
		SinceTime time.Time
		UntilTime time.Time
		result    string
	}{
		{
			"across 2 months",
			endTime.Add(-time.Hour * 24 * 31),
			endTime,
			client.DefaultIndex + "*",
		},
		{
			"across 2 days",
			endTime.Add(-time.Hour * 24 * 2),
			endTime,
			client.DefaultIndex + "2024.12.30" + "," + client.DefaultIndex + "2024.12.31" + "," + client.DefaultIndex + "2025.01.01",
		},
		{
			"within the same day",
			endTime.Add(-time.Hour * 2),
			endTime,
			client.DefaultIndex + "2025.01.01",
		},
		{
			"with the same time",
			endTime,
			endTime,
			client.DefaultIndex + "2025.01.01",
		},
		{
			"across 0 o'clock",
			endTime.Add(-time.Hour * 18),
			endTime.Add(-time.Hour * 10).Add(time.Minute),
			client.DefaultIndex + "2024.12.31" + "," + client.DefaultIndex + "2025.01.01",
		},
		{
			"at 0 o'clock",
			endTime.Add(-time.Hour * 18),
			endTime.Add(-time.Hour * 10),
			client.DefaultIndex + "2024.12.31" + "," + client.DefaultIndex + "2025.01.01",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result, err := client.generateIndexPattern(client.DefaultIndex, test.SinceTime, test.UntilTime)
			gassert.NilError(t, err)
			gassert.Equal(t, result, test.result)
		})
	}
}

func TestNormalizeLogResponseMessage(t *testing.T) {
	tests := []struct {
		name         string
		input        string
		expectMsg    []string
		expectChange bool
	}{
		{
			name: "fills empty message from log field",
			input: `{"took":1,"hits":{"total":{"value":2},"hits":[
				{"_id":"a","_source":{"@timestamp":"t1","log":"hello from log"}},
				{"_id":"b","_source":{"@timestamp":"t2","message":"already set","log":"shadowed"}}
			]}}`,
			expectMsg:    []string{"hello from log", "already set"},
			expectChange: true,
		},
		{
			name: "no change when message present and log absent",
			input: `{"took":1,"hits":{"total":{"value":1},"hits":[
				{"_id":"a","_source":{"@timestamp":"t1","message":"only message"}}
			]}}`,
			expectMsg:    []string{"only message"},
			expectChange: false,
		},
		{
			name: "treats null message as missing",
			input: `{"took":1,"hits":{"total":{"value":1},"hits":[
				{"_id":"a","_source":{"@timestamp":"t1","message":null,"log":"from log"}}
			]}}`,
			expectMsg:    []string{"from log"},
			expectChange: true,
		},
		{
			name: "treats empty-string message as missing",
			input: `{"took":1,"hits":{"total":{"value":1},"hits":[
				{"_id":"a","_source":{"@timestamp":"t1","message":"","log":"from log"}}
			]}}`,
			expectMsg:    []string{"from log"},
			expectChange: true,
		},
		{
			name: "skips hit when both fields missing",
			input: `{"took":1,"hits":{"total":{"value":1},"hits":[
				{"_id":"a","_source":{"@timestamp":"t1","stream":"stdout"}}
			]}}`,
			expectMsg:    []string{""},
			expectChange: false,
		},
		{
			name:         "tolerates empty body",
			input:        ``,
			expectMsg:    nil,
			expectChange: false,
		},
		{
			name:         "tolerates malformed body",
			input:        `{"hits":`,
			expectMsg:    nil,
			expectChange: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out := NormalizeLogResponseMessage([]byte(tt.input))
			if !tt.expectChange {
				gassert.Equal(t, string(out), tt.input)
				return
			}
			var parsed struct {
				Hits struct {
					Hits []struct {
						Source struct {
							Message string `json:"message"`
						} `json:"_source"`
					} `json:"hits"`
				} `json:"hits"`
			}
			gassert.NilError(t, json.Unmarshal(out, &parsed))
			gassert.Equal(t, len(parsed.Hits.Hits), len(tt.expectMsg))
			for i, want := range tt.expectMsg {
				gassert.Equal(t, parsed.Hits.Hits[i].Source.Message, want)
			}
		})
	}
}

func TestEffectiveMessageAndNormalize(t *testing.T) {
	resp := &OpenSearchLogResponse{}
	resp.Hits.Hits = make([]OpenSearchLogDoc, 3)
	resp.Hits.Hits[0].Source.Message = "primary"
	resp.Hits.Hits[0].Source.Log = "fallback ignored"
	resp.Hits.Hits[1].Source.Log = "fallback used"
	// hit [2] has neither field.

	gassert.Equal(t, resp.Hits.Hits[0].EffectiveMessage(), "primary")
	gassert.Equal(t, resp.Hits.Hits[1].EffectiveMessage(), "fallback used")
	gassert.Equal(t, resp.Hits.Hits[2].EffectiveMessage(), "")

	resp.NormalizeMessages()
	gassert.Equal(t, resp.Hits.Hits[0].Source.Message, "primary")
	gassert.Equal(t, resp.Hits.Hits[1].Source.Message, "fallback used")
	gassert.Equal(t, resp.Hits.Hits[2].Source.Message, "")
}

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

func TestSearchByTimeRangeContext_CancelsRequest(t *testing.T) {
	entered := make(chan struct{})
	canceled := make(chan struct{})
	sc, server := newSearchClientTo(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		close(entered)
		<-r.Context().Done()
		close(canceled)
	})
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	done := make(chan error, 1)
	go func() {
		_, err := sc.SearchByTimeRangeContext(ctx, time.Now(), time.Now(), "", "/_search", []byte(`{}`))
		done <- err
	}()
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("HTTP request did not start")
	}
	select {
	case err := <-done:
		assert.ErrorIs(t, err, context.DeadlineExceeded)
	case <-time.After(time.Second):
		t.Fatal("request outlived deadline")
	}
	select {
	case <-canceled:
	case <-time.After(time.Second):
		t.Fatal("HTTP request was not canceled")
	}
}
