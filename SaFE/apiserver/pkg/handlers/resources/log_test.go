/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package resources

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

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
			name:     "multi-word keyword uses span_near",
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
			name:     "trim leading comma",
			input:    ",error",
			expected: "error",
		},
		{
			name:     "trim trailing period",
			input:    "error.",
			expected: "error",
		},
		{
			name:     "trim question mark",
			input:    "error?",
			expected: "error",
		},
		{
			name:     "trim exclamation mark",
			input:    "error!",
			expected: "error",
		},
		{
			name:     "trim semicolon",
			input:    "error;",
			expected: "error",
		},
		{
			name:     "trim colon",
			input:    "error:",
			expected: "error",
		},
		{
			name:     "trim slash",
			input:    "/error/",
			expected: "error",
		},
		{
			name:     "mixed case and punctuation",
			input:    ",ERROR!",
			expected: "error",
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
