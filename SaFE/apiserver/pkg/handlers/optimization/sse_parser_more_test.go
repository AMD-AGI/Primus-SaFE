/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package optimization

import (
	"testing"

	"gotest.tools/assert"
)

func TestParseToolUsedUnparsed(t *testing.T) {
	p := NewSSEParser()
	out := p.parseToolUsed(ClawSSEEvent{Event: "tool_used", Data: "{not-json"})
	assert.Equal(t, len(out), 1)
	assert.Equal(t, out[0].Type, EventTypeLog)
}

func TestParseToolUsedIntermediate(t *testing.T) {
	p := NewSSEParser()
	out := p.parseToolUsed(ClawSSEEvent{Data: `{"tool":"foo","status":"running"}`})
	assert.Equal(t, len(out), 1)
	lp := out[0].Payload.(LogEventPayload)
	assert.Equal(t, lp.Message, "foo running")
}

func TestParseToolUsedDefaultWithBrief(t *testing.T) {
	p := NewSSEParser()
	out := p.parseToolUsed(ClawSSEEvent{Data: `{"tool":"edit","status":"success","brief":"edited file"}`})
	assert.Equal(t, len(out), 1)
	lp := out[0].Payload.(LogEventPayload)
	assert.Equal(t, lp.Message, "edited file")
}

func TestParseToolUsedBashScrape(t *testing.T) {
	p := NewSSEParser()
	data := `{"tool":"bash","status":"success","description":"output_throughput: 309.94\nmean_tpot_ms: 6.5"}`
	out := p.parseToolUsed(ClawSSEEvent{Data: data})
	// At least a log plus a benchmark event from the scraped stdout.
	assert.Assert(t, len(out) >= 2)
	foundBench := false
	for _, e := range out {
		if e.Type == EventTypeBenchmark {
			foundBench = true
		}
	}
	assert.Assert(t, foundBench)
}

func TestParseBenchmarkToolWrapper(t *testing.T) {
	p := NewSSEParser()
	data := `{"tool":"benchmark_serving","status":"success","result":{"result":{"output_throughput":420.0,"mean_tpot_ms":5.0}}}`
	out := p.parseToolUsed(ClawSSEEvent{Data: data})
	assert.Equal(t, len(out), 1)
	assert.Equal(t, out[0].Type, EventTypeBenchmark)
	bp := out[0].Payload.(BenchmarkEventPayload)
	assert.Equal(t, bp.OutputTokensPerSec, 420.0)
}

func TestParseBenchmarkToolFallbackText(t *testing.T) {
	p := NewSSEParser()
	data := `{"tool":"benchmark_serving","status":"success","description":"output_throughput: 123.4\nmean_tpot_ms: 7.7\nmean_ttft_ms: 30.1"}`
	out := p.parseToolUsed(ClawSSEEvent{Data: data})
	assert.Equal(t, len(out), 1)
	assert.Equal(t, out[0].Type, EventTypeBenchmark)
	bp := out[0].Payload.(BenchmarkEventPayload)
	assert.Equal(t, bp.OutputTokensPerSec, 123.4)
	assert.Equal(t, bp.TPOTMs, 7.7)
	assert.Equal(t, bp.TTFTMs, 30.1)
}

func TestParseBenchmarkToolNoData(t *testing.T) {
	p := NewSSEParser()
	out := p.parseToolUsed(ClawSSEEvent{Data: `{"tool":"benchmark_serving","status":"success"}`})
	assert.Equal(t, len(out), 0)
}

func TestParseBenchmarkToolUnstructuredLog(t *testing.T) {
	p := NewSSEParser()
	out := p.parseToolUsed(ClawSSEEvent{Data: `{"tool":"benchmark_serving","status":"success","description":"no metrics here"}`})
	assert.Equal(t, len(out), 1)
	assert.Equal(t, out[0].Type, EventTypeLog)
}

func TestScrapeBashOutputDirect(t *testing.T) {
	p := NewSSEParser()
	stdout := "## Phase 1: Classify\noutput_throughput: 200.0\nmean_tpot_ms: 4.0\nSUCCESS: build, 2s -> /work/kernel_opt/k1.py"
	out := p.scrapeBashOutput(stdout)
	var bench, kernel bool
	for _, e := range out {
		switch e.Type {
		case EventTypeBenchmark:
			bench = true
		case EventTypeKernel:
			kernel = true
		}
	}
	assert.Assert(t, bench)
	assert.Assert(t, kernel)

	// Re-scraping the same kernel file is deduped.
	out2 := p.scrapeBashOutput("SUCCESS: build, 2s -> /work/kernel_opt/k1.py")
	for _, e := range out2 {
		assert.Assert(t, e.Type != EventTypeKernel)
	}
}

func TestParseStatusUpdate(t *testing.T) {
	p := NewSSEParser()
	out := p.parseStatusUpdate(ClawSSEEvent{Data: `{"agentStatus":"thinking","brief":"working"}`})
	assert.Equal(t, len(out), 1)
	lp := out[0].Payload.(LogEventPayload)
	assert.Equal(t, lp.Message, "status: thinking working")
}

func TestParsePlanUpdate(t *testing.T) {
	p := NewSSEParser()
	out := p.parsePlanUpdate(ClawSSEEvent{Data: `{"tasks":[{"status":"done","title":"a"},{"status":"todo","title":"b"}]}`})
	assert.Equal(t, len(out), 1)
	lp := out[0].Payload.(LogEventPayload)
	assert.Equal(t, lp.Message, "done:a | todo:b")

	// Invalid JSON -> nil.
	assert.Assert(t, p.parsePlanUpdate(ClawSSEEvent{Data: "{bad"}) == nil)
}

func TestExtractChatTextVariants(t *testing.T) {
	assert.Equal(t, extractChatText(""), "")
	assert.Equal(t, extractChatText("{bad"), "")

	// New data.content[] format.
	assert.Equal(t, extractChatText(`{"data":{"content":[{"type":"text","text":"hi"}]}}`), "hi")
	// Old content.content[] format.
	assert.Equal(t, extractChatText(`{"content":{"content":[{"_type":"text","text":"yo"}]}}`), "yo")
	// Delta format.
	assert.Equal(t, extractChatText(`{"delta":{"content":"d"}}`), "d")
	// Message format.
	assert.Equal(t, extractChatText(`{"message":{"text":"m"}}`), "m")
	// Flat text.
	assert.Equal(t, extractChatText(`{"text":"flat"}`), "flat")
}

func TestParseKernelToolCodex(t *testing.T) {
	p := NewSSEParser()
	data := `{"tool":"mcp__oci-oob-agent__submit_task","status":"success","input":{"kernel_name":"k2","gpu_percent":50}}`
	out := p.parseToolUsed(ClawSSEEvent{Data: data})
	assert.Equal(t, len(out), 1)
	assert.Equal(t, out[0].Type, EventTypeKernel)
	kp := out[0].Payload.(KernelEventPayload)
	assert.Equal(t, kp.Name, "k2")
	assert.Equal(t, kp.Backend, KernelBackendCodex)
	assert.Equal(t, kp.Status, "optimizing")
}

func TestParseErrorAndUnknownEvents(t *testing.T) {
	p := NewSSEParser()
	out := p.Parse(ClawSSEEvent{Event: "error", Data: "boom"})
	assert.Equal(t, len(out), 1)
	lp := out[0].Payload.(LogEventPayload)
	assert.Equal(t, lp.Level, "error")

	out2 := p.Parse(ClawSSEEvent{Event: "weird", Data: "x"})
	assert.Equal(t, len(out2), 1)
	assert.Equal(t, out2[0].Payload.(LogEventPayload).Level, "debug")
}
