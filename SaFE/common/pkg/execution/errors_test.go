/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package execution

import (
	"errors"
	"testing"
	"time"
)

func TestAPIErrorHelpers(t *testing.T) {
	limited := &APIError{StatusCode: 429, Code: CodeRateLimited, Message: "slow", RequestID: "r"}
	if limited.RetryAfter() != 0 {
		t.Fatal("zero delay")
	}
	if RetryAfterOf(limited) != 10*time.Second {
		t.Fatal("rate limit default")
	}
	limited.RetryAfterS = 4
	if RetryAfterOf(limited) != 4*time.Second {
		t.Fatal("explicit delay")
	}
	if !IsCode(limited, CodeRateLimited) || CodeOf(errors.New("x")) != "" {
		t.Fatal("code")
	}
	if IsQueueable(limited) {
		t.Fatal("rate limit is not a queue answer")
	}
	if !IsQueueable(&APIError{Code: CodeImagePreparing}) || !IsQueueable(&APIError{Code: CodeCapacityUnavailable}) ||
		!IsQueueable(&APIError{Code: CodeProfileUnvalidated}) || !IsQueueable(&APIError{Code: CodeConstraintUnsatisfiable}) {
		t.Fatal("queueable codes")
	}
	if IsQueueable(errors.New("x")) || IsRetryable(errors.New("x")) || RetryAfterOf(errors.New("x")) != 0 {
		t.Fatal("plain errors carry no contract")
	}
	unavailable := &APIError{Code: CodeUnavailable}
	if RetryAfterOf(unavailable) != 10*time.Second {
		t.Fatal("unavailable default")
	}
	if got := limited.Error(); got == "" {
		t.Fatal("empty error string")
	}
}
