/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package optimization

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"sync/atomic"

	"k8s.io/klog/v2"

	dbclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
)

// fetchAndInjectMetrics is called after a task finishes (or on first replay of
// a succeeded task with no benchmark events). It reads the optimization report
// from the Claw session artifacts, parses benchmark and kernel data from the
// markdown tables, and injects them as synthetic events into the DB so the
// Detail page can display them without requiring a live SSE session.
//
// Errors are logged but never returned — metrics are best-effort enrichment.
func (h *Handler) fetchAndInjectMetrics(ctx context.Context, task *dbclient.OptimizationTask) {
	if task.ClawSessionID == "" {
		return
	}

	clawCtx := WithClawBearer(ctx, h.clawBearerForTask(ctx, task.UserID, task.UserName))

	// Locate the optimization report in session artifacts.
	reportPath := task.ReportPath
	if reportPath == "" {
		items, err := h.clawClient.ListSessionFiles(clawCtx, task.ClawSessionID)
		if err != nil {
			klog.V(4).InfoS("fetchAndInjectMetrics: list session files failed",
				"task_id", task.ID, "error", err)
			return
		}
		for _, item := range items {
			if looksLikeOptimizationReport(item.Path) {
				reportPath = item.Path
				_ = h.dbClient.UpdateOptimizationTaskResult(context.Background(), task.ID, task.FinalMetrics, reportPath)
				break
			}
		}
	}
	if reportPath == "" {
		klog.V(4).InfoS("fetchAndInjectMetrics: no optimization report found",
			"task_id", task.ID)
		return
	}

	content, err := h.clawClient.ReadSessionFile(clawCtx, task.ClawSessionID, reportPath)
	if err != nil {
		klog.V(4).InfoS("fetchAndInjectMetrics: read report failed",
			"task_id", task.ID, "path", reportPath, "error", err)
		return
	}

	injected := 0
	report := string(content)

	for _, payload := range parseReportBenchmarks(report) {
		if err := h.appendSyntheticEvent(task.ID, EventTypeBenchmark, payload); err != nil {
			klog.V(4).InfoS("fetchAndInjectMetrics: append benchmark failed",
				"task_id", task.ID, "error", err)
		} else {
			injected++
		}
	}

	for _, payload := range parseReportKernels(report) {
		if err := h.appendSyntheticEvent(task.ID, EventTypeKernel, payload); err != nil {
			klog.V(4).InfoS("fetchAndInjectMetrics: append kernel failed",
				"task_id", task.ID, "error", err)
		} else {
			injected++
		}
	}

	if injected > 0 {
		klog.InfoS("fetchAndInjectMetrics: injected from report",
			"task_id", task.ID, "count", injected, "report", reportPath)
	} else {
		klog.V(4).InfoS("fetchAndInjectMetrics: no metrics extracted from report",
			"task_id", task.ID, "report", reportPath)
	}
}

// appendSyntheticEvent persists a synthetic event to the DB only (no hub
// broadcast — the task stream is already closed at this point).
func (h *Handler) appendSyntheticEvent(taskID string, evType EventType, payload interface{}) error {
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	seq := nextSyntheticSeq()
	dbev := &dbclient.OptimizationEvent{
		EventID:   fmt.Sprintf("%s-m-%d", taskID, seq),
		TaskID:    taskID,
		Type:      string(evType),
		Payload:   string(raw),
		Seq:       seq,
		Timestamp: nowMillis(),
	}
	return h.dbClient.AppendOptimizationEvent(context.Background(), dbev)
}

// ── Report parsing ────────────────────────────────────────────────────────────

var mdTableSepRegex = regexp.MustCompile(`^\|[-:\s|]+\|$`)

// splitTableRow splits "| a | b | c |" into ["a", "b", "c"].
func splitTableRow(line string) []string {
	line = strings.TrimSpace(line)
	if !strings.HasPrefix(line, "|") || !strings.HasSuffix(line, "|") {
		return nil
	}
	parts := strings.Split(line[1:len(line)-1], "|")
	cells := make([]string, 0, len(parts))
	for _, p := range parts {
		cells = append(cells, strings.TrimSpace(p))
	}
	return cells
}

// parseCell extracts the first float from a markdown cell, stripping bold
// markers, backticks, commas, and units (tok/s, ms, %, ~).
func parseCell(cell string) float64 {
	cell = strings.ReplaceAll(cell, "**", "")
	cell = strings.ReplaceAll(cell, "`", "")
	cell = strings.ReplaceAll(cell, ",", "")
	cell = strings.ReplaceAll(cell, "~", "")
	m := regexp.MustCompile(`([0-9]+(?:\.[0-9]+)?)`).FindStringSubmatch(cell)
	if m == nil {
		return 0
	}
	v, _ := strconv.ParseFloat(m[1], 64)
	return v
}

func parseCellInt(cell string) int {
	return int(parseCell(cell))
}

// parseReportBenchmarks extracts BenchmarkEventPayloads from the report.
// It combines the Phase 1 key-value baseline table and the Phase 5 sweep table.
func parseReportBenchmarks(content string) []BenchmarkEventPayload {
	var results []BenchmarkEventPayload

	if b := parseBaselineTable(content); b != nil {
		results = append(results, *b)
	}

	sweepOffset := len(results) + 1
	for i, s := range parseSweepTable(content) {
		s.Round = sweepOffset + i
		results = append(results, s)
	}

	return results
}

// parseBaselineTable finds the Phase 1 key-value benchmark table.
// It looks for a table containing an "output_throughput" metric row.
//
//	| Metric              | Value          |
//	| output_throughput   | 309.94 tok/s   |
//	| mean_ttft_ms        | 1,365.65 ms    |
//	| mean_tpot_ms        | 201.24 ms      |
func parseBaselineTable(content string) *BenchmarkEventPayload {
	var payload BenchmarkEventPayload
	found := false

	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "|") || mdTableSepRegex.MatchString(trimmed) {
			continue
		}
		cells := splitTableRow(trimmed)
		if len(cells) < 2 {
			continue
		}
		key := strings.ToLower(strings.ReplaceAll(cells[0], "**", ""))
		val := cells[1]
		switch {
		case strings.Contains(key, "output_throughput"):
			payload.OutputTokensPerSec = parseCell(val)
			found = true
		case strings.Contains(key, "input_throughput"):
			payload.InputTokensPerSec = parseCell(val)
		case strings.Contains(key, "total_throughput"):
			payload.TotalTokensPerSec = parseCell(val)
		case strings.Contains(key, "mean_tpot_ms"):
			payload.TPOTMs = parseCell(val)
		case strings.Contains(key, "mean_ttft_ms"):
			payload.TTFTMs = parseCell(val)
		}
	}

	if !found || payload.OutputTokensPerSec == 0 {
		return nil
	}
	payload.Round = 1
	payload.Label = "Baseline"
	return &payload
}

// parseSweepTable finds the Phase 5 sweep table with columnar CONC/ISL/OSL/tok/s headers.
//
//	| CONC | ISL  | OSL  | tok/s      | tok/s/GPU | TTFT (ms) | TPOT (ms) |
//	| 128  | 1024 | 1024 | **478.79** | ...       | 2,016     | 259       |
func parseSweepTable(content string) []BenchmarkEventPayload {
	var results []BenchmarkEventPayload
	colConc, colISL, colOSL, colTok, colTTFT, colTPOT := -1, -1, -1, -1, -1, -1
	headerFound := false

	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "|") {
			if headerFound && len(results) > 0 {
				break
			}
			continue
		}
		if mdTableSepRegex.MatchString(trimmed) {
			continue
		}
		cells := splitTableRow(trimmed)
		if len(cells) == 0 {
			continue
		}

		if !headerFound {
			for i, c := range cells {
				cl := strings.ToLower(strings.ReplaceAll(c, "**", ""))
				switch {
				case strings.Contains(cl, "conc"):
					colConc = i
				case cl == "isl":
					colISL = i
				case cl == "osl":
					colOSL = i
				case strings.Contains(cl, "tok/s") && !strings.Contains(cl, "gpu"):
					colTok = i
				case strings.Contains(cl, "ttft"):
					colTTFT = i
				case strings.Contains(cl, "tpot"):
					colTPOT = i
				}
			}
			if colConc >= 0 && colTok >= 0 {
				headerFound = true
			}
			continue
		}

		if colTok < 0 || len(cells) <= colTok {
			continue
		}
		tok := parseCell(cells[colTok])
		if tok <= 0 {
			continue // OOM / failed row
		}

		payload := BenchmarkEventPayload{OutputTokensPerSec: tok}
		if colConc >= 0 && len(cells) > colConc {
			payload.Concurrency = parseCellInt(cells[colConc])
		}
		if colISL >= 0 && len(cells) > colISL {
			payload.ISL = parseCellInt(cells[colISL])
		}
		if colOSL >= 0 && len(cells) > colOSL {
			payload.OSL = parseCellInt(cells[colOSL])
		}
		if colTTFT >= 0 && len(cells) > colTTFT {
			payload.TTFTMs = parseCell(cells[colTTFT])
		}
		if colTPOT >= 0 && len(cells) > colTPOT {
			payload.TPOTMs = parseCell(cells[colTPOT])
		}
		if payload.Concurrency > 0 {
			payload.Label = fmt.Sprintf("CONC=%d ISL=%d OSL=%d", payload.Concurrency, payload.ISL, payload.OSL)
		}
		results = append(results, payload)
	}

	return results
}

// parseReportKernels finds the Phase 4 kernel table.
//
//	| # | Kernel                       | File         | Est GPU% | Optimization Applied |
//	| 1 | `fused_moe_kernel_gptq_awq`  | fused_moe.py | ~35%     | ...                  |
func parseReportKernels(content string) []KernelEventPayload {
	var results []KernelEventPayload
	colKernel, colGPU := -1, -1
	headerFound := false

	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "|") {
			if headerFound && len(results) > 0 {
				break
			}
			continue
		}
		if mdTableSepRegex.MatchString(trimmed) {
			continue
		}
		cells := splitTableRow(trimmed)
		if len(cells) == 0 {
			continue
		}

		if !headerFound {
			for i, c := range cells {
				cl := strings.ToLower(c)
				if cl == "kernel" || strings.Contains(cl, "kernel name") {
					colKernel = i
				} else if strings.Contains(cl, "gpu%") || strings.Contains(cl, "gpu pct") || strings.Contains(cl, "est gpu") {
					colGPU = i
				}
			}
			if colKernel >= 0 {
				headerFound = true
			}
			continue
		}

		if len(cells) <= colKernel {
			continue
		}
		name := strings.ReplaceAll(cells[colKernel], "`", "")
		name = strings.ReplaceAll(name, "**", "")
		name = strings.TrimSpace(name)
		if name == "" || name == "-" {
			continue
		}

		payload := KernelEventPayload{
			Name:    name,
			Backend: KernelBackendClaude,
			Status:  "patched",
		}
		if colGPU >= 0 && len(cells) > colGPU {
			payload.GPUPercent = parseCell(cells[colGPU])
		}
		results = append(results, payload)
	}

	return results
}

// ── small helpers ────────────────────────────────────────────────────────────

var syntheticSeq atomic.Int64

func nextSyntheticSeq() int64 {
	return 1_000_000_000 + syntheticSeq.Add(1) // well above any live seq
}

func hasBenchmarkEvents(events []*dbclient.OptimizationEvent) bool {
	for _, e := range events {
		if e.Type == string(EventTypeBenchmark) {
			return true
		}
	}
	return false
}

func firstKernelBackend(backends string) string {
	var list []string
	if err := json.Unmarshal([]byte(backends), &list); err == nil && len(list) > 0 {
		return list[0]
	}
	return ""
}
