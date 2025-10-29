/*
 * Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package main

import (
	"fmt"

	jobmgr "github.com/AMD-AIG-AIMA/SAFE/job-manager/pkg/server"
)

func main() {
	s, err := jobmgr.NewServer()
	if err != nil {
		fmt.Println("failed to new server")
		return
	}
	s.Start()
}
