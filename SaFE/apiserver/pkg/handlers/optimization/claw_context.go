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

// clawBearerOverride marks that a per-request bearer was chosen, including an empty
// one. An empty override means "send no Authorization"; ClawClient must not fall
// back to the default service apiKey.
type clawBearerOverride struct {
	token string
}

// WithClawBearer attaches a Bearer token for outbound PrimusClaw HTTP calls.
// An empty bearer still records an override so the client does not use its default apiKey.
// A nil ctx is returned unchanged.
func WithClawBearer(ctx context.Context, bearer string) context.Context {
	if ctx == nil {
		return nil
	}
	return context.WithValue(ctx, clawBearerCtxKey{}, clawBearerOverride{token: strings.TrimSpace(bearer)})
}

// clawBearerFromContext returns the per-request bearer and whether WithClawBearer set one.
func clawBearerFromContext(ctx context.Context) (bearer string, ok bool) {
	if ctx == nil {
		return "", false
	}
	v, ok := ctx.Value(clawBearerCtxKey{}).(clawBearerOverride)
	if !ok {
		return "", false
	}
	return v.token, true
}
