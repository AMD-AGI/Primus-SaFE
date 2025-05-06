/*
 * Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package monitors

import (
	"embed"
)

const (
	ScriptsPackagePath = "scripts"
)

//
//go:embed scripts/*
var ScriptsFS embed.FS
