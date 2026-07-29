/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package resources

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	testifyassert "github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	commonconfig "github.com/AMD-AIG-AIMA/SAFE/common/pkg/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"

	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/resources/view"
	dbclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
	commonsearch "github.com/AMD-AIG-AIMA/SAFE/common/pkg/opensearch"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/slice"
)

func TestParseLogQuery(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name      string
		body      string
		beginTime time.Time
		endTime   time.Time
		validate  func(*testing.T, *view.ListLogRequest, error)
	}{
		{
			name: "basic query with all fields",
			body: `{
    "since": "2006-01-02T15:04:05.000Z",
	"until": "2006-01-03T15:04:05.000Z",
    "keywords": ["key1", "key2"],
				"nodeNames": "node1,node2"
			}`,
			beginTime: time.Time{},
			endTime:   time.Time{},
			validate: func(t *testing.T, query *view.ListLogRequest, err error) {
				assert.NoError(t, err)
				assert.Equal(t, 0, query.Offset)
				assert.Equal(t, view.DefaultQueryLimit, query.Limit)
				assert.False(t, query.SinceTime.IsZero())
				assert.False(t, query.UntilTime.IsZero())
				assert.Equal(t, float64(24), query.UntilTime.Sub(query.SinceTime).Hours())
				assert.True(t, slice.EqualIgnoreOrder(query.Keywords, []string{"key1", "key2"}))
				assert.Equal(t, "node1,node2", query.NodeNames)
				assert.Equal(t, dbclient.ASC, query.Order)
				assert.Equal(t, 0, query.DispatchCount)
			},
		},
		{
			name:      "empty body uses defaults",
			body:      `{}`,
			beginTime: time.Time{},
			endTime:   time.Time{},
			validate: func(t *testing.T, query *view.ListLogRequest, err error) {
				assert.NoError(t, err)
				assert.Equal(t, 0, query.Offset)
				assert.Equal(t, view.DefaultQueryLimit, query.Limit)
				assert.Equal(t, dbclient.ASC, query.Order)
			},
		},
		{
			name: "custom offset and limit",
			body: `{
				"offset": 50,
				"limit": 200
			}`,
			beginTime: time.Time{},
			endTime:   time.Time{},
			validate: func(t *testing.T, query *view.ListLogRequest, err error) {
				assert.NoError(t, err)
				assert.Equal(t, 50, query.Offset)
				assert.Equal(t, 200, query.Limit)
			},
		},
		{
			name: "descending order",
			body: `{
				"order": "desc"
			}`,
			beginTime: time.Time{},
			endTime:   time.Time{},
			validate: func(t *testing.T, query *view.ListLogRequest, err error) {
				assert.NoError(t, err)
				assert.Equal(t, dbclient.DESC, query.Order)
			},
		},
		{
			name: "ascending order",
			body: `{
				"order": "asc"
			}`,
			beginTime: time.Time{},
			endTime:   time.Time{},
			validate: func(t *testing.T, query *view.ListLogRequest, err error) {
				assert.NoError(t, err)
				assert.Equal(t, dbclient.ASC, query.Order)
			},
		},
		{
			name: "invalid order value",
			body: `{
				"order": "invalid"
			}`,
			beginTime: time.Time{},
			endTime:   time.Time{},
			validate: func(t *testing.T, query *view.ListLogRequest, err error) {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), "order parameter only supports")
			},
		},
		{
			name: "negative offset",
			body: `{
				"offset": -1
			}`,
			beginTime: time.Time{},
			endTime:   time.Time{},
			validate: func(t *testing.T, query *view.ListLogRequest, err error) {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), "invalid query offset or limit")
			},
		},
		{
			name: "negative limit",
			body: `{
				"limit": -1
			}`,
			beginTime: time.Time{},
			endTime:   time.Time{},
			validate: func(t *testing.T, query *view.ListLogRequest, err error) {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), "invalid query offset or limit")
			},
		},
		{
			name: "offset exceeds max",
			body: `{
				"offset": 10001
			}`,
			beginTime: time.Time{},
			endTime:   time.Time{},
			validate: func(t *testing.T, query *view.ListLogRequest, err error) {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), "maximum offset")
			},
		},
		{
			name: "limit adjusted when offset plus limit exceeds max",
			body: `{
				"offset": 9000,
				"limit": 2000
			}`,
			beginTime: time.Time{},
			endTime:   time.Time{},
			validate: func(t *testing.T, query *view.ListLogRequest, err error) {
				assert.NoError(t, err)
				assert.Equal(t, 9000, query.Offset)
				assert.Equal(t, commonsearch.MaxDocsPerQuery-9000, query.Limit)
			},
		},
		{
			name: "since time after until time",
			body: `{
				"since": "2006-01-03T15:04:05.000Z",
				"until": "2006-01-02T15:04:05.000Z"
			}`,
			beginTime: time.Time{},
			endTime:   time.Time{},
			validate: func(t *testing.T, query *view.ListLogRequest, err error) {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), "since time is later than until time")
			},
		},
		{
			name: "with pod names filter",
			body: `{
				"podNames": "pod1,pod2,pod3"
			}`,
			beginTime: time.Time{},
			endTime:   time.Time{},
			validate: func(t *testing.T, query *view.ListLogRequest, err error) {
				assert.NoError(t, err)
				assert.Equal(t, "pod1,pod2,pod3", query.PodNames)
			},
		},
		{
			name: "with dispatch count",
			body: `{
				"dispatchCount": 5
			}`,
			beginTime: time.Time{},
			endTime:   time.Time{},
			validate: func(t *testing.T, query *view.ListLogRequest, err error) {
				assert.NoError(t, err)
				assert.Equal(t, 5, query.DispatchCount)
			},
		},
		{
			name:      "time constrained by beginTime parameter",
			body:      `{}`,
			beginTime: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
			endTime:   time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC),
			validate: func(t *testing.T, query *view.ListLogRequest, err error) {
				assert.NoError(t, err)
				assert.Equal(t, time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), query.SinceTime)
				assert.Equal(t, time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC), query.UntilTime)
			},
		},
		{
			name: "since time before beginTime is adjusted",
			body: `{
				"since": "2023-12-31T00:00:00.000Z"
			}`,
			beginTime: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
			endTime:   time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC),
			validate: func(t *testing.T, query *view.ListLogRequest, err error) {
				assert.NoError(t, err)
				assert.Equal(t, time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), query.SinceTime)
			},
		},
		{
			name: "until time after endTime is adjusted",
			body: `{
				"until": "2024-01-10T00:00:00.000Z"
			}`,
			beginTime: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
			endTime:   time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC),
			validate: func(t *testing.T, query *view.ListLogRequest, err error) {
				assert.NoError(t, err)
				assert.Equal(t, time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC), query.UntilTime)
			},
		},
		{
			name: "invalid since time format",
			body: `{
				"since": "invalid-time"
			}`,
			beginTime: time.Time{},
			endTime:   time.Time{},
			validate: func(t *testing.T, query *view.ListLogRequest, err error) {
				assert.Error(t, err)
			},
		},
		{
			name: "invalid until time format",
			body: `{
				"until": "invalid-time"
			}`,
			beginTime: time.Time{},
			endTime:   time.Time{},
			validate: func(t *testing.T, query *view.ListLogRequest, err error) {
				assert.Error(t, err)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rsp := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rsp)
			c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/workloads/test-workload/logs", strings.NewReader(tt.body))

			query, err := parseLogQuery(c.Request, tt.beginTime, tt.endTime)
			tt.validate(t, query, err)
		})
	}
}

func TestBuildSearchBody(t *testing.T) {
	baseTime := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name       string
		query      *view.ListLogRequest
		workloadId string
		validate   func(*testing.T, []byte)
	}{
		{
			name: "basic search body",
			query: &view.ListLogRequest{
				ListLogInput: view.ListLogInput{
					Offset: 0,
					Limit:  100,
					Order:  dbclient.ASC,
				},
				SinceTime: baseTime,
				UntilTime: baseTime.Add(time.Hour),
			},
			workloadId: "workload-001",
			validate: func(t *testing.T, body []byte) {
				var req commonsearch.OpenSearchRequest
				err := json.Unmarshal(body, &req)
				assert.NoError(t, err)
				assert.Equal(t, 0, req.From)
				assert.Equal(t, 100, req.Size)
				assert.NotEmpty(t, req.Sort)
			},
		},
		{
			name: "search with pagination",
			query: &view.ListLogRequest{
				ListLogInput: view.ListLogInput{
					Offset: 50,
					Limit:  200,
					Order:  dbclient.DESC,
				},
				SinceTime: baseTime,
				UntilTime: baseTime.Add(time.Hour),
			},
			workloadId: "workload-002",
			validate: func(t *testing.T, body []byte) {
				var req commonsearch.OpenSearchRequest
				err := json.Unmarshal(body, &req)
				assert.NoError(t, err)
				assert.Equal(t, 50, req.From)
				assert.Equal(t, 200, req.Size)
			},
		},
		{
			name: "search with filters",
			query: &view.ListLogRequest{
				ListLogInput: view.ListLogInput{
					Offset: 0,
					Limit:  100,
					Order:  dbclient.ASC,
				},
				SinceTime: baseTime,
				UntilTime: baseTime.Add(time.Hour),
				TermFilters: map[string]string{
					"app":         "test-app",
					"workload.id": "wl-001",
				},
			},
			workloadId: "workload-003",
			validate: func(t *testing.T, body []byte) {
				var req commonsearch.OpenSearchRequest
				err := json.Unmarshal(body, &req)
				assert.NoError(t, err)
				assert.NotEmpty(t, req.Query.Bool.Filter)
			},
		},
		{
			name: "search with pod names",
			query: &view.ListLogRequest{
				ListLogInput: view.ListLogInput{
					Offset:   0,
					Limit:    100,
					Order:    dbclient.ASC,
					PodNames: "pod1,pod2",
				},
				SinceTime: baseTime,
				UntilTime: baseTime.Add(time.Hour),
			},
			workloadId: "workload-004",
			validate: func(t *testing.T, body []byte) {
				var req commonsearch.OpenSearchRequest
				err := json.Unmarshal(body, &req)
				assert.NoError(t, err)
				assert.True(t, len(req.Query.Bool.Must) >= 2)
			},
		},
		{
			name: "search with node names",
			query: &view.ListLogRequest{
				ListLogInput: view.ListLogInput{
					Offset:    0,
					Limit:     100,
					Order:     dbclient.ASC,
					NodeNames: "node1,node2",
				},
				SinceTime: baseTime,
				UntilTime: baseTime.Add(time.Hour),
			},
			workloadId: "workload-005",
			validate: func(t *testing.T, body []byte) {
				var req commonsearch.OpenSearchRequest
				err := json.Unmarshal(body, &req)
				assert.NoError(t, err)
				assert.True(t, len(req.Query.Bool.Must) >= 2)
			},
		},
		{
			name: "search with single keyword",
			query: &view.ListLogRequest{
				ListLogInput: view.ListLogInput{
					Offset:   0,
					Limit:    100,
					Order:    dbclient.ASC,
					Keywords: []string{"error"},
				},
				SinceTime: baseTime,
				UntilTime: baseTime.Add(time.Hour),
			},
			workloadId: "workload-006",
			validate: func(t *testing.T, body []byte) {
				var req commonsearch.OpenSearchRequest
				err := json.Unmarshal(body, &req)
				assert.NoError(t, err)
				assert.True(t, len(req.Query.Bool.Must) >= 2)
			},
		},
		{
			name: "search with multi-word keyword",
			query: &view.ListLogRequest{
				ListLogInput: view.ListLogInput{
					Offset:   0,
					Limit:    100,
					Order:    dbclient.ASC,
					Keywords: []string{"connection timeout"},
				},
				SinceTime: baseTime,
				UntilTime: baseTime.Add(time.Hour),
			},
			workloadId: "workload-007",
			validate: func(t *testing.T, body []byte) {
				var req commonsearch.OpenSearchRequest
				err := json.Unmarshal(body, &req)
				assert.NoError(t, err)
				assert.True(t, len(req.Query.Bool.Must) >= 2)
			},
		},
		{
			name: "search without workload id",
			query: &view.ListLogRequest{
				ListLogInput: view.ListLogInput{
					Offset: 0,
					Limit:  100,
					Order:  dbclient.ASC,
				},
				SinceTime: baseTime,
				UntilTime: baseTime.Add(time.Hour),
			},
			workloadId: "",
			validate: func(t *testing.T, body []byte) {
				var req commonsearch.OpenSearchRequest
				err := json.Unmarshal(body, &req)
				assert.NoError(t, err)
				assert.NotContains(t, string(body), commonsearch.StreamField)
			},
		},
		{
			name: "cicd query",
			query: &view.ListLogRequest{
				ListLogInput: view.ListLogInput{
					Offset:   0,
					Limit:    100,
					Order:    dbclient.ASC,
					Keywords: []string{common.CICDScaleRunnerSetKind, "project1-dev", "primus-lm-cicd-jax-m42vb"},
				},
				SinceTime:     baseTime,
				UntilTime:     baseTime.Add(time.Hour),
				TermFilters:   map[string]string{"kubernetes.namespace_name": "arc-systems"},
				PrefixFilters: map[string]string{"kubernetes.pod_name": "gha-runner-scale-set-gha-rs-controller"},
			},
			workloadId: "",
			validate: func(t *testing.T, body []byte) {
				var req commonsearch.OpenSearchRequest
				err := json.Unmarshal(body, &req)
				assert.NoError(t, err)
				fmt.Println(string(body))
				assert.True(t, len(req.Query.Bool.Must) >= 4)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := buildSearchBody(tt.query, tt.workloadId)
			assert.NotEmpty(t, body)
			tt.validate(t, body)
		})
	}
}

func TestBuildFilter(t *testing.T) {
	tests := []struct {
		name       string
		query      *view.ListLogRequest
		wantFilter int
		wantMust   int
	}{
		{
			name: "no filters",
			query: &view.ListLogRequest{
				ListLogInput: view.ListLogInput{},
			},
			wantFilter: 0,
			wantMust:   0,
		},
		{
			name: "only label filters",
			query: &view.ListLogRequest{
				ListLogInput: view.ListLogInput{},
				TermFilters: map[string]string{
					"app": "test",
				},
			},
			wantFilter: 1,
			wantMust:   0,
		},
		{
			name: "with pod names",
			query: &view.ListLogRequest{
				ListLogInput: view.ListLogInput{
					PodNames: "pod1,pod2",
				},
			},
			wantFilter: 0,
			wantMust:   1,
		},
		{
			name: "with node names (no pod names)",
			query: &view.ListLogRequest{
				ListLogInput: view.ListLogInput{
					NodeNames: "node1,node2",
				},
			},
			wantFilter: 0,
			wantMust:   1,
		},
		{
			name: "pod names takes precedence over node names",
			query: &view.ListLogRequest{
				ListLogInput: view.ListLogInput{
					PodNames:  "pod1",
					NodeNames: "node1",
				},
			},
			wantFilter: 0,
			wantMust:   1,
		},
		{
			name: "combined label and pod filters",
			query: &view.ListLogRequest{
				ListLogInput: view.ListLogInput{
					PodNames: "pod1",
				},
				TermFilters: map[string]string{
					"app": "test",
				},
			},
			wantFilter: 1,
			wantMust:   1,
		},
		{
			name: "empty filters map",
			query: &view.ListLogRequest{
				ListLogInput: view.ListLogInput{},
				TermFilters:  map[string]string{},
			},
			wantFilter: 0,
			wantMust:   0,
		},
		{
			name: "filter with empty key is skipped",
			query: &view.ListLogRequest{
				ListLogInput: view.ListLogInput{},
				TermFilters: map[string]string{
					"":    "value",
					"app": "test",
				},
			},
			wantFilter: 1,
			wantMust:   0,
		},
		{
			name: "filter with empty value is skipped",
			query: &view.ListLogRequest{
				ListLogInput: view.ListLogInput{},
				TermFilters: map[string]string{
					"key": "",
					"app": "test",
				},
			},
			wantFilter: 1,
			wantMust:   0,
		},
		{
			name: "filter key with dots is converted",
			query: &view.ListLogRequest{
				ListLogInput: view.ListLogInput{},
				TermFilters: map[string]string{
					"primus.safe.workload": "test",
				},
			},
			wantFilter: 1,
			wantMust:   0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := &commonsearch.OpenSearchRequest{}
			buildFilter(req, tt.query)
			assert.Len(t, req.Query.Bool.Filter, tt.wantFilter)
			assert.Len(t, req.Query.Bool.Must, tt.wantMust)
		})
	}
}

func TestBuildMultiTermsFilter(t *testing.T) {
	tests := []struct {
		name    string
		key     string
		values  string
		wantLen int
	}{
		{
			name:    "single value",
			key:     "pod_name",
			values:  "pod1",
			wantLen: 1,
		},
		{
			name:    "multiple values",
			key:     "pod_name",
			values:  "pod1,pod2,pod3",
			wantLen: 1,
		},
		{
			name:    "empty values",
			key:     "pod_name",
			values:  "",
			wantLen: 0,
		},
		{
			name:    "host filter",
			key:     "host",
			values:  "node1,node2",
			wantLen: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := &commonsearch.OpenSearchRequest{}
			req.Query.Bool.Must = []commonsearch.OpenSearchField{}
			buildMultiTermsFilter(req, tt.key, tt.values)
			assert.Len(t, req.Query.Bool.Must, tt.wantLen)
		})
	}
}

func TestBuildKeywords(t *testing.T) {
	tests := []struct {
		name     string
		keywords []string
		wantLen  int
	}{
		{
			name:     "no keywords",
			keywords: []string{},
			wantLen:  0,
		},
		{
			name:     "single word keyword",
			keywords: []string{"error"},
			wantLen:  1,
		},
		{
			name:     "multiple single word keywords",
			keywords: []string{"error", "warning", "fatal"},
			wantLen:  3,
		},
		{
			name:     "multi-word keyword uses match_phrase with slop",
			keywords: []string{"connection timeout"},
			wantLen:  1,
		},
		{
			name:     "mixed keywords",
			keywords: []string{"error", "connection timeout", "fatal"},
			wantLen:  3,
		},
		{
			name:     "empty keyword is skipped",
			keywords: []string{"", "error"},
			wantLen:  1,
		},
		{
			name:     "whitespace only keyword is skipped",
			keywords: []string{"   ", "error"},
			wantLen:  1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := &commonsearch.OpenSearchRequest{}
			query := &view.ListLogRequest{
				ListLogInput: view.ListLogInput{
					Keywords: tt.keywords,
				},
			}
			buildKeywords(req, query)
			assert.Len(t, req.Query.Bool.Must, tt.wantLen)
		})
	}
}

func TestBuildOutput(t *testing.T) {
	tests := []struct {
		name         string
		workloadId   string
		podNames     string
		expectSource []string
	}{
		{
			name:       "with workload id includes stream field",
			workloadId: "workload-001",
			podNames:   "",
			expectSource: []string{
				commonsearch.TimeField,
				commonsearch.MessageField,
				"kubernetes.host",
				commonsearch.StreamField,
			},
		},
		{
			name:       "without workload id excludes stream field",
			workloadId: "",
			podNames:   "",
			expectSource: []string{
				commonsearch.TimeField,
				commonsearch.MessageField,
				"kubernetes.host",
				"kubernetes.pod_name",
			},
		},
		{
			name:       "single pod name excludes pod_name field",
			workloadId: "workload-001",
			podNames:   "pod1",
			expectSource: []string{
				commonsearch.TimeField,
				commonsearch.MessageField,
				"kubernetes.host",
				commonsearch.StreamField,
			},
		},
		{
			name:       "multiple pod names includes pod_name field",
			workloadId: "workload-001",
			podNames:   "pod1,pod2",
			expectSource: []string{
				commonsearch.TimeField,
				commonsearch.MessageField,
				"kubernetes.host",
				commonsearch.StreamField,
				"kubernetes.pod_name",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := &commonsearch.OpenSearchRequest{}
			query := &view.ListLogRequest{
				ListLogInput: view.ListLogInput{
					PodNames: tt.podNames,
				},
				UseK8sLabel: true,
			}
			buildOutput(req, query, tt.workloadId)
			for _, expected := range tt.expectSource {
				assert.Contains(t, req.Source, expected)
			}
		})
	}
}

func TestNormalize(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "lowercase conversion",
			input:    "ERROR",
			expected: "error",
		},
		{
			name:     "punctuation preserved",
			input:    ",error",
			expected: ",error",
		},
		{
			name:     "trailing period preserved",
			input:    "error.",
			expected: "error.",
		},
		{
			name:     "question mark preserved",
			input:    "error?",
			expected: "error?",
		},
		{
			name:     "exclamation preserved",
			input:    "error!",
			expected: "error!",
		},
		{
			name:     "semicolon preserved",
			input:    "error;",
			expected: "error;",
		},
		{
			name:     "colon preserved",
			input:    "error:",
			expected: "error:",
		},
		{
			name:     "slash preserved",
			input:    "/error/",
			expected: "/error/",
		},
		{
			name:     "mixed case and punctuation",
			input:    ",ERROR!",
			expected: ",error!",
		},
		{
			name:     "no changes needed",
			input:    "error",
			expected: "error",
		},
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := normalize(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestAddContextDoc(t *testing.T) {
	tests := []struct {
		name      string
		query     view.ListContextLogRequest
		response  *commonsearch.OpenSearchLogResponse
		isAsc     bool
		wantErr   bool
		wantCount int
	}{
		{
			name: "doc id not found",
			query: view.ListContextLogRequest{
				DocId: "non-existent",
				Limit: 3,
			},
			response: &commonsearch.OpenSearchLogResponse{
				Hits: commonsearch.OpenSearchLogHits{
					Hits: []commonsearch.OpenSearchLogDoc{
						{Id: "doc-001", Source: struct {
							Timestamp  string `json:"@timestamp"`
							Stream     string `json:"stream,omitempty"`
							Message    string `json:"message,omitempty"`
							Log        string `json:"log,omitempty"`
							Line       int    `json:"line,omitempty"`
							Kubernetes struct {
								PodName string `json:"pod_name,omitempty"`
								Labels  struct {
									DispatchCount string `json:"primus-safe_workload_dispatch_count,omitempty"`
									ReplicaIndex  string `json:"training_kubeflow_org/replica-index,omitempty"`
									ReplicaType   string `json:"training_kubeflow_org/replica-type,omitempty"`
									JobName       string `json:"training_kubeflow_org/job-name,omitempty"`
									WorkloadId    string `json:"primus-safe_workload_id,omitempty"`
								} `json:"labels,omitempty"`
								Host          string `json:"host,omitempty"`
								ContainerName string `json:"container_name,omitempty"`
							} `json:"kubernetes,omitempty"`
						}{Message: "message 1"}},
					},
				},
			},
			isAsc:     true,
			wantErr:   true,
			wantCount: 0,
		},
		{
			name: "skip empty messages",
			query: view.ListContextLogRequest{
				DocId: "doc-001",
				Limit: 5,
			},
			response: &commonsearch.OpenSearchLogResponse{
				Hits: commonsearch.OpenSearchLogHits{
					Hits: []commonsearch.OpenSearchLogDoc{
						{Id: "doc-001", Source: struct {
							Timestamp  string `json:"@timestamp"`
							Stream     string `json:"stream,omitempty"`
							Message    string `json:"message,omitempty"`
							Log        string `json:"log,omitempty"`
							Line       int    `json:"line,omitempty"`
							Kubernetes struct {
								PodName string `json:"pod_name,omitempty"`
								Labels  struct {
									DispatchCount string `json:"primus-safe_workload_dispatch_count,omitempty"`
									ReplicaIndex  string `json:"training_kubeflow_org/replica-index,omitempty"`
									ReplicaType   string `json:"training_kubeflow_org/replica-type,omitempty"`
									JobName       string `json:"training_kubeflow_org/job-name,omitempty"`
									WorkloadId    string `json:"primus-safe_workload_id,omitempty"`
								} `json:"labels,omitempty"`
								Host          string `json:"host,omitempty"`
								ContainerName string `json:"container_name,omitempty"`
							} `json:"kubernetes,omitempty"`
						}{Message: "message 1"}},
						{Id: "doc-002", Source: struct {
							Timestamp  string `json:"@timestamp"`
							Stream     string `json:"stream,omitempty"`
							Message    string `json:"message,omitempty"`
							Log        string `json:"log,omitempty"`
							Line       int    `json:"line,omitempty"`
							Kubernetes struct {
								PodName string `json:"pod_name,omitempty"`
								Labels  struct {
									DispatchCount string `json:"primus-safe_workload_dispatch_count,omitempty"`
									ReplicaIndex  string `json:"training_kubeflow_org/replica-index,omitempty"`
									ReplicaType   string `json:"training_kubeflow_org/replica-type,omitempty"`
									JobName       string `json:"training_kubeflow_org/job-name,omitempty"`
									WorkloadId    string `json:"primus-safe_workload_id,omitempty"`
								} `json:"labels,omitempty"`
								Host          string `json:"host,omitempty"`
								ContainerName string `json:"container_name,omitempty"`
							} `json:"kubernetes,omitempty"`
						}{Message: ""}},
						{Id: "doc-003", Source: struct {
							Timestamp  string `json:"@timestamp"`
							Stream     string `json:"stream,omitempty"`
							Message    string `json:"message,omitempty"`
							Log        string `json:"log,omitempty"`
							Line       int    `json:"line,omitempty"`
							Kubernetes struct {
								PodName string `json:"pod_name,omitempty"`
								Labels  struct {
									DispatchCount string `json:"primus-safe_workload_dispatch_count,omitempty"`
									ReplicaIndex  string `json:"training_kubeflow_org/replica-index,omitempty"`
									ReplicaType   string `json:"training_kubeflow_org/replica-type,omitempty"`
									JobName       string `json:"training_kubeflow_org/job-name,omitempty"`
									WorkloadId    string `json:"primus-safe_workload_id,omitempty"`
								} `json:"labels,omitempty"`
								Host          string `json:"host,omitempty"`
								ContainerName string `json:"container_name,omitempty"`
							} `json:"kubernetes,omitempty"`
						}{Message: "message 3"}},
					},
				},
			},
			isAsc:     true,
			wantErr:   false,
			wantCount: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := &commonsearch.OpenSearchLogResponse{}
			err := addContextDoc(result, tt.query, tt.response, tt.isAsc)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.wantCount, len(result.Hits.Hits))
				assert.Equal(t, tt.wantCount, result.Hits.Total.Value)
				// Check line numbers are set correctly
				for i, hit := range result.Hits.Hits {
					if tt.isAsc {
						assert.Equal(t, i+1, hit.Source.Line)
					} else {
						assert.Equal(t, -(i + 1), hit.Source.Line)
					}
				}
			}
		})
	}
}

// --- merged from log_download_test.go ---

// withS3 enables both OpenSearch and S3 for the duration of the test.
func withS3(t *testing.T) {
	t.Helper()
	commonconfig.SetValue("opensearch.enable", "true")
	commonconfig.SetValue("s3.enable", "true")
	t.Cleanup(func() {
		commonconfig.SetValue("opensearch.enable", "false")
		commonconfig.SetValue("s3.enable", "false")
	})
}

func succeededDumpLogJob(name, endpoint string) *v1.OpsJob {
	job := &v1.OpsJob{ObjectMeta: metav1.ObjectMeta{Name: name}}
	job.Status.Phase = v1.OpsJobSucceeded
	job.Status.Outputs = []v1.Parameter{{Name: v1.ParameterEndpoint, Value: endpoint}}
	return job
}

func TestCreateDumpLogJobInternal(t *testing.T) {
	wl := newWorkloadForLog("wl-1", "c1", "ws-1")
	h, user := newAdminHandlerWithObjects(wl)

	job, err := h.createDumpLogJobInternal(context.Background(), wl, user, &view.DownloadWorkloadLogRequest{TimeoutSecond: 60})
	testifyassert.NoError(t, err)
	testifyassert.Contains(t, job.Name, "down-")

	// Idempotent: second call returns the existing job.
	job2, err := h.createDumpLogJobInternal(context.Background(), wl, user, &view.DownloadWorkloadLogRequest{TimeoutSecond: 60})
	testifyassert.NoError(t, err)
	assert.Equal(t, job.Name, job2.Name)
}

func TestWaitForDumpLogJobCompletion(t *testing.T) {
	// Succeeded -> returns endpoint URL immediately.
	h, _ := newAdminHandlerWithObjects(succeededDumpLogJob("job-ok", "https://s3/log.tar.gz"))
	url, err := h.waitForDumpLogJobCompletion(context.Background(), "job-ok", 60)
	testifyassert.NoError(t, err)
	assert.Equal(t, "https://s3/log.tar.gz", url)

	// Failed -> error.
	failed := &v1.OpsJob{ObjectMeta: metav1.ObjectMeta{Name: "job-fail"}}
	failed.Status.Phase = v1.OpsJobFailed
	failed.Status.Conditions = []metav1.Condition{{Status: metav1.ConditionFalse, Message: "boom"}}
	h2, _ := newAdminHandlerWithObjects(failed)
	_, err = h2.waitForDumpLogJobCompletion(context.Background(), "job-fail", 60)
	testifyassert.Error(t, err)

	// Timeout (deadline already passed) -> error, no real sleeping.
	h3, _ := newAdminHandlerWithObjects()
	_, err = h3.waitForDumpLogJobCompletion(context.Background(), "missing", 0)
	testifyassert.Error(t, err)
}

func TestDownloadWorkloadLogHandler(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("s3 disabled", func(t *testing.T) {
		commonconfig.SetValue("opensearch.enable", "true")
		t.Cleanup(func() { commonconfig.SetValue("opensearch.enable", "false") })
		h, user := newAdminHandlerWithObjects(newWorkloadForLog("wl-1", "c1", "ws-1"))
		rsp := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rsp)
		c.Request = httptest.NewRequest(http.MethodPost, "/", bytes.NewReader([]byte(`{}`)))
		c.Request.Header.Set("Content-Type", "application/json")
		c.Set(common.UserId, user.Name)
		c.Set(common.Name, "wl-1")
		_, err := h.downloadWorkloadLog(c)
		testifyassert.Error(t, err)
	})

	t.Run("success with pre-seeded succeeded job", func(t *testing.T) {
		withS3(t)
		wl := newWorkloadForLog("wl-1", "c1", "ws-1")
		// Pre-seed the dump-log job so createDumpLogJobInternal returns it and
		// waitForDumpLogJobCompletion immediately reads the endpoint output.
		dumpJob := succeededDumpLogJob("down-wl-1", "https://s3/wl-1.tar.gz")
		h, user := newAdminHandlerWithObjects(wl, dumpJob)

		rsp := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rsp)
		c.Request = httptest.NewRequest(http.MethodPost, "/", bytes.NewReader([]byte(`{"timeoutSecond":60}`)))
		c.Request.Header.Set("Content-Type", "application/json")
		c.Set(common.UserId, user.Name)
		c.Set(common.Name, "wl-1")
		resp, err := h.downloadWorkloadLog(c)
		testifyassert.NoError(t, err)
		assert.Equal(t, "https://s3/wl-1.tar.gz", resp.DownloadURL)
	})
}

// --- merged from log_handlers_test.go ---

// withOpenSearch enables OpenSearch and registers a stub client for the given
// cluster that returns the provided canned `_search` response body.
func withOpenSearch(t *testing.T, clusterId string, respBody string) {
	t.Helper()
	commonconfig.SetValue("opensearch.enable", "true")
	sc := commonsearch.NewTestSearchClient(
		func(_, _ time.Time, _, _ string, _ []byte) ([]byte, error) {
			return []byte(respBody), nil
		},
	)
	cleanup := commonsearch.RegisterClientForTest(clusterId, sc)
	t.Cleanup(func() {
		cleanup()
		commonconfig.SetValue("opensearch.enable", "false")
	})
}

const emptyHits = `{"hits":{"total":{"value":0},"hits":[]}}`

func newWorkloadForLog(name, cluster, workspace string) *v1.Workload {
	wl := &v1.Workload{
		ObjectMeta: metav1.ObjectMeta{
			Name:   name,
			Labels: map[string]string{v1.ClusterIdLabel: cluster},
		},
		Spec: v1.WorkloadSpec{Workspace: workspace},
	}
	return wl
}

func TestGetWorkloadLogHandler(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("opensearch disabled", func(t *testing.T) {
		h, user := newAdminHandlerWithObjects(newWorkloadForLog("wl-1", "c1", "ws-1"))
		rsp := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rsp)
		c.Request = httptest.NewRequest(http.MethodPost, "/", bytes.NewReader([]byte(`{}`)))
		c.Request.Header.Set("Content-Type", "application/json")
		c.Set(common.UserId, user.Name)
		c.Set(common.Name, "wl-1")
		h.GetWorkloadLog(c)
		testifyassert.NotEqual(t, http.StatusOK, rsp.Code)
	})

	t.Run("success", func(t *testing.T) {
		withOpenSearch(t, "c1", emptyHits)
		h, user := newAdminHandlerWithObjects(newWorkloadForLog("wl-1", "c1", "ws-1"))
		rsp := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rsp)
		c.Request = httptest.NewRequest(http.MethodPost, "/", bytes.NewReader([]byte(`{}`)))
		c.Request.Header.Set("Content-Type", "application/json")
		c.Set(common.UserId, user.Name)
		c.Set(common.Name, "wl-1")
		h.GetWorkloadLog(c)
		assert.Equal(t, http.StatusOK, rsp.Code)
	})
}

func TestGetServiceLogHandler(t *testing.T) {
	gin.SetMode(gin.TestMode)
	withOpenSearch(t, "", emptyHits)
	h, user := newAdminHandlerWithObjects()

	rsp := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rsp)
	c.Request = httptest.NewRequest(http.MethodPost, "/", bytes.NewReader([]byte(`{}`)))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set(common.UserId, user.Name)
	c.Set(common.Name, "my-svc")
	h.GetServiceLog(c)
	assert.Equal(t, http.StatusOK, rsp.Code)
}

func TestGetWorkloadEventHandler(t *testing.T) {
	gin.SetMode(gin.TestMode)
	withOpenSearch(t, "c1", emptyHits)
	h, user := newAdminHandlerWithObjects(newWorkloadForLog("wl-1", "c1", "ws-1"))

	rsp := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rsp)
	c.Request = httptest.NewRequest(http.MethodPost, "/", bytes.NewReader([]byte(`{}`)))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set(common.UserId, user.Name)
	c.Set(common.Name, "wl-1")
	h.GetWorkloadEvent(c)
	assert.Equal(t, http.StatusOK, rsp.Code)
}

func TestGetCICDArcLogHandler(t *testing.T) {
	gin.SetMode(gin.TestMode)
	withOpenSearch(t, "c1", emptyHits)

	wl := newWorkloadForLog("wl-cicd", "c1", "ws-1")
	wl.Spec.GroupVersionKind = v1.GroupVersionKind{Kind: common.CICDScaleRunnerSetKind}
	h, user := newAdminHandlerWithObjects(wl)

	rsp := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rsp)
	c.Request = httptest.NewRequest(http.MethodPost, "/", bytes.NewReader([]byte(`{}`)))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set(common.UserId, user.Name)
	c.Set(common.Name, "wl-cicd")
	h.GetCICDArcLog(c)
	assert.Equal(t, http.StatusOK, rsp.Code)
}

func TestGetWorkloadLogContextHandler(t *testing.T) {
	gin.SetMode(gin.TestMode)
	// Stub returns a hit matching the requested docId so addContextDoc succeeds.
	respWithDoc := `{"hits":{"total":{"value":1},"hits":[{"_id":"doc-1","_source":{"message":"hello"}}]}}`
	withOpenSearch(t, "c1", respWithDoc)
	h, user := newAdminHandlerWithObjects(newWorkloadForLog("wl-1", "c1", "ws-1"))

	rsp := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rsp)
	// parseContextQuery needs a `since` field and a docId path param.
	c.Request = httptest.NewRequest(http.MethodPost, "/", bytes.NewReader([]byte(`{"since":"2025-01-01T00:00:00.000Z"}`)))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set(common.UserId, user.Name)
	c.Set(common.Name, "wl-1")
	c.Params = gin.Params{{Key: "docId", Value: "doc-1"}}
	h.GetWorkloadLogContext(c)
	assert.Equal(t, http.StatusOK, rsp.Code)
}

func TestGetAndAuthWorkload(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h, user := newAdminHandlerWithObjects(newWorkloadForLog("wl-1", "c1", "ws-1"))

	rsp := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rsp)
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	c.Set(common.UserId, user.Name)
	c.Set(common.Name, "wl-1")
	wl, err := h.getAndAuthWorkload(c)
	testifyassert.NoError(t, err)
	assert.Equal(t, "wl-1", wl.Name)

	// Empty name -> bad request.
	rsp2 := httptest.NewRecorder()
	c2, _ := gin.CreateTestContext(rsp2)
	c2.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	c2.Set(common.UserId, user.Name)
	_, err = h.getAndAuthWorkload(c2)
	testifyassert.Error(t, err)
}

// --- merged from log_helpers_test.go ---

func TestBuildSingleTermFilter(t *testing.T) {
	// Plain term filter.
	req := &commonsearch.OpenSearchRequest{}
	buildSingleTermFilter(req, map[string]string{"app": "svc"}, false, false)
	testifyassert.Len(t, req.Query.Bool.Filter, 1)

	// k8s label + prefix match.
	req2 := &commonsearch.OpenSearchRequest{}
	buildSingleTermFilter(req2, map[string]string{"my.label": "v"}, true, true)
	testifyassert.Len(t, req2.Query.Bool.Filter, 1)

	// Empty key/value is skipped.
	req3 := &commonsearch.OpenSearchRequest{}
	buildSingleTermFilter(req3, map[string]string{"": "v", "k": ""}, false, false)
	testifyassert.Empty(t, req3.Query.Bool.Filter)
}

func TestKeywordMatchAnyField(t *testing.T) {
	field := keywordMatchAnyField("error", 0)
	testifyassert.Contains(t, field, "bool")
}

func TestGetLogQueryStartTime(t *testing.T) {
	created := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	wl := &v1.Workload{ObjectMeta: metav1.ObjectMeta{CreationTimestamp: metav1.NewTime(created)}}

	// No start time -> creation time.
	assert.Equal(t, created, getLogQueryStartTime(wl))

	// Start time set later -> start - 1h.
	start := created.Add(3 * time.Hour)
	wl.Status.StartTime = &metav1.Time{Time: start}
	assert.Equal(t, start.Add(-time.Hour), getLogQueryStartTime(wl))
}

// newLogCtx builds a gin context with an empty JSON body for log query parsing.
func newLogCtx() *gin.Context {
	rsp := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rsp)
	c.Request = httptest.NewRequest(http.MethodPost, "/", bytes.NewReader([]byte(`{}`)))
	c.Request.Header.Set("Content-Type", "application/json")
	return c
}

func TestParseWorkloadLogQuery(t *testing.T) {
	gin.SetMode(gin.TestMode)
	wl := &v1.Workload{ObjectMeta: metav1.ObjectMeta{Name: "wl-1"}}
	c := newLogCtx()
	q, err := parseWorkloadLogQuery(c, wl)
	testifyassert.NoError(t, err)
	testifyassert.True(t, q.UseK8sLabel)
	assert.Equal(t, "wl-1", q.TermFilters[v1.WorkloadIdLabel])
}

func TestParseServiceLogQuery(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// Missing name -> bad request.
	c0 := newLogCtx()
	_, err := parseServiceLogQuery(c0)
	testifyassert.Error(t, err)

	// With name set.
	c := newLogCtx()
	c.Set(common.Name, "my-svc")
	q, err := parseServiceLogQuery(c)
	testifyassert.NoError(t, err)
	assert.Equal(t, "my-svc", q.TermFilters["app"])
}

func TestParseEventLogQuery(t *testing.T) {
	gin.SetMode(gin.TestMode)
	wl := &v1.Workload{
		ObjectMeta: metav1.ObjectMeta{Name: "wl-1"},
		Spec:       v1.WorkloadSpec{Workspace: "ws-1"},
	}
	c := newLogCtx()
	q, err := parseEventLogQuery(c, wl)
	testifyassert.NoError(t, err)
	testifyassert.True(t, q.DisableOutput)
	assert.Equal(t, "ws-1", q.TermFilters["involvedObject.namespace"])
	assert.Equal(t, "wl-1", q.PrefixFilters["involvedObject.name"])
}
