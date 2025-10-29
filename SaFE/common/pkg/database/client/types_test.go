/*
 * Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package client

import (
	"fmt"
	"testing"
)

func TestGenInsertWorkloadCmd(t *testing.T) {
	workload := Workload{}
	cmd := genInsertCommand(workload, insertWorkloadFormat, "id")
	fmt.Println(cmd)
}
