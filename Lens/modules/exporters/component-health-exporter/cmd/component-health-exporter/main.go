// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package main

import (
	"context"

	"github.com/AMD-AGI/Primus-SaFE/Lens/component-health-exporter/pkg/bootstrap"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/server"
)

func main() {
	err := server.InitServerWithPreInitFunc(context.Background(), bootstrap.Init)
	if err != nil {
		panic(err)
	}
}

// ci trigger: 2026-02-28 component-health-exporter build trigger, delete this later