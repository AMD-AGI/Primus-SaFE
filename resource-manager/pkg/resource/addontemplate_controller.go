/*
 * Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package resource

import (
	"context"
	"strings"

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

// AddonTemplateController manages AddonTemplate resources by fetching and storing Helm chart default values
type AddonTemplateController struct {
	client.Client
	getter *RESTClientGetter
}

// SetupAddonTemplateController initializes and registers the AddonTemplateController with the controller manager
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

// Reconcile processes AddonTemplate resources by fetching Helm chart default values and storing them in status
func (r *AddonTemplateController) Reconcile(ctx context.Context, req ctrlruntime.Request) (ctrlruntime.Result, error) {
	template := &v1.AddonTemplate{}
	err := r.Get(ctx, req.NamespacedName, template)
	if err != nil {
		if errors.IsNotFound(err) {
			return ctrlruntime.Result{}, nil
		}
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

// initializeActionConfig creates and initializes Helm action configuration
func (r *AddonTemplateController) initializeActionConfig() (*action.Configuration, error) {
	settings := cli.New()
	settings.Debug = true

	actionConfig := new(action.Configuration)
	var err error
	actionConfig.RegistryClient, err = newDefaultRegistryClient(false, settings)
	if err != nil {
		return nil, err
	}

	if err = actionConfig.Init(r.getter, v1.DefaultNamespace, helmDriver, klog.Infof); err != nil {
		return nil, err
	}

	return actionConfig, nil
}

// fetchChart downloads and loads a Helm chart
func (r *AddonTemplateController) fetchChart(template *v1.AddonTemplate, actionConfig *action.Configuration) (*chart.Chart, error) {
	installClient := action.NewInstall(actionConfig)
	installClient.Timeout = Timeout
	installClient.Namespace = v1.DefaultNamespace
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

// getChartName determines the chart name and configures repository URL if needed
func (r *AddonTemplateController) getChartName(template *v1.AddonTemplate, installClient *action.Install) string {
	name := template.Spec.URL
	if !strings.HasPrefix(name, "oci://") {
		name = template.Name
		installClient.ChartPathOptions.RepoURL = template.Spec.URL
	}
	return name
}

// updateTemplateStatus updates the template status with chart values
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
