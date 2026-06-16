/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package resource

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"helm.sh/helm/v3/pkg/release"
	helmtime "helm.sh/helm/v3/pkg/time"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrlruntime "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlfake "sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
)

func newAddonController(t *testing.T, objs ...client.Object) *AddonController {
	t.Helper()
	scheme, err := genMockScheme()
	assert.NoError(t, err)
	cl := ctrlfake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&v1.Addon{}).
		WithObjects(objs...).
		Build()
	return &AddonController{Client: cl}
}

func TestAddonTemplateReconcileEarlyReturns(t *testing.T) {
	scheme, err := genMockScheme()
	assert.NoError(t, err)
	tmplEmptyURL := &v1.AddonTemplate{ObjectMeta: metav1.ObjectMeta{Name: "t-empty"}}
	cl := ctrlfake.NewClientBuilder().WithScheme(scheme).
		WithStatusSubresource(&v1.AddonTemplate{}).WithObjects(tmplEmptyURL).Build()
	r := &AddonTemplateController{Client: cl}
	ctx := context.Background()

	// not found -> no error
	_, err = r.Reconcile(ctx, ctrlruntime.Request{NamespacedName: types.NamespacedName{Name: "missing"}})
	assert.NoError(t, err)

	// empty URL -> short-circuit
	_, err = r.Reconcile(ctx, ctrlruntime.Request{NamespacedName: types.NamespacedName{Name: "t-empty"}})
	assert.NoError(t, err)
}

func TestReplaceValuesNested(t *testing.T) {
	values := map[string]interface{}{
		"a": "override",
		"nested": map[string]interface{}{
			"x": 1,
		},
	}
	base := map[string]interface{}{
		"a": "base",
		"b": "added",
		"nested": map[string]interface{}{
			"x": 99,
			"y": 2,
		},
	}
	out := replaceValues(values, base)
	assert.Equal(t, "override", out["a"])
	assert.Equal(t, "added", out["b"])
	nested := out["nested"].(map[string]interface{})
	assert.Equal(t, 1, nested["x"])
	assert.Equal(t, 2, nested["y"])
}

func TestIsTemplateVersionEqualCases(t *testing.T) {
	// no template -> equal
	a1 := &v1.Addon{Spec: v1.AddonSpec{AddonSource: v1.AddonSource{HelmRepository: &v1.HelmRepository{}}}}
	assert.True(t, isTemplateVersionEqual(a1))

	// template set but no recorded template in status -> not equal
	a2 := &v1.Addon{Spec: v1.AddonSpec{AddonSource: v1.AddonSource{HelmRepository: &v1.HelmRepository{
		Template: &corev1.ObjectReference{Name: "tmpl.0.1.5"},
	}}}}
	a2.Status.AddonSourceStatus.HelmRepositoryStatus = &v1.HelmRepositoryStatus{}
	assert.False(t, isTemplateVersionEqual(a2))

	// names match -> equal
	a3 := &v1.Addon{Spec: v1.AddonSpec{AddonSource: v1.AddonSource{HelmRepository: &v1.HelmRepository{
		Template: &corev1.ObjectReference{Name: "tmpl.0.1.5"},
	}}}}
	a3.Status.AddonSourceStatus.HelmRepositoryStatus = &v1.HelmRepositoryStatus{
		Template: &corev1.ObjectReference{Name: "tmpl.0.1.5"},
	}
	assert.True(t, isTemplateVersionEqual(a3))
}

func TestIsChartVersionEqualCases(t *testing.T) {
	// template-based -> always equal (chart version not used)
	a1 := &v1.Addon{Spec: v1.AddonSpec{AddonSource: v1.AddonSource{HelmRepository: &v1.HelmRepository{
		Template: &corev1.ObjectReference{Name: "t"},
	}}}}
	a1.Status.AddonSourceStatus.HelmRepositoryStatus = &v1.HelmRepositoryStatus{}
	assert.True(t, isChartVersionEqual(a1))

	// chart version matches
	a2 := &v1.Addon{Spec: v1.AddonSpec{AddonSource: v1.AddonSource{HelmRepository: &v1.HelmRepository{
		ChartVersion: "1.2.3",
	}}}}
	a2.Status.AddonSourceStatus.HelmRepositoryStatus = &v1.HelmRepositoryStatus{ChartVersion: "1.2.3"}
	assert.True(t, isChartVersionEqual(a2))

	// chart version differs
	a3 := &v1.Addon{Spec: v1.AddonSpec{AddonSource: v1.AddonSource{HelmRepository: &v1.HelmRepository{
		ChartVersion: "1.2.4",
	}}}}
	a3.Status.AddonSourceStatus.HelmRepositoryStatus = &v1.HelmRepositoryStatus{ChartVersion: "1.2.3"}
	assert.False(t, isChartVersionEqual(a3))
}

func TestRollbackValuesFull(t *testing.T) {
	base := map[string]interface{}{
		"a": "base-a",
		"nested": map[string]interface{}{
			"x": "base-x",
		},
	}
	out := rollbackValues("a: override\nnested:\n  x: override-x\n", base)
	assert.Contains(t, out, "base-a")
	// non-map yaml fails to unmarshal into a map and returns empty
	assert.Equal(t, "", rollbackValues("- a\n- b\n", base))
}

func TestHelmStatusNoRepository(t *testing.T) {
	addon := &v1.Addon{ObjectMeta: metav1.ObjectMeta{Name: "a-norepo"}}
	r := newAddonController(t, addon)
	assert.NoError(t, r.helmStatus(context.Background(), addon))
}

func TestUpdateAddonHelmStatusFull(t *testing.T) {
	addon := &v1.Addon{
		ObjectMeta: metav1.ObjectMeta{Name: "a-helm"},
		Spec: v1.AddonSpec{AddonSource: v1.AddonSource{HelmRepository: &v1.HelmRepository{
			ReleaseName: "rel",
			Values:      "k: v",
		}}},
	}
	r := newAddonController(t, addon)
	resp := &release.Release{
		Version: 3,
		Info: &release.Info{
			FirstDeployed: helmtime.Now(),
			LastDeployed:  helmtime.Now(),
			Description:   "ok",
			Notes:         "notes",
			Status:        release.StatusPendingInstall,
		},
	}
	assert.NoError(t, r.updateAddonHelmStatus(context.Background(), addon, resp))
	assert.NotNil(t, addon.Status.AddonSourceStatus.HelmRepositoryStatus)
	assert.Equal(t, 3, addon.Status.AddonSourceStatus.HelmRepositoryStatus.Version)
}

func TestPatchErrorStatus(t *testing.T) {
	addon := &v1.Addon{ObjectMeta: metav1.ObjectMeta{Name: "a1"}}
	r := newAddonController(t, addon)
	err := r.patchErrorStatus(context.Background(), addon, errors.New("boom"))
	assert.Error(t, err)
	assert.Equal(t, v1.AddonPhaseType(v1.AddonError), addon.Status.Phase)
}

func TestGetClusterMissing(t *testing.T) {
	r := newAddonController(t)
	_, err := r.getCluster(context.Background(), &corev1.ObjectReference{Name: "missing"})
	assert.Error(t, err)
}

func TestRegisterRobustEndpointNonRobust(t *testing.T) {
	addon := &v1.Addon{ObjectMeta: metav1.ObjectMeta{Name: "regular-addon"}}
	r := newAddonController(t)
	// Non-robust addon -> no-op, no panic.
	r.registerRobustEndpointIfApplicable(context.Background(), addon)
}

func TestRegisterRobustEndpointNoCluster(t *testing.T) {
	addon := &v1.Addon{ObjectMeta: metav1.ObjectMeta{Name: "primus-robust-x"}}
	r := newAddonController(t)
	// Robust prefix but no cluster ref -> no-op.
	r.registerRobustEndpointIfApplicable(context.Background(), addon)
}

func TestRegisterRobustEndpointSetsAnnotation(t *testing.T) {
	cluster := testCluster("c1")
	addon := &v1.Addon{
		ObjectMeta: metav1.ObjectMeta{Name: "primus-robust-x"},
		Spec: v1.AddonSpec{
			Cluster: &corev1.ObjectReference{Name: "c1"},
			AddonSource: v1.AddonSource{
				HelmRepository: &v1.HelmRepository{Namespace: "primus-robust"},
			},
		},
	}
	r := newAddonController(t, cluster)
	r.registerRobustEndpointIfApplicable(context.Background(), addon)
	updated := &v1.Cluster{}
	assert.NoError(t, r.Get(context.Background(), client.ObjectKey{Name: "c1"}, updated))
	assert.NotEmpty(t, updated.Annotations[annotationRobustEndpoint])
}

func TestCleanupRobustGrafanaDatasourcesNoSyncer(t *testing.T) {
	r := newAddonController(t)
	// No grafana syncer -> no-op.
	r.cleanupRobustGrafanaDatasources(context.Background(), &v1.Addon{})
}

func TestParseHelmValues(t *testing.T) {
	empty, err := parseHelmValues("")
	assert.NoError(t, err)
	assert.Empty(t, empty)

	vals, err := parseHelmValues("replicas: 3\nimage:\n  tag: v1")
	assert.NoError(t, err)
	assert.Contains(t, vals, "replicas")
	assert.Contains(t, vals, "image")
}

func TestGuaranteeHelmAddonNoHelm(t *testing.T) {
	r := newAddonController(t)
	// No HelmRepository -> nil.
	assert.NoError(t, r.guaranteeHelmAddon(context.Background(), &v1.Addon{}))
}

func TestGetHelmDirect(t *testing.T) {
	r := newAddonController(t)
	addon := &v1.Addon{
		Spec: v1.AddonSpec{
			AddonSource: v1.AddonSource{
				HelmRepository: &v1.HelmRepository{
					URL:          "https://charts.example.com",
					ChartVersion: "1.0",
					Values:       "k: v",
				},
			},
		},
	}
	name, url, version, values, err := r.getHelm(context.Background(), addon)
	assert.NoError(t, err)
	assert.Equal(t, "https://charts.example.com", name)
	assert.Equal(t, "", url)
	assert.Equal(t, "1.0", version)
	assert.Equal(t, "k: v", values)
}

func TestGetHelmFromTemplateOCI(t *testing.T) {
	template := &v1.AddonTemplate{
		ObjectMeta: metav1.ObjectMeta{Name: "t1"},
		Spec:       v1.AddonTemplateSpec{URL: "oci://registry/chart", Version: "2.0", HelmDefaultValues: "d: 1"},
	}
	r := newAddonController(t, template)
	addon := &v1.Addon{
		Spec: v1.AddonSpec{
			AddonSource: v1.AddonSource{
				HelmRepository: &v1.HelmRepository{
					Template: &corev1.ObjectReference{Name: "t1"},
				},
			},
		},
	}
	name, _, version, values, err := r.getHelm(context.Background(), addon)
	assert.NoError(t, err)
	assert.Equal(t, "oci://registry/chart", name)
	assert.Equal(t, "2.0", version)
	assert.Equal(t, "d: 1", values)
}
