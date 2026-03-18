// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package service

import "errors"

// Sentinel errors for error classification across all services
var (
	ErrNotFound      = errors.New("not found")
	ErrAccessDenied  = errors.New("access denied")
	ErrNotConfigured = errors.New("not configured")
	ErrAlreadyLiked  = errors.New("already liked")
)
