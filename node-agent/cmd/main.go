/*
 * Copyright © AMD. 2025-2026. All rights reserved.
 */

package main

import (
	"fmt"

	"github.com/AMD-AIG-AIMA/SAFE/node-agent/pkg/daemon"
)

func main() {
	d, err := daemon.NewDaemon()
	if err != nil {
		fmt.Println("fail to new node-agent daemon, err: ", err.Error())
		return
	}
	d.Start()
}
