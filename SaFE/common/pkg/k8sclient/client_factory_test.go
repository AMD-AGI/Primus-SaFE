/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package k8sclient

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	k8sfake "k8s.io/client-go/kubernetes/fake"
)

func TestClientFactoryWithOnlyClient(t *testing.T) {
	cs := k8sfake.NewSimpleClientset()
	f := NewClientFactoryWithOnlyClient(context.Background(), "c1", cs)
	assert.Equal(t, "c1", f.Name())
	assert.NotNil(t, f.ClientSet())

	f.SetValid(false, "down")
	assert.False(t, f.IsValid())
	assert.Equal(t, "down", f.GetInvalidReason())
	f.SetValid(true, "")
	assert.True(t, f.IsValid())

	// Release on a factory without informers should not error.
	assert.NoError(t, f.Release())
}

func TestNewClientFactoryWithFallbacksInvalidInput(t *testing.T) {
	_, err := NewClientFactoryWithFallbacks(context.Background(), "c1", "", []string{"https://10.0.0.2:6443"},
		"", "", "", DisableInformer)
	assert.Error(t, err)
}
