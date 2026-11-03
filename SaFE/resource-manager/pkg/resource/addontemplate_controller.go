/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package resource

import (
	"context"
	"reflect"
	"strings"

	"gopkg.in/yaml.v2"
	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/chart/loader"
	"helm.sh/helm/v3/pkg/cli"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/util/version"
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
		For(&v1.AddonTemplate{}, builder.WithPredicates(predicate.Or(predicate.GenerationChangedPredicate{}, predicate.LabelChangedPredicate{}))).
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

	if template.Spec.URL == "" {
		return ctrlruntime.Result{}, nil
	}

	if template.Status.HelmStatus.Values == "" {
		actionConfig, err := r.initializeActionConfig()
		if err != nil {
			return ctrlruntime.Result{}, err
		}

		chart, err := r.fetchChart(template, actionConfig)
		if err != nil {
			klog.Errorf("loading chart failed: %v", err)
			return ctrlruntime.Result{}, nil
		}

		if err = r.updateTemplateStatus(ctx, template, chart); err != nil {
			return ctrlruntime.Result{}, err
		}
	}

	return ctrlruntime.Result{}, r.upgradeAddons(ctx, template)
}

// upgradeAddons points Addons that use an older version of the template's chart to the highest synced version.
func (r *AddonTemplateController) upgradeAddons(ctx context.Context, template *v1.AddonTemplate) error {
	if _, ok := v1.AutoUpgradeVersion(template); !ok {
		return nil
	}
	templates := &v1.AddonTemplateList{}
	if err := r.List(ctx, templates); err != nil {
		return err
	}
	family, versions := chartTemplates(templates.Items, template)
	if len(family) < 2 {
		return nil
	}
	defaults := defaultComponents(templates.Items)
	addons := &v1.AddonList{}
	if err := r.List(ctx, addons); err != nil {
		return err
	}
	for i := range addons.Items {
		addon := &addons.Items[i]
		if isDefaultAddon(addon, defaults) {
			continue
		}
		target := upgradeTarget(addon, family, versions)
		if target == "" {
			continue
		}
		helm := addon.Spec.AddonSource.HelmRepository
		previous := helm.Template.Name
		values, ok := upgradedValues(helm.Values, family[previous], family[target])
		if !ok {
			klog.Infof("keep addon %s on template %s: its values differ from the template defaults", addon.Name, previous)
			continue
		}
		original := client.MergeFromWithOptions(addon.DeepCopy(), client.MergeFromWithOptimisticLock{})
		helm.Values = values
		helm.Template.Name = target
		if err := r.Patch(ctx, addon, original); err != nil {
			return err
		}
		klog.Infof("upgrade addon %s template from %s to %s", addon.Name, previous, target)
	}
	return nil
}

// defaultComponents returns the components of the templates labelled as cluster default Addons.
func defaultComponents(templates []v1.AddonTemplate) map[string]bool {
	components := map[string]bool{}
	for i := range templates {
		if _, ok := templates[i].Labels[v1.AddonDefaultLabel]; ok {
			components[getComponentName(templates[i].Name)] = true
		}
	}
	return components
}

// isDefaultAddon reports whether an Addon is the cluster default Addon of a component, named {cluster}-{component}.
// Its template follows the template labelled as the default, so it is not upgraded here.
func isDefaultAddon(addon *v1.Addon, defaults map[string]bool) bool {
	helm := addon.Spec.AddonSource.HelmRepository
	if addon.Spec.Cluster == nil || helm == nil || helm.Template == nil {
		return false
	}
	component := getComponentName(helm.Template.Name)
	return defaults[component] && addon.Name == addon.Spec.Cluster.Name+"-"+component
}

// chartTemplates returns the auto-upgrade templates that share the given template's chart, and their parsed
// versions, keyed by name. The given template is used as passed, so a status written in this reconcile is seen.
func chartTemplates(templates []v1.AddonTemplate, template *v1.AddonTemplate) (map[string]*v1.AddonTemplate, map[string]*version.Version) {
	family := map[string]*v1.AddonTemplate{}
	versions := map[string]*version.Version{}
	for i := range templates {
		t := &templates[i]
		if t.Name == template.Name {
			t = template
		}
		if !v1.IsSameChart(t, template) {
			continue
		}
		if v, ok := v1.AutoUpgradeVersion(t); ok {
			family[t.Name], versions[t.Name] = t, v
		}
	}
	return family, versions
}

// upgradeTarget returns the template an Addon should move to, or "" to keep it. The target is the highest version
// above the Addon's whose chart values are synced; a pre-release is a target only for an Addon on a pre-release.
// When several templates share the highest version, the Addon is kept.
func upgradeTarget(addon *v1.Addon, family map[string]*v1.AddonTemplate, versions map[string]*version.Version) string {
	helm := addon.Spec.AddonSource.HelmRepository
	if helm == nil || helm.Template == nil || !addon.DeletionTimestamp.IsZero() ||
		addon.Labels[v1.AddonAutoUpgradeLabel] == "false" {
		return ""
	}
	current, ok := versions[helm.Template.Name]
	if !ok {
		return ""
	}
	target, tied := "", ""
	for name, v := range versions {
		if !v1.IsUpgradeTarget(family[name], v, current) {
			continue
		}
		switch {
		case target == "" || v.GreaterThan(versions[target]):
			target, tied = name, ""
		case v.EqualTo(versions[target]):
			tied = name
		}
	}
	if tied != "" {
		klog.Infof("keep addon %s on template %s: templates %s and %s share the highest version",
			addon.Name, helm.Template.Name, target, tied)
		return ""
	}
	return target
}

// upgradedValues returns the values of an Addon moved from one template to another, and false when the Addon keeps
// its template. Empty values stay empty, values with the same data as the old template's defaults become the new
// template's defaults, and other values keep the Addon on its template.
func upgradedValues(values string, from, to *v1.AddonTemplate) (string, bool) {
	if strings.TrimSpace(values) == "" {
		return values, true
	}
	if sameValues(values, from.Spec.HelmDefaultValues) {
		return to.Spec.HelmDefaultValues, true
	}
	return values, false
}

// sameValues reports whether two Helm values documents hold the same data.
func sameValues(a, b string) bool {
	var x, y interface{}
	if yaml.Unmarshal([]byte(a), &x) != nil || yaml.Unmarshal([]byte(b), &y) != nil {
		return false
	}
	return reflect.DeepEqual(x, y)
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
