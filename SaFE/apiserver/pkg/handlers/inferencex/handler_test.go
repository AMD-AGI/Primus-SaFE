/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package inferencex

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func init() {
	gin.SetMode(gin.TestMode)
}

func newCtx(target string) (*gin.Context, *httptest.ResponseRecorder) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, target, nil)
	return c, w
}

// seed pre-populates the cache so handlers never perform real network calls.
func seed(h *Handler, model string, rows []BenchmarkRow) {
	h.cache.Store(model, &cacheEntry{data: rows, fetchedAt: time.Now()})
}

func sampleRows() []BenchmarkRow {
	return []BenchmarkRow{
		{Hardware: "MI300X", Framework: "vllm", Model: "m", Precision: "fp8", ISL: 128, OSL: 256, DecodeTP: 8, Conc: 4, Date: "2026-01-01",
			Metrics: map[string]interface{}{"tput_per_gpu": 1000.5, "mean_ttft": json.Number("12.3")}},
		{Hardware: "MI300X", Framework: "sglang", Model: "m", Precision: "bf16", ISL: 128, OSL: 256},
		{Hardware: "H100", Framework: "vllm", Model: "m", Precision: "fp8", ISL: 64, OSL: 128},
	}
}

func TestNewHandler(t *testing.T) {
	h := NewHandler(time.Hour)
	if h == nil || h.httpClient == nil || h.ttl != time.Hour {
		t.Fatalf("unexpected handler: %+v", h)
	}
}

func TestGetBenchmarks(t *testing.T) {
	h := NewHandler(time.Hour)
	seed(h, "m", sampleRows())

	// Missing required params.
	c, w := newCtx("/?model=&gpu=")
	h.GetBenchmarks(c)
	if w.Code != http.StatusBadRequest {
		t.Errorf("missing params: status = %d, want 400", w.Code)
	}

	// Success with csv format and precision filter.
	c2, w2 := newCtx("/?model=m&gpu=MI300X&precision=fp8&format=csv")
	h.GetBenchmarks(c2)
	if w2.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w2.Code)
	}
	var resp BenchmarksResponse
	if err := json.Unmarshal(w2.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.TotalCount != 1 || resp.CSV == "" {
		t.Errorf("unexpected response: total=%d csvEmpty=%v", resp.TotalCount, resp.CSV == "")
	}
}

func TestGetFilters(t *testing.T) {
	h := NewHandler(time.Hour)
	// Seed every known model so no network fetch happens.
	for _, m := range []string{
		"DeepSeek-R1-0528", "GLM-5", "gpt-oss-120b", "Llama-3.3-70B-Instruct-FP8",
		"Qwen-3.5-397B-A17B", "Kimi-K2.5", "MiniMax-M2.5",
	} {
		seed(h, m, sampleRows())
	}

	c, w := newCtx("/filters")
	h.GetFilters(c)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	var resp FiltersResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(resp.Models) != 7 || len(resp.GPUs) == 0 {
		t.Errorf("unexpected filters: %+v", resp)
	}
}

func TestGetCachedOrFetch(t *testing.T) {
	h := NewHandler(time.Hour)
	seed(h, "m", sampleRows())
	rows, _, err := h.getCachedOrFetch("m")
	if err != nil || len(rows) != 3 {
		t.Errorf("expected cached rows, got %d (err=%v)", len(rows), err)
	}
}

func TestFilterRows(t *testing.T) {
	rows := sampleRows()
	// GPU filter only.
	if got := filterRows(rows, "MI300X", "", "", "", ""); len(got) != 2 {
		t.Errorf("gpu filter: got %d, want 2", len(got))
	}
	// GPU + isl/osl + framework + precision.
	if got := filterRows(rows, "MI300X", "128", "256", "vllm", "fp8"); len(got) != 1 {
		t.Errorf("full filter: got %d, want 1", len(got))
	}
	// Non-matching isl.
	if got := filterRows(rows, "MI300X", "999", "", "", ""); len(got) != 0 {
		t.Errorf("isl filter: got %d, want 0", len(got))
	}
}

func TestToCSV(t *testing.T) {
	csv := toCSV("display-model", sampleRows())
	if !strings.Contains(csv, "display-model") || !strings.Contains(csv, csvColumns) {
		t.Errorf("csv missing expected content: %s", csv)
	}
}

func TestGetFloat(t *testing.T) {
	m := map[string]interface{}{
		"a": float64(1.5),
		"b": json.Number("2.5"),
		"c": "not-a-number",
	}
	if got := getFloat(m, "a"); got != 1.5 {
		t.Errorf("float64: got %v", got)
	}
	if got := getFloat(m, "b"); got != 2.5 {
		t.Errorf("json.Number: got %v", got)
	}
	if got := getFloat(m, "c"); got != 0 {
		t.Errorf("string: got %v, want 0", got)
	}
	if got := getFloat(m, "missing"); got != 0 {
		t.Errorf("missing: got %v, want 0", got)
	}
}

func TestSetToSlice(t *testing.T) {
	s := map[string]bool{"x": true, "y": true}
	if got := setToSlice(s); len(got) != 2 {
		t.Errorf("got %d, want 2", len(got))
	}
	if got := setToSlice(map[string]bool{}); len(got) != 0 {
		t.Errorf("empty: got %d, want 0", len(got))
	}
}

func TestInitInferenceXRouters(t *testing.T) {
	engine := gin.New()
	InitInferenceXRouters(engine, NewHandler(time.Hour))
	if len(engine.Routes()) < 2 {
		t.Errorf("expected at least 2 routes, got %d", len(engine.Routes()))
	}
}
