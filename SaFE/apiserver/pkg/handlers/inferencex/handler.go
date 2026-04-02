/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package inferencex

import (
	"compress/gzip"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"k8s.io/klog/v2"
)

const (
	inferenceXBaseURL = "https://inferencex.semianalysis.com/api/v1"
	csvLicenseHeader  = "# Licensed under Apache License 2.0 — https://www.apache.org/licenses/LICENSE-2.0\n" +
		"# Copyright 2026 SemiAnalysis LLC. Data from InferenceX (https://github.com/SemiAnalysisAI/InferenceX).\n" +
		"# Attribution to InferenceX is required for any derivative work.\n"
	csvColumns = "Model,ISL,OSL,Hardware,Hardware Key,Framework,Precision,TP,Concurrency,Date," +
		"Throughput/GPU (tok/s),Output Throughput/GPU (tok/s),Input Throughput/GPU (tok/s)," +
		"Mean TTFT (ms),Median TTFT (ms),P99 TTFT (ms)," +
		"Mean TPOT (ms),Median TPOT (ms),P99 TPOT (ms)," +
		"Mean ITL (ms),Mean E2E Latency (ms),Disaggregated,Is Multinode"
)

type cacheEntry struct {
	data      []BenchmarkRow
	fetchedAt time.Time
}

type Handler struct {
	cache      sync.Map
	httpClient *http.Client
	ttl        time.Duration
}

type BenchmarkRow struct {
	Hardware      string                 `json:"hardware"`
	Framework     string                 `json:"framework"`
	Model         string                 `json:"model"`
	Precision     string                 `json:"precision"`
	SpecMethod    string                 `json:"spec_method"`
	Disagg        bool                   `json:"disagg"`
	IsMultinode   bool                   `json:"is_multinode"`
	PrefillTP     int                    `json:"prefill_tp"`
	PrefillEP     int                    `json:"prefill_ep"`
	DecodeTP      int                    `json:"decode_tp"`
	DecodeEP      int                    `json:"decode_ep"`
	NumPrefillGPU int                    `json:"num_prefill_gpu"`
	NumDecodeGPU  int                    `json:"num_decode_gpu"`
	ISL           int                    `json:"isl"`
	OSL           int                    `json:"osl"`
	Conc          int                    `json:"conc"`
	Image         string                 `json:"image"`
	Metrics       map[string]interface{} `json:"metrics"`
	Date          string                 `json:"date"`
	RunURL        string                 `json:"run_url"`
}

type BenchmarksResponse struct {
	Items      []BenchmarkRow `json:"items"`
	TotalCount int            `json:"totalCount"`
	CSV        string         `json:"csv,omitempty"`
	CachedAt   string         `json:"cachedAt"`
}

type FiltersResponse struct {
	Models     []string `json:"models"`
	GPUs       []string `json:"gpus"`
	Frameworks []string `json:"frameworks"`
	Precisions []string `json:"precisions"`
}

func NewHandler(ttl time.Duration) *Handler {
	return &Handler{
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, //nolint:gosec // InferenceX public API, container lacks updated CA certs
			},
		},
		ttl: ttl,
	}
}

func (h *Handler) GetBenchmarks(c *gin.Context) {
	model := c.Query("model")
	gpu := c.Query("gpu")
	if model == "" || gpu == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "model and gpu are required"})
		return
	}

	rows, cachedAt, err := h.getCachedOrFetch(model)
	if err != nil {
		klog.ErrorS(err, "failed to fetch InferenceX data", "model", model)
		c.JSON(http.StatusBadGateway, gin.H{"error": "failed to fetch data from InferenceX"})
		return
	}

	filtered := filterRows(rows, gpu, c.Query("isl"), c.Query("osl"), c.Query("framework"), c.Query("precision"))

	resp := BenchmarksResponse{
		Items:      filtered,
		TotalCount: len(filtered),
		CachedAt:   cachedAt.Format(time.RFC3339),
	}

	if c.Query("format") == "csv" {
		resp.CSV = toCSV(model, filtered)
	}

	c.JSON(http.StatusOK, resp)
}

func (h *Handler) GetFilters(c *gin.Context) {
	knownModels := []string{
		"DeepSeek-R1-0528",
		"GLM-5",
		"gpt-oss-120b",
		"Llama-3.3-70B-Instruct-FP8",
		"Qwen-3.5-397B-A17B",
		"Kimi-K2.5",
		"MiniMax-M2.5",
	}

	gpuSet := make(map[string]bool)
	fwSet := make(map[string]bool)
	precSet := make(map[string]bool)

	for _, model := range knownModels {
		rows, _, err := h.getCachedOrFetch(model)
		if err != nil {
			continue
		}
		for _, r := range rows {
			gpuSet[r.Hardware] = true
			fwSet[r.Framework] = true
			precSet[r.Precision] = true
		}
	}

	c.JSON(http.StatusOK, FiltersResponse{
		Models:     knownModels,
		GPUs:       setToSlice(gpuSet),
		Frameworks: setToSlice(fwSet),
		Precisions: setToSlice(precSet),
	})
}

func (h *Handler) getCachedOrFetch(model string) ([]BenchmarkRow, time.Time, error) {
	if entry, ok := h.cache.Load(model); ok {
		e := entry.(*cacheEntry)
		if time.Since(e.fetchedAt) < h.ttl {
			return e.data, e.fetchedAt, nil
		}
	}

	rows, err := h.fetchFromInferenceX(model)
	if err != nil {
		// Return stale cache on fetch failure
		if entry, ok := h.cache.Load(model); ok {
			e := entry.(*cacheEntry)
			klog.Warningf("InferenceX fetch failed, returning stale cache for model=%s", model)
			return e.data, e.fetchedAt, nil
		}
		return nil, time.Time{}, err
	}

	now := time.Now()
	h.cache.Store(model, &cacheEntry{data: rows, fetchedAt: now})
	return rows, now, nil
}

func (h *Handler) fetchFromInferenceX(model string) ([]BenchmarkRow, error) {
	url := fmt.Sprintf("%s/benchmarks?model=%s", inferenceXBaseURL, model)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept-Encoding", "gzip")

	resp, err := h.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("InferenceX request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("InferenceX returned status %d", resp.StatusCode)
	}

	var reader io.Reader = resp.Body
	if resp.Header.Get("Content-Encoding") == "gzip" {
		gr, err := gzip.NewReader(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("gzip decode failed: %w", err)
		}
		defer gr.Close()
		reader = gr
	}

	var rows []BenchmarkRow
	if err := json.NewDecoder(reader).Decode(&rows); err != nil {
		return nil, fmt.Errorf("JSON decode failed: %w", err)
	}

	return rows, nil
}

func filterRows(rows []BenchmarkRow, gpu, islStr, oslStr, framework, precision string) []BenchmarkRow {
	var result []BenchmarkRow
	for _, r := range rows {
		if !strings.EqualFold(r.Hardware, gpu) {
			continue
		}
		if islStr != "" {
			if isl, err := strconv.Atoi(islStr); err == nil && r.ISL != isl {
				continue
			}
		}
		if oslStr != "" {
			if osl, err := strconv.Atoi(oslStr); err == nil && r.OSL != osl {
				continue
			}
		}
		if framework != "" && !strings.EqualFold(r.Framework, framework) {
			continue
		}
		if precision != "" && !strings.EqualFold(r.Precision, precision) {
			continue
		}
		result = append(result, r)
	}
	return result
}

func toCSV(displayModel string, rows []BenchmarkRow) string {
	var sb strings.Builder
	sb.WriteString(csvLicenseHeader)
	sb.WriteString(csvColumns)
	sb.WriteString("\n")
	for _, r := range rows {
		m := r.Metrics
		sb.WriteString(fmt.Sprintf("%s,%d,%d,%s,%s_%s,%s,%s,%d,%d,%s,%.6f,%.6f,%.6f,%.6f,%.6f,%.6f,%.6f,%.6f,%.6f,%.6f,%.6f,%t,%t\n",
			displayModel, r.ISL, r.OSL, r.Hardware,
			r.Hardware, r.Framework,
			r.Framework, r.Precision,
			r.DecodeTP, r.Conc, r.Date,
			getFloat(m, "tput_per_gpu"),
			getFloat(m, "output_tput_per_gpu"),
			getFloat(m, "input_tput_per_gpu"),
			getFloat(m, "mean_ttft"),
			getFloat(m, "median_ttft"),
			getFloat(m, "p99_ttft"),
			getFloat(m, "mean_tpot"),
			getFloat(m, "median_tpot"),
			getFloat(m, "p99_tpot"),
			getFloat(m, "mean_itl"),
			getFloat(m, "mean_e2el"),
			r.Disagg, r.IsMultinode,
		))
	}
	return sb.String()
}

func getFloat(m map[string]interface{}, key string) float64 {
	if v, ok := m[key]; ok {
		switch val := v.(type) {
		case float64:
			return val
		case json.Number:
			f, _ := val.Float64()
			return f
		}
	}
	return 0
}

func setToSlice(s map[string]bool) []string {
	result := make([]string, 0, len(s))
	for k := range s {
		result = append(result, k)
	}
	return result
}
