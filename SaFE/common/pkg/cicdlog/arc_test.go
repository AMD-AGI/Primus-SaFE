/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package cicdlog

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"gotest.tools/assert"

	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	commonconfig "github.com/AMD-AIG-AIMA/SAFE/common/pkg/config"
	commonsearch "github.com/AMD-AIG-AIMA/SAFE/common/pkg/opensearch"
)

func arcLogRecord(t *testing.T, scope Scope, when time.Time, message string, legacy bool) commonsearch.OpenSearchLogDoc {
	t.Helper()
	source := map[string]interface{}{"@timestamp": when.Format(time.RFC3339Nano), "kubernetes": map[string]string{
		"namespace_name": common.CICDArcNamespace, "pod_name": commonconfig.GetCICDControllerName() + "-example"}}
	if legacy {
		source["log"] = message
	} else {
		source["message"] = message
	}
	data, err := json.Marshal(map[string]interface{}{"_source": source})
	assert.NilError(t, err)
	var record commonsearch.OpenSearchLogDoc
	assert.NilError(t, json.Unmarshal(data, &record))
	return record
}

func arcErrorMessage(scope Scope, level, message string) string {
	data, _ := json.Marshal(map[string]interface{}{"level": level, "controllerKind": scope.Kind, "namespace": scope.Workspace, "name": scope.Name, "msg": message})
	return string(data)
}

func TestARCControllerLogQuery_Scope(t *testing.T) {
	commonconfig.SetValue("opensearch.enable", "true")
	t.Cleanup(func() { commonconfig.SetValue("opensearch.enable", "false") })
	since := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	until := since.Add(10 * time.Minute)
	scope := Scope{Cluster: "test-cluster", Kind: common.CICDScaleRunnerSetKind, Workspace: "test-workspace", Name: "example-set"}
	query := Query{ListLogInput: ListLogInput{Limit: 20, Order: "desc"}, SinceTime: since, UntilTime: until}
	queries, err := ARCControllerQueries(scope, query)
	assert.NilError(t, err)
	assert.Equal(t, len(queries), 1)
	calls := 0
	stub := commonsearch.NewTestSearchClient(func(start, end time.Time, index, uri string, body []byte) ([]byte, error) {
		calls++
		assert.Equal(t, start, since)
		assert.Equal(t, end, until)
		assert.Equal(t, uri, "/_search")
		text := string(body)
		for _, value := range []string{common.CICDArcNamespace, commonconfig.GetCICDControllerName(), scope.Kind, scope.Workspace, scope.Name, `"order":"desc"`, `"size":20`, `"message"`, `"log"`} {
			assert.Assert(t, strings.Contains(text, value))
		}
		return []byte(`{"hits":{"hits":[{"_source":{"log":"legacy message"}}]}}`), nil
	})
	t.Cleanup(commonsearch.RegisterClientForTest(scope.Cluster, stub))
	responses, err := SearchARCControllerLogs(context.Background(), scope, SearchOptions{Queries: queries, ErrorsOnly: true})
	assert.NilError(t, err)
	assert.Equal(t, calls, 1)
	assert.Equal(t, responses[0].Hits.Hits[0].Source.Message, "legacy message")
	scope.Kind, scope.Name, scope.RunnerID, scope.ScaleSetID = common.CICDEphemeralRunnerKind, "example-child", "associated-runner", "1"
	queries, err = ARCControllerQueries(scope, query)
	assert.NilError(t, err)
	assert.Equal(t, len(queries), 2)
	assert.Equal(t, queries[0].Keywords[2], scope.RunnerID)
	assert.Equal(t, queries[1].Keywords[2], scope.ScaleSetID)
}

func TestARCControllerLogQuery_Attribution(t *testing.T) {
	since := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	until := since.Add(10 * time.Minute)
	for _, kind := range []string{common.CICDScaleRunnerSetKind, common.CICDEphemeralRunnerKind} {
		t.Run(kind, func(t *testing.T) {
			scope := Scope{Kind: kind, Workspace: "test-workspace", Name: "example-runner"}
			match := arcErrorMessage(scope, "error", "proxy connection refused")
			base := arcLogRecord(t, scope, since.Add(time.Minute), match, true)
			for _, tc := range []struct {
				name   string
				change func(*commonsearch.OpenSearchLogDoc)
			}{
				{"prior attempt", func(r *commonsearch.OpenSearchLogDoc) {
					r.Source.Timestamp = since.Add(-time.Second).Format(time.RFC3339Nano)
				}},
				{"future", func(r *commonsearch.OpenSearchLogDoc) {
					r.Source.Timestamp = until.Add(time.Second).Format(time.RFC3339Nano)
				}},
				{"invalid timestamp", func(r *commonsearch.OpenSearchLogDoc) { r.Source.Timestamp = "invalid" }},
				{"prefix name", func(r *commonsearch.OpenSearchLogDoc) {
					other := scope
					other.Name += "-sibling"
					r.Source.Log = arcErrorMessage(other, "error", "different runner")
				}},
				{"workspace", func(r *commonsearch.OpenSearchLogDoc) {
					other := scope
					other.Workspace = "other-workspace"
					r.Source.Log = arcErrorMessage(other, "error", "different workspace")
				}},
				{"info", func(r *commonsearch.OpenSearchLogDoc) { r.Source.Log = arcErrorMessage(scope, "info", "healthy") }},
				{"blank", func(r *commonsearch.OpenSearchLogDoc) { r.Source.Log = " \n" }},
				{"controller", func(r *commonsearch.OpenSearchLogDoc) {
					r.SourceFields["kubernetes"] = map[string]interface{}{"namespace_name": "other-namespace", "pod_name": "unrelated"}
				}},
			} {
				t.Run(tc.name, func(t *testing.T) {
					record := arcLogRecord(t, scope, since.Add(time.Minute), match, true)
					tc.change(&record)
					responses := []commonsearch.OpenSearchLogResponse{{}}
					responses[0].Hits.Hits = []commonsearch.OpenSearchLogDoc{record}
					assert.Equal(t, NewestARCControllerError(scope, responses, since, until), "")
				})
			}
			newer := arcLogRecord(t, scope, until, "2026-01-01 ERROR registration failed {\"controllerKind\":\""+kind+"\",\"namespace\":\"test-workspace\",\"name\":\"example-runner\"}", false)
			responses := []commonsearch.OpenSearchLogResponse{{}}
			responses[0].Hits.Hits = []commonsearch.OpenSearchLogDoc{newer, base}
			assert.Equal(t, NewestARCControllerError(scope, responses, since, until), newer.EffectiveMessage())
		})
	}
}

func TestARCControllerLogQuery_InformationalErrorMentions(t *testing.T) {
	assert.Assert(t, !isARCError(map[string]interface{}{"level": "info", "error": "recovered"}, "informational record"))
	assert.Assert(t, !isARCError(nil, "2026-01-01 INFO previous ERROR was recovered"))
	assert.Assert(t, isARCError(map[string]interface{}{"error": "registration failed"}, "structured error"))
	assert.Assert(t, isARCError(nil, "2026-01-01T01:02:03Z ERROR registration failed"))
}
