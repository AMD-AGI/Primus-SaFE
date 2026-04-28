/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package optimization

import (
	"encoding/json"
	"time"
)

// OptimizationTaskStatus represents the lifecycle state of an optimization task.
type OptimizationTaskStatus string

const (
	StatusPending     OptimizationTaskStatus = "Pending"
	StatusRunning     OptimizationTaskStatus = "Running"
	StatusSucceeded   OptimizationTaskStatus = "Succeeded"
	StatusFailed      OptimizationTaskStatus = "Failed"
	StatusInterrupted OptimizationTaskStatus = "Interrupted"
)

// Execution modes forwarded to the Hyperloom skill via the Claw prompt.
const (
	ModeClaw  = "claw"
	ModeLocal = "local"
)

// Supported inference frameworks for the prompt builder.
const (
	FrameworkSGLang = "sglang"
	FrameworkVLLM   = "vllm"
)

// Kernel optimization backends understood by the Hyperloom skill.
// These are kept as display strings because the skill prompt reads them verbatim.
const (
	KernelBackendGEAK   = "GEAK"
	KernelBackendClaude = "Claude Code"
	KernelBackendCodex  = "Codex"
)

// CreateTaskRequest is the HTTP body for POST /v1/optimization/tasks.
type CreateTaskRequest struct {
	DisplayName string `json:"displayName"`

	// ModelID is required and must point to a Model with phase=Ready whose
	// localPaths include the target workspace.
	ModelID   string `json:"modelId" binding:"required"`
	Workspace string `json:"workspace" binding:"required"`

	// Execution mode: "claw" runs on PrimusClaw sandbox via RayJob; "local"
	// runs directly in a GPU sandbox. Empty defaults to "local" (same as
	// Hyperloom-Web useInferOptTemplate).
	Mode string `json:"mode"`

	// Inference workload configuration. These are passed through the prompt
	// builder into the Hyperloom skill and must match the fields accepted by
	// claw_build_hyperloom_prompt on the Claw side.
	Framework   string `json:"framework"`
	Precision   string `json:"precision"`
	TP          int    `json:"tp"`
	EP          int    `json:"ep"`
	GPUType     string `json:"gpuType"`
	ISL         int    `json:"isl"`
	OSL         int    `json:"osl"`
	Concurrency int    `json:"concurrency"`

	// Kernel optimization controls.
	KernelBackends []string `json:"kernelBackends"`
	GeakStepLimit  int      `json:"geakStepLimit"`

	// Sandbox / framework image used for the benchmark and kernel opt runs.
	Image          string `json:"image"`
	InferenceXPath string `json:"inferencexPath"`
	ResultsPath    string `json:"resultsPath"`

	// RayJob resource sizing (only used in claw mode).
	RayReplica int `json:"rayReplica"`
	RayGpu     int `json:"rayGpu"`
	RayCpu     int `json:"rayCpu"`
	RayMemory  int `json:"rayMemory"`

	// Optional InferenceX benchmark baseline (CSV) to anchor the skill's
	// target-gap heuristic. When set the skill tries to beat this baseline.
	TargetGpu     string `json:"targetGpu,omitempty"`
	BaselineCSV   string `json:"baselineCSV,omitempty"`
	BaselineCount int    `json:"baselineCount,omitempty"`
}

// TaskInfo is the response shape for a single task (list item or detail).
type TaskInfo struct {
	ID             string                 `json:"id"`
	DisplayName    string                 `json:"displayName"`
	ModelID        string                 `json:"modelId"`
	ModelPath      string                 `json:"modelPath"`
	Workspace      string                 `json:"workspace"`
	UserID         string                 `json:"userId,omitempty"`
	UserName       string                 `json:"userName,omitempty"`
	Mode           string                 `json:"mode"`
	Framework      string                 `json:"framework"`
	Precision      string                 `json:"precision,omitempty"`
	TP             int                    `json:"tp"`
	EP             int                    `json:"ep"`
	GPUType        string                 `json:"gpuType,omitempty"`
	ISL            int                    `json:"isl"`
	OSL            int                    `json:"osl"`
	Concurrency    int                    `json:"concurrency"`
	KernelBackends []string               `json:"kernelBackends,omitempty"`
	GeakStepLimit  int                    `json:"geakStepLimit,omitempty"`
	Image          string                 `json:"image,omitempty"`
	ResultsPath    string                 `json:"resultsPath,omitempty"`
	ClawSessionID  string                 `json:"clawSessionId,omitempty"`
	Status         OptimizationTaskStatus `json:"status"`
	CurrentPhase   int                    `json:"currentPhase"`
	Message        string                 `json:"message,omitempty"`
	Prompt         string                 `json:"prompt,omitempty"`
	CreatedAt      string                 `json:"createdAt"`
	UpdatedAt      string                 `json:"updatedAt"`
	StartedAt      string                 `json:"startedAt,omitempty"`
	FinishedAt     string                 `json:"finishedAt,omitempty"`
}

// ListTasksQuery holds filter/pagination options for GET /v1/optimization/tasks.
type ListTasksQuery struct {
	Workspace string `form:"workspace"`
	Status    string `form:"status"`
	ModelID   string `form:"modelId"`
	UserID    string `form:"userId"`
	Search    string `form:"search"`
	Limit     int    `form:"limit,default=50"`
	Offset    int    `form:"offset,default=0"`
}

// ListTasksResponse is the response shape for list endpoint.
type ListTasksResponse struct {
	Total int        `json:"total"`
	Items []TaskInfo `json:"items"`
}

// CreateTaskResponse is returned after a task has been created and submitted to Claw.
type CreateTaskResponse struct {
	ID            string `json:"id"`
	ClawSessionID string `json:"clawSessionId"`
}

// BatchCreateTasksRequest is a convenience wrapper for bulk submission. v1
// executes sequentially in-process and relies on the existing workspace
// concurrency gate; later versions can fan this out to a queue.
type BatchCreateTasksRequest struct {
	Items []CreateTaskRequest `json:"items" binding:"required"`
}

// BatchCreateTaskResponseItem holds the result for a single item in a batch
// create request. Error is non-empty when that item failed; ID and
// ClawSessionID are populated on success.
type BatchCreateTaskResponseItem struct {
	ID            string `json:"id,omitempty"`
	ClawSessionID string `json:"clawSessionId,omitempty"`
	Error         string `json:"error,omitempty"`
}

type BatchCreateTasksResponse struct {
	Items []BatchCreateTaskResponseItem `json:"items"`
}

// RetryTaskResponse returns the new session id after retrying a failed task.
// The task id is preserved so URLs stay stable.
type RetryTaskResponse struct {
	ID            string `json:"id"`
	ClawSessionID string `json:"clawSessionId"`
}

// ArtifactInfo is a flattened view over the files Claw stores for one session.
// Paths are session-relative (e.g. "claw-1/optimization_report.md").
type ArtifactInfo struct {
	Path         string `json:"path"`
	Run          *int   `json:"run,omitempty"`
	Size         int64  `json:"size,omitempty"`
	LastModified string `json:"lastModified,omitempty"`
	DownloadPath string `json:"downloadPath,omitempty"`
}

type ListArtifactsResponse struct {
	Items []ArtifactInfo `json:"items"`
}

// ApplyTaskRequest carries optional deployment overrides when materializing a
// Hyperloom result into a real SaFE Workload.
type ApplyTaskRequest struct {
	DisplayName string `json:"displayName,omitempty"`
	Workspace   string `json:"workspace,omitempty"`
	Image       string `json:"image,omitempty"`
	CPU         string `json:"cpu,omitempty"`
	Memory      string `json:"memory,omitempty"`
	GPU         string `json:"gpu,omitempty"`
	Replica     int    `json:"replica,omitempty"`
	Port        int    `json:"port,omitempty"`
}

type ApplyTaskResponse struct {
	TaskID        string `json:"taskId"`
	WorkloadID    string `json:"workloadId"`
	DisplayName   string `json:"displayName"`
	LaunchCommand string `json:"launchCommand"`
	ReportPath    string `json:"reportPath,omitempty"`
}

// Event type enum used on the wire for the structured SSE stream.
type EventType string

const (
	EventTypePhase     EventType = "phase"
	EventTypeBenchmark EventType = "benchmark"
	EventTypeKernel    EventType = "kernel"
	EventTypeLog       EventType = "log"
	EventTypeStatus    EventType = "status"
	EventTypeDone      EventType = "done"
)

// Event is the envelope emitted to clients subscribing to SSE.
// Payload is opaque JSON matching one of the *Payload structs below.
type Event struct {
	ID        string          `json:"id"`
	TaskID    string          `json:"taskId"`
	Type      EventType       `json:"type"`
	Timestamp int64           `json:"timestamp"`
	Payload   json.RawMessage `json:"payload"`
}

// PhaseEventPayload maps to Hyperloom's Phase 0..10 progression.
type PhaseEventPayload struct {
	Phase     int    `json:"phase"`
	PhaseName string `json:"phaseName"`
	Status    string `json:"status"` // started | running | succeeded | failed
	Message   string `json:"message,omitempty"`
}

// BenchmarkEventPayload is emitted whenever the skill finishes a benchmark run.
// The parser extracts this from tool_use events targeting benchmark_serving.py
// or from text output containing tok/s lines.
type BenchmarkEventPayload struct {
	Round              int     `json:"round"`
	Label              string  `json:"label"`
	InputTokensPerSec  float64 `json:"inputTokensPerSec,omitempty"`
	OutputTokensPerSec float64 `json:"outputTokensPerSec,omitempty"`
	TotalTokensPerSec  float64 `json:"totalTokensPerSec,omitempty"`
	TPOTMs             float64 `json:"tpotMs,omitempty"`
	TTFTMs             float64 `json:"ttftMs,omitempty"`
	Concurrency        int     `json:"concurrency,omitempty"`
	ISL                int     `json:"isl,omitempty"`
	OSL                int     `json:"osl,omitempty"`
	Framework          string  `json:"framework,omitempty"`
}

// KernelEventPayload tracks the lifecycle of a single kernel optimization attempt.
type KernelEventPayload struct {
	Name        string  `json:"name"`
	GPUPercent  float64 `json:"gpuPercent,omitempty"`
	Source      string  `json:"source,omitempty"`  // inductor | aiter | hip
	Backend     string  `json:"backend,omitempty"` // GEAK | Codex | Claude
	BaselineUs  float64 `json:"baselineUs,omitempty"`
	OptimizedUs float64 `json:"optimizedUs,omitempty"`
	// Status lifecycle: pending | optimizing | patched | reverted | failed
	Status string `json:"status"`
}

// LogEventPayload is the fallback channel for anything the parser could not
// translate into structured events. It carries the raw text verbatim.
type LogEventPayload struct {
	Level   string `json:"level"` // info | warn | error | debug
	Source  string `json:"source,omitempty"`
	Message string `json:"message"`
}

// StatusEventPayload mirrors the task lifecycle transitions for client UIs.
type StatusEventPayload struct {
	Status  OptimizationTaskStatus `json:"status"`
	Message string                 `json:"message,omitempty"`
}

// DoneEventPayload is emitted as the very last event on a task stream.
type DoneEventPayload struct {
	Status  OptimizationTaskStatus `json:"status"`
	Message string                 `json:"message,omitempty"`
}

// Phase name lookup table. The skill emits "Phase N" strings in its markdown
// output; we map them to human-readable names for the UI.
var PhaseNames = map[int]string{
	0:  "Classify",
	1:  "Setup",
	2:  "Baseline",
	3:  "Profile",
	4:  "TraceLens Analysis",
	5:  "Identify Candidates",
	6:  "Server Tuning",
	7:  "Kernel Optimization",
	8:  "Patch & Benchmark",
	9:  "Parameter Sweep",
	10: "Report",
}

// PhaseName returns the canonical name for a phase index.
func PhaseName(phase int) string {
	if name, ok := PhaseNames[phase]; ok {
		return name
	}
	return ""
}

// NewEventID mints a monotonically-ordered event id based on unix nanos.
// Collisions within the same nanosecond are broken by the sequence argument.
func NewEventID(taskID string, seq uint64) string {
	return taskID + "-" + timeToHex(time.Now().UnixNano()) + "-" + seqToHex(seq)
}
