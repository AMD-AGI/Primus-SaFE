/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package resource

import (
	"context"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"helm.sh/helm/v3/pkg/cli"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/rest"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
)

func newHelmAddon() *v1.Addon {
	return &v1.Addon{
		Spec: v1.AddonSpec{
			AddonSource: v1.AddonSource{
				HelmRepository: &v1.HelmRepository{},
			},
		},
		Status: v1.AddonStatus{
			AddonSourceStatus: v1.AddonSourceStatus{
				HelmRepositoryStatus: &v1.HelmRepositoryStatus{},
			},
		},
	}
}

func TestIsStatusReady(t *testing.T) {
	addon := newHelmAddon()
	assert.True(t, isStatusReady(addon))
	addon.Status.AddonSourceStatus.HelmRepositoryStatus.Status = v1.AddonFailed
	assert.False(t, isStatusReady(addon))

	// Nil status -> ready.
	addon2 := &v1.Addon{}
	assert.True(t, isStatusReady(addon2))
}

func TestAreValuesEqual(t *testing.T) {
	addon := newHelmAddon()
	// Empty spec values -> equal.
	assert.True(t, areValuesEqual(addon))

	addon.Spec.AddonSource.HelmRepository.Values = "replicas: 3"
	addon.Status.AddonSourceStatus.HelmRepositoryStatus.Values = "replicas: 3"
	assert.True(t, areValuesEqual(addon))

	addon.Status.AddonSourceStatus.HelmRepositoryStatus.Values = "replicas: 5"
	assert.False(t, areValuesEqual(addon))
}

func TestIsChartVersionEqual(t *testing.T) {
	addon := newHelmAddon()
	addon.Spec.AddonSource.HelmRepository.ChartVersion = "1.0"
	addon.Status.AddonSourceStatus.HelmRepositoryStatus.ChartVersion = "1.0"
	assert.True(t, isChartVersionEqual(addon))

	addon.Status.AddonSourceStatus.HelmRepositoryStatus.ChartVersion = "2.0"
	assert.False(t, isChartVersionEqual(addon))
}

func TestIsTemplateVersionEqual(t *testing.T) {
	// No template -> equal.
	addon := newHelmAddon()
	assert.True(t, isTemplateVersionEqual(addon))
}

func TestShouldIgnoreUpgrade(t *testing.T) {
	addon := newHelmAddon()
	// Ready, no values, no template, matching chart version -> ignore.
	assert.True(t, shouldIgnoreUpgrade(addon))

	// Failed status -> do not ignore.
	addon.Status.AddonSourceStatus.HelmRepositoryStatus.Status = v1.AddonFailed
	assert.False(t, shouldIgnoreUpgrade(addon))
}

func TestReplaceValues(t *testing.T) {
	values := map[string]interface{}{"a": 1}
	base := map[string]interface{}{"b": 2, "nested": map[string]interface{}{"x": 1}}
	out := replaceValues(values, base)
	assert.Equal(t, 1, out["a"])
	assert.Equal(t, 2, out["b"])
	assert.Contains(t, out, "nested")
}

func TestRollbackValues(t *testing.T) {
	base := map[string]interface{}{"replicas": 5}
	out := rollbackValues("replicas: 3", base)
	assert.Contains(t, out, "replicas")

	// Invalid YAML -> empty.
	assert.Equal(t, "", rollbackValues("\t: bad: :", base))
}

func TestGetReleaseNamespace(t *testing.T) {
	addon := newHelmAddon()
	assert.Equal(t, DefaultNamespace, GetReleaseNamespace(addon))
	addon.Spec.AddonSource.HelmRepository.Namespace = "custom-ns"
	assert.Equal(t, "custom-ns", GetReleaseNamespace(addon))
}

func TestRESTClientGetterWithNamespace(t *testing.T) {
	g := &RESTClientGetter{namespace: "a"}
	cpy := g.WithNamespace("b")
	assert.Equal(t, "b", cpy.namespace)
	// Original unchanged.
	assert.Equal(t, "a", g.namespace)
}

func TestRESTClientGetterWithNamespaceKeepsCachesIndependent(t *testing.T) {
	getter := NewRESTClientGetter(&rest.Config{Host: "https://example.com"}, func(g *RESTClientGetter) {
		g.namespace = "original"
		g.persistent = true
		g.impersonate = "example-user"
	})
	originalConfig := getter.ToRawKubeConfigLoader()
	getter.restMapperMu.Lock()
	defer getter.restMapperMu.Unlock()
	getter.discoveryMu.Lock()
	defer getter.discoveryMu.Unlock()
	getter.clientCfgMu.Lock()
	defer getter.clientCfgMu.Unlock()
	copy := getter.WithNamespace("replacement")
	for _, mutex := range []*sync.Mutex{&copy.restMapperMu, &copy.discoveryMu, &copy.clientCfgMu} {
		if !assert.True(t, mutex.TryLock(), "namespace copy must have independent unlocked mutexes") {
			continue
		}
		mutex.Unlock()
	}
	assert.Same(t, getter.cfg, copy.cfg)
	assert.Equal(t, getter.impersonate, copy.impersonate)
	assert.Equal(t, getter.persistent, copy.persistent)
	assert.Nil(t, copy.clientCfg)
	assert.Nil(t, copy.discoveryClient)
	assert.Nil(t, copy.restMapper)
	assert.Same(t, originalConfig, getter.clientCfg)
	assert.Equal(t, "original", getter.namespace)
	assert.Equal(t, "replacement", copy.namespace)
}

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
