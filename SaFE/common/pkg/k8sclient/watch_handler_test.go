/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package k8sclient

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/tools/cache"
)

func TestWatchErrorHandler(t *testing.T) {
	cs := k8sfake.NewSimpleClientset()
	factory := NewClientFactoryWithOnlyClient(context.Background(), "c1", cs)
	factory.SetValid(true, "")

	h := WatchErrorHandler(context.Background(), factory)
	h(&cache.Reflector{}, errors.New("connection refused"))

	assert.False(t, factory.IsValid())
	assert.Equal(t, "connection refused", factory.GetInvalidReason())
}

func TestWatchErrorHandlerRecoverableError(t *testing.T) {
	cs := k8sfake.NewSimpleClientset()
	factory := NewClientFactoryWithOnlyClient(context.Background(), "c1", cs)
	factory.SetValid(true, "")

	h := WatchErrorHandler(context.Background(), factory)
	h(&cache.Reflector{}, errors.New("too old resource version"))

	assert.True(t, factory.IsValid())
}

func TestShouldMarkFactoryInvalidOnWatchError(t *testing.T) {
	assert.False(t, shouldMarkFactoryInvalidOnWatchError(nil))
	assert.False(t, shouldMarkFactoryInvalidOnWatchError(errors.New("too old resource version")))
	assert.True(t, shouldMarkFactoryInvalidOnWatchError(errors.New("connection refused")))
}
