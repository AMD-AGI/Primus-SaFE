/*
 * Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package netutil

import (
	"fmt"
	"testing"

	"gotest.tools/assert"
)

func TestGetSecondLevelDomain(t *testing.T) {
	host := "https://arsenal.amd.ai"
	domain := GetSecondLevelDomain(host)
	fmt.Println(domain)
	assert.Equal(t, domain, "amd.ai")
}
