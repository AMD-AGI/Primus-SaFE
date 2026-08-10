/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package k8sclient

import (
	"context"
	"errors"
	"strings"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog/v2"
)

// WatchErrorHandler marks the client factory invalid for non-recoverable watch errors
// and delegates to the default handler.
func WatchErrorHandler(ctx context.Context, factory *ClientFactory) cache.WatchErrorHandler {
	return func(reflector *cache.Reflector, err error) {
		cache.DefaultWatchErrorHandler(ctx, reflector, err)
		if factory == nil || !shouldMarkFactoryInvalidOnWatchError(err) {
			return
		}
		klog.Warningf("set clients: %s invalid, watch error: %v", factory.Name(), err)
		factory.SetValid(false, err.Error())
	}
}

// shouldMarkFactoryInvalidOnWatchError reports whether a watch error should invalidate the factory.
// Reflector-retriable errors (e.g. expired resource version) are ignored.
func shouldMarkFactoryInvalidOnWatchError(err error) bool {
	if err == nil {
		return false
	}
	if apierrors.IsResourceExpired(err) {
		return false
	}
	if errors.Is(err, context.Canceled) {
		return false
	}
	errMsg := strings.ToLower(err.Error())
	if strings.Contains(errMsg, "too old resource version") {
		return false
	}
	if apierrors.IsUnauthorized(err) || apierrors.IsForbidden(err) {
		return true
	}
	if strings.Contains(errMsg, "x509") || strings.Contains(errMsg, "tls:") {
		return true
	}
	return true
}
