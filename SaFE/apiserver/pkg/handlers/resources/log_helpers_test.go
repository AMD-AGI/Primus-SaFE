/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package resources

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	commonsearch "github.com/AMD-AIG-AIMA/SAFE/common/pkg/opensearch"
)

func TestBuildSingleTermFilter(t *testing.T) {
	// Plain term filter.
	req := &commonsearch.OpenSearchRequest{}
	buildSingleTermFilter(req, map[string]string{"app": "svc"}, false, false)
	assert.Len(t, req.Query.Bool.Filter, 1)

	// k8s label + prefix match.
	req2 := &commonsearch.OpenSearchRequest{}
	buildSingleTermFilter(req2, map[string]string{"my.label": "v"}, true, true)
	assert.Len(t, req2.Query.Bool.Filter, 1)

	// Empty key/value is skipped.
	req3 := &commonsearch.OpenSearchRequest{}
	buildSingleTermFilter(req3, map[string]string{"": "v", "k": ""}, false, false)
	assert.Empty(t, req3.Query.Bool.Filter)
}

func TestKeywordMatchAnyField(t *testing.T) {
	field := keywordMatchAnyField("error", 0)
	assert.Contains(t, field, "bool")
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
	assert.NoError(t, err)
	assert.True(t, q.UseK8sLabel)
	assert.Equal(t, "wl-1", q.TermFilters[v1.WorkloadIdLabel])
}

func TestParseServiceLogQuery(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// Missing name -> bad request.
	c0 := newLogCtx()
	_, err := parseServiceLogQuery(c0)
	assert.Error(t, err)

	// With name set.
	c := newLogCtx()
	c.Set(common.Name, "my-svc")
	q, err := parseServiceLogQuery(c)
	assert.NoError(t, err)
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
	assert.NoError(t, err)
	assert.True(t, q.DisableOutput)
	assert.Equal(t, "ws-1", q.TermFilters["involvedObject.namespace"])
	assert.Equal(t, "wl-1", q.PrefixFilters["involvedObject.name"])
}
