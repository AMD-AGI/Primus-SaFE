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
	"k8s.io/client-go/rest"
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

func TestNewClientSetWithRestConfig(t *testing.T) {
	cs, err := NewClientSetWithRestConfig(&rest.Config{Host: "http://127.0.0.1:60999"})
	assert.NoError(t, err)
	assert.NotNil(t, cs)
}
