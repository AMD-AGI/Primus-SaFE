/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package scim

import (
	"context"
	"errors"
	"time"

	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/identity"
	commonconfig "github.com/AMD-AIG-AIMA/SAFE/common/pkg/config"
)

// tokenRefreshInterval is how often the token Secret is read again after a registration.
const tokenRefreshInterval = time.Minute

// StartRegistration retries PUT /oauth/scim-target until the identity service accepts it, then
// reads the token Secret every minute and registers again when the token changes. tok always holds
// the token the endpoint accepts. A configuration error stops the registration; ctx cancel stops it all.
func StartRegistration(ctx context.Context, cli client.Client, tok *Token) {
	go func() {
		if !registerLoop(ctx, cli, tok) {
			return
		}
		ticker := time.NewTicker(tokenRefreshInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
			changed, err := refreshToken(ctx, cli, tok)
			if err != nil {
				klog.ErrorS(err, "failed to read the scim token secret")
				continue
			}
			if changed && !registerLoop(ctx, cli, tok) {
				return
			}
		}
	}()
}

// registerLoop calls the identity service with exponential backoff, capped at one minute. Each attempt
// reads the token Secret first, so a retry sends the current token. It reports whether it registered.
func registerLoop(ctx context.Context, cli client.Client, tok *Token) bool {
	delay := time.Second
	for {
		if _, err := refreshToken(ctx, cli, tok); err != nil {
			klog.ErrorS(err, "failed to read the scim token secret")
		}
		publicURL := commonconfig.GetSCIMPublicURL()
		err := identity.RegisterTarget(ctx, publicURL, tok.Load())
		if err == nil {
			klog.Infof("registered the scim target with the identity service at %s", publicURL)
			return true
		}
		var cfg *identity.ConfigError
		if errors.As(err, &cfg) {
			klog.ErrorS(err, "scim target registration stopped")
			return false
		}
		klog.ErrorS(err, "scim target registration failed; retrying", "after", delay)
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return false
		case <-timer.C:
		}
		if delay < time.Minute {
			delay *= 2
			if delay > time.Minute {
				delay = time.Minute
			}
		}
	}
}
