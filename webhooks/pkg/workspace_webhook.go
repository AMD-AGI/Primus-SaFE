/*
 * Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package webhooks

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"

	admissionv1 "k8s.io/api/admission/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/klog/v2"
	ctrlruntime "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	commonerrors "github.com/AMD-AIG-AIMA/SAFE/common/pkg/errors"
	commonnodes "github.com/AMD-AIG-AIMA/SAFE/common/pkg/nodes"
	commonworkload "github.com/AMD-AIG-AIMA/SAFE/common/pkg/workload"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/maps"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/sets"
	sliceutil "github.com/AMD-AIG-AIMA/SAFE/utils/pkg/slice"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/stringutil"
)

func AddWorkspaceWebhook(mgr ctrlruntime.Manager, server *webhook.Server, decoder admission.Decoder) {
	(*server).Register(generateMutatePath(v1.WorkspaceKind), &webhook.Admission{Handler: &WorkspaceMutator{
		Client:  mgr.GetClient(),
		decoder: decoder,
	}})
	(*server).Register(generateValidatePath(v1.WorkspaceKind), &webhook.Admission{Handler: &WorkspaceValidator{
		Client:  mgr.GetClient(),
		decoder: decoder,
	}})
}

type WorkspaceMutator struct {
	client.Client
	decoder admission.Decoder
}

func (m *WorkspaceMutator) Handle(ctx context.Context, req admission.Request) admission.Response {
	if req.Operation == admissionv1.Delete {
		return admission.Allowed("")
	}
	workspace := &v1.Workspace{}
	if err := m.decoder.Decode(req, workspace); err != nil {
		return handleError(v1.WorkspaceKind, err)
	}
	if !workspace.GetDeletionTimestamp().IsZero() {
		return admission.Allowed("")
	}

	switch req.Operation {
	case admissionv1.Create:
		m.mutateOnCreation(ctx, workspace)
	case admissionv1.Update:
		oldWorkspace := &v1.Workspace{}
		if m.decoder.DecodeRaw(req.OldObject, oldWorkspace) == nil {
			if err := m.mutateOnUpdate(ctx, oldWorkspace, workspace); err != nil {
				return handleError(v1.WorkspaceKind, err)
			}
		}
	}
	data, err := json.Marshal(workspace)
	if err != nil {
		return handleError(v1.WorkspaceKind, err)
	}
	return admission.PatchResponseFromRaw(req.Object.Raw, data)
}

func (m *WorkspaceMutator) mutateOnCreation(ctx context.Context, workspace *v1.Workspace) {
	m.mutateMeta(ctx, workspace)
	m.mutateSpec(workspace)
	m.mutateCommon(ctx, workspace)
	m.mutateVolumes(workspace)
}

func (m *WorkspaceMutator) mutateOnUpdate(ctx context.Context, oldWorkspace, newWorkspace *v1.Workspace) error {
	m.mutateCommon(ctx, newWorkspace)
	if v1.GetWorkspaceNodesAction(oldWorkspace) != v1.GetWorkspaceNodesAction(newWorkspace) {
		if err := m.mutateNodesAction(ctx, oldWorkspace, newWorkspace); err != nil {
			return err
		}
	} else if err := m.mutateScaleDown(ctx, oldWorkspace, newWorkspace); err != nil {
		return err
	}
	if oldWorkspace.Spec.EnablePreempt != newWorkspace.Spec.EnablePreempt {
		if err := m.mutatePreempt(ctx, newWorkspace); err != nil {
			return err
		}
	}
	return nil
}

func (m *WorkspaceMutator) mutateMeta(ctx context.Context, workspace *v1.Workspace) {
	workspace.Name = stringutil.NormalizeName(workspace.Name)
	if workspace.Spec.Cluster != "" {
		cl, _ := getCluster(ctx, m.Client, workspace.Spec.Cluster)
		if cl != nil {
			if !hasOwnerReferences(workspace, cl.Name) {
				if err := controllerutil.SetControllerReference(cl, workspace, m.Client.Scheme()); err != nil {
					klog.ErrorS(err, "failed to SetControllerReference")
				}
			}
			v1.SetLabel(workspace, v1.ClusterIdLabel, workspace.Spec.Cluster)
		}
	}
	v1.SetLabel(workspace, v1.WorkspaceIdLabel, workspace.Name)
	controllerutil.AddFinalizer(workspace, v1.WorkspaceFinalizer)
}

func (m *WorkspaceMutator) mutateNodesAction(ctx context.Context, oldWorkspace, newWorkspace *v1.Workspace) error {
	if oldWorkspace.Spec.Replica != newWorkspace.Spec.Replica {
		return fmt.Errorf("the operation of specifying nodes and the modification of " +
			"workspace replica cannot be performed simultaneously")
	}

	actions, err := parseNodesAction(newWorkspace)
	if err != nil {
		return err
	}
	for key, val := range actions {
		n, _ := getNode(ctx, m.Client, key)
		if n == nil {
			klog.ErrorS(err, "failed to get node")
			return commonerrors.NewNotFound(v1.NodeKind, key)
		}
		if v1.GetClusterId(n) != newWorkspace.Spec.Cluster {
			err = fmt.Errorf("The cluster(%s) of the operation and the workspace's cluster do not match", v1.GetClusterId(n))
			return err
		}
		if newWorkspace.Spec.Replica == 0 {
			if val == v1.NodeActionAdd {
				newWorkspace.Spec.NodeFlavor = v1.GetNodeFlavorId(n)
				newWorkspace.Spec.Replica = 1
			}
		} else {
			if v1.GetNodeFlavorId(n) != newWorkspace.Spec.NodeFlavor {
				err = fmt.Errorf("The flavor(%s) of the operation and the workspace's flavor do not match", v1.GetNodeFlavorId(n))
				return err
			}
			if val == v1.NodeActionAdd {
				newWorkspace.Spec.Replica++
			} else if val == v1.NodeActionRemove {
				newWorkspace.Spec.Replica--
			}
		}
	}
	return nil
}

func (m *WorkspaceMutator) mutateSpec(workspace *v1.Workspace) {
	if workspace.Spec.QueuePolicy == "" {
		workspace.Spec.QueuePolicy = v1.QueueFifoPolicy
	}
}

func (m *WorkspaceMutator) mutateVolumes(workspace *v1.Workspace) {
	for i := range workspace.Spec.Volumes {
		if workspace.Spec.Volumes[i].MountPath == "" && workspace.Spec.Volumes[i].HostPath != "" {
			workspace.Spec.Volumes[i].MountPath = workspace.Spec.Volumes[i].HostPath
		}
		workspace.Spec.Volumes[i].MountPath = strings.TrimSuffix(workspace.Spec.Volumes[i].MountPath, "/")
		workspace.Spec.Volumes[i].SubPath = strings.Trim(workspace.Spec.Volumes[i].SubPath, "/")
		if workspace.Spec.Volumes[i].AccessMode == "" {
			workspace.Spec.Volumes[i].AccessMode = corev1.ReadWriteMany
		}
	}
}

func (m *WorkspaceMutator) mutateCommon(ctx context.Context, workspace *v1.Workspace) {
	if workspace.Spec.NodeFlavor == "" {
		workspace.Spec.Replica = 0
	} else if v1.GetGpuResourceName(workspace) == "" {
		nf, _ := getNodeFlavor(ctx, m.Client, workspace.Spec.NodeFlavor)
		if nf != nil && nf.HasGpu() {
			v1.SetAnnotation(workspace, v1.GpuResourceNameAnnotation, nf.Spec.Gpu.ResourceName)
			v1.SetAnnotation(workspace, v1.GpuProductNameAnnotation, nf.Spec.Gpu.Product)
		}
	}
}

// A scale-down operation is performed by deleting specific nodes via nodeAction.
func (m *WorkspaceMutator) mutateScaleDown(ctx context.Context, oldWorkspace, newWorkspace *v1.Workspace) error {
	oldCount := oldWorkspace.Spec.Replica
	newCount := newWorkspace.Spec.Replica
	if oldCount <= newCount {
		return nil
	}
	count := oldCount - newCount
	nodes, err := commonnodes.GetNodesForScalingDown(ctx, m.Client, newWorkspace.Name, count)
	if err != nil {
		return err
	}
	if len(nodes) != count {
		return commonerrors.NewInternalError("Unable to get enough nodes for scaling down")
	}
	nodeNames := make([]string, 0, count)
	for _, n := range nodes {
		nodeNames = append(nodeNames, n.Name)
	}
	action := commonnodes.BuildAction(v1.NodeActionRemove, nodeNames...)
	v1.SetAnnotation(newWorkspace, v1.WorkspaceNodesAction, action)
	return nil
}

func (m *WorkspaceMutator) mutatePreempt(ctx context.Context, workspace *v1.Workspace) error {
	filterFunc := func(w *v1.Workload) bool {
		if w.IsEnd() {
			return true
		}
		return false
	}
	workloads, err := commonworkload.GetWorkloadsOfWorkspace(ctx, m.Client,
		workspace.Spec.Cluster, []string{workspace.Name}, filterFunc)
	if err != nil {
		return err
	}
	for _, w := range workloads {
		patch := client.MergeFrom(w.DeepCopy())
		if workspace.Spec.EnablePreempt {
			if v1.IsWorkloadEnablePreempt(w) {
				continue
			}
			v1.SetAnnotation(w, v1.WorkloadEnablePreemptAnnotation, "true")
		} else {
			if !v1.IsWorkloadEnablePreempt(w) {
				continue
			}
			v1.RemoveAnnotation(w, v1.WorkloadEnablePreemptAnnotation)
		}
		if err = m.Patch(ctx, w, patch); err != nil {
			klog.ErrorS(err, "failed to patch workload")
		}
	}
	return nil
}

type WorkspaceValidator struct {
	client.Client
	decoder admission.Decoder
}

func (v *WorkspaceValidator) Handle(ctx context.Context, req admission.Request) admission.Response {
	workspace := &v1.Workspace{}
	var err error
	switch req.Operation {
	case admissionv1.Create:
		if err = v.decoder.Decode(req, workspace); err != nil {
			break
		}
		err = v.validateOnCreation(ctx, workspace)
	case admissionv1.Update:
		if err = v.decoder.Decode(req, workspace); err != nil {
			break
		}
		if !workspace.GetDeletionTimestamp().IsZero() {
			break
		}
		oldWorkspace := &v1.Workspace{}
		if err = v.decoder.DecodeRaw(req.OldObject, oldWorkspace); err == nil {
			err = v.validateOnUpdate(ctx, workspace, oldWorkspace)
		}
	default:
	}
	if err != nil {
		return handleError(v1.WorkspaceKind, err)
	}
	return admission.Allowed("")
}

func (v *WorkspaceValidator) validateOnCreation(ctx context.Context, workspace *v1.Workspace) error {
	if err := v.validateCommon(ctx, workspace, nil); err != nil {
		return err
	}
	if err := validateDisplayName(v1.GetDisplayName(workspace)); err != nil {
		return err
	}
	if err := v.validateResource(ctx, workspace); err != nil {
		return err
	}
	return nil
}

func (v *WorkspaceValidator) validateOnUpdate(ctx context.Context, newWorkspace, oldWorkspace *v1.Workspace) error {
	if err := v.validateImmutableFields(newWorkspace, oldWorkspace); err != nil {
		return err
	}
	if err := v.validateCommon(ctx, newWorkspace, oldWorkspace); err != nil {
		return err
	}
	if err := v.validateNodesAction(ctx, newWorkspace, oldWorkspace); err != nil {
		return err
	}
	if newWorkspace.Spec.Replica > oldWorkspace.Spec.Replica {
		if err := v.validateResource(ctx, newWorkspace); err != nil {
			return err
		}
	}
	if err := v.validateVolumeRemoved(ctx, newWorkspace, oldWorkspace); err != nil {
		return err
	}
	return nil
}

func (v *WorkspaceValidator) validateCommon(_ context.Context, newWorkspace, oldWorkspace *v1.Workspace) error {
	if err := v.validateRequiredParams(newWorkspace); err != nil {
		return err
	}
	if err := v.validateVolumes(newWorkspace, oldWorkspace); err != nil {
		return err
	}
	return nil
}

func (v *WorkspaceValidator) validateRequiredParams(workspace *v1.Workspace) error {
	var errs []error
	if workspace.Spec.Cluster == "" || v1.GetClusterId(workspace) == "" {
		errs = append(errs, fmt.Errorf("the cluster is empty"))
	}
	if workspace.Spec.QueuePolicy != v1.QueueFifoPolicy && workspace.Spec.QueuePolicy != v1.QueueBalancePolicy {
		errs = append(errs, fmt.Errorf("invalid queue policy. unsupported: %s, supported: [%s, %s]",
			workspace.Spec.QueuePolicy, v1.QueueFifoPolicy, v1.QueueBalancePolicy))
	}
	if workspace.Name == corev1.NamespaceDefault ||
		workspace.Name == common.KubePublicNamespace || workspace.Name == common.KubeSystemNamespace {
		errs = append(errs,
			fmt.Errorf("the name of workspace is invalid. It cannot be reserved words"))
	}
	if v1.GetDisplayName(workspace) == "" {
		errs = append(errs, fmt.Errorf("the displayName is empty"))
	}
	if err := utilerrors.NewAggregate(errs); err != nil {
		return err
	}
	return nil
}

func (v *WorkspaceValidator) validateResource(ctx context.Context, workspace *v1.Workspace) error {
	if workspace.Spec.Replica <= 0 || workspace.Spec.NodeFlavor == "" {
		return nil
	}
	nf, _ := getNodeFlavor(ctx, v.Client, workspace.Spec.NodeFlavor)
	if nf == nil {
		return commonerrors.NewNotFound(v1.NodeFlavorKind, workspace.Spec.NodeFlavor)
	}
	cl, _ := getCluster(ctx, v.Client, workspace.Spec.Cluster)
	if cl == nil {
		return commonerrors.NewNotFound(v1.ClusterKind, workspace.Spec.Cluster)
	}
	return nil
}

func (v *WorkspaceValidator) validateVolumes(newWorkspace, oldWorkspace *v1.Workspace) error {
	newCapacityMap := make(map[string]string)
	var oldCapacityMap map[string]string
	if oldWorkspace != nil {
		oldCapacityMap = make(map[string]string)
		for _, vol := range oldWorkspace.Spec.Volumes {
			oldCapacityMap[string(vol.StorageType)] = vol.Capacity
		}
	}
	supportedStorageType := []v1.StorageUseType{v1.RBD, v1.FS, v1.OBS, v1.HOSTPATH}
	supportedAccessMode := []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce,
		corev1.ReadWriteMany, corev1.ReadOnlyMany, corev1.ReadWriteOncePod}

	for _, vol := range newWorkspace.Spec.Volumes {
		if vol.MountPath == "" {
			return fmt.Errorf("the mountPath of volume is required")
		}
		if !sliceutil.Contains(supportedStorageType, vol.StorageType) {
			return fmt.Errorf("invalid volume storage type. only %v supported", supportedStorageType)
		}
		if vol.StorageType == v1.HOSTPATH {
			if vol.HostPath == "" {
				return fmt.Errorf("the hostPath of volume is required for hostpath storage")
			}
			continue
		}

		if vol.StorageClass == "" && vol.PersistentVolumeName == "" {
			return fmt.Errorf("the storageClass or persistentVolumeName is empty")
		}
		if vol.Capacity == "" {
			return fmt.Errorf("the capacity of volume is empty")
		}
		if resp, err := resource.ParseQuantity(vol.Capacity); err != nil {
			return err
		} else if resp.IsZero() {
			return fmt.Errorf("the capacity of volume is zero")
		}
		storageType := string(vol.StorageType)

		oldCapacity, ok := oldCapacityMap[storageType]
		if ok && oldCapacity != vol.Capacity {
			return fmt.Errorf("The capacity of volume(%s) can not be changed", storageType)
		}
		oldCapacity, ok = newCapacityMap[storageType]
		if ok {
			if oldCapacity != vol.Capacity {
				return fmt.Errorf("The capacity of the same volume(%s) must be the same", storageType)
			}
		} else {
			newCapacityMap[storageType] = vol.Capacity
		}
		if !sliceutil.Contains(supportedAccessMode, vol.AccessMode) {
			return fmt.Errorf("invalid volume access mode. only %v supported", supportedAccessMode)
		}
	}
	return nil
}

func (v *WorkspaceValidator) validateImmutableFields(newWorkspace, oldWorkspace *v1.Workspace) error {
	if newWorkspace.Spec.Cluster != "" && newWorkspace.Spec.Cluster != oldWorkspace.Spec.Cluster {
		return field.Forbidden(field.NewPath("spec").Key("cluster"), "immutable")
	}
	if oldWorkspace.Spec.NodeFlavor != "" && newWorkspace.Spec.NodeFlavor != "" {
		if newWorkspace.Spec.NodeFlavor != oldWorkspace.Spec.NodeFlavor {
			return field.Forbidden(field.NewPath("spec").Key("nodeFlavor"), "immutable")
		}
	}
	return nil
}

func (v *WorkspaceValidator) validateVolumeRemoved(ctx context.Context, newWorkspace, oldWorkspace *v1.Workspace) error {
	if reflect.DeepEqual(oldWorkspace.Spec.Volumes, newWorkspace.Spec.Volumes) {
		return nil
	}
	newPvcSets := sets.NewSet()
	for _, vol := range newWorkspace.Spec.Volumes {
		if vol.StorageType == v1.HOSTPATH {
			continue
		}
		newPvcSets.Insert(string(vol.StorageType))
	}
	filterFunc := func(w *v1.Workload) bool {
		if w.IsEnd() || !v1.IsWorkloadDispatched(w) {
			return true
		}
		return false
	}
	for _, vol := range oldWorkspace.Spec.Volumes {
		if vol.StorageType == v1.HOSTPATH {
			continue
		}
		if newPvcSets.Has(string(vol.StorageType)) {
			continue
		}
		runningWorkloads, _ := commonworkload.GetWorkloadsOfWorkspace(ctx, v.Client,
			v1.GetClusterId(newWorkspace), []string{newWorkspace.Name}, filterFunc)
		if len(runningWorkloads) > 0 {
			return commonerrors.NewForbidden(fmt.Sprintf("the pvc(%s) is used by workload(%s), "+
				"it can not be removed", vol.StorageType, runningWorkloads[0].Name))
		}
	}
	return nil
}

func (v *WorkspaceValidator) validateNodesAction(ctx context.Context, newWorkspace, oldWorkspace *v1.Workspace) error {
	oldActions, _ := parseNodesAction(oldWorkspace)
	newActions, err := parseNodesAction(newWorkspace)
	if err != nil {
		return err
	}
	if len(oldActions) > 0 && len(newActions) > 0 && !maps.EqualIgnoreOrder(oldActions, newActions) {
		return commonerrors.NewResourceProcessing(
			fmt.Sprintf("%s is processing", v1.GetWorkspaceNodesAction(oldWorkspace)))
	}
	var toRemoveNodes []string
	for key, val := range newActions {
		n, _ := getNode(ctx, v.Client, key)
		if n == nil {
			return commonerrors.NewNotFound(v1.NodeKind, key)
		}
		if v1.GetClusterId(n) != newWorkspace.Spec.Cluster {
			return fmt.Errorf("the node %s and workspace %s are not in the same cluster", n.Name, newWorkspace.Name)
		}
		if val == v1.NodeActionAdd {
			if v1.GetWorkspaceId(n) != "" {
				return fmt.Errorf("the node(%s) is bound for %s. it can't be added",
					key, v1.GetWorkspaceId(n))
			}
		} else if val == v1.NodeActionRemove {
			if v1.GetWorkspaceId(n) != newWorkspace.Name {
				return fmt.Errorf("the node(%s) belongs to workspace(%s). it can't be removed",
					key, v1.GetWorkspaceId(n))
			}
			toRemoveNodes = append(toRemoveNodes, key)
		}
	}
	if err = v.validateNodesRemoved(ctx, newWorkspace, toRemoveNodes); err != nil {
		return err
	}
	return nil
}

func parseNodesAction(w *v1.Workspace) (map[string]string, error) {
	actionsStr := v1.GetWorkspaceNodesAction(w)
	if actionsStr == "" {
		return nil, nil
	}
	var actions map[string]string
	if err := json.Unmarshal([]byte(actionsStr), &actions); err != nil {
		klog.ErrorS(err, "invalid nodes action json", "data", v1.GetWorkspaceNodesAction(w))
		return nil, err
	}
	if len(actions) == 0 {
		return nil, nil
	}
	return actions, nil
}

// Check whether there are any tasks running on the node to be removed
func (v *WorkspaceValidator) validateNodesRemoved(ctx context.Context, workspace *v1.Workspace, nodeNames []string) error {
	if len(nodeNames) == 0 {
		return nil
	}
	nodeNamesSet := sets.NewSetByKeys(nodeNames...)
	filterFunc := func(w *v1.Workload) bool {
		if w.IsEnd() || !v1.IsWorkloadDispatched(w) {
			return true
		}
		return false
	}
	runningWorkloads, err := commonworkload.GetWorkloadsOfWorkspace(ctx, v.Client,
		workspace.Spec.Cluster, []string{workspace.Name}, filterFunc)
	if err != nil || len(runningWorkloads) == 0 {
		return err
	}

	for _, workload := range runningWorkloads {
		for _, p := range workload.Status.Pods {
			if !nodeNamesSet.Has(p.AdminNodeName) {
				continue
			}
			if !v1.IsPodRunning(&p) {
				continue
			}
			return commonerrors.NewForbidden(fmt.Sprintf("the node(%s) is currently in use by"+
				" the workload(%s) and cannot be removed", p.AdminNodeName, workload.Name))
		}
	}
	return nil
}

func getWorkspace(ctx context.Context, cli client.Client, workspaceName string) (*v1.Workspace, error) {
	if workspaceName == "" {
		return nil, fmt.Errorf("empty workspace name")
	}
	workspace := &v1.Workspace{}
	if err := cli.Get(ctx, client.ObjectKey{Name: workspaceName}, workspace); err != nil {
		return nil, err
	}
	return workspace, nil
}
