/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package optimization

import (
	"context"
	"strings"
)

type clawBearerCtxKey struct{}

// WithClawBearer attaches a Bearer token for outbound PrimusClaw HTTP calls.
// Empty bearer leaves ctx unchanged.
func WithClawBearer(ctx context.Context, bearer string) context.Context {
	b := strings.TrimSpace(bearer)
	if b == "" || ctx == nil {
		return ctx
	}
	return context.WithValue(ctx, clawBearerCtxKey{}, b)
}

func clawBearerFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	v, _ := ctx.Value(clawBearerCtxKey{}).(string)
	return v
}
