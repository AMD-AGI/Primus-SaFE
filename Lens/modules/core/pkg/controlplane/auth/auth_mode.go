// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package auth

import (
	"context"
	"sync"

	cpdb "github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/controlplane/database"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
)

var (
	currentAuthMode AuthMode = AuthModeLocal
	authModeMu      sync.RWMutex
)

// GetCurrentAuthMode returns the current authentication mode
func GetCurrentAuthMode(ctx context.Context) (AuthMode, error) {
	authModeMu.RLock()
	defer authModeMu.RUnlock()

	// Try to get from database
	configFacade := cpdb.GetFacade().GetSystemConfig()
	value, err := configFacade.GetValue(ctx, ConfigKeyAuthMode)
	if err != nil {
		log.Debugf("Failed to get auth mode from config, using default: %v", err)
		return currentAuthMode, nil
	}

	if value != "" {
		return AuthMode(value), nil
	}

	return currentAuthMode, nil
}

// SetCurrentAuthMode sets the current authentication mode
func SetCurrentAuthMode(ctx context.Context, mode AuthMode) error {
	authModeMu.Lock()
	defer authModeMu.Unlock()

	// Validate mode
	switch mode {
	case AuthModeNone, AuthModeLocal, AuthModeLDAP, AuthModeSSO, AuthModeSaFE:
		// Valid modes
	default:
		return ErrAuthModeNotSupported
	}

	// Save to database
	configFacade := cpdb.GetFacade().GetSystemConfig()
	if err := configFacade.SetValue(ctx, ConfigKeyAuthMode, string(mode), "auth"); err != nil {
		return err
	}

	currentAuthMode = mode
	log.Infof("Authentication mode changed to: %s", mode)
	return nil
}

// IsAuthModeValid checks if the given auth mode is valid
func IsAuthModeValid(mode AuthMode) bool {
	switch mode {
	case AuthModeNone, AuthModeLocal, AuthModeLDAP, AuthModeSSO, AuthModeSaFE:
		return true
	default:
		return false
	}
}
