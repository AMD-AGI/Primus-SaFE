/*
 * Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package main

import (
	"fmt"

	"github.com/AMD-AIG-AIMA/SAFE/resource-manager/pkg/server"
)

func main() {
	s, err := server.NewServer()
	if err != nil {
		fmt.Println("fail to new server, err: ", err.Error())
		return
	}
	s.Start()
}
