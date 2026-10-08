/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package exportimage

import (
	"io"
	"log"
)

func nopLogger() *log.Logger { return log.New(io.Discard, "", 0) }
