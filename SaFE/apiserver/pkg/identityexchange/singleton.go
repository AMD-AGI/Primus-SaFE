/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package identityexchange

import "sync/atomic"

var defaultService atomic.Pointer[Service]

// SetDefault installs the process-wide service used by the authentication chain.
func SetDefault(s *Service) { defaultService.Store(s) }

// Default returns the process-wide service, or nil when token exchange is not enabled.
func Default() *Service { return defaultService.Load() }
