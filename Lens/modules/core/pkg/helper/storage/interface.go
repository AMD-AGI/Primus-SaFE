// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package storage

import "context"

type Query interface {
	Stat(ctx context.Context, name string) (float64, float64, float64, float64, error)
	Bandwidth(ctx context.Context, name string) (float64, float64, error)
}
