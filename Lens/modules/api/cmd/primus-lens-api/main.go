// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package main

import (
	"context"
	"github.com/AMD-AGI/Primus-SaFE/Lens/api/pkg/bootstrap"
)

func main() {
	err := bootstrap.StartServer(context.Background())
	if err != nil {
		panic(err)
	}
}
