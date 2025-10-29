/*
 * Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package main

import (
	"fmt"

	"github.com/AMD-AIG-AIMA/SAFE/node-agent/pkg/daemon"
)

func main() {
	d, err := daemon.NewDaemon()
	if err != nil {
		fmt.Println("failed to new node-agent daemon, err: ", err.Error())
		return
	}
	d.Start()
}
