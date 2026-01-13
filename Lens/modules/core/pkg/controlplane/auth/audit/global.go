// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package audit

import (
	"sync"
)

var (
	globalService *Service
	serviceOnce   sync.Once
	serviceMu     sync.RWMutex
)

// InitGlobalService initializes the global audit service
func InitGlobalService(retentionDays int) {
	serviceOnce.Do(func() {
		serviceMu.Lock()
		defer serviceMu.Unlock()
		if retentionDays > 0 {
			globalService = NewServiceWithRetention(retentionDays)
		} else {
			globalService = NewService()
		}
	})
}

// GetService returns the global audit service
func GetService() *Service {
	serviceMu.RLock()
	defer serviceMu.RUnlock()

	if globalService == nil {
		// Initialize with default config if not initialized
		serviceMu.RUnlock()
		InitGlobalService(0)
		serviceMu.RLock()
	}

	return globalService
}

// SetService sets a custom audit service (for testing)
func SetService(s *Service) {
	serviceMu.Lock()
	defer serviceMu.Unlock()
	globalService = s
}
