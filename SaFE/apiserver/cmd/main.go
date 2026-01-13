/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package main

import (
	"fmt"

	apiserver "github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/server"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/trace"
	"k8s.io/klog/v2"
)

func main() {
	// Initialize tracer
	if err := trace.InitTracer("primus-safe-apiserver"); err != nil {
		klog.Warningf("Failed to init tracer: %v", err)
		// Don't block startup, gracefully degrade to no tracing
	}
	defer trace.CloseTracer()

	s, err := apiserver.NewServer()
	if err != nil {
		fmt.Println("failed to new server")
		return
	}
	s.Start()
}
