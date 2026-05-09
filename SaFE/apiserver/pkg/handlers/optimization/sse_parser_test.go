/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package optimization

import (
	"testing"

	"gotest.tools/assert"
)

func TestSSEParserParsesPhaseFromChat(t *testing.T) {
	parser := NewSSEParser()

	raw := ClawSSEEvent{
		Event: "chat",
		Data: `{
			"content": {
				"content": [
					{ "_type": "TextBlock", "text": "## Phase 2: Baseline Benchmark\nRunning baseline..." }
				]
			}
		}`,
	}

	parsed := parser.Parse(raw)
	assert.Assert(t, len(parsed) >= 2)

	// First event is the raw log.
	logPayload, ok := parsed[0].Payload.(LogEventPayload)
	assert.Assert(t, ok)
	assert.Assert(t, logPayload.Message != "")

	phasePayload, ok := parsed[1].Payload.(PhaseEventPayload)
	assert.Assert(t, ok)
	assert.Equal(t, phasePayload.Phase, 2)
	assert.Equal(t, phasePayload.PhaseName, "Baseline Benchmark")
	assert.Equal(t, phasePayload.Status, "started")
}

func TestSSEParserParsesBenchmarkToolResult(t *testing.T) {
	parser := NewSSEParser()

	raw := ClawSSEEvent{
		Event: "tool_used",
		Data: `{
			"tool": "benchmark_serving",
			"status": "success",
			"brief": "baseline benchmark",
			"result": {
				"output_throughput": 571.3,
				"mean_tpot_ms": 6.78,
				"mean_ttft_ms": 44.12,
				"max_concurrency": 64,
				"random_input_len": 1024,
				"random_output_len": 256,
				"framework": "sglang"
			}
		}`,
	}

	parsed := parser.Parse(raw)
	assert.Assert(t, len(parsed) == 1)
	assert.Equal(t, parsed[0].Type, EventTypeBenchmark)

	payload, ok := parsed[0].Payload.(BenchmarkEventPayload)
	assert.Assert(t, ok)
	assert.Equal(t, payload.Round, 1)
	assert.Equal(t, payload.Label, "baseline benchmark")
	assert.Equal(t, payload.OutputTokensPerSec, 571.3)
	assert.Equal(t, payload.TPOTMs, 6.78)
	assert.Equal(t, payload.TTFTMs, 44.12)
	assert.Equal(t, payload.Concurrency, 64)
	assert.Equal(t, payload.Framework, "sglang")
}

func TestSSEParserParsesKernelToolResult(t *testing.T) {
	parser := NewSSEParser()

	raw := ClawSSEEvent{
		Event: "tool_used",
		Data: `{
			"tool": "mcp__geak__get_outputs",
			"status": "success",
			"result": {
				"kernel_name": "triton_tem_fused_mm_0",
				"status": "patched",
				"baseline_us": 120.5,
				"optimized_us": 88.1
			}
		}`,
	}

	parsed := parser.Parse(raw)
	assert.Assert(t, len(parsed) == 1)
	assert.Equal(t, parsed[0].Type, EventTypeKernel)

	payload, ok := parsed[0].Payload.(KernelEventPayload)
	assert.Assert(t, ok)
	assert.Equal(t, payload.Name, "triton_tem_fused_mm_0")
	assert.Equal(t, payload.Backend, KernelBackendGEAK)
	assert.Equal(t, payload.Status, "patched")
	assert.Equal(t, payload.BaselineUs, 120.5)
	assert.Equal(t, payload.OptimizedUs, 88.1)
}
