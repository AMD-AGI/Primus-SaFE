/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package dispatcher

import (
	"os"
	"testing"

	commonconfig "github.com/AMD-AIG-AIMA/SAFE/common/pkg/config"
)

// TestMain disables crypto for the package so CICD test fixtures can use plaintext
// encrypted-api-key annotations without a mounted crypto secret.
func TestMain(m *testing.M) {
	commonconfig.SetValue("crypto.enable", "false")
	os.Exit(m.Run())
}
