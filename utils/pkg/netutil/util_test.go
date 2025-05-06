/*
 * Copyright © AMD. 2025-2026. All rights reserved.
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
