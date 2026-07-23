/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package resources

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/authority"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/resources/view"
	apiutils "github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/utils"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	commonconfig "github.com/AMD-AIG-AIMA/SAFE/common/pkg/config"
	commonerrors "github.com/AMD-AIG-AIMA/SAFE/common/pkg/errors"
	commonutils "github.com/AMD-AIG-AIMA/SAFE/common/pkg/utils"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/timeutil"
)

// A Primus-SaFE "Slurm cluster" is a per-workspace Helm release of the Slinky
// `slurm` chart (v1.2.0), deployed into the workspace's Kubernetes namespace via
// the Addon mechanism. This handler translates the UI's node-pool form into
// `slurm` chart values, creates/patches/deletes an Addon CR (which the
// AddonController turns into a helm install/upgrade/uninstall), and enriches the
// list/get responses with live status read from the Slinky NodeSet/Controller
// CRs on the target cluster.
//
// The defunct `clusters.slinky.slurm.net` CR from slurm-operator v0.3.0 is no
// longer used; v1.2.0 has no such Cluster CRD.

const (
	// slurmChartTemplate is the AddonTemplate name for the Slinky `slurm` chart.
	// It must match the AddonTemplate installed by the primus-safe-cr chart
	// (see charts/primus-safe-cr/templates/addon_template/slurm.1.2.0.yaml).
	slurmChartTemplate = "slurm.1.2.0"

	// slurmClusterLabel marks Addons that represent a Slurm cluster.
	slurmClusterLabel = v1.PrimusSafePrefix + "slurm.cluster"
	// slurmSpecAnnotation stores the node-pool/accounting/image spec as JSON so
	// the list/get responses can render the pools without re-parsing helm values.
	slurmSpecAnnotation = v1.PrimusSafePrefix + "slurm.spec"

	// amdGPUResourceName is the Kubernetes extended-resource name for AMD GPUs,
	// requested per slurmd pod when a pool declares GPUs.
	amdGPUResourceName = "amd.com/gpu"
)

// Slinky v1.2.0 CRDs (group slinky.slurm.net, version v1beta1) used for status reads.
var (
	slurmNodeSetGVR = schema.GroupVersionResource{
		Group:    "slinky.slurm.net",
		Version:  "v1beta1",
		Resource: "nodesets",
	}
	// The Slurm `slurm` chart deploys the controller (slurmctld) as a StatefulSet
	// named "<release>-controller"; the Controller CR itself carries no status in
	// v1.2.0, so controller readiness is read from the StatefulSet.
	statefulSetGVR = schema.GroupVersionResource{
		Group:    "apps",
		Version:  "v1",
		Resource: "statefulsets",
	}
)

// slurmSpec is the persisted form of a Slurm cluster's inputs (annotation JSON).
type slurmSpec struct {
	AccountingEnabled bool            `json:"accountingEnabled"`
	ImageTag          string          `json:"imageTag,omitempty"`
	Pools             []view.NodePool `json:"pools"`
	// Stopped marks a cluster whose components have been scaled to zero (via a
	// helm upgrade). The Addon and its config/PVCs are retained so the cluster
	// stays in the list as history and can be resumed. Pool node counts are kept
	// in Pools so Resume can restore the original replica counts.
	Stopped bool `json:"stopped,omitempty"`
	// RestapiDown tracks the second phase of a stop. Stopping a cluster is a
	// two-phase operation: first the worker NodeSets (and login node) scale to
	// zero while the REST API (slurmrestd) is kept running, because the Slinky
	// slurm-operator drains a NodeSet by creating a maintenance reservation via
	// slurmrestd before deleting worker pods; tearing slurmrestd down at the same
	// time deadlocks the drain and leaves the workers (and their GPUs) running.
	// Only once the workers have actually drained is RestapiDown set so a
	// subsequent render also scales the REST API to zero. It is always false when
	// the cluster is not Stopped.
	RestapiDown bool `json:"restapiDown,omitempty"`
	// Volumes are the workspace's configured volumes (HostPath/PFS) captured at
	// create/patch time. They are translated into per-component volume/mount
	// entries in the Slinky chart values so the shared filesystem is mounted into
	// the login and worker (slurmd) pods, mirroring how the job-manager
	// dispatcher mounts workspace storage into SaFE Workload pods. Persisting them
	// on the spec means Stop/Resume (which re-render from readSlurmSpec) keep the
	// mounts without needing to re-fetch the workspace.
	Volumes []v1.WorkspaceVolume `json:"volumes,omitempty"`
}

// ListSlurmCluster lists Slurm clusters (helm releases) in a workspace.
func (h *Handler) ListSlurmCluster(c *gin.Context) {
	handle(c, h.listSlurmCluster)
}

// GetSlurmCluster returns a single Slurm cluster.
func (h *Handler) GetSlurmCluster(c *gin.Context) {
	handle(c, h.getSlurmCluster)
}

// CreateSlurmCluster creates a Slurm cluster (helm release) in a workspace.
func (h *Handler) CreateSlurmCluster(c *gin.Context) {
	handle(c, h.createSlurmCluster)
}

// PatchSlurmCluster updates a Slurm cluster (helm upgrade).
func (h *Handler) PatchSlurmCluster(c *gin.Context) {
	handle(c, h.patchSlurmCluster)
}

// DeleteSlurmCluster deletes a Slurm cluster (helm uninstall).
func (h *Handler) DeleteSlurmCluster(c *gin.Context) {
	handle(c, h.deleteSlurmCluster)
}

// GetSlurmClusterLogin returns an SSH command for a Slurm cluster's login node.
func (h *Handler) GetSlurmClusterLogin(c *gin.Context) {
	handle(c, h.getSlurmClusterLogin)
}

// StopSlurmCluster scales a Slurm cluster's components to zero, keeping the
// Addon so the cluster remains in the list as history and can be resumed.
func (h *Handler) StopSlurmCluster(c *gin.Context) {
	handle(c, h.stopSlurmCluster)
}

// ResumeSlurmCluster restores a stopped Slurm cluster's replicas.
func (h *Handler) ResumeSlurmCluster(c *gin.Context) {
	handle(c, h.resumeSlurmCluster)
}

func (h *Handler) listSlurmCluster(c *gin.Context) (interface{}, error) {
	ctx := c.Request.Context()
	clusterName := c.GetString(common.Name)
	workspaceId := c.Query("workspaceId")
	if workspaceId == "" {
		return nil, commonerrors.NewBadRequest("workspaceId is required")
	}
	workspace, err := h.getAdminWorkspace(ctx, workspaceId)
	if err != nil {
		return nil, err
	}
	if err = h.accessController.Authorize(authority.AccessInput{
		Context:    ctx,
		Resource:   workspace,
		Verb:       v1.ListVerb,
		Workspaces: []string{workspaceId},
		UserId:     c.GetString(common.UserId),
	}); err != nil {
		return nil, err
	}

	addonList := &v1.AddonList{}
	if err = h.List(ctx, addonList, client.MatchingLabels{
		slurmClusterLabel:   v1.TrueStr,
		v1.WorkspaceIdLabel: workspaceId,
	}); err != nil {
		return nil, err
	}

	dyn, _ := h.slurmDynamicClient(clusterName)
	result := view.ListSlurmClusterResponse{}
	for i := range addonList.Items {
		addon := &addonList.Items[i]
		// Only include Slurm clusters deployed onto the requested target cluster.
		if addon.Spec.Cluster != nil && addon.Spec.Cluster.Name != clusterName {
			continue
		}
		result.Items = append(result.Items, h.cvtAddonToSlurmCluster(ctx, dyn, addon))
	}
	sort.Slice(result.Items, func(i, j int) bool {
		return result.Items[i].Name < result.Items[j].Name
	})
	result.TotalCount = len(result.Items)
	return result, nil
}

func (h *Handler) getSlurmCluster(c *gin.Context) (interface{}, error) {
	ctx := c.Request.Context()
	clusterName := c.GetString(common.Name)
	name := c.Param(common.SlurmClusterName)
	if name == "" {
		return nil, commonerrors.NewBadRequest("the slurm cluster name is empty")
	}
	workspaceId := c.Query("workspaceId")
	if workspaceId == "" {
		return nil, commonerrors.NewBadRequest("workspaceId is required")
	}
	workspace, err := h.getAdminWorkspace(ctx, workspaceId)
	if err != nil {
		return nil, err
	}
	if err = h.accessController.Authorize(authority.AccessInput{
		Context:    ctx,
		Resource:   workspace,
		Verb:       v1.GetVerb,
		Workspaces: []string{workspaceId},
		UserId:     c.GetString(common.UserId),
	}); err != nil {
		return nil, err
	}

	addon, err := h.getAdminAddon(ctx, slurmAddonName(clusterName, workspaceId, name))
	if err != nil {
		return nil, err
	}
	dyn, _ := h.slurmDynamicClient(clusterName)
	item := h.cvtAddonToSlurmCluster(ctx, dyn, addon)
	// Enrich the detail response with the live pod list (controller/login/worker/
	// restapi/accounting). Best-effort: skipped if the clientset is unavailable.
	if cs, csErr := h.slurmClientSet(clusterName); csErr == nil && addon.Spec.AddonSource.HelmRepository != nil {
		item.Pods = listSlurmPods(ctx, cs, workspaceId, addon.Spec.AddonSource.HelmRepository.ReleaseName)
	}
	return item, nil
}

func (h *Handler) createSlurmCluster(c *gin.Context) (interface{}, error) {
	ctx := c.Request.Context()
	clusterName := c.GetString(common.Name)

	req := &view.CreateSlurmClusterRequest{}
	body, err := apiutils.ParseRequestBody(c.Request, req)
	if err != nil {
		klog.ErrorS(err, "failed to parse request", "body", string(body))
		return nil, err
	}
	if req.Name == "" {
		return nil, commonerrors.NewBadRequest("name is required")
	}
	if req.WorkspaceId == "" {
		return nil, commonerrors.NewBadRequest("workspaceId is required")
	}
	if len(req.Pools) == 0 {
		return nil, commonerrors.NewBadRequest("at least one node pool is required")
	}
	if err = validateNodePools(req.Pools); err != nil {
		return nil, err
	}
	if err = validateSlurmClusterName(req.Name, req.WorkspaceId); err != nil {
		return nil, err
	}

	workspace, err := h.getAdminWorkspace(ctx, req.WorkspaceId)
	if err != nil {
		return nil, err
	}
	if !workspace.HasScope(v1.SlurmScope) {
		return nil, commonerrors.NewBadRequest("the workspace does not have the Slurm scope")
	}
	// Deploying a Slurm cluster into a workspace is an addon-create action scoped
	// to that workspace.
	if err = h.accessController.Authorize(authority.AccessInput{
		Context:      ctx,
		ResourceKind: v1.AddonKind,
		Verb:         v1.CreateVerb,
		Workspaces:   []string{req.WorkspaceId},
		UserId:       c.GetString(common.UserId),
	}); err != nil {
		return nil, err
	}

	cluster, err := h.getAdminCluster(ctx, clusterName)
	if err != nil {
		return nil, err
	}
	tmpl, err := h.getAdminAddonTemplate(ctx, slurmChartTemplate)
	if err != nil {
		klog.ErrorS(err, "failed to get slurm addon template", "template", slurmChartTemplate)
		return nil, err
	}
	if tmpl.Spec.Type != v1.AddonTemplateHelm {
		return nil, commonerrors.NewBadRequest("the slurm addon template is not a helm template")
	}

	spec := slurmSpec{
		AccountingEnabled: req.AccountingEnabled,
		ImageTag:          req.ImageTag,
		Pools:             req.Pools,
		Volumes:           workspace.Spec.Volumes,
	}
	releaseName := slurmReleaseName(req.Name)
	values, err := renderSlurmValues(spec, releaseName)
	if err != nil {
		return nil, err
	}
	specJSON, _ := json.Marshal(spec)
	addon := &v1.Addon{
		ObjectMeta: metav1.ObjectMeta{
			Name: slurmAddonName(clusterName, req.WorkspaceId, req.Name),
			Labels: map[string]string{
				v1.DisplayNameLabel: req.Name,
				v1.UserIdLabel:      c.GetString(common.UserId),
				v1.WorkspaceIdLabel: req.WorkspaceId,
				slurmClusterLabel:   v1.TrueStr,
			},
			Annotations: map[string]string{
				slurmSpecAnnotation: string(specJSON),
			},
		},
		Spec: v1.AddonSpec{
			Cluster: &corev1.ObjectReference{
				APIVersion: cluster.APIVersion,
				Kind:       cluster.Kind,
				Name:       cluster.Name,
			},
			AddonSource: v1.AddonSource{
				HelmRepository: &v1.HelmRepository{
					ReleaseName:  releaseName,
					URL:          tmpl.Spec.URL,
					ChartVersion: tmpl.Spec.Version,
					Namespace:    req.WorkspaceId,
					Values:       values,
					PlainHTTP:    false,
					Template: &corev1.ObjectReference{
						APIVersion: "amd.com/v1",
						Kind:       v1.AddOnTemplateKind,
						Name:       tmpl.Name,
					},
				},
			},
		},
	}
	if req.Description != "" {
		v1.SetAnnotation(addon, v1.DescriptionAnnotation, req.Description)
	}

	if err = h.Create(ctx, addon); err != nil {
		klog.ErrorS(err, "failed to create slurm cluster addon", "cluster", clusterName, "name", req.Name)
		return nil, err
	}
	klog.Infof("created slurm cluster %s (release %s) in workspace %s on cluster %s",
		req.Name, releaseName, req.WorkspaceId, clusterName)

	// Accounting needs a database that the slurm chart does not provision; stand
	// up a release-scoped MariaDB so slurmdbd (and `sacct`) work. Best-effort:
	// the operator retries once the secret/service appear.
	if spec.AccountingEnabled {
		if err = h.ensureMariaDB(ctx, clusterName, req.WorkspaceId, releaseName); err != nil {
			klog.ErrorS(err, "failed to provision MariaDB for slurm accounting",
				"cluster", clusterName, "release", releaseName)
		}
	}

	dyn, _ := h.slurmDynamicClient(clusterName)
	return h.cvtAddonToSlurmCluster(ctx, dyn, addon), nil
}

func (h *Handler) patchSlurmCluster(c *gin.Context) (interface{}, error) {
	ctx := c.Request.Context()
	clusterName := c.GetString(common.Name)
	name := c.Param(common.SlurmClusterName)
	if name == "" {
		return nil, commonerrors.NewBadRequest("the slurm cluster name is empty")
	}

	req := &view.PatchSlurmClusterRequest{}
	body, err := apiutils.ParseRequestBody(c.Request, req)
	if err != nil {
		klog.ErrorS(err, "failed to parse request", "body", string(body))
		return nil, err
	}

	workspaceId := c.Query("workspaceId")
	if workspaceId == "" {
		return nil, commonerrors.NewBadRequest("workspaceId is required")
	}
	addon, err := h.getAdminAddon(ctx, slurmAddonName(clusterName, workspaceId, name))
	if err != nil {
		return nil, err
	}
	if err = h.accessController.Authorize(authority.AccessInput{
		Context:  ctx,
		Resource: addon,
		Verb:     v1.UpdateVerb,
		UserId:   c.GetString(common.UserId),
	}); err != nil {
		return nil, err
	}

	spec := readSlurmSpec(addon)
	accountingWas := spec.AccountingEnabled
	if req.Pools != nil {
		if err = validateNodePools(req.Pools); err != nil {
			return nil, err
		}
		spec.Pools = req.Pools
	}
	if req.AccountingEnabled != nil {
		spec.AccountingEnabled = *req.AccountingEnabled
	}
	if req.ImageTag != nil {
		spec.ImageTag = *req.ImageTag
	}
	// Refresh the captured workspace volumes so edits to the workspace's storage
	// configuration are picked up on the next patch and re-rendered into the
	// login/worker volume mounts.
	if workspace, wsErr := h.getAdminWorkspace(ctx, workspaceId); wsErr == nil {
		spec.Volumes = workspace.Spec.Volumes
	} else {
		return nil, wsErr
	}

	releaseName := slurmReleaseName(name)
	if addon.Spec.AddonSource.HelmRepository != nil && addon.Spec.AddonSource.HelmRepository.ReleaseName != "" {
		releaseName = addon.Spec.AddonSource.HelmRepository.ReleaseName
	}
	values, err := renderSlurmValues(spec, releaseName)
	if err != nil {
		return nil, err
	}
	specJSON, _ := json.Marshal(spec)
	if addon.Spec.AddonSource.HelmRepository != nil {
		addon.Spec.AddonSource.HelmRepository.Values = values
	}
	v1.SetAnnotation(addon, slurmSpecAnnotation, string(specJSON))
	if req.Description != nil && *req.Description != "" {
		v1.SetAnnotation(addon, v1.DescriptionAnnotation, *req.Description)
	}

	if err = h.Update(ctx, addon); err != nil {
		klog.ErrorS(err, "failed to update slurm cluster addon", "cluster", clusterName, "name", name)
		return nil, err
	}
	klog.Infof("updated slurm cluster %s in workspace %s on cluster %s", name, workspaceId, clusterName)

	// Reconcile the accounting database to match the (possibly toggled) setting.
	if spec.AccountingEnabled {
		if err = h.ensureMariaDB(ctx, clusterName, workspaceId, releaseName); err != nil {
			klog.ErrorS(err, "failed to provision MariaDB for slurm accounting",
				"cluster", clusterName, "release", releaseName)
		}
	} else if accountingWas {
		if err = h.deleteMariaDB(ctx, clusterName, workspaceId, releaseName); err != nil {
			klog.ErrorS(err, "failed to remove MariaDB after disabling accounting",
				"cluster", clusterName, "release", releaseName)
		}
	}

	dyn, _ := h.slurmDynamicClient(clusterName)
	return h.cvtAddonToSlurmCluster(ctx, dyn, addon), nil
}

func (h *Handler) deleteSlurmCluster(c *gin.Context) (interface{}, error) {
	ctx := c.Request.Context()
	clusterName := c.GetString(common.Name)
	name := c.Param(common.SlurmClusterName)
	if name == "" {
		return nil, commonerrors.NewBadRequest("the slurm cluster name is empty")
	}
	workspaceId := c.Query("workspaceId")
	if workspaceId == "" {
		return nil, commonerrors.NewBadRequest("workspaceId is required")
	}

	addon, err := h.getAdminAddon(ctx, slurmAddonName(clusterName, workspaceId, name))
	if err != nil {
		return nil, err
	}
	if err = h.accessController.Authorize(authority.AccessInput{
		Context:  ctx,
		Resource: addon,
		Verb:     v1.DeleteVerb,
		UserId:   c.GetString(common.UserId),
	}); err != nil {
		return nil, err
	}
	if err = h.Delete(ctx, addon); err != nil {
		klog.ErrorS(err, "failed to delete slurm cluster addon", "cluster", clusterName, "name", name)
		return nil, err
	}
	// Best-effort cleanup of resources that helm uninstall leaves behind runs on a
	// detached context: it must survive client disconnects / request cancellation,
	// otherwise client-go rate limiting mid-request can abort it and orphan PVCs.
	cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Minute)
	defer cancel()
	// Tear down the accounting database (no-op if it was never created).
	if err = h.deleteMariaDB(cleanupCtx, clusterName, workspaceId, slurmReleaseName(name)); err != nil {
		klog.ErrorS(err, "failed to remove MariaDB during slurm cluster delete",
			"cluster", clusterName, "name", name)
	}
	// Remove the slurmctld statesave PVC that helm uninstall leaves behind
	// (StatefulSet volumeClaimTemplate PVCs are not garbage-collected).
	if err = h.deleteSlurmStatesave(cleanupCtx, clusterName, workspaceId, slurmReleaseName(name)); err != nil {
		klog.ErrorS(err, "failed to remove slurmctld statesave PVC during slurm cluster delete",
			"cluster", clusterName, "name", name)
	}
	klog.Infof("deleted slurm cluster %s in workspace %s on cluster %s", name, workspaceId, clusterName)
	return nil, nil
}

// setSlurmClusterStopped is the shared implementation for stop (stopped=true)
// and resume (stopped=false). It re-renders the helm values with the desired
// replica state, updates the Addon (triggering a helm upgrade), and scales the
// accounting MariaDB to match.
func (h *Handler) setSlurmClusterStopped(c *gin.Context, stopped bool) (interface{}, error) {
	ctx := c.Request.Context()
	clusterName := c.GetString(common.Name)
	name := c.Param(common.SlurmClusterName)
	if name == "" {
		return nil, commonerrors.NewBadRequest("the slurm cluster name is empty")
	}
	workspaceId := c.Query("workspaceId")
	if workspaceId == "" {
		return nil, commonerrors.NewBadRequest("workspaceId is required")
	}

	addon, err := h.getAdminAddon(ctx, slurmAddonName(clusterName, workspaceId, name))
	if err != nil {
		return nil, err
	}
	if err = h.accessController.Authorize(authority.AccessInput{
		Context:  ctx,
		Resource: addon,
		Verb:     v1.UpdateVerb,
		UserId:   c.GetString(common.UserId),
	}); err != nil {
		return nil, err
	}

	spec := readSlurmSpec(addon)
	spec.Stopped = stopped
	// Stop is two-phase: this first phase scales the workers/login to zero but
	// keeps slurmrestd running so the operator can drain the NodeSets. The REST
	// API is only scaled down later (phase 2) once the workers have drained, so
	// RestapiDown always starts false here (and stays false on resume).
	spec.RestapiDown = false

	releaseName := slurmReleaseName(name)
	if addon.Spec.AddonSource.HelmRepository != nil && addon.Spec.AddonSource.HelmRepository.ReleaseName != "" {
		releaseName = addon.Spec.AddonSource.HelmRepository.ReleaseName
	}
	values, err := renderSlurmValues(spec, releaseName)
	if err != nil {
		return nil, err
	}
	specJSON, _ := json.Marshal(spec)
	if addon.Spec.AddonSource.HelmRepository != nil {
		addon.Spec.AddonSource.HelmRepository.Values = values
	}
	v1.SetAnnotation(addon, slurmSpecAnnotation, string(specJSON))
	if err = h.Update(ctx, addon); err != nil {
		klog.ErrorS(err, "failed to update slurm cluster addon for stop/resume",
			"cluster", clusterName, "name", name, "stopped", stopped)
		return nil, err
	}

	// Free/restore the accounting database alongside the cluster (best-effort).
	if spec.AccountingEnabled {
		desired := int32(1)
		if stopped {
			desired = 0
		}
		if err = h.scaleMariaDB(ctx, clusterName, workspaceId, releaseName, desired); err != nil {
			klog.ErrorS(err, "failed to scale MariaDB during slurm stop/resume",
				"cluster", clusterName, "release", releaseName, "stopped", stopped)
		}
	}
	klog.Infof("set slurm cluster %s stopped=%v in workspace %s on cluster %s",
		name, stopped, workspaceId, clusterName)

	// Phase 2 of a stop: once the worker NodeSets have drained (slurmrestd was
	// deliberately kept up so the operator could reserve/drain them), scale the
	// REST API to zero too. This runs in the background so the stop request
	// returns promptly; correctness of the usage accounting does not depend on
	// it (only slurmd worker pods are counted), so it is best-effort.
	if stopped {
		go h.scaleRestapiDownAfterDrain(clusterName, workspaceId, name, releaseName)
	}

	dyn, _ := h.slurmDynamicClient(clusterName)
	return h.cvtAddonToSlurmCluster(ctx, dyn, addon), nil
}

// scaleRestapiDownAfterDrain waits for a stopped cluster's worker NodeSets to
// drain to zero, then flips spec.RestapiDown and re-renders the Addon so the
// REST API (slurmrestd) is scaled down as well. It runs detached from the
// request, so it uses its own bounded context and re-reads the Addon before
// updating to avoid clobbering a concurrent resume/edit.
func (h *Handler) scaleRestapiDownAfterDrain(clusterName, workspaceId, name, releaseName string) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	dyn, err := h.slurmDynamicClient(clusterName)
	if err != nil {
		klog.ErrorS(err, "slurm stop phase 2: dynamic client unavailable; leaving restapi running",
			"cluster", clusterName, "name", name)
		return
	}

	// Poll until the worker NodeSets report no ready or desired replicas.
	drained := false
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	for !drained {
		ready, desired := readNodeSetStatus(ctx, dyn, workspaceId, releaseName)
		if ready == 0 && desired == 0 {
			drained = true
			break
		}
		select {
		case <-ctx.Done():
			klog.Warningf("slurm stop phase 2: timed out waiting for %s workers to drain "+
				"(ready=%d desired=%d); leaving restapi running", name, ready, desired)
			return
		case <-ticker.C:
		}
	}

	// Re-read the Addon and bail if the cluster is no longer stopped (resumed or
	// deleted in the meantime), so we don't scale the REST API down under a
	// running cluster.
	addon, err := h.getAdminAddon(ctx, slurmAddonName(clusterName, workspaceId, name))
	if err != nil {
		klog.ErrorS(err, "slurm stop phase 2: failed to re-read addon; leaving restapi running",
			"cluster", clusterName, "name", name)
		return
	}
	spec := readSlurmSpec(addon)
	if !spec.Stopped {
		klog.Infof("slurm stop phase 2: cluster %s is no longer stopped; leaving restapi running", name)
		return
	}
	if spec.RestapiDown {
		return // already done
	}
	spec.RestapiDown = true
	values, err := renderSlurmValues(spec, releaseName)
	if err != nil {
		klog.ErrorS(err, "slurm stop phase 2: failed to render values", "cluster", clusterName, "name", name)
		return
	}
	specJSON, _ := json.Marshal(spec)
	if addon.Spec.AddonSource.HelmRepository != nil {
		addon.Spec.AddonSource.HelmRepository.Values = values
	}
	v1.SetAnnotation(addon, slurmSpecAnnotation, string(specJSON))
	if err = h.Update(ctx, addon); err != nil {
		klog.ErrorS(err, "slurm stop phase 2: failed to scale restapi down", "cluster", clusterName, "name", name)
		return
	}
	klog.Infof("slurm stop phase 2: %s workers drained, scaled restapi down", name)
}

func (h *Handler) stopSlurmCluster(c *gin.Context) (interface{}, error) {
	return h.setSlurmClusterStopped(c, true)
}

func (h *Handler) resumeSlurmCluster(c *gin.Context) (interface{}, error) {
	return h.setSlurmClusterStopped(c, false)
}

// listSlurmPods returns the live pods belonging to a Slurm helm release in the
// workspace namespace, tagging each with a coarse role for the detail view.
func listSlurmPods(ctx context.Context, cs kubernetes.Interface, ns, release string) []view.SlurmPod {
	pods, err := cs.CoreV1().Pods(ns).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil
	}
	prefix := release + "-"
	var out []view.SlurmPod
	for i := range pods.Items {
		p := &pods.Items[i]
		instance := p.Labels["app.kubernetes.io/instance"]
		// Match pods owned by this release (Slinky components and the MariaDB we
		// provision are both named/labeled "<release>-...").
		if !strings.HasPrefix(instance, prefix) && !strings.HasPrefix(p.Name, prefix) {
			continue
		}
		out = append(out, view.SlurmPod{
			Name:   p.Name,
			Role:   slurmPodRole(p, release),
			Node:   p.Spec.NodeName,
			Phase:  string(p.Status.Phase),
			PodIP:  p.Status.PodIP,
			HostIP: p.Status.HostIP,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Role != out[j].Role {
			return out[i].Role < out[j].Role
		}
		return out[i].Name < out[j].Name
	})
	return out
}

// slurmPodRole derives a coarse component role from pod labels (falling back to
// the pod name) for display on the detail page.
func slurmPodRole(p *corev1.Pod, release string) string {
	if c := p.Labels["app.kubernetes.io/component"]; c != "" {
		return c
	}
	name := strings.TrimPrefix(p.Name, release+"-")
	switch {
	case strings.HasPrefix(name, "controller"):
		return "controller"
	case strings.HasPrefix(name, "login"):
		return "login"
	case strings.HasPrefix(name, "restapi"):
		return "restapi"
	case strings.HasPrefix(name, "accounting"):
		return "accounting"
	case strings.HasPrefix(name, "mariadb"):
		return "accounting-db"
	default:
		return "worker"
	}
}

// getSlurmClusterLogin locates the cluster's login pod and builds an SSH command
// that routes through the apiserver's SSH gateway into it. The command is only
// populated when SSH is enabled on the deployment and a login pod is Running.
func (h *Handler) getSlurmClusterLogin(c *gin.Context) (interface{}, error) {
	ctx := c.Request.Context()
	clusterName := c.GetString(common.Name)
	name := c.Param(common.SlurmClusterName)
	if name == "" {
		return nil, commonerrors.NewBadRequest("the slurm cluster name is empty")
	}
	workspaceId := c.Query("workspaceId")
	if workspaceId == "" {
		return nil, commonerrors.NewBadRequest("workspaceId is required")
	}
	workspace, err := h.getAdminWorkspace(ctx, workspaceId)
	if err != nil {
		return nil, err
	}
	if err = h.accessController.Authorize(authority.AccessInput{
		Context:    ctx,
		Resource:   workspace,
		Verb:       v1.GetVerb,
		Workspaces: []string{workspaceId},
		UserId:     c.GetString(common.UserId),
	}); err != nil {
		return nil, err
	}

	resp := &view.SlurmLoginResponse{Enabled: commonconfig.IsSSHEnable()}
	if !resp.Enabled {
		resp.Message = "SSH is not enabled on this deployment."
		return resp, nil
	}
	host := commonconfig.GetSystemHost()
	port := commonconfig.GetSSHServerPort()
	if host == "" || port <= 0 {
		resp.Message = "SSH is enabled but the SSH server host/port is not configured."
		return resp, nil
	}

	cs, err := h.slurmClientSet(clusterName)
	if err != nil {
		return nil, err
	}
	// The login pod's Helm instance label is "<release>-login-slinky" (the single
	// login node created by renderSlurmValues).
	instance := slurmReleaseName(name) + "-login-slinky"
	pods, err := cs.CoreV1().Pods(workspaceId).List(ctx, metav1.ListOptions{
		LabelSelector: "app.kubernetes.io/instance=" + instance,
	})
	if err != nil {
		return nil, commonerrors.NewInternalError(err.Error())
	}
	var loginPod *corev1.Pod
	for i := range pods.Items {
		if pods.Items[i].Status.Phase == corev1.PodRunning {
			loginPod = &pods.Items[i]
			break
		}
	}
	if loginPod == nil {
		resp.Message = "The login node is not running yet. Wait until the cluster reaches Running and try again."
		return resp, nil
	}

	container := "login"
	if len(loginPod.Spec.Containers) > 0 {
		container = loginPod.Spec.Containers[0].Name
	}
	userId := c.GetString(common.UserId)
	if userId == "" {
		userId = "none"
	}
	resp.Ready = true
	resp.PodName = loginPod.Name
	resp.Container = container
	// Username format matches the SSH gateway parser:
	// {userId}.{pod}.{container}.{shell}.{namespace}
	resp.SSHCommand = fmt.Sprintf("ssh -o ServerAliveInterval=60 %s.%s.%s.bash.%s@%s -p %d",
		userId, loginPod.Name, container, workspaceId, host, port)
	return resp, nil
}

// slurmReleaseName returns the helm release name for a Slurm cluster.
func slurmReleaseName(name string) string {
	return "slurm-" + name
}

// slurmClientSet returns a typed clientset for the target/data cluster.
func (h *Handler) slurmClientSet(clusterName string) (kubernetes.Interface, error) {
	if h.clientManager == nil {
		return nil, commonerrors.NewInternalError("client manager is not configured")
	}
	k8sClients, err := commonutils.GetK8sClientFactory(h.clientManager, clusterName)
	if err != nil {
		return nil, err
	}
	cs := k8sClients.ClientSet()
	if cs == nil {
		return nil, commonerrors.NewInternalError("the clientset for the cluster is not available")
	}
	return cs, nil
}

// slurmAddonName returns the cluster-scoped Addon CR name for a Slurm cluster,
// matching genAddonName(cluster, namespace=workspaceId, releaseName).
func slurmAddonName(cluster, workspaceId, name string) string {
	return genAddonName(cluster, workspaceId, slurmReleaseName(name))
}

// slurmClusterNameMaxLen is the Slinky operator's limit on the internal Slurm
// ClusterName, which is derived as "<namespace>_<release>" (release =
// "slurm-<name>"). Exceeding it makes the helm install fail at admission.
const slurmClusterNameMaxLen = 40

// validateSlurmClusterName rejects names that would make the derived Slinky
// ClusterName exceed slurmClusterNameMaxLen, with a message that tells the
// caller exactly how many characters to trim.
func validateSlurmClusterName(name, workspaceId string) error {
	clusterName := workspaceId + "_" + slurmReleaseName(name)
	if over := len(clusterName) - slurmClusterNameMaxLen; over > 0 {
		maxName := slurmClusterNameMaxLen - len(workspaceId) - len("_"+slurmReleaseName(""))
		if maxName < 0 {
			maxName = 0
		}
		return commonerrors.NewBadRequest(fmt.Sprintf(
			"cluster name is too long: the internal Slurm name %q is %d characters but must be at most %d; "+
				"shorten the name by %d character(s) (max %d characters in workspace %q)",
			clusterName, len(clusterName), slurmClusterNameMaxLen, over, maxName, workspaceId))
	}
	return nil
}

// validateNodePools performs basic request validation on node pools.
func validateNodePools(pools []view.NodePool) error {
	seen := map[string]bool{}
	for _, p := range pools {
		if p.Name == "" {
			return commonerrors.NewBadRequest("each node pool requires a name")
		}
		if seen[p.Name] {
			return commonerrors.NewBadRequest(fmt.Sprintf("duplicate node pool name %q", p.Name))
		}
		seen[p.Name] = true
		if p.Nodes < 0 {
			return commonerrors.NewBadRequest(fmt.Sprintf("node pool %q has a negative node count", p.Name))
		}
	}
	return nil
}

// slurmWorkspaceVolumes translates the workspace's configured volumes into the
// Slinky chart's pod `volumes` and container `volumeMounts` entries, mirroring
// the shapes the job-manager dispatcher builds for SaFE Workload pods. HostPath
// volumes become a direct hostPath source (DirectoryOrCreate so a missing path
// on a node does not fail the pod); PFS volumes reference the `pfs-<id>` PVC that
// resource-manager provisions in the workspace namespace (the same namespace the
// Slurm release deploys into). The mounts are shared cluster-wide, so the
// per-user directory behaviour (enableUserDir) is intentionally not applied.
func slurmWorkspaceVolumes(vols []v1.WorkspaceVolume) (volumes []interface{}, mounts []interface{}) {
	for _, vol := range vols {
		name := vol.GenFullVolumeId()
		var volume map[string]interface{}
		if vol.Type == v1.HOSTPATH {
			volume = map[string]interface{}{
				"name": name,
				"hostPath": map[string]interface{}{
					"path": vol.HostPath,
					"type": "DirectoryOrCreate",
				},
			}
		} else {
			volume = map[string]interface{}{
				"name": name,
				"persistentVolumeClaim": map[string]interface{}{
					"claimName": name,
				},
			}
		}
		volumes = append(volumes, volume)

		if vol.MountPath == "" {
			continue
		}
		mount := map[string]interface{}{
			"name":      name,
			"mountPath": vol.MountPath,
			"readOnly":  vol.AccessMode == corev1.ReadOnlyMany,
		}
		if vol.SubPath != "" {
			mount["subPath"] = vol.SubPath
		}
		mounts = append(mounts, mount)
	}
	return volumes, mounts
}

// renderSlurmValues builds the Slinky `slurm` chart values from the spec and
// marshals them to a YAML string suitable for the Addon's helm values. When
// accounting is enabled, slurmdbd is pointed at the release-scoped MariaDB that
// ensureMariaDB provisions.
func renderSlurmValues(spec slurmSpec, releaseName string) (string, error) {
	nodesets := map[string]interface{}{}
	partitions := map[string]interface{}{}
	for i, p := range spec.Pools {
		limits := map[string]interface{}{}
		if p.CPU != "" {
			limits["cpu"] = p.CPU
		}
		if p.Memory != "" {
			limits["memory"] = p.Memory
		}
		if p.GPU > 0 {
			limits[amdGPUResourceName] = p.GPU
		}
		slurmd := map[string]interface{}{}
		if len(limits) > 0 {
			slurmd["resources"] = map[string]interface{}{"limits": limits}
		}
		if spec.ImageTag != "" {
			slurmd["image"] = map[string]interface{}{"tag": spec.ImageTag}
		}
		// A stopped cluster scales its worker NodeSets to zero (freeing compute)
		// while keeping the NodeSet/partition config so Resume restores replicas.
		replicas := p.Nodes
		if spec.Stopped {
			replicas = 0
		}
		nodeset := map[string]interface{}{
			"enabled":  true,
			"replicas": replicas,
		}
		if len(slurmd) > 0 {
			nodeset["slurmd"] = slurmd
		}
		nodesets[p.Name] = nodeset

		partitionConfig := map[string]interface{}{
			"State":   "UP",
			"MaxTime": "UNLIMITED",
		}
		// Mark the first pool's partition as the cluster default so jobs can be
		// submitted without an explicit `-p/--partition` flag.
		if i == 0 {
			partitionConfig["Default"] = "YES"
		}
		partitions[p.Name] = map[string]interface{}{
			"enabled":   true,
			"nodesets":  []interface{}{p.Name},
			"configMap": partitionConfig,
		}
	}

	accounting := map[string]interface{}{"enabled": spec.AccountingEnabled}
	if spec.AccountingEnabled {
		// The Slinky `slurm` chart's accounting (slurmdbd) needs an external
		// MariaDB plus a password secret; the chart provisions neither. We stand
		// up a release-scoped MariaDB (see ensureMariaDB) and point slurmdbd at it.
		// Names are release-scoped so multiple clusters can coexist in one namespace.
		accounting["storageConfig"] = map[string]interface{}{
			"host": mariadbServiceName(releaseName),
			"passwordKeyRef": map[string]interface{}{
				"name": mariadbSecretName(releaseName),
				"key":  mariadbPasswordKey,
			},
		}
	}
	// When stopped, scale the login node to zero (only the slurmctld controller,
	// a chart singleton with no replica toggle, then remains besides the REST
	// API). The REST API (slurmrestd) is kept running through the first stop
	// phase so the slurm-operator can drain the worker NodeSets; it is only
	// scaled to zero once the workers have drained (spec.RestapiDown). See the
	// two-phase logic in setSlurmClusterStopped.
	restapiReplicas := 1
	loginReplicas := 1
	if spec.Stopped {
		loginReplicas = 0
		if spec.RestapiDown {
			restapiReplicas = 0
		}
	}
	// One default login node so users can reach the cluster (sinfo/srun).
	loginset := map[string]interface{}{"enabled": true, "replicas": loginReplicas}
	values := map[string]interface{}{
		"restapi":    map[string]interface{}{"replicas": restapiReplicas},
		"accounting": accounting,
		"loginsets": map[string]interface{}{
			"slinky": loginset,
		},
		"nodesets":   nodesets,
		"partitions": partitions,
	}
	// Mount the workspace's shared filesystem into the login node and every
	// worker (slurmd) pod so the cluster is usable for real workloads. Login
	// volumes go on the single "slinky" loginset; worker volumes go on
	// nodesetDefaults so they apply to all node pools.
	if volList, mountList := slurmWorkspaceVolumes(spec.Volumes); len(volList) > 0 {
		loginset["podSpec"] = map[string]interface{}{"volumes": volList}
		loginset["login"] = map[string]interface{}{"volumeMounts": mountList}
		values["nodesetDefaults"] = map[string]interface{}{
			"podSpec": map[string]interface{}{"volumes": volList},
			"slurmd":  map[string]interface{}{"volumeMounts": mountList},
		}
	}
	if spec.ImageTag != "" {
		values["controller"] = map[string]interface{}{
			"slurmctld": map[string]interface{}{"image": map[string]interface{}{"tag": spec.ImageTag}},
		}
		values["restapi"].(map[string]interface{})["slurmrestd"] = map[string]interface{}{
			"image": map[string]interface{}{"tag": spec.ImageTag},
		}
	}

	out, err := yaml.Marshal(values)
	if err != nil {
		return "", commonerrors.NewInternalError(err.Error())
	}
	return string(out), nil
}

// readSlurmSpec reconstructs the persisted spec from the Addon annotation.
func readSlurmSpec(addon *v1.Addon) slurmSpec {
	spec := slurmSpec{}
	if raw := v1.GetAnnotation(addon, slurmSpecAnnotation); raw != "" {
		_ = json.Unmarshal([]byte(raw), &spec)
	}
	return spec
}

// slurmDynamicClient returns a dynamic client for the target/data cluster.
func (h *Handler) slurmDynamicClient(clusterName string) (dynamic.Interface, error) {
	k8sClients, err := commonutils.GetK8sClientFactory(h.clientManager, clusterName)
	if err != nil {
		return nil, err
	}
	dyn := k8sClients.DynamicClient()
	if dyn == nil {
		return nil, commonerrors.NewInternalError("the dynamic client for the cluster is not available")
	}
	return dyn, nil
}

// cvtAddonToSlurmCluster maps an Addon (Slurm helm release) to a response item,
// enriching it with live NodeSet/Controller status from the target cluster.
func (h *Handler) cvtAddonToSlurmCluster(ctx context.Context, dyn dynamic.Interface, addon *v1.Addon) view.SlurmClusterResponseItem {
	workspaceId := v1.GetLabel(addon, v1.WorkspaceIdLabel)
	spec := readSlurmSpec(addon)

	item := view.SlurmClusterResponseItem{
		Name:              v1.GetDisplayName(addon),
		Workspace:         workspaceId,
		Namespace:         workspaceId,
		Phase:             mapAddonPhaseToSlurmPhase(addon),
		AccountingEnabled: spec.AccountingEnabled,
		Pools:             spec.Pools,
		Stopped:           spec.Stopped,
		ImageTag:          spec.ImageTag,
		Description:       v1.GetAnnotation(addon, v1.DescriptionAnnotation),
		CreationTime:      timeutil.FormatRFC3339(addon.CreationTimestamp.Time),
	}
	if item.Name == "" {
		// Fall back to release name without the "slurm-" prefix.
		if addon.Spec.AddonSource.HelmRepository != nil {
			item.Name = trimSlurmPrefix(addon.Spec.AddonSource.HelmRepository.ReleaseName)
		}
	}
	if addon.Spec.Cluster != nil {
		item.Cluster = addon.Spec.Cluster.Name
	}
	for _, p := range spec.Pools {
		item.Partitions = append(item.Partitions, p.Name)
	}

	// Live status is best-effort: the helm release may still be installing.
	if dyn != nil && workspaceId != "" && addon.Spec.AddonSource.HelmRepository != nil {
		release := addon.Spec.AddonSource.HelmRepository.ReleaseName
		ready, desired := readNodeSetStatus(ctx, dyn, workspaceId, release)
		item.NodesReady = ready
		item.NodesDesired = desired
		if !spec.Stopped && controllerReady(ctx, dyn, workspaceId, release) {
			item.Phase = "Running"
		}
	}
	// A stopped cluster reports "Stopping" while its workers are still draining
	// (residual ready NodeSet replicas) and only "Stopped" once compute is
	// actually freed. This keeps the UI honest instead of claiming Stopped while
	// GPUs are still held.
	if spec.Stopped {
		if item.NodesReady > 0 {
			item.Phase = "Stopping"
		} else {
			item.Phase = "Stopped"
		}
	}
	return item
}

// readNodeSetStatus sums ready/desired slurmd replicas across the release's
// NodeSets. The chart's NodeSets carry no per-release label, but are named
// "<release>-...", so they are matched by name prefix within the workspace ns.
func readNodeSetStatus(ctx context.Context, dyn dynamic.Interface, ns, release string) (ready, desired int) {
	list, err := dyn.Resource(slurmNodeSetGVR).Namespace(ns).List(ctx, metav1.ListOptions{})
	if err != nil {
		return 0, 0
	}
	prefix := release + "-"
	for i := range list.Items {
		obj := &list.Items[i]
		if !strings.HasPrefix(obj.GetName(), prefix) {
			continue
		}
		m := obj.Object
		if r, found, _ := unstructured.NestedInt64(m, "status", "readyReplicas"); found {
			ready += int(r)
		}
		if d, found, _ := unstructured.NestedInt64(m, "status", "desired"); found {
			desired += int(d)
		} else if r, found, _ := unstructured.NestedInt64(m, "spec", "replicas"); found {
			desired += int(r)
		}
	}
	return ready, desired
}

// controllerReady reports whether the release's controller (slurmctld) StatefulSet
// "<release>-controller" has at least one ready replica.
func controllerReady(ctx context.Context, dyn dynamic.Interface, ns, release string) bool {
	sts, err := dyn.Resource(statefulSetGVR).Namespace(ns).Get(ctx, release+"-controller", metav1.GetOptions{})
	if err != nil {
		return false
	}
	ready, found, _ := unstructured.NestedInt64(sts.Object, "status", "readyReplicas")
	return found && ready >= 1
}

// mapAddonPhaseToSlurmPhase derives a user-facing phase from the Addon status.
func mapAddonPhaseToSlurmPhase(addon *v1.Addon) string {
	switch addon.Status.Phase {
	case v1.AddonRunning, v1.AddonDeployed:
		return "Deployed"
	case v1.AddonFailed, v1.AddonError:
		return "Failed"
	case v1.AddonDeleting:
		return "Deleting"
	case "":
		return "Pending"
	default:
		return string(addon.Status.Phase)
	}
}

func trimSlurmPrefix(release string) string {
	const prefix = "slurm-"
	if len(release) > len(prefix) && release[:len(prefix)] == prefix {
		return release[len(prefix):]
	}
	return release
}
