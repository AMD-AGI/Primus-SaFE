// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package stringUtil

import "strconv"

func IsNumeric(s string) bool {
	_, err := strconv.Atoi(s)
	return err == nil
}
