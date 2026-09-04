/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package model_prewarm

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
)

const (
	DefaultGlob        = "*.safetensors"
	DefaultParallelism = 4
	// MaxK8sAnnotationKeyLength is the Kubernetes annotation key length limit.
	MaxK8sAnnotationKeyLength = 63

	PhaseRunning   = "Running"
	PhaseSucceeded = "Succeeded"
	PhaseFailed    = "Failed"
)

// Request carries model prewarm instructions from the reconciler to node-agent.
type Request struct {
	OpsJobId    string    `json:"opsJobId"`
	ModelPath   string    `json:"modelPath"`
	Glob        string    `json:"glob"`
	Parallelism int       `json:"parallelism"`
	RequestedAt time.Time `json:"requestedAt"`
}

// Result reports per-node model prewarm progress back to the reconciler.
type Result struct {
	OpsJobId        string    `json:"opsJobId"`
	Phase           string    `json:"phase"`
	Message         string    `json:"message"`
	BytesRead       int64     `json:"bytesRead"`
	DurationSeconds int64     `json:"durationSeconds"`
	FinishedAt      time.Time `json:"finishedAt,omitempty"`
}

// NodeDetail is persisted in OpsJob outputs for per-node status.
type NodeDetail struct {
	Node            string `json:"node"`
	AdminNodeId     string `json:"adminNodeId"`
	Phase           string `json:"phase"`
	Message         string `json:"message,omitempty"`
	DurationSeconds int64  `json:"durationSeconds,omitempty"`
	BytesRead       int64  `json:"bytesRead,omitempty"`
}

// RequestAnnotationPrefix returns the model prewarm request annotation key prefix.
func RequestAnnotationPrefix() string {
	return v1.OpsJobModelPrewarmRequestAnnotation
}

// ResultAnnotationPrefix returns the model prewarm result annotation key prefix.
func ResultAnnotationPrefix() string {
	return v1.OpsJobModelPrewarmResultAnnotation
}

// AnnotationKeySuffix returns the annotation key suffix for an OpsJob (its UID).
func AnnotationKeySuffix(jobUID string) string {
	return jobUID
}

// RequestAnnotationKey returns the request annotation key for an OpsJob UID suffix.
func RequestAnnotationKey(jobUID string) string {
	return v1.OpsJobModelPrewarmRequestAnnotation + jobUID
}

// ResultAnnotationKey returns the result annotation key for an OpsJob UID suffix.
func ResultAnnotationKey(jobUID string) string {
	return v1.OpsJobModelPrewarmResultAnnotation + jobUID
}

// ValidateAnnotationKeySuffix ensures the composed annotation key fits Kubernetes limits.
func ValidateAnnotationKeySuffix(jobUID string) error {
	if jobUID == "" {
		return fmt.Errorf("ops job uid is empty")
	}
	if len(RequestAnnotationKey(jobUID)) > MaxK8sAnnotationKeyLength {
		return fmt.Errorf("model prewarm annotation key exceeds %d characters", MaxK8sAnnotationKeyLength)
	}
	return nil
}

// IsTerminal reports whether the result phase is a finished state.
func IsTerminal(phase string) bool {
	return phase == PhaseSucceeded || phase == PhaseFailed
}

// ValidateModelPath ensures the model path is a safe absolute path.
func ValidateModelPath(modelPath string) error {
	modelPath = strings.TrimSpace(modelPath)
	if modelPath == "" {
		return fmt.Errorf("model.path is required")
	}
	if !strings.HasPrefix(modelPath, "/") {
		return fmt.Errorf("model.path must be an absolute path")
	}
	if strings.Contains(modelPath, "..") {
		return fmt.Errorf("model.path must not contain '..'")
	}
	return nil
}

// ValidateGlob ensures the glob pattern is safe for find -name.
func ValidateGlob(glob string) error {
	glob = strings.TrimSpace(glob)
	if glob == "" {
		return fmt.Errorf("model.glob must not be empty")
	}
	if strings.ContainsAny(glob, ";|&$`\"'\\") {
		return fmt.Errorf("model.glob contains invalid characters")
	}
	return nil
}

// ParseRequest decodes a model prewarm request annotation value.
func ParseRequest(raw string) (*Request, error) {
	req := &Request{}
	if err := json.Unmarshal([]byte(raw), req); err != nil {
		return nil, err
	}
	return req, nil
}

// ParseResult decodes a model prewarm result annotation value.
func ParseResult(raw string) (*Result, error) {
	result := &Result{}
	if err := json.Unmarshal([]byte(raw), result); err != nil {
		return nil, err
	}
	return result, nil
}

// MarshalRequest encodes a model prewarm request for annotation storage.
func MarshalRequest(req *Request) (string, error) {
	data, err := json.Marshal(req)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// MarshalResult encodes a model prewarm result for annotation storage.
func MarshalResult(result *Result) (string, error) {
	data, err := json.Marshal(result)
	if err != nil {
		return "", err
	}
	return string(data), nil
}
