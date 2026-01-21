// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package client

import (
	coreclient "github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/nodeexporter"
)

// Re-export all client types from core for backward compatibility
// This allows existing code to continue using this package while the implementation is in core

type Client = coreclient.Client
type Config = coreclient.Config

var (
	DefaultConfig    = coreclient.DefaultConfig
	NewClient        = coreclient.NewClient
	NewClientForNode = coreclient.NewClientForNode
)
