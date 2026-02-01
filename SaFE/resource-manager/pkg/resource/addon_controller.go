/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package resource

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"gopkg.in/yaml.v2"
	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart/loader"
	"helm.sh/helm/v3/pkg/cli"
	"helm.sh/helm/v3/pkg/release"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/rest"
	"k8s.io/klog/v2"
	ctrlruntime "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	commonclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/k8sclient"
	commonutils "github.com/AMD-AIG-AIMA/SAFE/common/pkg/utils"
)

// AddonController manages Helm addon installations and updates for clusters.
type AddonController struct {
	client.Client
	clustersGetter *ClustersGetter
}

// SetupAddonController initializes and registers the AddonController with the controller manager.
func SetupAddonController(mgr manager.Manager) error {
	addon := &AddonController{
		Client: mgr.GetClient(),
		clustersGetter: &ClustersGetter{
			Mutex:  sync.Mutex{},
			getter: map[string]*RESTClientGetter{},
		},
	}
	err := ctrlruntime.NewControllerManagedBy(mgr).
		For(&v1.Addon{}, builder.WithPredicates(predicate.ResourceVersionChangedPredicate{})).Complete(addon)
	if err != nil {
		return err
	}
	return nil
}

// Reconcile processes Addon resources by installing, upgrading, or uninstalling Helm charts.
func (r *AddonController) Reconcile(ctx context.Context, req ctrlruntime.Request) (ctrlruntime.Result, error) {
	startTime := time.Now().UTC()
	defer func() {
		klog.Infof("Finished reconcile addon %s cost (%v)", req.Name, time.Since(startTime))
	}()
	addon := &v1.Addon{}
	err := r.Get(ctx, req.NamespacedName, addon)
	if err != nil {
		if errors.IsNotFound(err) {
			return ctrlruntime.Result{}, nil
		}
		return ctrlruntime.Result{}, err
	}
	if err = r.guaranteeHelmAddon(ctx, addon); err != nil {
		return ctrlruntime.Result{}, err
	}
	return ctrlruntime.Result{}, nil
}

// guaranteeHelmAddon ensures the Helm addon is in the desired state based on its lifecycle.
func (r *AddonController) guaranteeHelmAddon(ctx context.Context, addon *v1.Addon) error {
	if addon.Spec.AddonSource.HelmRepository == nil {
		return nil
	}
	if addon.Spec.Cluster != nil {
		if _, err := getAdminCluster(ctx, r.Client, addon.Spec.Cluster.Name); err != nil {
			return client.IgnoreNotFound(err)
		}
	}
	if !addon.DeletionTimestamp.IsZero() {
		if addon.Status.Phase == v1.AddonDeleted {
			if controllerutil.RemoveFinalizer(addon, v1.AddonFinalizer) {
				return commonutils.PatchObjectFinalizer(ctx, r.Client, addon)
			}
		} else if addon.Status.Phase != v1.AddonDeleting {
			originalAddon := client.MergeFrom(addon.DeepCopy())
			addon.Status.Phase = v1.AddonDeleting
			return r.Status().Patch(ctx, addon, originalAddon)
		}
		originalAddon := client.MergeFrom(addon.DeepCopy())
		err := r.helmUninstall(ctx, addon)
		if err != nil {
			return err
		}
		addon.Status.Phase = v1.AddonDeleted
		err = r.Status().Patch(ctx, addon, originalAddon)
		if err != nil {
			return err
		}
		return nil
	}
	if controllerutil.AddFinalizer(addon, v1.AddonFinalizer) {
		return commonutils.PatchObjectFinalizer(ctx, r.Client, addon)
	}
	if addon.Status.AddonSourceStatus.HelmRepositoryStatus == nil {
		r.helmInstall(ctx, addon)
	}
	return r.helmUpgrade(ctx, addon)
}

// getHelm retrieves Helm chart information from either template or direct specification.
func (r *AddonController) getHelm(ctx context.Context, addon *v1.Addon) (string, string, string, string, error) {
	if addon.Spec.AddonSource.HelmRepository.Template != nil {
		template := new(v1.AddonTemplate)
		err := r.Get(ctx, types.NamespacedName{Name: addon.Spec.AddonSource.HelmRepository.Template.Name}, template)
		if err != nil {
			return "", "", "", "", fmt.Errorf("get addon template failed %s", err)
		}
		values := addon.Spec.AddonSource.HelmRepository.Values
		if values == "" {
			values = template.Spec.HelmDefaultValues
		}
		if strings.HasPrefix(template.Spec.URL, "oci://") {
			return template.Spec.URL, "", template.Spec.Version, values, nil
		}
		index := strings.LastIndex(template.Spec.URL, "/")
		if index == -1 || index == len(template.Spec.URL)-1 {
			return "", "", "", "", fmt.Errorf("get addon template url error ")
		}
		return template.Spec.URL[index+1:], template.Spec.URL[:index], template.Spec.Version, values, nil
	}
	return addon.Spec.AddonSource.HelmRepository.URL, "", addon.Spec.AddonSource.HelmRepository.ChartVersion, addon.Spec.AddonSource.HelmRepository.Values, nil
}

// helmInstall installs a Helm chart for the addon.
func (r *AddonController) helmInstall(ctx context.Context, addon *v1.Addon) error {
	name, url, version, values, err := r.getHelm(ctx, addon)
	if err != nil {
		return r.patchErrorStatus(ctx, addon, err)
	}
	actionConfig, settings, err := r.getActionConfig(ctx, addon)
	if err != nil {
		return err
	}
	installClient := action.NewInstall(actionConfig)
	installClient.Timeout = Timeout
	installClient.Namespace = GetReleaseNamespace(addon)
	installClient.ReleaseName = addon.Spec.AddonSource.HelmRepository.ReleaseName
	installClient.CreateNamespace = true
	installClient.Version = version
	installClient.PlainHTTP = addon.Spec.AddonSource.HelmRepository.PlainHTTP
	installClient.InsecureSkipTLSverify = true
	if url != "" {
		installClient.RepoURL = url
	}

	chartRequested, err := installClient.ChartPathOptions.LocateChart(name, settings)
	if err != nil {
		return r.patchErrorStatus(ctx, addon, fmt.Errorf("helm install helm chart download %s", err))
	}

	chart, err := loader.Load(chartRequested)
	if err != nil {
		return r.patchErrorStatus(ctx, addon, fmt.Errorf("helm install helm chart load %s", err))
	}
	valuesMap := map[string]interface{}{}
	if values != "" {
		err = yaml.Unmarshal([]byte(values), valuesMap)
		if err != nil {
			return r.patchErrorStatus(ctx, addon, err)
		}
	}
	valuesMap = replaceValues(valuesMap, chart.Values)

	resp, err := installClient.RunWithContext(ctx, chart, valuesMap)
	if err != nil {
		if err.Error() == installedMsg {
			return r.helmStatus(ctx, addon)
		}
		return r.patchErrorStatus(ctx, addon, fmt.Errorf("helm chart install %s", err))
	}
	return r.updateAddonHelmStatus(ctx, addon, resp)
}

// getActionConfig creates and configures Helm action configuration for the addon's cluster.
func (r *AddonController) getActionConfig(ctx context.Context, addon *v1.Addon) (*action.Configuration, *cli.EnvSettings, error) {
	settings := cli.New()
	actionConfig := new(action.Configuration)
	if err := r.configureHelmClient(ctx, actionConfig, settings, addon); err != nil {
		return nil, nil, r.patchErrorStatus(ctx, addon, fmt.Errorf("helm install initializ action failed %s", err))
	}
	return actionConfig, settings, nil
}

// helmUpgrade upgrades an existing Helm release for the addon.
func (r *AddonController) helmUpgrade(ctx context.Context, addon *v1.Addon) error {
	if shouldIgnoreUpgrade(addon) {
		return r.handleIgnoredUpgrade(ctx, addon)
	}

	name, url, version, values, err := r.getHelm(ctx, addon)
	if err != nil {
		return r.patchErrorStatus(ctx, addon, err)
	}

	upgradeClient, err := r.createUpgradeClient(ctx, addon, url, version)
	if err != nil {
		return err
	}

	return r.executeUpgrade(ctx, addon, upgradeClient, name, values)
}

// handleIgnoredUpgrade handles upgrade logic when upgrade should be ignored.
func (r *AddonController) handleIgnoredUpgrade(ctx context.Context, addon *v1.Addon) error {
	if addon.Spec.AddonSource.HelmRepository.PreviousVersion != nil &&
		addon.Status.AddonSourceStatus.HelmRepositoryStatus.PreviousVersion != *addon.Spec.AddonSource.HelmRepository.PreviousVersion {
		return r.helmRollback(ctx, addon)
	}
	return r.helmStatus(ctx, addon)
}

// createUpgradeClient creates and configures a Helm upgrade client.
func (r *AddonController) createUpgradeClient(ctx context.Context, addon *v1.Addon, url, version string) (*action.Upgrade, error) {
	actionConfig, _, err := r.getActionConfig(ctx, addon)
	if err != nil {
		return nil, err
	}

	upgradeClient := action.NewUpgrade(actionConfig)
	upgradeClient.Install = true
	upgradeClient.Timeout = Timeout
	upgradeClient.Namespace = GetReleaseNamespace(addon)
	upgradeClient.Version = version
	upgradeClient.PlainHTTP = addon.Spec.AddonSource.HelmRepository.PlainHTTP
	upgradeClient.MaxHistory = MaxHistory
	upgradeClient.InsecureSkipTLSverify = true
	upgradeClient.DisableHooks = true // Disable pre-upgrade hooks and all other hooks

	if url != "" {
		upgradeClient.RepoURL = url
	}

	return upgradeClient, nil
}

// executeUpgrade executes the Helm upgrade operation.
func (r *AddonController) executeUpgrade(ctx context.Context, addon *v1.Addon, upgradeClient *action.Upgrade, name, values string) error {
	settings := cli.New()
	chartRequested, err := upgradeClient.ChartPathOptions.LocateChart(name, settings)
	if err != nil {
		return r.patchErrorStatus(ctx, addon, fmt.Errorf("helm install helm chart download failed %s", err))
	}

	chart, err := loader.Load(chartRequested)
	if err != nil {
		return r.patchErrorStatus(ctx, addon, fmt.Errorf("helm install helm chart load failed %s", err))
	}

	valuesMap := map[string]interface{}{}
	if values != "" {
		err = yaml.Unmarshal([]byte(values), valuesMap)
		if err != nil {
			return r.patchErrorStatus(ctx, addon, err)
		}
	}
	valuesMap = replaceValues(valuesMap, chart.Values)

	resp, err := upgradeClient.RunWithContext(ctx, addon.Spec.AddonSource.HelmRepository.ReleaseName, chart, valuesMap)
	if err != nil {
		if err.Error() == installedMsg {
			return r.helmStatus(ctx, addon)
		}
		return r.patchErrorStatus(ctx, addon, fmt.Errorf("helm chart install failed %s", err))
	}

	return r.updateAddonHelmStatus(ctx, addon, resp)
}

// helmRollback rollback a Helm release to a previous version.
func (r *AddonController) helmRollback(ctx context.Context, addon *v1.Addon) error {
	if addon.Spec.AddonSource.HelmRepository.PreviousVersion == nil {
		return r.helmStatus(ctx, addon)
	}
	if addon.Status.AddonSourceStatus.HelmRepositoryStatus.PreviousVersion == *addon.Spec.AddonSource.HelmRepository.PreviousVersion {
		return r.helmStatus(ctx, addon)
	}
	actionConfig, _, err := r.getActionConfig(ctx, addon)
	if err != nil {
		return err
	}
	originalAddon := client.MergeFrom(addon.DeepCopy())
	rollback := action.NewRollback(actionConfig)
	rollback.Version = *addon.Spec.AddonSource.HelmRepository.PreviousVersion
	err = rollback.Run(addon.Spec.AddonSource.HelmRepository.ReleaseName)
	if err != nil {
		return err
	}

	statusClient := action.NewStatus(actionConfig)
	resp, err := statusClient.Run(addon.Spec.AddonSource.HelmRepository.ReleaseName)
	if err != nil {
		return err
	}
	values := rollbackValues(addon.Spec.AddonSource.HelmRepository.Values, resp.Config)
	addon.Spec.AddonSource.HelmRepository.Values = values
	addon.Spec.AddonSource.HelmRepository.ChartVersion = resp.Chart.Metadata.Version
	err = r.Patch(ctx, addon, originalAddon)
	if err != nil {
		return err
	}
	addon.Status.AddonSourceStatus.HelmRepositoryStatus.Values = values
	addon.Status.AddonSourceStatus.HelmRepositoryStatus.PreviousVersion = *addon.Spec.AddonSource.HelmRepository.PreviousVersion
	return r.Status().Patch(ctx, addon, originalAddon)
}

// helmUninstall uninstalls the Helm chart for the addon.
func (r *AddonController) helmUninstall(ctx context.Context, addon *v1.Addon) error {
	if addon.Spec.AddonSource.HelmRepository == nil {
		return nil
	}
	if addon.Status.AddonSourceStatus.HelmRepositoryStatus == nil {
		addon.Status.Phase = v1.AddonDeleted
		return nil
	}
	actionConfig, _, err := r.getActionConfig(ctx, addon)
	if err != nil {
		if errors.IsNotFound(err) {
			return nil
		}
		return err
	}
	uninstallClient := action.NewUninstall(actionConfig)
	uninstallClient.Timeout = Timeout
	uninstallClient.IgnoreNotFound = true

	_, err = uninstallClient.Run(addon.Spec.AddonSource.HelmRepository.ReleaseName)
	if err != nil {
		if errors.IsNotFound(err) {
			return nil
		}
		return err
	}
	return nil
}

// helmStatus retrieves and updates the status of a Helm release.
func (r *AddonController) helmStatus(ctx context.Context, addon *v1.Addon) error {
	if addon.Spec.AddonSource.HelmRepository == nil {
		return nil
	}

	actionConfig, settings, err := r.getActionConfig(ctx, addon)
	if err != nil {
		if errors.IsNotFound(err) {
			return nil
		}
		return err
	}
	if err = r.configureHelmClient(ctx, actionConfig, settings, addon); err != nil {
		return err
	}
	statusClient := action.NewStatus(actionConfig)
	resp, err := statusClient.Run(addon.Spec.AddonSource.HelmRepository.ReleaseName)
	if err != nil {
		if errors.IsNotFound(err) {
			originalAddon := client.MergeFrom(addon.DeepCopy())
			addon.Status = v1.AddonStatus{}
			return r.Status().Patch(ctx, addon, originalAddon)
		}
		return err
	}
	return r.updateAddonHelmStatus(ctx, addon, resp)
}

// updateAddonHelmStatus updates the addon's status based on Helm release information.
func (r *AddonController) updateAddonHelmStatus(ctx context.Context, addon *v1.Addon, resp *release.Release) error {
	originalAddon := client.MergeFrom(addon.DeepCopy())
	if addon.Status.AddonSourceStatus.HelmRepositoryStatus == nil {
		addon.Status.AddonSourceStatus.HelmRepositoryStatus = &v1.HelmRepositoryStatus{}
	}
	addon.Status.AddonSourceStatus.HelmRepositoryStatus.FirstDeployed = metav1.NewTime(resp.Info.FirstDeployed.Time)
	addon.Status.AddonSourceStatus.HelmRepositoryStatus.LastDeployed = metav1.NewTime(resp.Info.LastDeployed.Time)
	addon.Status.AddonSourceStatus.HelmRepositoryStatus.Deleted = metav1.NewTime(resp.Info.Deleted.Time)
	addon.Status.AddonSourceStatus.HelmRepositoryStatus.Description = resp.Info.Description
	addon.Status.AddonSourceStatus.HelmRepositoryStatus.Notes = resp.Info.Notes
	addon.Status.AddonSourceStatus.HelmRepositoryStatus.Status = string(resp.Info.Status)
	addon.Status.AddonSourceStatus.HelmRepositoryStatus.Version = resp.Version
	addon.Status.AddonSourceStatus.HelmRepositoryStatus.Values = addon.Spec.AddonSource.HelmRepository.Values
	if addon.Status.AddonSourceStatus.HelmRepositoryStatus.Status == v1.AddonDeployed {
		addon.Status.Phase = v1.AddonRunning
	} else {
		addon.Status.Phase = v1.AddonPhaseType(addon.Status.AddonSourceStatus.HelmRepositoryStatus.Status)
	}
	return r.Status().Patch(ctx, addon, originalAddon)
}

// patchErrorStatus updates the addon's status to indicate an error occurred.
func (r *AddonController) patchErrorStatus(ctx context.Context, addon *v1.Addon, err error) error {
	klog.Errorf("patch Error Status: %v", err)
	originalAddon := client.MergeFrom(addon.DeepCopy())
	addon.Status.Phase = v1.AddonError
	patchErr := r.Status().Patch(ctx, addon, originalAddon)
	if patchErr != nil {
		klog.Errorf("Addon Failed : %v", err)
		return patchErr
	}
	return err
}

// getter retrieves or creates a REST client getter for the specified cluster.
func (r *AddonController) getter(ctx context.Context, addon *v1.Addon) (*RESTClientGetter, error) {
	return r.clustersGetter.get(ctx, addon.Spec.Cluster, r.getCluster)
}

// getCluster retrieves the REST configuration for a cluster.
func (r *AddonController) getCluster(ctx context.Context, cluster *corev1.ObjectReference) (*rest.Config, error) {
	c, err := getAdminCluster(ctx, r.Client, cluster.Name)
	if err != nil {
		return nil, err
	}
	cert := c.Status.ControlPlaneStatus
	_, restCfg, err := commonclient.NewClientSet(fmt.Sprintf("https://%s", c.Name),
		cert.CertData, cert.KeyData, cert.CAData, true)
	if err != nil {
		return nil, err
	}
	restCfg.Insecure = true
	return restCfg, err
}

// configureHelmClient configures the Helm client with registry and getter settings.
func (r *AddonController) configureHelmClient(ctx context.Context, actionConfig *action.Configuration, settings *cli.EnvSettings, addon *v1.Addon) error {
	var err error
	actionConfig.RegistryClient, err = newDefaultRegistryClient(addon.Spec.AddonSource.HelmRepository.PlainHTTP, settings)
	if err != nil {
		return err
	}
	getter, err := r.getter(ctx, addon)
	if err != nil {
		return err
	}

	if err = actionConfig.Init(getter, GetReleaseNamespace(addon), helmDriver, klog.Infof); err != nil {
		return fmt.Errorf("helm initializ action failed %s", err)
	}
	settings.KubeInsecureSkipTLSVerify = true
	return nil
}
