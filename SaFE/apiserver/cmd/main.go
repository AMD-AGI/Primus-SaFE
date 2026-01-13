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
	// 初始化 tracer
	if err := trace.InitTracer("primus-safe-apiserver"); err != nil {
		klog.Warningf("Failed to init tracer: %v", err)
		// 不阻塞启动，降级为无 tracing
	}
	defer trace.CloseTracer()

	s, err := apiserver.NewServer()
	if err != nil {
		fmt.Println("failed to new server")
		return
	}
	s.Start()
}
