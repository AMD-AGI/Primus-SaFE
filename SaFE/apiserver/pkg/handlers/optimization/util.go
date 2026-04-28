/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package optimization

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"k8s.io/klog/v2"

	commonerrors "github.com/AMD-AIG-AIMA/SAFE/common/pkg/errors"
)

// workspaceLockMap gives each workspace its own mutex so that the
// concurrency-check + DB-insert pair in submitTask is atomic per workspace.
// Concurrent requests targeting different workspaces never block each other.
type workspaceLockMap struct {
	mu    sync.Mutex
	locks map[string]*sync.Mutex
}

func newWorkspaceLockMap() *workspaceLockMap {
	return &workspaceLockMap{locks: make(map[string]*sync.Mutex)}
}

func (m *workspaceLockMap) lock(workspace string) func() {
	m.mu.Lock()
	l, ok := m.locks[workspace]
	if !ok {
		l = &sync.Mutex{}
		m.locks[workspace] = l
	}
	m.mu.Unlock()
	l.Lock()
	return l.Unlock
}

// validateCreateTaskRequest enforces business-logic constraints that struct
// binding tags cannot express. Call this before any DB or Claw interaction.
func validateCreateTaskRequest(req *CreateTaskRequest) error {
	if req.TP < 0 || req.TP > 256 {
		return commonerrors.NewBadRequest(fmt.Sprintf("tp must be between 0 and 256, got %d", req.TP))
	}
	if req.EP < 0 || req.EP > 256 {
		return commonerrors.NewBadRequest(fmt.Sprintf("ep must be between 0 and 256, got %d", req.EP))
	}
	if req.ISL < 0 || req.ISL > 1_000_000 {
		return commonerrors.NewBadRequest(fmt.Sprintf("isl must be between 0 and 1000000, got %d", req.ISL))
	}
	if req.OSL < 0 || req.OSL > 1_000_000 {
		return commonerrors.NewBadRequest(fmt.Sprintf("osl must be between 0 and 1000000, got %d", req.OSL))
	}
	if req.Concurrency < 0 || req.Concurrency > 10_000 {
		return commonerrors.NewBadRequest(fmt.Sprintf("concurrency must be between 0 and 10000, got %d", req.Concurrency))
	}
	if req.GeakStepLimit < 0 || req.GeakStepLimit > 10_000 {
		return commonerrors.NewBadRequest(fmt.Sprintf("geakStepLimit must be between 0 and 10000, got %d", req.GeakStepLimit))
	}
	if req.Mode != "" && req.Mode != ModeLocal && req.Mode != ModeClaw {
		return commonerrors.NewBadRequest(fmt.Sprintf("mode must be %q or %q, got %q", ModeLocal, ModeClaw, req.Mode))
	}
	if req.Framework != "" && req.Framework != FrameworkSGLang && req.Framework != FrameworkVLLM {
		return commonerrors.NewBadRequest(fmt.Sprintf("framework must be %q or %q, got %q", FrameworkSGLang, FrameworkVLLM, req.Framework))
	}
	if req.ResultsPath != "" && strings.Contains(req.ResultsPath, "..") {
		return commonerrors.NewBadRequest("resultsPath must not contain '..'")
	}
	return nil
}

// withClawRetry calls fn up to 3 times with exponential back-off (1s, 4s)
// so that transient Claw network blips don't permanently fail a task.
// Each attempt gets its own 30-second timeout context.
func withClawRetry[T any](ctx context.Context, bearer, op string, fn func(context.Context) (T, error)) (T, error) {
	delays := []time.Duration{0, 1 * time.Second, 4 * time.Second}
	var zero T
	var lastErr error
	for i, delay := range delays {
		if delay > 0 {
			select {
			case <-time.After(delay):
			case <-ctx.Done():
				return zero, ctx.Err()
			}
		}
		rctx, cancel := context.WithTimeout(WithClawBearer(ctx, bearer), 30*time.Second)
		result, err := fn(rctx)
		cancel()
		if err == nil {
			return result, nil
		}
		lastErr = err
		klog.V(2).InfoS("claw retry", "op", op, "attempt", i+1, "error", err)
	}
	return zero, lastErr
}

// timeToHex formats a unix-nano timestamp as a compact lowercase hex string.
func timeToHex(n int64) string {
	return strconv.FormatInt(n, 16)
}

// seqToHex formats a sequence number as a zero-padded 6-char hex suffix.
func seqToHex(n uint64) string {
	buf := make([]byte, 3)
	buf[0] = byte(n >> 16)
	buf[1] = byte(n >> 8)
	buf[2] = byte(n)
	return hex.EncodeToString(buf)
}

// marshalPayload serializes an event payload into json.RawMessage without
// losing the type tag. Returns "null" on marshalling error so the client
// never sees an invalid envelope.
func marshalPayload(v interface{}) json.RawMessage {
	if v == nil {
		return json.RawMessage(`null`)
	}
	data, err := json.Marshal(v)
	if err != nil {
		return json.RawMessage(`null`)
	}
	return data
}
