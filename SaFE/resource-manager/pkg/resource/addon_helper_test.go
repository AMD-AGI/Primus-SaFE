/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package resource

import (
	"testing"

	"github.com/stretchr/testify/assert"

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
