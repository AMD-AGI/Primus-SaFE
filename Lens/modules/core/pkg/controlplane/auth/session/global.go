// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package session

import (
	"sync"
)

var (
	globalManager Manager
	managerOnce   sync.Once
	managerMu     sync.RWMutex
)

// InitGlobalManager initializes the global session manager
func InitGlobalManager(config *Config) {
	managerOnce.Do(func() {
		managerMu.Lock()
		defer managerMu.Unlock()
		globalManager = NewDBManager(config)
	})
}

// GetManager returns the global session manager
func GetManager() Manager {
	managerMu.RLock()
	defer managerMu.RUnlock()
	
	if globalManager == nil {
		// Initialize with default config if not initialized
		managerMu.RUnlock()
		InitGlobalManager(nil)
		managerMu.RLock()
	}
	
	return globalManager
}

// SetManager sets a custom session manager (for testing)
func SetManager(m Manager) {
	managerMu.Lock()
	defer managerMu.Unlock()
	globalManager = m
}
