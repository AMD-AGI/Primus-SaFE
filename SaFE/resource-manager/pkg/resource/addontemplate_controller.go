/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package resource

import (
	"context"
	"strings"

	"github.com/Masterminds/semver/v3"
	"gopkg.in/yaml.v2"
	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/chart/loader"
	"helm.sh/helm/v3/pkg/cli"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/klog/v2"
	ctrlruntime "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	commonclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/k8sclient"
)

// AddonTemplateController manages AddonTemplate resources by fetching and storing Helm chart default values.
type AddonTemplateController struct {
	client.Client
	getter *RESTClientGetter
}

// SetupAddonTemplateController initializes and registers the AddonTemplateController with the controller manager.
func SetupAddonTemplateController(mgr manager.Manager) error {
	cfg, err := commonclient.GetRestConfigInCluster()
	if err != nil {
		return nil
	}
	at := &AddonTemplateController{
		Client: mgr.GetClient(),
		getter: &RESTClientGetter{
			cfg: cfg,
		},
	}
	err = ctrlruntime.NewControllerManagedBy(mgr).
		For(&v1.AddonTemplate{}, builder.WithPredicates(predicate.GenerationChangedPredicate{})).
		Complete(at)
	if err != nil {
		return nil
	}
	klog.Infof("Setup AddonTemplate Controller successfully")
	return nil
}

// Reconcile processes AddonTemplate resources by fetching Helm chart default values and storing them in status.
func (r *AddonTemplateController) Reconcile(ctx context.Context, req ctrlruntime.Request) (ctrlruntime.Result, error) {
	template := &v1.AddonTemplate{}
	err := r.Get(ctx, req.NamespacedName, template)
	if err != nil {
		if errors.IsNotFound(err) {
			return ctrlruntime.Result{}, nil
		}
		return ctrlruntime.Result{}, err
	}

	if err = r.upgradeAddons(ctx, template); err != nil {
		return ctrlruntime.Result{}, err
	}

	if template.Spec.URL == "" || template.Status.HelmStatus.Values != "" {
		return ctrlruntime.Result{}, nil
	}

	actionConfig, err := r.initializeActionConfig()
	if err != nil {
		return ctrlruntime.Result{}, err
	}

	chart, err := r.fetchChart(template, actionConfig)
	if err != nil {
		klog.Errorf("loading chart failed: %v", err)
		return ctrlruntime.Result{}, nil
	}

	return ctrlruntime.Result{}, r.updateTemplateStatus(ctx, template, chart)
}

// upgradeAddons points Addons that use an older version of the template's chart to the highest version.
func (r *AddonTemplateController) upgradeAddons(ctx context.Context, template *v1.AddonTemplate) error {
	if !isAutoUpgradeTemplate(template) {
		return nil
	}
	templates := &v1.AddonTemplateList{}
	if err := r.List(ctx, templates); err != nil {
		return err
	}
	latest, versions := latestAddonTemplate(templates.Items, template)
	if latest == "" {
		return nil
	}
	addons := &v1.AddonList{}
	if err := r.List(ctx, addons); err != nil {
		return err
	}
	for i := range addons.Items {
		addon := &addons.Items[i]
		if !shouldUpgradeAddon(addon, latest, versions) {
			continue
		}
		previous := addon.Spec.AddonSource.HelmRepository.Template.Name
		original := client.MergeFrom(addon.DeepCopy())
		addon.Spec.AddonSource.HelmRepository.Template.Name = latest
		if err := r.Patch(ctx, addon, original); err != nil {
			return err
		}
		klog.Infof("upgrade addon %s template from %s to %s", addon.Name, previous, latest)
	}
	return nil
}

// isAutoUpgradeTemplate reports whether a template takes part in automatic Addon upgrades.
func isAutoUpgradeTemplate(template *v1.AddonTemplate) bool {
	return template.Spec.URL != "" && template.DeletionTimestamp.IsZero() &&
		template.Labels[v1.AddonAutoUpgradeLabel] != "false"
}

// latestAddonTemplate returns the highest-version template sharing the given template's chart,
// and the parsed versions of all such templates keyed by name.
func latestAddonTemplate(templates []v1.AddonTemplate, template *v1.AddonTemplate) (string, map[string]*semver.Version) {
	latest := ""
	versions := map[string]*semver.Version{}
	for i := range templates {
		t := &templates[i]
		if !isAutoUpgradeTemplate(t) || t.Spec.URL != template.Spec.URL || t.Spec.Type != template.Spec.Type {
			continue
		}
		version, err := semver.NewVersion(t.Spec.Version)
		if err != nil {
			continue
		}
		versions[t.Name] = version
		if latest == "" || version.GreaterThan(versions[latest]) {
			latest = t.Name
		}
	}
	return latest, versions
}

// shouldUpgradeAddon reports whether an Addon references a lower version than the latest template.
func shouldUpgradeAddon(addon *v1.Addon, latest string, versions map[string]*semver.Version) bool {
	helm := addon.Spec.AddonSource.HelmRepository
	if helm == nil || helm.Template == nil || !addon.DeletionTimestamp.IsZero() ||
		addon.Labels[v1.AddonAutoUpgradeLabel] == "false" {
		return false
	}
	current, ok := versions[helm.Template.Name]
	return ok && current.LessThan(versions[latest])
}

// initializeActionConfig creates and initializes Helm action configuration.
func (r *AddonTemplateController) initializeActionConfig() (*action.Configuration, error) {
	settings := cli.New()
	settings.Debug = true

	actionConfig := new(action.Configuration)
	var err error
	actionConfig.RegistryClient, err = newDefaultRegistryClient(false, settings)
	if err != nil {
		return nil, err
	}

	if err = actionConfig.Init(r.getter, DefaultNamespace, helmDriver, klog.Infof); err != nil {
		return nil, err
	}

	return actionConfig, nil
}

// fetchChart downloads and loads a Helm chart.
func (r *AddonTemplateController) fetchChart(template *v1.AddonTemplate, actionConfig *action.Configuration) (*chart.Chart, error) {
	installClient := action.NewInstall(actionConfig)
	installClient.Timeout = Timeout
	installClient.Namespace = DefaultNamespace
	installClient.ReleaseName = template.Name
	installClient.CreateNamespace = true
	installClient.ChartPathOptions.PlainHTTP = false
	installClient.ChartPathOptions.Version = template.Spec.Version

	name := r.getChartName(template, installClient)
	settings := cli.New()

	chartRequested, err := installClient.ChartPathOptions.LocateChart(name, settings)
	if err != nil {
		return nil, err
	}

	return loader.Load(chartRequested)
}

// getChartName determines the chart name and configures repository URL if needed.
func (r *AddonTemplateController) getChartName(template *v1.AddonTemplate, installClient *action.Install) string {
	name := template.Spec.URL
	if !strings.HasPrefix(name, "oci://") {
		name = template.Name
		installClient.ChartPathOptions.RepoURL = template.Spec.URL
	}
	return name
}

// updateTemplateStatus updates the template status with chart values.
func (r *AddonTemplateController) updateTemplateStatus(ctx context.Context, template *v1.AddonTemplate, chart *chart.Chart) error {
	originalTemplate := client.MergeFrom(template.DeepCopy())
	values, err := yaml.Marshal(chart.Values)
	if err != nil {
		return err
	}

	template.Status.HelmStatus.Values = string(values)

	for _, raw := range chart.Raw {
		if raw != nil && raw.Name == "values.yaml" {
			template.Status.HelmStatus.ValuesYAML = string(raw.Data)
		}
	}

	return r.Status().Patch(ctx, template, originalTemplate)
}
