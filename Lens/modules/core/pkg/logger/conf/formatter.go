// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package conf

type Formatter string

const (
	JSONFormater       Formatter = "json"
	ConsoleFormater    Formatter = "console"
	StructuredFormater Formatter = "structured"
)

func isValidFormatter(f Formatter) bool {
	return (f == JSONFormater) ||
		(f == ConsoleFormater) ||
		(f == StructuredFormater)
}
