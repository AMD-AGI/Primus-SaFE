// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package conf

type Core string

const (
	ZapCore    Core = "zap"
	LogrusCore      = "logrus"
)

func isValidCore(c Core) bool {
	return (c == ZapCore) ||
		(c == LogrusCore)
}
