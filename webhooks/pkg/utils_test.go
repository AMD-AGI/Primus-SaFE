/*
 * Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package webhooks

import (
	"testing"

	"gotest.tools/assert"
)

func TestValidateDisplayName(t *testing.T) {
	name := "prod-29pvc"
	err := validateDisplayName(name)
	assert.NilError(t, err)
}
