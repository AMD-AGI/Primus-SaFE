/*
 * Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package main

import (
	"fmt"

	webhooks "github.com/AMD-AIG-AIMA/SAFE/webhooks/pkg"
)

func main() {
	s, err := webhooks.NewServer()
	if err != nil {
		fmt.Println("failed to new server ")
		return
	}
	s.Start()
}
