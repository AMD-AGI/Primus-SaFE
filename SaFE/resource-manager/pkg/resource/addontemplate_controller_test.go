/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package resource

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlfake "sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
)

func TestGetChartName(t *testing.T) {
	r := &AddonTemplateController{}

	// OCI URL -> name is the URL itself.
	ociTmpl := &v1.AddonTemplate{
		ObjectMeta: metav1.ObjectMeta{Name: "t1"},
		Spec:       v1.AddonTemplateSpec{URL: "oci://registry/chart"},
	}
	installClient := &action.Install{}
	name := r.getChartName(ociTmpl, installClient)
	assert.Equal(t, "oci://registry/chart", name)
	assert.Equal(t, "", installClient.ChartPathOptions.RepoURL)

	// HTTP repo URL -> name is template name, RepoURL set.
	httpTmpl := &v1.AddonTemplate{
		ObjectMeta: metav1.ObjectMeta{Name: "t2"},
		Spec:       v1.AddonTemplateSpec{URL: "https://charts.example.com"},
	}
	installClient2 := &action.Install{}
	name = r.getChartName(httpTmpl, installClient2)
	assert.Equal(t, "t2", name)
	assert.Equal(t, "https://charts.example.com", installClient2.ChartPathOptions.RepoURL)
}

func TestAddonTemplateReconcileNotFound(t *testing.T) {
	scheme, _ := genMockScheme()
	cl := ctrlfake.NewClientBuilder().WithScheme(scheme).Build()
	r := &AddonTemplateController{Client: cl}
	res, err := r.Reconcile(context.Background(), reconcileRequest("missing"))
	assert.NoError(t, err)
	assert.Equal(t, int64(0), res.RequeueAfter.Nanoseconds())
}

func TestAddonTemplateReconcileNoURL(t *testing.T) {
	scheme, _ := genMockScheme()
	tmpl := &v1.AddonTemplate{ObjectMeta: metav1.ObjectMeta{Name: "t1"}}
	cl := ctrlfake.NewClientBuilder().WithScheme(scheme).WithObjects(tmpl).Build()
	r := &AddonTemplateController{Client: cl}
	// Empty URL -> early return.
	_, err := r.Reconcile(context.Background(), reconcileRequest("t1"))
	assert.NoError(t, err)
}

func TestAddonTemplateReconcileAlreadySynced(t *testing.T) {
	scheme, _ := genMockScheme()
	tmpl := &v1.AddonTemplate{
		ObjectMeta: metav1.ObjectMeta{Name: "t1"},
		Spec:       v1.AddonTemplateSpec{URL: "https://charts"},
		Status:     v1.AddonTemplateStatus{HelmStatus: v1.HelmStatus{Values: "already"}},
	}
	cl := ctrlfake.NewClientBuilder().WithScheme(scheme).WithObjects(tmpl).Build()
	r := &AddonTemplateController{Client: cl}
	// Status.Values already set -> early return.
	_, err := r.Reconcile(context.Background(), reconcileRequest("t1"))
	assert.NoError(t, err)
}

// newHelmTemplate builds a synced helm AddonTemplate for upgrade tests; its default values name it.
func newHelmTemplate(name, url, version string, labels map[string]string) *v1.AddonTemplate {
	return &v1.AddonTemplate{
		ObjectMeta: metav1.ObjectMeta{Name: name, Labels: labels},
		Spec:       v1.AddonTemplateSpec{Type: v1.AddonTemplateHelm, URL: url, Version: version, HelmDefaultValues: "defaults: " + name},
		Status:     v1.AddonTemplateStatus{HelmStatus: v1.HelmStatus{Values: "synced"}},
	}
}

// newTemplateAddon builds an Addon that references the given template.
func newTemplateAddon(name, template string, labels map[string]string) *v1.Addon {
	return &v1.Addon{
		ObjectMeta: metav1.ObjectMeta{Name: name, Labels: labels},
		Spec: v1.AddonSpec{AddonSource: v1.AddonSource{HelmRepository: &v1.HelmRepository{
			ReleaseName: name,
			Template:    &corev1.ObjectReference{Name: template},
		}}},
	}
}

// withValues sets the helm values of an Addon.
func withValues(addon *v1.Addon, values string) *v1.Addon {
	addon.Spec.AddonSource.HelmRepository.Values = values
	return addon
}

func TestAddonTemplateReconcileUpgradesAddons(t *testing.T) {
	scheme, _ := genMockScheme()
	const chartURL = "oci://registry/primussafe/chart"
	unsynced := newHelmTemplate("chart.0.1.11", chartURL, "0.1.11", nil)
	unsynced.Status.HelmStatus.Values = ""
	objs := []client.Object{
		newHelmTemplate("chart.0.1.2", chartURL, "0.1.2", nil),
		newHelmTemplate("chart.0.1.10", chartURL, "0.1.10", nil),
		unsynced,
		newHelmTemplate("chart.0.2.0-rc0", chartURL, "0.2.0-rc0", nil),
		newHelmTemplate("chart.0.2.0-rc1", chartURL, "0.2.0-rc1", nil),
		newHelmTemplate("chart.bad", chartURL, "bad version", nil),
		newHelmTemplate("other.0.9.0", "oci://registry/primussafe/other", "0.9.0", nil),
		newHelmTemplate("pinned.0.31.8", "oci://registry/pinned", "0.31.8", map[string]string{v1.AddonAutoUpgradeLabel: "false"}),
		newHelmTemplate("pinned.0.35.7", "oci://registry/pinned", "0.35.7", map[string]string{v1.AddonAutoUpgradeLabel: "false"}),
		newTemplateAddon("old", "chart.0.1.2", nil),
		withValues(newTemplateAddon("defaults", "chart.0.1.2", nil), "defaults: chart.0.1.2"),
		withValues(newTemplateAddon("custom", "chart.0.1.2", nil), "custom: true"),
		withValues(newTemplateAddon("defaults-newline", "chart.0.1.2", nil), "defaults: chart.0.1.2\n"),
		withValues(newTemplateAddon("one-override", "chart.0.1.2", nil), "defaults: chart.0.1.2\nextra: 1\n"),
		newTemplateAddon("opt-out", "chart.0.1.2", map[string]string{v1.AddonAutoUpgradeLabel: "false"}),
		newTemplateAddon("latest", "chart.0.1.10", nil),
		newTemplateAddon("prerelease", "chart.0.2.0-rc0", nil),
		newTemplateAddon("other", "other.0.9.0", nil),
		newTemplateAddon("pinned", "pinned.0.31.8", nil),
	}
	cl := ctrlfake.NewClientBuilder().WithScheme(scheme).WithObjects(objs...).Build()
	r := &AddonTemplateController{Client: cl}

	for _, name := range []string{"chart.0.1.10", "pinned.0.35.7"} {
		_, err := r.Reconcile(context.Background(), reconcileRequest(name))
		assert.NoError(t, err)
	}

	expected := map[string][2]string{
		"old":              {"chart.0.1.10", ""},
		"defaults":         {"chart.0.1.10", "defaults: chart.0.1.10"},
		"custom":           {"chart.0.1.2", "custom: true"},
		"defaults-newline": {"chart.0.1.10", "defaults: chart.0.1.10"},
		"one-override":     {"chart.0.1.2", "defaults: chart.0.1.2\nextra: 1\n"},
		"opt-out":          {"chart.0.1.2", ""},
		"latest":           {"chart.0.1.10", ""},
		"prerelease":       {"chart.0.2.0-rc1", ""},
		"other":            {"other.0.9.0", ""},
		"pinned":           {"pinned.0.31.8", ""},
	}
	for name, want := range expected {
		addon := &v1.Addon{}
		assert.NoError(t, cl.Get(context.Background(), types.NamespacedName{Name: name}, addon))
		assert.Equal(t, want[0], addon.Spec.AddonSource.HelmRepository.Template.Name, name)
		assert.Equal(t, want[1], addon.Spec.AddonSource.HelmRepository.Values, name)
	}
}

func TestUpgradeTargetSkipsVersionTies(t *testing.T) {
	const chartURL = "oci://registry/primussafe/tie"
	templates := []v1.AddonTemplate{
		*newHelmTemplate("tie.0.9.0", chartURL, "0.9.0", nil),
		*newHelmTemplate("tie.0.9.5", chartURL, "0.9.5", nil),
		*newHelmTemplate("tie.b.1.0.0", chartURL, "1.0.0", nil),
		*newHelmTemplate("tie.a.1.0.0", chartURL, "1.0.0", nil),
	}
	family, versions := chartTemplates(templates, &templates[0])
	old := newTemplateAddon("old", "tie.0.9.0", nil)
	same := newTemplateAddon("same", "tie.b.1.0.0", nil)
	for i := 0; i < 50; i++ {
		assert.Equal(t, "", upgradeTarget(old, family, versions))
		assert.Equal(t, "", upgradeTarget(same, family, versions))
	}

	family, versions = chartTemplates(templates[:3], &templates[0])
	assert.Equal(t, "tie.b.1.0.0", upgradeTarget(old, family, versions))
}

func TestAutoUpgradeKeepsDefaultAddon(t *testing.T) {
	const chartURL = "oci://registry/primussafe/training-operator"
	cluster := testCluster("c1")
	current := newHelmTemplate("training-operator.1.9.2", chartURL, "1.9.2", map[string]string{v1.AddonDefaultLabel: ""})
	next := newHelmTemplate("training-operator.1.10.0", chartURL, "1.10.0", nil)
	user := newTemplateAddon("user-training-operator", "training-operator.1.9.2", nil)
	plane := newPlaneReconciler(t, cluster, current, next, user)
	templates := &AddonTemplateController{Client: plane.Client}

	_, err := plane.guaranteeDefaultAddon(context.Background(), cluster)
	assert.NoError(t, err)
	created := &v1.Addon{}
	assert.NoError(t, plane.Get(context.Background(), types.NamespacedName{Name: "c1-training-operator"}, created))
	_, err = templates.Reconcile(context.Background(), reconcileRequest("training-operator.1.10.0"))
	assert.NoError(t, err)
	_, err = plane.guaranteeDefaultAddon(context.Background(), cluster)
	assert.NoError(t, err)
	settled := &v1.Addon{}
	assert.NoError(t, plane.Get(context.Background(), types.NamespacedName{Name: "c1-training-operator"}, settled))
	assert.Equal(t, created.ResourceVersion, settled.ResourceVersion, "the default Addon is not patched")

	for name, want := range map[string]string{
		"c1-training-operator":   "training-operator.1.9.2",
		"user-training-operator": "training-operator.1.10.0",
	} {
		addon := &v1.Addon{}
		assert.NoError(t, plane.Get(context.Background(), types.NamespacedName{Name: name}, addon))
		assert.Equal(t, want, addon.Spec.AddonSource.HelmRepository.Template.Name, name)
	}
}

func TestUpdateTemplateStatus(t *testing.T) {
	scheme, _ := genMockScheme()
	tmpl := &v1.AddonTemplate{ObjectMeta: metav1.ObjectMeta{Name: "t1"}}
	cl := ctrlfake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&v1.AddonTemplate{}).
		WithObjects(tmpl).
		Build()
	r := &AddonTemplateController{Client: cl}

	ch := &chart.Chart{
		Values: map[string]interface{}{"replicas": 3},
		Raw:    []*chart.File{{Name: "values.yaml", Data: []byte("replicas: 3")}},
	}
	err := r.updateTemplateStatus(context.Background(), tmpl, ch)
	assert.NoError(t, err)
	assert.Contains(t, tmpl.Status.HelmStatus.Values, "replicas")
	assert.Equal(t, "replicas: 3", tmpl.Status.HelmStatus.ValuesYAML)
}
