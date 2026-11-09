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

	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/identity"
	commonconfig "github.com/AMD-AIG-AIMA/SAFE/common/pkg/config"
)

// StartRegistration retries PUT /oauth/scim-target until the identity service accepts it.
// A configuration error stops the loop. ctx cancel stops it.
func StartRegistration(ctx context.Context, token string) {
	go registerLoop(ctx, token)
}

// registerLoop calls the identity service with exponential backoff, capped at one minute.
func registerLoop(ctx context.Context, token string) {
	delay := time.Second
	for {
		publicURL := commonconfig.GetSCIMPublicURL()
		err := identity.RegisterTarget(ctx, publicURL, token)
		if err == nil {
			klog.Infof("registered the scim target with the identity service at %s", publicURL)
			return
		}
		var cfg *identity.ConfigError
		if errors.As(err, &cfg) {
			klog.ErrorS(err, "scim target registration stopped")
			return
		}
		klog.ErrorS(err, "scim target registration failed; retrying", "after", delay)
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
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
