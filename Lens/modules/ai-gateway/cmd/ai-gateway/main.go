// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package main

import (
	"context"

	"github.com/AMD-AGI/Primus-SaFE/Lens/ai-gateway/pkg/bootstrap"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
)

func main() {
	err := bootstrap.Bootstrap(context.Background())
	if err != nil {
		log.Fatalf("Failed to bootstrap AI Gateway: %v", err)
	} else {
		log.Infof("AI Gateway started successfully")
	}
}
