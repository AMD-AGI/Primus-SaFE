/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package optimization

import (
	"testing"

	"github.com/stretchr/testify/assert"

	dbclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
)

func TestSplitTableRow(t *testing.T) {
	assert.Equal(t, []string{"a", "b", "c"}, splitTableRow("| a | b | c |"))
	assert.Nil(t, splitTableRow("not a table row"))
}

func TestParseCellAndInt(t *testing.T) {
	assert.Equal(t, 309.94, parseCell("**309.94** tok/s"))
	assert.Equal(t, 1365.65, parseCell("`1,365.65` ms"))
	assert.Equal(t, float64(0), parseCell("n/a"))
	assert.Equal(t, 478, parseCellInt("~478.79"))
}

func TestParseBaselineTable(t *testing.T) {
	content := `
| Metric | Value |
| --- | --- |
| output_throughput | 309.94 tok/s |
| input_throughput | 100.5 tok/s |
| mean_ttft_ms | 1,365.65 ms |
| mean_tpot_ms | 201.24 ms |
`
	b := parseBaselineTable(content)
	assert.NotNil(t, b)
	assert.Equal(t, 309.94, b.OutputTokensPerSec)
	assert.Equal(t, 1, b.Round)
	assert.Equal(t, "Baseline", b.Label)

	// No throughput row -> nil.
	assert.Nil(t, parseBaselineTable("| Metric | Value |\n| foo | 1 |"))
}

func TestParseSweepTable(t *testing.T) {
	content := `
| CONC | ISL | OSL | tok/s | tok/s/GPU | TTFT (ms) | TPOT (ms) |
| --- | --- | --- | --- | --- | --- | --- |
| 128 | 1024 | 1024 | **478.79** | 60 | 2,016 | 259 |
| 256 | 1024 | 1024 | 0 | 0 | 0 | 0 |
`
	rows := parseSweepTable(content)
	assert.Len(t, rows, 1) // zero-tok row skipped
	assert.Equal(t, 478.79, rows[0].OutputTokensPerSec)
	assert.Equal(t, 128, rows[0].Concurrency)
}

func TestParseReportBenchmarks(t *testing.T) {
	content := `
| Metric | Value |
| output_throughput | 309.94 tok/s |

| CONC | ISL | OSL | tok/s |
| --- | --- | --- | --- |
| 128 | 1024 | 1024 | 478.79 |
`
	results := parseReportBenchmarks(content)
	assert.GreaterOrEqual(t, len(results), 2)
}

func TestParseReportKernels(t *testing.T) {
	content := `
| # | Kernel | File | Est GPU% | Optimization Applied |
| --- | --- | --- | --- | --- |
| 1 | ` + "`fused_moe_kernel`" + ` | fused_moe.py | ~35% | applied |
`
	kernels := parseReportKernels(content)
	assert.Len(t, kernels, 1)
	assert.Equal(t, "fused_moe_kernel", kernels[0].Name)
	assert.Equal(t, float64(35), kernels[0].GPUPercent)
}

func TestNextSyntheticSeq(t *testing.T) {
	a := nextSyntheticSeq()
	b := nextSyntheticSeq()
	assert.Greater(t, b, a)
	assert.Greater(t, a, int64(1_000_000_000))
}

func TestHasBenchmarkEvents(t *testing.T) {
	assert.True(t, hasBenchmarkEvents([]*dbclient.OptimizationEvent{{Type: string(EventTypeBenchmark)}}))
	assert.False(t, hasBenchmarkEvents([]*dbclient.OptimizationEvent{{Type: string(EventTypeLog)}}))
}

func TestFirstKernelBackend(t *testing.T) {
	assert.Equal(t, "triton", firstKernelBackend(`["triton","cuda"]`))
	assert.Equal(t, "", firstKernelBackend(`not-json`))
	assert.Equal(t, "", firstKernelBackend(`[]`))
}

func TestTimeAndSeqToHex(t *testing.T) {
	assert.Equal(t, "ff", timeToHex(255))
	assert.Equal(t, "000001", seqToHex(1))
}

func TestPhaseNameAndNewEventID(t *testing.T) {
	assert.Equal(t, "Report", PhaseName(10))
	assert.Equal(t, "", PhaseName(999))

	id := NewEventID("task-1", 5)
	assert.Contains(t, id, "task-1-")
}

func TestSelectLocalPath(t *testing.T) {
	// No local paths recorded -> error.
	_, _, err := selectLocalPath(nil, &dbclient.Model{ID: "m1"}, "ws-1")
	assert.Error(t, err)

	// Exact workspace match.
	m := &dbclient.Model{
		ID:         "m1",
		LocalPaths: `[{"workspace":"ws-1","status":"Ready","path":"/data/m1"}]`,
	}
	path, ws, err := selectLocalPath(nil, m, "ws-1")
	assert.NoError(t, err)
	assert.Equal(t, "/data/m1", path)
	assert.Equal(t, "ws-1", ws)

	// No workspace -> first ready path.
	path2, _, err := selectLocalPath(nil, m, "")
	assert.NoError(t, err)
	assert.Equal(t, "/data/m1", path2)

	// Workspace mismatch + no k8s client -> error.
	_, _, err = selectLocalPath(nil, m, "other-ws")
	assert.Error(t, err)
}
