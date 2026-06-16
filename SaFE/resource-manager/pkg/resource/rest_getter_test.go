/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package resource

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/rest"
	"helm.sh/helm/v3/pkg/cli"
)

func testRESTConfig() *rest.Config {
	return &rest.Config{Host: "https://127.0.0.1:6443"}
}

func TestNewRESTClientGetterDefaults(t *testing.T) {
	g := NewRESTClientGetter(testRESTConfig())
	assert.Equal(t, "default", g.namespace)
}

func TestRESTClientGetterToRESTConfig(t *testing.T) {
	g := NewRESTClientGetter(testRESTConfig())
	cfg, err := g.ToRESTConfig()
	assert.NoError(t, err)
	assert.Equal(t, "https://127.0.0.1:6443", cfg.Host)

	// Nil config -> error.
	empty := &RESTClientGetter{}
	_, err = empty.ToRESTConfig()
	assert.Error(t, err)
}

func TestRESTClientGetterDiscoveryAndMapper(t *testing.T) {
	g := NewRESTClientGetter(testRESTConfig())
	dc, err := g.ToDiscoveryClient()
	assert.NoError(t, err)
	assert.NotNil(t, dc)

	mapper, err := g.ToRESTMapper()
	assert.NoError(t, err)
	assert.NotNil(t, mapper)

	loader := g.ToRawKubeConfigLoader()
	assert.NotNil(t, loader)
}

func TestRESTClientGetterPersistent(t *testing.T) {
	g := NewRESTClientGetter(testRESTConfig())
	g.persistent = true
	dc1, err := g.ToDiscoveryClient()
	assert.NoError(t, err)
	dc2, err := g.ToDiscoveryClient()
	assert.NoError(t, err)
	// Persistent path returns the cached instance.
	assert.Equal(t, dc1, dc2)

	mapper, err := g.ToRESTMapper()
	assert.NoError(t, err)
	assert.NotNil(t, mapper)

	loader := g.ToRawKubeConfigLoader()
	assert.NotNil(t, loader)
}

func TestClustersGetterGet(t *testing.T) {
	cg := &ClustersGetter{}
	ref := &corev1.ObjectReference{Name: "c1"}
	getFn := func(_ context.Context, _ *corev1.ObjectReference) (*rest.Config, error) {
		return testRESTConfig(), nil
	}
	g1, err := cg.get(context.Background(), ref, getFn)
	assert.NoError(t, err)
	assert.NotNil(t, g1)
	// Cached: same config -> same getter.
	g2, err := cg.get(context.Background(), ref, getFn)
	assert.NoError(t, err)
	assert.Equal(t, g1, g2)
}

func TestNewDefaultRegistryClient(t *testing.T) {
	settings := cli.New()
	rc, err := newDefaultRegistryClient(false, settings)
	assert.NoError(t, err)
	assert.NotNil(t, rc)

	rc, err = newDefaultRegistryClient(true, settings)
	assert.NoError(t, err)
	assert.NotNil(t, rc)
}
