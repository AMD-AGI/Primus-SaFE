/*
 * Copyright © AMD. 2025-2026. All rights reserved.
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
