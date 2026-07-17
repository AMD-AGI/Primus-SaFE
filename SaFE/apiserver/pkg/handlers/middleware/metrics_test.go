// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	ctrlmetrics "sigs.k8s.io/controller-runtime/pkg/metrics"
)

func TestHandleMetricsRecordsRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(HandleMetrics())
	r.GET("/api/v1/workloads/:name", func(c *gin.Context) { c.Status(http.StatusOK) })

	req := httptest.NewRequest(http.MethodGet, "/api/v1/workloads/job-abc", nil)
	r.ServeHTTP(httptest.NewRecorder(), req)

	// handler label must be the route template, not the raw URL (bounded cardinality).
	if v := counterValue(t, "http_requests_total", map[string]string{
		"method": "GET", "code": "200", "handler": "/api/v1/workloads/:name",
	}); v < 1 {
		t.Fatalf("http_requests_total not recorded with route template, got %v", v)
	}
}

func TestHandleMetricsUnmatchedRoute(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(HandleMetrics())

	req := httptest.NewRequest(http.MethodGet, "/nope", nil)
	r.ServeHTTP(httptest.NewRecorder(), req)

	if v := counterValue(t, "http_requests_total", map[string]string{
		"method": "GET", "code": "404", "handler": "unmatched",
	}); v < 1 {
		t.Fatalf("unmatched route should be labeled handler=unmatched, got %v", v)
	}
}

func counterValue(t *testing.T, name string, labels map[string]string) float64 {
	t.Helper()
	mfs, err := ctrlmetrics.Registry.Gather()
	if err != nil {
		t.Fatalf("gather: %v", err)
	}
	for _, mf := range mfs {
		if mf.GetName() != name {
			continue
		}
		for _, m := range mf.GetMetric() {
			seen := map[string]string{}
			for _, lp := range m.GetLabel() {
				seen[lp.GetName()] = lp.GetValue()
			}
			match := true
			for k, v := range labels {
				if seen[k] != v {
					match = false
					break
				}
			}
			if match && m.Counter != nil {
				return m.Counter.GetValue()
			}
		}
	}
	return 0
}
