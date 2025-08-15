/*
 * Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package resource

import (
	"context"
	"strings"
	"time"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	commonclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/k8sclient"
	"gopkg.in/yaml.v2"
	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart/loader"
	"helm.sh/helm/v3/pkg/cli"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/klog/v2"
	ctrlruntime "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

type AddonTemplateController struct {
	client.Client
	getter *RESTClientGetter
}

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

func (r *AddonTemplateController) Reconcile(ctx context.Context, req ctrlruntime.Request) (ctrlruntime.Result, error) {
	startTime := time.Now().UTC()
	defer func() {
		klog.Infof("Finished reconcile addon template %s cost (%v)", req.Name, time.Since(startTime))
	}()
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
	plainHTTP := false
	settings := cli.New()
	actionConfig := new(action.Configuration)
	actionConfig.RegistryClient, err = newDefaultRegistryClient(plainHTTP, settings)
	if err != nil {
		return ctrlruntime.Result{}, err
	}
	settings.Debug = true

	if err := actionConfig.Init(r.getter, v1.DefaultNamespace, helmDriver, klog.Infof); err != nil {
		return ctrlruntime.Result{}, err
	}
	installClient := action.NewInstall(actionConfig)
	installClient.Timeout = Timeout
	installClient.Namespace = v1.DefaultNamespace
	installClient.ReleaseName = template.Name
	installClient.CreateNamespace = true
	installClient.ChartPathOptions.PlainHTTP = plainHTTP
	installClient.ChartPathOptions.Version = template.Spec.Version
	name := template.Spec.URL
	if !strings.HasPrefix(name, "oci://") {
		name = template.Name
		installClient.ChartPathOptions.RepoURL = template.Spec.URL
	}
	chartRequested, err := installClient.ChartPathOptions.LocateChart(name, settings)
	if err != nil {
		return ctrlruntime.Result{}, err
	}
	chart, err := loader.Load(chartRequested)
	if err != nil {
		klog.Errorf("loading chart failed: %v", err)
		return ctrlruntime.Result{}, nil
	}
	p := client.MergeFrom(template.DeepCopy())
	values, err := yaml.Marshal(chart.Values)
	if err != nil {
		return ctrlruntime.Result{}, err
	}
	template.Status.HelmStatus.Values = string(values)
	for _, raw := range chart.Raw {
		if raw.Name == "values.yaml" {
			template.Status.HelmStatus.ValuesYAMl = string(raw.Data)

		}
	}
	return ctrlruntime.Result{}, r.Status().Patch(ctx, template, p)
}
