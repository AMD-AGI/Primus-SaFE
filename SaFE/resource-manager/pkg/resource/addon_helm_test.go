/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package resource

import (
	"context"
	"io"
	"testing"

	"github.com/agiledragon/gomonkey/v2"
	"github.com/stretchr/testify/assert"
	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/chartutil"
	"helm.sh/helm/v3/pkg/cli"
	kubefake "helm.sh/helm/v3/pkg/kube/fake"
	"helm.sh/helm/v3/pkg/release"
	"helm.sh/helm/v3/pkg/storage"
	"helm.sh/helm/v3/pkg/storage/driver"
	helmtime "helm.sh/helm/v3/pkg/time"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrlruntime "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlfake "sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
)

func reconcileRequest(name string) ctrlruntime.Request {
	return ctrlruntime.Request{NamespacedName: types.NamespacedName{Name: name}}
}

func newMemActionConfig(t *testing.T, releaseName string, version int) *action.Configuration {
	t.Helper()
	store := storage.Init(driver.NewMemory())
	cfg := &action.Configuration{
		Releases:     store,
		KubeClient:   &kubefake.PrintingKubeClient{Out: io.Discard},
		Capabilities: chartutil.DefaultCapabilities,
		Log:          func(string, ...interface{}) {},
	}
	rel := &release.Release{
		Name:      releaseName,
		Namespace: "default",
		Version:   version,
		Info: &release.Info{
			Status:        release.StatusDeployed,
			FirstDeployed: helmtime.Now(),
			LastDeployed:  helmtime.Now(),
			Description:   "Install complete",
		},
		Chart: &chart.Chart{Metadata: &chart.Metadata{Name: releaseName, Version: "1.0.0"}},
		Config: map[string]interface{}{},
	}
	if err := store.Create(rel); err != nil {
		t.Fatal(err)
	}
	return cfg
}

func newHelmAddonController(t *testing.T, objs ...client.Object) *AddonController {
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

func helmAddon(name, releaseName string) *v1.Addon {
	return &v1.Addon{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Spec: v1.AddonSpec{
			AddonSource: v1.AddonSource{
				HelmRepository: &v1.HelmRepository{
					ReleaseName: releaseName,
					Namespace:   "default",
				},
			},
		},
		Status: v1.AddonStatus{
			AddonSourceStatus: v1.AddonSourceStatus{
				HelmRepositoryStatus: &v1.HelmRepositoryStatus{},
			},
		},
	}
}

func patchAddonHelm(r *AddonController, cfg *action.Configuration) *gomonkey.Patches {
	p := gomonkey.NewPatches()
	p.ApplyPrivateMethod(r, "getActionConfig",
		func(_ context.Context, _ *v1.Addon) (*action.Configuration, *cli.EnvSettings, error) {
			return cfg, cli.New(), nil
		})
	p.ApplyPrivateMethod(r, "configureHelmClient",
		func(_ context.Context, _ *action.Configuration, _ *cli.EnvSettings, _ *v1.Addon) error {
			return nil
		})
	return p
}

func TestHelmStatusViaMemory(t *testing.T) {
	addon := helmAddon("a1", "rel1")
	r := newHelmAddonController(t, addon)
	cfg := newMemActionConfig(t, "rel1", 1)
	patches := patchAddonHelm(r, cfg)
	defer patches.Reset()

	err := r.helmStatus(context.Background(), addon)
	assert.NoError(t, err)
	assert.Equal(t, v1.AddonPhaseType(v1.AddonRunning), addon.Status.Phase)
}

func TestHelmStatusNotFoundResets(t *testing.T) {
	addon := helmAddon("a1", "missing-rel")
	r := newHelmAddonController(t, addon)
	// Empty store -> status returns not found -> resets addon status.
	cfg := &action.Configuration{
		Releases:     storage.Init(driver.NewMemory()),
		KubeClient:   &kubefake.PrintingKubeClient{Out: io.Discard},
		Capabilities: chartutil.DefaultCapabilities,
		Log:          func(string, ...interface{}) {},
	}
	patches := patchAddonHelm(r, cfg)
	defer patches.Reset()
	// Helm's release-not-found is not a k8s NotFound, so it surfaces as an error.
	err := r.helmStatus(context.Background(), addon)
	assert.Error(t, err)
}

func TestGuaranteeHelmAddonInstall(t *testing.T) {
	addon := helmAddon("a1", "rel-gi")
	addon.Spec.Cluster = nil
	addon.Status.AddonSourceStatus.HelmRepositoryStatus = nil
	addon.Finalizers = []string{v1.AddonFinalizer}
	r := newHelmAddonController(t, addon)
	store := storage.Init(driver.NewMemory())
	cfg := &action.Configuration{
		Releases:     store,
		KubeClient:   &kubefake.PrintingKubeClient{Out: io.Discard},
		Capabilities: chartutil.DefaultCapabilities,
		Log:          func(string, ...interface{}) {},
	}
	chartPath := createTestChart(t)
	patches := patchAddonHelmWithChart(r, cfg, chartPath)
	defer patches.Reset()
	// HelmRepositoryStatus nil + finalizer present -> helmInstall path.
	err := r.guaranteeHelmAddon(context.Background(), addon)
	assert.NoError(t, err)
}

func TestHelmUninstallViaMemory(t *testing.T) {
	addon := helmAddon("a1", "rel1")
	r := newHelmAddonController(t, addon)
	cfg := newMemActionConfig(t, "rel1", 1)
	patches := patchAddonHelm(r, cfg)
	defer patches.Reset()

	err := r.helmUninstall(context.Background(), addon)
	assert.NoError(t, err)
}

func TestHelmUninstallNoStatus(t *testing.T) {
	addon := helmAddon("a1", "rel1")
	addon.Status.AddonSourceStatus.HelmRepositoryStatus = nil
	r := newHelmAddonController(t, addon)
	// No helm status -> marks deleted, returns nil without action config.
	err := r.helmUninstall(context.Background(), addon)
	assert.NoError(t, err)
	assert.Equal(t, v1.AddonPhaseType(v1.AddonDeleted), addon.Status.Phase)
}

func TestHelmRollbackNoPreviousVersion(t *testing.T) {
	addon := helmAddon("a1", "rel1")
	r := newHelmAddonController(t, addon)
	cfg := newMemActionConfig(t, "rel1", 1)
	patches := patchAddonHelm(r, cfg)
	defer patches.Reset()
	// No PreviousVersion -> falls back to helmStatus.
	err := r.helmRollback(context.Background(), addon)
	assert.NoError(t, err)
}

func newMemActionConfigWithHistory(t *testing.T, releaseName string) *action.Configuration {
	t.Helper()
	store := storage.Init(driver.NewMemory())
	cfg := &action.Configuration{
		Releases:     store,
		KubeClient:   &kubefake.PrintingKubeClient{Out: io.Discard},
		Capabilities: chartutil.DefaultCapabilities,
		Log:          func(string, ...interface{}) {},
	}
	ch := &chart.Chart{Metadata: &chart.Metadata{Name: releaseName, Version: "1.0.0"}}
	// Version 1 superseded, version 2 currently deployed.
	v1rel := &release.Release{
		Name: releaseName, Namespace: "default", Version: 1,
		Info:  &release.Info{Status: release.StatusSuperseded, FirstDeployed: helmtime.Now(), LastDeployed: helmtime.Now()},
		Chart: ch, Config: map[string]interface{}{},
	}
	v2rel := &release.Release{
		Name: releaseName, Namespace: "default", Version: 2,
		Info:  &release.Info{Status: release.StatusDeployed, FirstDeployed: helmtime.Now(), LastDeployed: helmtime.Now()},
		Chart: ch, Config: map[string]interface{}{},
	}
	if err := store.Create(v1rel); err != nil {
		t.Fatal(err)
	}
	if err := store.Create(v2rel); err != nil {
		t.Fatal(err)
	}
	return cfg
}

func TestHelmRollbackToPreviousVersion(t *testing.T) {
	addon := helmAddon("a1", "rel-rb")
	prev := 1
	addon.Spec.AddonSource.HelmRepository.PreviousVersion = &prev
	r := newHelmAddonController(t, addon)
	cfg := newMemActionConfigWithHistory(t, "rel-rb")
	patches := patchAddonHelm(r, cfg)
	defer patches.Reset()
	err := r.helmRollback(context.Background(), addon)
	assert.NoError(t, err)
	assert.Equal(t, 1, addon.Status.AddonSourceStatus.HelmRepositoryStatus.PreviousVersion)
}

func TestHelmRollbackAlreadyAtPrevious(t *testing.T) {
	addon := helmAddon("a1", "rel1")
	prev := 1
	addon.Spec.AddonSource.HelmRepository.PreviousVersion = &prev
	addon.Status.AddonSourceStatus.HelmRepositoryStatus.PreviousVersion = 1
	r := newHelmAddonController(t, addon)
	cfg := newMemActionConfig(t, "rel1", 1)
	patches := patchAddonHelm(r, cfg)
	defer patches.Reset()
	// status.PreviousVersion already equals spec -> helmStatus path.
	err := r.helmRollback(context.Background(), addon)
	assert.NoError(t, err)
}

func TestHandleIgnoredUpgrade(t *testing.T) {
	addon := helmAddon("a1", "rel1")
	r := newHelmAddonController(t, addon)
	cfg := newMemActionConfig(t, "rel1", 1)
	patches := patchAddonHelm(r, cfg)
	defer patches.Reset()
	// No previous version mismatch -> helmStatus path.
	err := r.handleIgnoredUpgrade(context.Background(), addon)
	assert.NoError(t, err)
}

func TestAddonReconcileNotFound(t *testing.T) {
	r := newHelmAddonController(t)
	res, err := r.Reconcile(context.Background(), reconcileRequest("missing"))
	assert.NoError(t, err)
	assert.Equal(t, int64(0), res.RequeueAfter.Nanoseconds())
}

func TestAddonReconcileNoHelm(t *testing.T) {
	addon := &v1.Addon{ObjectMeta: metav1.ObjectMeta{Name: "a1"}}
	r := newHelmAddonController(t, addon)
	// No HelmRepository -> guaranteeHelmAddon returns nil.
	_, err := r.Reconcile(context.Background(), reconcileRequest("a1"))
	assert.NoError(t, err)
}

func createTestChart(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	chartName := "testchart"
	if _, err := chartutil.Create(chartName, dir); err != nil {
		t.Fatal(err)
	}
	return dir + "/" + chartName
}

func patchAddonHelmWithChart(r *AddonController, cfg *action.Configuration, chartPath string) *gomonkey.Patches {
	p := patchAddonHelm(r, cfg)
	p.ApplyPrivateMethod(r, "getHelm",
		func(_ context.Context, _ *v1.Addon) (string, string, string, string, error) {
			return chartPath, "", "", "", nil
		})
	return p
}

func TestHelmInstallViaMemory(t *testing.T) {
	addon := helmAddon("a1", "rel-install")
	addon.Status.AddonSourceStatus.HelmRepositoryStatus = nil
	r := newHelmAddonController(t, addon)

	store := storage.Init(driver.NewMemory())
	cfg := &action.Configuration{
		Releases:     store,
		KubeClient:   &kubefake.PrintingKubeClient{Out: io.Discard},
		Capabilities: chartutil.DefaultCapabilities,
		Log:          func(string, ...interface{}) {},
	}
	chartPath := createTestChart(t)
	patches := patchAddonHelmWithChart(r, cfg, chartPath)
	defer patches.Reset()

	err := r.helmInstall(context.Background(), addon)
	assert.NoError(t, err)
}

func TestHelmUpgradeViaMemory(t *testing.T) {
	addon := helmAddon("a1", "rel-upg")
	// Force upgrade (not ignored) by setting differing values.
	addon.Spec.AddonSource.HelmRepository.Values = "replicas: 2"
	addon.Status.AddonSourceStatus.HelmRepositoryStatus.Values = "replicas: 1"
	r := newHelmAddonController(t, addon)

	cfg := newMemActionConfig(t, "rel-upg", 1)
	chartPath := createTestChart(t)
	patches := patchAddonHelmWithChart(r, cfg, chartPath)
	defer patches.Reset()

	err := r.helmUpgrade(context.Background(), addon)
	assert.NoError(t, err)
}

func TestGuaranteeHelmAddonDelete(t *testing.T) {
	addon := helmAddon("a1", "rel1")
	now := metav1.Now()
	addon.DeletionTimestamp = &now
	addon.Finalizers = []string{v1.AddonFinalizer}
	addon.Status.Phase = v1.AddonDeleting
	r := newHelmAddonController(t, addon)
	cfg := newMemActionConfig(t, "rel1", 1)
	patches := patchAddonHelm(r, cfg)
	defer patches.Reset()
	// Deleting phase -> uninstall + mark deleted.
	err := r.guaranteeHelmAddon(context.Background(), addon)
	assert.NoError(t, err)
}
