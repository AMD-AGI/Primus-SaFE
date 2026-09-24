/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package execution

import (
	"errors"
	"fmt"
	"time"
)

// Contract error codes. Callers branch on these rather than on the HTTP status, because
// several distinct conditions share status 409 and require different handling.
const (
	CodeUnauthorized            = "Unauthorized"
	CodeForbidden               = "Forbidden"
	CodeConflict                = "Conflict"
	CodeExpired                 = "Expired"
	CodeNotArmed                = "NotArmed"
	CodeNotFound                = "NotFound"
	CodeCapacityUnavailable     = "CapacityUnavailable"
	CodeProfileUnvalidated      = "ProfileUnvalidated"
	CodeImagePreparing          = "ImagePreparing"
	CodeConstraintUnsatisfiable = "ConstraintUnsatisfiable"
	CodeStartUnknown            = "StartUnknown"
	CodeCleanupUnverified       = "CleanupUnverified"
	CodeRateLimited             = "RateLimited"
	CodeUnavailable             = "Unavailable"
	CodeInvalidRequest          = "InvalidRequest"
)

// wireError is the error body as published. It is kept separate from APIError so the
// HTTP status can be attached without adding a field the schema does not declare.
type wireError struct {
	APIVersion  string `json:"api_version"`
	RequestID   string `json:"request_id"`
	Code        string `json:"code"`
	Message     string `json:"message"`
	Retryable   bool   `json:"retryable"`
	RetryAfterS int64  `json:"retry_after_s"`
}

// APIError is a structured refusal from the capacity controller.
type APIError struct {
	StatusCode  int
	RequestID   string
	Code        string
	Message     string
	Retryable   bool
	RetryAfterS int64
}

// Error renders the code and status, which are what distinguish one refusal from another.
func (e *APIError) Error() string {
	return fmt.Sprintf("external execution %s (http %d, request %s): %s",
		e.Code, e.StatusCode, e.RequestID, e.Message)
}

// RetryAfter is the backoff the server asked for. Zero means the caller chooses.
func (e *APIError) RetryAfter() time.Duration {
	if e.RetryAfterS <= 0 {
		return 0
	}
	return time.Duration(e.RetryAfterS) * time.Second
}

// RetryAfterOf returns the backoff carried by a contract error. Rate-limited and
// unavailable answers without an explicit delay still get a short default so the
// caller does not immediately reissue the same write.
func RetryAfterOf(err error) time.Duration {
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		return 0
	}
	if d := apiErr.RetryAfter(); d > 0 {
		return d
	}
	switch apiErr.Code {
	case CodeRateLimited, CodeUnavailable:
		return 10 * time.Second
	default:
		return 0
	}
}

// CodeOf returns the contract code carried by err, or the empty string when err did not
// come from the capacity controller. A transport failure has no code by design: it is not
// a decision, and must not be read as one.
func CodeOf(err error) string {
	var apiErr *APIError
	if errors.As(err, &apiErr) {
		return apiErr.Code
	}
	return ""
}

// IsCode reports whether err carries the given contract code.
func IsCode(err error, code string) bool {
	return CodeOf(err) == code
}

// IsQueueable reports whether err means the workload keeps waiting in place. These are
// answers about the current state of capacity, not faults, and they must not be retried
// as though a repeat request could change them.
func IsQueueable(err error) bool {
	switch CodeOf(err) {
	case CodeCapacityUnavailable, CodeImagePreparing, CodeProfileUnvalidated,
		CodeConstraintUnsatisfiable:
		return true
	default:
		return false
	}
}

// IsRetryable reports whether the server invited a bounded retry of the same request.
// A retry must reuse the original request id and body; the server treats a changed body
// under a reused id as a conflict.
func IsRetryable(err error) bool {
	var apiErr *APIError
	if errors.As(err, &apiErr) {
		return apiErr.Retryable
	}
	return false
}
