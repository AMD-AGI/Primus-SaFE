/*
 * Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package main

import (
	"fmt"

	apiserver "github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/server"
)

func main() {
	s, err := apiserver.NewServer()
	if err != nil {
		fmt.Println("failed to new server, err: ", err.Error())
		return
	}
	s.Start()
}
