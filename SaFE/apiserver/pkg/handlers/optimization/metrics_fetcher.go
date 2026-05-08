/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package optimization

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync/atomic"

	"k8s.io/klog/v2"

	dbclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
)

// ciMetrics is the schema written by the Hyperloom pipeline to ci_metrics.json.
// Only the fields we display in the UI are decoded; the rest are ignored.
type ciMetrics struct {
	Baseline  *ciRun  `json:"baseline"`
	Optimized *ciRun  `json:"optimized"`
	Summary   *ciRun  `json:"summary"`
	Framework string  `json:"framework"`
	ISL       int     `json:"isl"`
	OSL       int     `json:"osl"`
	CONC      int     `json:"concurrency"`
}

type ciRun struct {
	Label              string  `json:"label"`
	OutputTokensPerSec float64 `json:"output_throughput"`
	InputTokensPerSec  float64 `json:"input_throughput"`
	TotalTokensPerSec  float64 `json:"total_throughput"`
	TPOTMs             float64 `json:"mean_tpot_ms"`
	TTFTMs             float64 `json:"mean_ttft_ms"`
	Concurrency        int     `json:"concurrency"`
	ISL                int     `json:"isl"`
	OSL                int     `json:"osl"`
	Framework          string  `json:"framework"`
}

// kernelCandidate maps to a single entry in kernel_candidates.json.
type kernelCandidate struct {
	Name        string  `json:"name"`
	GPUPercent  float64 `json:"gpu_pct"`
	Count       int     `json:"count"`
	AvgUs       float64 `json:"avg_us"`
	IsVendor    bool    `json:"is_vendor"`
	IsCandidate bool    `json:"is_candidate"`
}

// fetchAndInjectMetrics is called after a task finishes. It walks the Claw
// session artifact list looking for ci_metrics.json and kernel_candidates.json,
// parses them, and injects synthetic benchmark/kernel events into the DB so
// the Detail page can display them without requiring a live SSE session.
//
// Errors are logged but never returned — metrics are best-effort enrichment.
func (h *Handler) fetchAndInjectMetrics(ctx context.Context, task *dbclient.OptimizationTask) {
	if task.ClawSessionID == "" {
		return
	}

	clawCtx := WithClawBearer(ctx, h.clawClient.apiKey)

	items, err := h.clawClient.ListSessionFiles(clawCtx, task.ClawSessionID)
	if err != nil {
		klog.V(4).InfoS("fetchAndInjectMetrics: list session files failed",
			"task_id", task.ID, "error", err)
		return
	}

	var ciPath, kernelPath string
	for _, item := range items {
		base := item.Path
		if idx := strings.LastIndex(base, "/"); idx >= 0 {
			base = base[idx+1:]
		}
		switch base {
		case "ci_metrics.json":
			if ciPath == "" {
				ciPath = item.Path
			}
		case "kernel_candidates.json":
			if kernelPath == "" {
				kernelPath = item.Path
			}
		}
	}

	injected := false

	if ciPath != "" {
		if err := h.injectBenchmarkEvents(clawCtx, task, ciPath); err != nil {
			klog.V(4).InfoS("fetchAndInjectMetrics: benchmark inject failed",
				"task_id", task.ID, "path", ciPath, "error", err)
		} else {
			injected = true
		}
	}

	if kernelPath != "" {
		if err := h.injectKernelEvents(clawCtx, task, kernelPath); err != nil {
			klog.V(4).InfoS("fetchAndInjectMetrics: kernel inject failed",
				"task_id", task.ID, "path", kernelPath, "error", err)
		} else {
			injected = true
		}
	}

	if injected {
		klog.InfoS("fetchAndInjectMetrics: metrics injected",
			"task_id", task.ID, "ci_path", ciPath, "kernel_path", kernelPath)
	} else {
		klog.V(4).InfoS("fetchAndInjectMetrics: no ci_metrics.json or kernel_candidates.json found",
			"task_id", task.ID, "artifacts", len(items))
	}
}

func (h *Handler) injectBenchmarkEvents(ctx context.Context, task *dbclient.OptimizationTask, path string) error {
	content, err := h.clawClient.ReadSessionFile(ctx, task.ClawSessionID, path)
	if err != nil {
		return fmt.Errorf("read %s: %w", path, err)
	}

	var m ciMetrics
	if err := json.Unmarshal(content, &m); err != nil {
		return fmt.Errorf("parse ci_metrics.json: %w", err)
	}

	round := 1
	for _, run := range []*ciRun{m.Baseline, m.Optimized, m.Summary} {
		if run == nil {
			continue
		}
		if run.OutputTokensPerSec == 0 && run.TotalTokensPerSec == 0 {
			continue
		}
		label := run.Label
		if label == "" {
			switch round {
			case 1:
				label = "Baseline"
			case 2:
				label = "Optimized"
			default:
				label = fmt.Sprintf("Round %d", round)
			}
		}
		fw := firstNonEmpty(run.Framework, m.Framework, task.Framework)
		isl := run.ISL
		if isl == 0 {
			isl = firstPositive(m.ISL, task.ISL)
		}
		osl := run.OSL
		if osl == 0 {
			osl = firstPositive(m.OSL, task.OSL)
		}
		conc := run.Concurrency
		if conc == 0 {
			conc = firstPositive(m.CONC, task.Concurrency)
		}
		payload := BenchmarkEventPayload{
			Round:              round,
			Label:              label,
			OutputTokensPerSec: run.OutputTokensPerSec,
			InputTokensPerSec:  run.InputTokensPerSec,
			TotalTokensPerSec:  run.TotalTokensPerSec,
			TPOTMs:             run.TPOTMs,
			TTFTMs:             run.TTFTMs,
			Concurrency:        conc,
			ISL:                isl,
			OSL:                osl,
			Framework:          fw,
		}
		if err := h.appendSyntheticEvent(task.ID, EventTypeBenchmark, payload); err != nil {
			klog.V(4).InfoS("injectBenchmarkEvents: append failed", "task_id", task.ID, "error", err)
		}
		round++
	}
	return nil
}

func (h *Handler) injectKernelEvents(ctx context.Context, task *dbclient.OptimizationTask, path string) error {
	content, err := h.clawClient.ReadSessionFile(ctx, task.ClawSessionID, path)
	if err != nil {
		return fmt.Errorf("read %s: %w", path, err)
	}

	var candidates []kernelCandidate
	if err := json.Unmarshal(content, &candidates); err != nil {
		return fmt.Errorf("parse kernel_candidates.json: %w", err)
	}

	for _, c := range candidates {
		payload := KernelEventPayload{
			Name:       c.Name,
			GPUPercent: c.GPUPercent,
			BaselineUs: c.AvgUs,
			Backend:    firstNonEmpty(firstKernelBackend(task.KernelBackends), "Claude Code"),
			Status:     "patched",
		}
		if err := h.appendSyntheticEvent(task.ID, EventTypeKernel, payload); err != nil {
			klog.V(4).InfoS("injectKernelEvents: append failed", "task_id", task.ID, "error", err)
		}
	}
	return nil
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

// ── small helpers ────────────────────────────────────────────────────────────

var syntheticSeq atomic.Int64

func nextSyntheticSeq() int64 {
	return 1_000_000_000 + syntheticSeq.Add(1) // well above any live seq
}

func firstPositive(vals ...int) int {
	for _, v := range vals {
		if v > 0 {
			return v
		}
	}
	return 0
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
