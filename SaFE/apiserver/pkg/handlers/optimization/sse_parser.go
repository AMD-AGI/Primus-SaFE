/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package optimization

import (
	"encoding/json"
	"regexp"
	"strconv"
	"strings"
	"sync"

	"k8s.io/klog/v2"
)

// SSEParser translates raw Claw SSE events (which carry Claude Agent SDK
// messages, planUpdate entries, tool invocations, etc.) into the structured
// Event envelopes that Model Optimization clients consume.
//
// The parser is a pure state machine: it takes raw frames in, emits zero or
// more structured events, and keeps a small internal state (current phase,
// benchmark round counter, seen kernel names) to suppress duplicates.
//
// Parsing is best-effort and fail-soft: anything that isn't recognized as a
// phase/benchmark/kernel signal is forwarded as a LogEvent so clients never
// lose information.
type SSEParser struct {
	mu           sync.Mutex
	currentPhase int
	round        int
	kernelSeen   map[string]struct{}
}

// NewSSEParser creates an empty parser. Callers typically allocate one per
// optimization task so state doesn't leak between runs.
func NewSSEParser() *SSEParser {
	return &SSEParser{
		currentPhase: -1,
		kernelSeen:   map[string]struct{}{},
	}
}

// Parse interprets a raw Claw SSE frame and returns the list of structured
// payloads it produced. A single raw frame can generate multiple events
// (e.g. a tool_used result might yield a BenchmarkEvent AND a LogEvent) so
// the return type is a slice of (EventType, payload interface{}).
type ParsedEvent struct {
	Type    EventType
	Payload interface{}
}

// Parse routes raw Claw frames to the appropriate handler based on the
// outer `event:` label, then returns structured payloads.
func (p *SSEParser) Parse(raw ClawSSEEvent) []ParsedEvent {
	p.mu.Lock()
	defer p.mu.Unlock()

	switch raw.Event {
	case "", "chat", "chat_delta":
		return p.parseChat(raw)
	case "tool_used":
		return p.parseToolUsed(raw)
	case "status_update":
		return p.parseStatusUpdate(raw)
	case "plan_update":
		return p.parsePlanUpdate(raw)
	case "error":
		return []ParsedEvent{{
			Type: EventTypeLog,
			Payload: LogEventPayload{
				Level:   "error",
				Source:  "claw",
				Message: raw.Data,
			},
		}}
	default:
		// Unknown event types — surface as debug logs.
		return []ParsedEvent{{
			Type: EventTypeLog,
			Payload: LogEventPayload{
				Level:   "debug",
				Source:  "claw",
				Message: raw.Event + ": " + truncate(raw.Data, 512),
			},
		}}
	}
}

// ── Chat path ────────────────────────────────────────────────────────────

// phaseHeaderRegex matches markdown headers like "## Phase 2: Baseline" or
// "# Phase 10 Report" in the assistant's streamed output. The skill is
// trained to emit these at phase transitions, so they are the main source
// of structured phase progression.
var phaseHeaderRegex = regexp.MustCompile(`(?m)^#{1,6}\s*Phase\s+(\d+)\s*[:\-\s]\s*([^\n]*)$`)

// chatEnvelope is the subset of the Claude/Claw chat payload we care about.
// The skill's text reaches us either in content.content[].text (full message)
// or delta.content (streaming token delta) — handle both shapes.
type chatEnvelope struct {
	Content *struct {
		Content []struct {
			Type string `json:"_type"`
			Text string `json:"text"`
		} `json:"content"`
	} `json:"content"`
	Delta *struct {
		Content string `json:"content"`
	} `json:"delta"`
	Message *struct {
		Text string `json:"text"`
	} `json:"message"`
	// Fallback when Claw flattens the payload.
	Text string `json:"text"`
}

func (p *SSEParser) parseChat(raw ClawSSEEvent) []ParsedEvent {
	text := extractChatText(raw.Data)
	if text == "" {
		return nil
	}

	out := make([]ParsedEvent, 0, 2)
	// Always forward the text as a log event so UIs with a transcript view
	// have something to show.
	out = append(out, ParsedEvent{
		Type: EventTypeLog,
		Payload: LogEventPayload{
			Level:   "info",
			Source:  "hyperloom",
			Message: text,
		},
	})

	// Look for phase headers in the new chunk.
	matches := phaseHeaderRegex.FindAllStringSubmatch(text, -1)
	if len(matches) == 0 && len(text) > 20 {
		// Log at high verbosity so we can diagnose if the skill changes its
		// header format without producing structured phase events.
		klog.V(3).InfoS("sse_parser: chat text contained no phase header", "preview", truncate(text, 120))
	}
	for _, match := range matches {
		phaseNum, err := strconv.Atoi(match[1])
		if err != nil || phaseNum < 0 || phaseNum > 20 {
			klog.V(2).InfoS("sse_parser: ignoring out-of-range phase number", "raw", match[1])
			continue
		}
		phaseLabel := strings.TrimSpace(match[2])
		out = append(out, p.transitionPhase(phaseNum, phaseLabel)...)
	}
	return out
}

// transitionPhase handles a phase boundary: closes the previous phase as
// succeeded and opens the new one. No-op when the phase hasn't changed.
func (p *SSEParser) transitionPhase(newPhase int, label string) []ParsedEvent {
	if newPhase == p.currentPhase {
		return nil
	}
	out := make([]ParsedEvent, 0, 2)
	if p.currentPhase >= 0 {
		out = append(out, ParsedEvent{
			Type: EventTypePhase,
			Payload: PhaseEventPayload{
				Phase:     p.currentPhase,
				PhaseName: PhaseName(p.currentPhase),
				Status:    "succeeded",
			},
		})
	}
	phaseName := label
	if phaseName == "" {
		phaseName = PhaseName(newPhase)
	}
	out = append(out, ParsedEvent{
		Type: EventTypePhase,
		Payload: PhaseEventPayload{
			Phase:     newPhase,
			PhaseName: phaseName,
			Status:    "started",
		},
	})
	p.currentPhase = newPhase
	return out
}

// ── Tool path (the richest source of structured data) ───────────────────

type toolUsedEnvelope struct {
	Tool       string          `json:"tool"`
	ActionID   string          `json:"actionId"`
	Status     string          `json:"status"`
	Brief      string          `json:"brief,omitempty"`
	Desc       string          `json:"description,omitempty"`
	Arguments  json.RawMessage `json:"arguments,omitempty"`
	Input      json.RawMessage `json:"input,omitempty"`
	Result     json.RawMessage `json:"result,omitempty"`
	Output     json.RawMessage `json:"output,omitempty"`
	PlanStepID string          `json:"planStepId,omitempty"`
}

// parseToolUsed turns tool lifecycle events into the richest structured
// payloads. The three interesting cases are:
//   - benchmark_serving.py invocation   → BenchmarkEvent
//   - geak_create_task / geak_submit_task → KernelEvent (optimizing)
//   - geak_get_outputs                   → KernelEvent (patched)
func (p *SSEParser) parseToolUsed(raw ClawSSEEvent) []ParsedEvent {
	var env toolUsedEnvelope
	if err := json.Unmarshal([]byte(raw.Data), &env); err != nil {
		return []ParsedEvent{{
			Type: EventTypeLog,
			Payload: LogEventPayload{
				Level:   "debug",
				Source:  "claw",
				Message: "tool_used (unparsed): " + truncate(raw.Data, 256),
			},
		}}
	}

	// Only act on completed tool calls — intermediate streaming states just
	// become log entries.
	if env.Status != "success" && env.Status != "error" {
		return []ParsedEvent{{
			Type: EventTypeLog,
			Payload: LogEventPayload{
				Level:   "info",
				Source:  "tool",
				Message: env.Tool + " " + env.Status,
			},
		}}
	}

	tool := strings.ToLower(env.Tool)
	switch {
	case containsAny(tool, "benchmark_serving", "run_baseline", "benchmark"):
		return p.parseBenchmarkTool(env)
	case strings.HasPrefix(tool, "mcp__geak__") ||
		strings.HasPrefix(tool, "geak_") ||
		strings.HasPrefix(tool, "mcp__oci-oob-agent__"):
		return p.parseKernelTool(env)
	default:
		msg := env.Brief
		if msg == "" {
			msg = env.Desc
		}
		if msg == "" {
			msg = env.Tool
		}
		return []ParsedEvent{{
			Type: EventTypeLog,
			Payload: LogEventPayload{
				Level:   "info",
				Source:  "tool:" + env.Tool,
				Message: msg,
			},
		}}
	}
}

// benchmarkResultShape captures the common fields that benchmark_serving.py
// returns after a successful run. Field names match the InferenceX / SGLang
// convention used by Hyperloom.
type benchmarkResultShape struct {
	InputTokensPerSec  float64 `json:"input_throughput"`
	OutputTokensPerSec float64 `json:"output_throughput"`
	TotalTokensPerSec  float64 `json:"total_token_throughput"`
	TPOTMs             float64 `json:"mean_tpot_ms"`
	TTFTMs             float64 `json:"mean_ttft_ms"`
	Concurrency        int     `json:"max_concurrency"`
	ISL                int     `json:"random_input_len"`
	OSL                int     `json:"random_output_len"`
	Framework          string  `json:"framework"`
}

// benchmarkTextRegex scrapes tok/s and tpot from Bash stdout in case the
// skill invoked run_baseline.sh instead of the structured benchmark tool.
// Matches lines like "Output throughput: 571.32 tok/s" or "mean TPOT: 6.78 ms".
var (
	benchTokRegex  = regexp.MustCompile(`(?i)(?:output\s+)?throughput[:\s]+([0-9]+(?:\.[0-9]+)?)\s*(?:tok(?:ens)?/s|t/s)`)
	benchTPOTRegex = regexp.MustCompile(`(?i)(?:mean\s+)?tpot[:\s]+([0-9]+(?:\.[0-9]+)?)\s*ms`)
	benchTTFTRegex = regexp.MustCompile(`(?i)(?:mean\s+)?ttft[:\s]+([0-9]+(?:\.[0-9]+)?)\s*ms`)
)

func (p *SSEParser) parseBenchmarkTool(env toolUsedEnvelope) []ParsedEvent {
	payload := BenchmarkEventPayload{
		Round: p.nextRound(),
		Label: env.Brief,
	}
	if payload.Label == "" {
		payload.Label = "benchmark-" + strconv.Itoa(payload.Round)
	}

	// Prefer structured JSON if present.
	raw := env.Result
	if len(raw) == 0 {
		raw = env.Output
	}
	if len(raw) > 0 {
		var shape benchmarkResultShape
		// benchmark_serving sometimes wraps the result: { "result": {...} }.
		if err := json.Unmarshal(raw, &shape); err == nil && shape.OutputTokensPerSec > 0 {
			fillBenchmarkFromShape(&payload, shape)
			return []ParsedEvent{{Type: EventTypeBenchmark, Payload: payload}}
		}
		var wrapper struct {
			Result benchmarkResultShape `json:"result"`
		}
		if err := json.Unmarshal(raw, &wrapper); err == nil && wrapper.Result.OutputTokensPerSec > 0 {
			fillBenchmarkFromShape(&payload, wrapper.Result)
			return []ParsedEvent{{Type: EventTypeBenchmark, Payload: payload}}
		}
	}

	// Fallback: grep the stdout blob in description.
	text := env.Desc
	if text == "" && len(raw) > 0 {
		text = string(raw)
	}
	if text == "" {
		return nil
	}
	filled := false
	if m := benchTokRegex.FindStringSubmatch(text); m != nil {
		if v, err := strconv.ParseFloat(m[1], 64); err == nil {
			payload.OutputTokensPerSec = v
			filled = true
		}
	}
	if m := benchTPOTRegex.FindStringSubmatch(text); m != nil {
		if v, err := strconv.ParseFloat(m[1], 64); err == nil {
			payload.TPOTMs = v
			filled = true
		}
	}
	if m := benchTTFTRegex.FindStringSubmatch(text); m != nil {
		if v, err := strconv.ParseFloat(m[1], 64); err == nil {
			payload.TTFTMs = v
			filled = true
		}
	}
	if !filled {
		// Nothing structured found — drop back to a log.
		return []ParsedEvent{{
			Type: EventTypeLog,
			Payload: LogEventPayload{
				Level:   "info",
				Source:  "benchmark",
				Message: truncate(text, 512),
			},
		}}
	}
	return []ParsedEvent{{Type: EventTypeBenchmark, Payload: payload}}
}

func fillBenchmarkFromShape(p *BenchmarkEventPayload, s benchmarkResultShape) {
	p.InputTokensPerSec = s.InputTokensPerSec
	p.OutputTokensPerSec = s.OutputTokensPerSec
	p.TotalTokensPerSec = s.TotalTokensPerSec
	p.TPOTMs = s.TPOTMs
	p.TTFTMs = s.TTFTMs
	p.Concurrency = s.Concurrency
	p.ISL = s.ISL
	p.OSL = s.OSL
	p.Framework = s.Framework
}

func (p *SSEParser) nextRound() int {
	p.round++
	return p.round
}

// ── Kernel optimization path ────────────────────────────────────────────

// kernelSubmitArgs captures the relevant fields in a geak_create_task /
// geak_submit_task input. Hyperloom passes kernel_name + source repo.
type kernelSubmitArgs struct {
	KernelName string  `json:"kernel_name,omitempty"`
	Name       string  `json:"name,omitempty"`
	Source     string  `json:"source,omitempty"`
	GPUPercent float64 `json:"gpu_percent,omitempty"`
	Backend    string  `json:"backend,omitempty"`
}

// kernelResultShape covers GEAK's output polling result shape.
type kernelResultShape struct {
	KernelName     string  `json:"kernel_name,omitempty"`
	Status         string  `json:"status,omitempty"`
	BaselineUs     float64 `json:"baseline_us,omitempty"`
	OptimizedUs    float64 `json:"optimized_us,omitempty"`
	SpeedupPercent float64 `json:"speedup_percent,omitempty"`
}

func (p *SSEParser) parseKernelTool(env toolUsedEnvelope) []ParsedEvent {
	tool := strings.ToLower(env.Tool)

	// Work out the lifecycle stage implied by the tool name.
	stage := "optimizing"
	switch {
	case strings.Contains(tool, "get_outputs"), strings.Contains(tool, "download_file"):
		stage = "patched"
	case strings.Contains(tool, "create_task"), strings.Contains(tool, "submit_task"):
		stage = "optimizing"
	}

	backend := "GEAK"
	switch {
	case strings.Contains(tool, "oci-oob-agent"), strings.Contains(tool, "codex"):
		backend = KernelBackendCodex
	case strings.Contains(tool, "claude"):
		backend = KernelBackendClaude
	case strings.Contains(tool, "geak"):
		backend = KernelBackendGEAK
	}

	// Try to pull the kernel name from either input (submit) or output (poll).
	name := ""
	var gpuPct float64
	if len(env.Input) > 0 {
		var args kernelSubmitArgs
		if err := json.Unmarshal(env.Input, &args); err == nil {
			name = firstNonEmpty(args.KernelName, args.Name)
			gpuPct = args.GPUPercent
			if args.Backend != "" {
				backend = args.Backend
			}
		}
	}
	if len(env.Arguments) > 0 && name == "" {
		var args kernelSubmitArgs
		if err := json.Unmarshal(env.Arguments, &args); err == nil {
			name = firstNonEmpty(args.KernelName, args.Name)
			gpuPct = args.GPUPercent
			if args.Backend != "" {
				backend = args.Backend
			}
		}
	}

	var baselineUs, optimizedUs float64
	if len(env.Result) > 0 {
		var res kernelResultShape
		if err := json.Unmarshal(env.Result, &res); err == nil {
			if res.KernelName != "" {
				name = res.KernelName
			}
			baselineUs = res.BaselineUs
			optimizedUs = res.OptimizedUs
			if res.Status != "" {
				stage = res.Status
			}
		}
	}
	if name == "" {
		name = env.Brief
	}

	payload := KernelEventPayload{
		Name:        name,
		GPUPercent:  gpuPct,
		Backend:     backend,
		BaselineUs:  baselineUs,
		OptimizedUs: optimizedUs,
		Status:      stage,
	}

	// Always emit a kernel event, even if it's a duplicate — the event id
	// differentiates them and the UI can dedupe if it wants to.
	return []ParsedEvent{{Type: EventTypeKernel, Payload: payload}}
}

// ── Status / plan path ──────────────────────────────────────────────────

type statusEnv struct {
	AgentStatus string `json:"agentStatus"`
	Brief       string `json:"brief"`
}

func (p *SSEParser) parseStatusUpdate(raw ClawSSEEvent) []ParsedEvent {
	var env statusEnv
	_ = json.Unmarshal([]byte(raw.Data), &env)
	return []ParsedEvent{{
		Type: EventTypeLog,
		Payload: LogEventPayload{
			Level:   "info",
			Source:  "agent",
			Message: "status: " + env.AgentStatus + " " + env.Brief,
		},
	}}
}

type planUpdateEnv struct {
	Tasks []struct {
		Status string `json:"status"`
		Title  string `json:"title"`
	} `json:"tasks"`
}

func (p *SSEParser) parsePlanUpdate(raw ClawSSEEvent) []ParsedEvent {
	var env planUpdateEnv
	if err := json.Unmarshal([]byte(raw.Data), &env); err != nil {
		return nil
	}
	titles := make([]string, 0, len(env.Tasks))
	for _, t := range env.Tasks {
		titles = append(titles, t.Status+":"+t.Title)
	}
	return []ParsedEvent{{
		Type: EventTypeLog,
		Payload: LogEventPayload{
			Level:   "info",
			Source:  "plan",
			Message: strings.Join(titles, " | "),
		},
	}}
}

// ── Helpers ─────────────────────────────────────────────────────────────

func extractChatText(data string) string {
	if data == "" {
		return ""
	}
	var env chatEnvelope
	if err := json.Unmarshal([]byte(data), &env); err != nil {
		return ""
	}
	// Preferred: full AssistantMessage content blocks.
	if env.Content != nil {
		parts := make([]string, 0, len(env.Content.Content))
		for _, b := range env.Content.Content {
			t := strings.ToLower(b.Type)
			if (t == "textblock" || t == "text") && strings.TrimSpace(b.Text) != "" {
				parts = append(parts, b.Text)
			}
		}
		if len(parts) > 0 {
			return strings.Join(parts, "\n\n")
		}
	}
	if env.Delta != nil && env.Delta.Content != "" {
		return env.Delta.Content
	}
	if env.Message != nil && env.Message.Text != "" {
		return env.Message.Text
	}
	return env.Text
}

func containsAny(s string, tokens ...string) bool {
	for _, t := range tokens {
		if strings.Contains(s, t) {
			return true
		}
	}
	return false
}
