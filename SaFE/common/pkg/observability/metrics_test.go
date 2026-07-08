/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package observability

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func newStubBackend(t *testing.T, path, payload string) (*MetricsClient, func()) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != path {
			http.Error(w, "unexpected path "+r.URL.Path, http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(payload))
	}))
	client := NewMetricsClient(MetricsClientConfig{BaseURL: srv.URL})
	return client, srv.Close
}

func TestQueryInstantVector(t *testing.T) {
	payload := `{"status":"success","data":{"resultType":"vector","result":[
		{"metric":{"node":"gpu-1"},"value":[1700000000,"87.5"]},
		{"metric":{"node":"gpu-2"},"value":[1700000000,"90"]}
	]}}`
	client, closeFn := newStubBackend(t, "/api/v1/query", payload)
	defer closeFn()

	samples, err := client.QueryInstant(context.Background(), `avg(gpu_utilization) by (node)`)
	if err != nil {
		t.Fatalf("QueryInstant: %v", err)
	}
	if len(samples) != 2 {
		t.Fatalf("expected 2 samples, got %d", len(samples))
	}
	if samples[0].Metric["node"] != "gpu-1" || samples[0].Value != 87.5 {
		t.Fatalf("unexpected first sample: %+v", samples[0])
	}
}

func TestQueryInstantScalar(t *testing.T) {
	payload := `{"status":"success","data":{"resultType":"scalar","result":[1700000000,"42.25"]}}`
	client, closeFn := newStubBackend(t, "/api/v1/query", payload)
	defer closeFn()

	v, err := client.QueryInstantScalar(context.Background(), `sum(gpu_allocated)`)
	if err != nil {
		t.Fatalf("QueryInstantScalar: %v", err)
	}
	if v != 42.25 {
		t.Fatalf("expected 42.25, got %v", v)
	}
}

func TestQueryInstantScalarNoData(t *testing.T) {
	payload := `{"status":"success","data":{"resultType":"vector","result":[]}}`
	client, closeFn := newStubBackend(t, "/api/v1/query", payload)
	defer closeFn()

	if _, err := client.QueryInstantScalar(context.Background(), `avg(missing_metric)`); err == nil {
		t.Fatal("expected error for empty result, got nil")
	}
}

func TestQueryRangeMatrix(t *testing.T) {
	payload := `{"status":"success","data":{"resultType":"matrix","result":[
		{"metric":{"node":"gpu-1"},"values":[[1700000000,"10"],[1700000015,"20"]]}
	]}}`
	client, closeFn := newStubBackend(t, "/api/v1/query_range", payload)
	defer closeFn()

	end := time.Unix(1700000015, 0)
	start := time.Unix(1700000000, 0)
	series, err := client.QueryRange(context.Background(), `avg(gpu_utilization)`, start, end, 15*time.Second)
	if err != nil {
		t.Fatalf("QueryRange: %v", err)
	}
	if len(series) != 1 || len(series[0].Points) != 2 {
		t.Fatalf("unexpected series: %+v", series)
	}
	if series[0].Points[1].Value != 20 {
		t.Fatalf("unexpected second point: %+v", series[0].Points[1])
	}
}

func TestQueryInstantError(t *testing.T) {
	payload := `{"status":"error","errorType":"bad_data","error":"invalid query"}`
	client, closeFn := newStubBackend(t, "/api/v1/query", payload)
	defer closeFn()

	if _, err := client.QueryInstant(context.Background(), `bogus{`); err == nil {
		t.Fatal("expected error for error-status response, got nil")
	}
}

func TestRegistry(t *testing.T) {
	reg := NewMetricsRegistry(MetricsClientConfig{})
	reg.RegisterCluster("core42", "http://vmselect:8481/select/0/prometheus")
	if reg.ForCluster("core42") == nil {
		t.Fatal("expected client for core42")
	}
	if reg.ForCluster("missing") != nil {
		t.Fatal("expected nil for unknown cluster")
	}
	if names := reg.ClusterNames(); len(names) != 1 || names[0] != "core42" {
		t.Fatalf("unexpected cluster names: %v", names)
	}
	reg.RemoveCluster("core42")
	if reg.ForCluster("core42") != nil {
		t.Fatal("expected nil after removal")
	}
}
