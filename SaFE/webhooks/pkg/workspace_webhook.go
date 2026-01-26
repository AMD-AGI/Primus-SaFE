/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package webhooks

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	commonutils "github.com/AMD-AIG-AIMA/SAFE/common/pkg/utils"
	admissionv1 "k8s.io/api/admission/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/klog/v2"
	"k8s.io/utils/pointer"
	ctrlruntime "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	commonerrors "github.com/AMD-AIG-AIMA/SAFE/common/pkg/errors"
	commonnodes "github.com/AMD-AIG-AIMA/SAFE/common/pkg/nodes"
	commonuser "github.com/AMD-AIG-AIMA/SAFE/common/pkg/user"
	commonworkload "github.com/AMD-AIG-AIMA/SAFE/common/pkg/workload"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/maps"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/sets"
	sliceutil "github.com/AMD-AIG-AIMA/SAFE/utils/pkg/slice"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/stringutil"
)

// AddWorkspaceWebhook registers the workspace validation and mutation webhooks.
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

// WorkspaceMutator handles mutation logic for Workspace resources.
type WorkspaceMutator struct {
	client.Client
	decoder admission.Decoder
}

// Handle processes workspace admission requests and applies mutations on create and update.
func (m *WorkspaceMutator) Handle(ctx context.Context, req admission.Request) admission.Response {
	if req.Operation == admissionv1.Delete {
		return admission.Allowed("")
	}
	workspace := &v1.Workspace{}
	var err error
	if err = m.decoder.Decode(req, workspace); err != nil {
		return handleError(v1.WorkspaceKind, err)
	}
	if !workspace.GetDeletionTimestamp().IsZero() {
		return admission.Allowed("")
	}

	switch req.Operation {
	case admissionv1.Create:
		err = m.mutateOnCreation(ctx, workspace)
	case admissionv1.Update:
		oldWorkspace := &v1.Workspace{}
		if m.decoder.DecodeRaw(req.OldObject, oldWorkspace) == nil {
			err = m.mutateOnUpdate(ctx, oldWorkspace, workspace)
		}
	}
	if err != nil {
		return handleError(v1.WorkspaceKind, err)
	}
	data, err := json.Marshal(workspace)
	if err != nil {
		return handleError(v1.WorkspaceKind, err)
	}
	return admission.PatchResponseFromRaw(req.Object.Raw, data)
}

// mutateOnCreation applies default values and normalizations during creation.
func (m *WorkspaceMutator) mutateOnCreation(ctx context.Context, workspace *v1.Workspace) error {
	if err := m.mutateMeta(ctx, workspace); err != nil {
		return err
	}
	if err := m.mutateCommon(ctx, nil, workspace); err != nil {
		return err
	}
	return nil
}

// mutateOnUpdate applies mutations during updates.
func (m *WorkspaceMutator) mutateOnUpdate(ctx context.Context, oldWorkspace, newWorkspace *v1.Workspace) error {
	if err := m.mutateCommon(ctx, oldWorkspace, newWorkspace); err != nil {
		return err
	}
	if v1.GetWorkspaceNodesAction(oldWorkspace) != v1.GetWorkspaceNodesAction(newWorkspace) {
		if err := m.mutateNodesAction(ctx, oldWorkspace, newWorkspace); err != nil {
			return err
		}
	} else if err := m.mutateScaleDown(ctx, oldWorkspace, newWorkspace); err != nil {
		return err
	}
	return nil
}

// mutateCommon applies node flavor, image secrets, volumes, queue policy, preemption and manager mutations.
func (m *WorkspaceMutator) mutateCommon(ctx context.Context, oldWorkspace, newWorkspace *v1.Workspace) error {
	if err := m.mutateByNodeFlavor(ctx, newWorkspace); err != nil {
		return err
	}
	m.mutateVolumes(newWorkspace)
	m.mutateQueuePolicy(newWorkspace)
	if oldWorkspace != nil && (oldWorkspace.Spec.EnablePreempt != newWorkspace.Spec.EnablePreempt ||
		!isMaxRuntimeEqual(oldWorkspace.Spec.MaxRuntime, newWorkspace.Spec.MaxRuntime)) {
		if err := m.mutateWorkloadsOfWorkspace(ctx, newWorkspace); err != nil {
			return err
		}
	}
	if err := m.mutateManagers(ctx, oldWorkspace, newWorkspace); err != nil {
		return err
	}
	if err := m.mutateDefaultWorkspaceUsers(ctx, oldWorkspace, newWorkspace); err != nil {
		return err
	}
	return nil
}

func isMaxRuntimeEqual(old, new map[v1.WorkspaceScope]int) bool {
	if len(old) != len(new) {
		return false
	}
	for k, v := range old {
		if new[k] != v {
			return false
		}
	}
	return true
}

// mutateMeta sets workspace name, labels, finalizer and owner references.
func (m *WorkspaceMutator) mutateMeta(ctx context.Context, workspace *v1.Workspace) error {
	workspace.Name = stringutil.NormalizeName(workspace.Name)
	if workspace.Spec.Cluster != "" {
		cl, err := getCluster(ctx, m.Client, workspace.Spec.Cluster)
		if err != nil {
			return err
		}
		if !commonutils.HasOwnerReferences(workspace, cl.Name) {
			if err = controllerutil.SetControllerReference(cl, workspace, m.Client.Scheme()); err != nil {
				klog.ErrorS(err, "failed to SetControllerReference")
			}
		}
		v1.SetLabel(workspace, v1.ClusterIdLabel, workspace.Spec.Cluster)
	}
	v1.SetLabel(workspace, v1.WorkspaceIdLabel, workspace.Name)
	controllerutil.AddFinalizer(workspace, v1.WorkspaceFinalizer)
	return nil
}

// mutateNodesAction adjusts workspace replica count based on node add/remove actions.
func (m *WorkspaceMutator) mutateNodesAction(ctx context.Context, oldWorkspace, newWorkspace *v1.Workspace) error {
	if oldWorkspace.Spec.Replica != newWorkspace.Spec.Replica {
		return fmt.Errorf("the operation of specifying nodes and the modification of " +
			"workspace replica cannot be performed simultaneously")
	}

	actions, err := parseNodesAction(newWorkspace)
	if err != nil {
		return err
	}
	if len(actions) == 0 {
		v1.RemoveAnnotation(newWorkspace, v1.WorkspaceForcedAction)
		return nil
	}
	for key, val := range actions {
		n, _ := getNode(ctx, m.Client, key)
		if n == nil {
			klog.ErrorS(err, "failed to get node")
			return commonerrors.NewNotFound(v1.NodeKind, key)
		}
		if v1.GetClusterId(n) != newWorkspace.Spec.Cluster {
			err = fmt.Errorf("the cluster(%s) of the operation and the workspace's"+
				" cluster do not match", v1.GetClusterId(n))
			return err
		}
		if newWorkspace.Spec.Replica == 0 {
			if val == v1.NodeActionAdd {
				newWorkspace.Spec.NodeFlavor = v1.GetNodeFlavorId(n)
				newWorkspace.Spec.Replica = 1
			}
		} else {
			if v1.GetNodeFlavorId(n) != newWorkspace.Spec.NodeFlavor {
				err = fmt.Errorf("the flavor(%s) of the operation and the workspace's "+
					"flavor do not match", v1.GetNodeFlavorId(n))
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

// mutateQueuePolicy sets default queue policy to FIFO if not specified.
func (m *WorkspaceMutator) mutateQueuePolicy(workspace *v1.Workspace) {
	if workspace.Spec.QueuePolicy == "" {
		workspace.Spec.QueuePolicy = v1.QueueFifoPolicy
	}
}

// mutateVolumes assigns IDs, normalizes paths and sets default access modes for volumes.
func (m *WorkspaceMutator) mutateVolumes(workspace *v1.Workspace) {
	maxId := 0
	for _, vol := range workspace.Spec.Volumes {
		if vol.Id > maxId {
			maxId = vol.Id
		}
	}
	for i := range workspace.Spec.Volumes {
		if workspace.Spec.Volumes[i].Id <= 0 {
			maxId++
			workspace.Spec.Volumes[i].Id = maxId
		}
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

// mutateByNodeFlavor resets replica if node flavor is empty, or sets GPU resource annotation if available.
func (m *WorkspaceMutator) mutateByNodeFlavor(ctx context.Context, workspace *v1.Workspace) error {
	if workspace.Spec.NodeFlavor == "" {
		workspace.Spec.Replica = 0
	} else if v1.GetGpuResourceName(workspace) == "" {
		nf, err := getNodeFlavor(ctx, m.Client, workspace.Spec.NodeFlavor)
		if err != nil {
			return err
		}
		if nf != nil && nf.HasGpu() {
			v1.SetAnnotation(workspace, v1.GpuResourceNameAnnotation, nf.Spec.Gpu.ResourceName)
		}
	}
	return nil
}

// mutateScaleDown selects nodes for removal when workspace replica is decreased.
func (m *WorkspaceMutator) mutateScaleDown(ctx context.Context, oldWorkspace, newWorkspace *v1.Workspace) error {
	oldCount := oldWorkspace.Spec.Replica
	newCount := newWorkspace.Spec.Replica
	if oldCount <= newCount {
		return nil
	}
	if newCount >= oldWorkspace.CurrentReplica() {
		return nil
	}

	count := oldWorkspace.CurrentReplica() - newCount
	nodes, err := commonnodes.GetNodesForScalingDown(ctx, m.Client, newWorkspace.Name, count)
	if err != nil {
		return err
	}
	if len(nodes) != count {
		return commonerrors.NewInternalError("failed to get enough nodes for scaling down")
	}
	nodeNames := make([]string, 0, count)
	for _, n := range nodes {
		nodeNames = append(nodeNames, n.Name)
	}
	action := commonnodes.BuildAction(v1.NodeActionRemove, nodeNames...)
	v1.SetAnnotation(newWorkspace, v1.WorkspaceNodesAction, action)
	return nil
}

// mutateWorkloadsOfWorkspace Modify all workloads on this workspace — currently primarily preempt and timeout settings.
func (m *WorkspaceMutator) mutateWorkloadsOfWorkspace(ctx context.Context, workspace *v1.Workspace) error {
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
		isChanged := false
		if workspace.Spec.EnablePreempt {
			if v1.SetAnnotation(w, v1.WorkloadEnablePreemptAnnotation, v1.TrueStr) {
				isChanged = true
			}
			if v1.RemoveAnnotation(w, v1.WorkloadStickyNodesAnnotation) {
				isChanged = true
			}
		} else {
			if v1.RemoveAnnotation(w, v1.WorkloadEnablePreemptAnnotation) {
				isChanged = true
			}
		}

		if w.Spec.Timeout == nil {
			scope := commonworkload.GetScope(w)
			if maxRuntime := workspace.GetMaxRunTime(scope); maxRuntime > 0 {
				w.Spec.Timeout = pointer.Int(maxRuntime)
				isChanged = true
			}
		}
		if isChanged {
			if err = m.Update(ctx, w); err != nil {
				klog.ErrorS(err, "failed to patch workload")
			}
		}
	}
	return nil
}

// mutateDefaultWorkspaceUsers adds workspace access to all users when marked as default.
func (m *WorkspaceMutator) mutateDefaultWorkspaceUsers(ctx context.Context, oldWorkspace, newWorkspace *v1.Workspace) error {
	if !newWorkspace.Spec.IsDefault {
		return nil
	}
	if oldWorkspace != nil && oldWorkspace.Spec.IsDefault {
		return nil
	}
	userList := &v1.UserList{}
	if err := m.List(ctx, userList); err != nil {
		return err
	}
	for _, user := range userList.Items {
		if commonuser.AddWorkspace(&user, newWorkspace.Name) {
			if err := m.Update(ctx, &user); err != nil {
				return err
			}
		}
	}
	return nil
}

// mutateManagers synchronizes manager changes by updating user attributes when workspace managers are added or removed.
// For added managers: validates user exists, adds workspace to user's lists and user's managed list, and updates user.
// For removed managers: validates user exists, removes workspace from user's managed list, and updates user.
// If user not found during add/remove, removes user ID from workspace managers list
// Note: Granting a user as a workspace manager also grants the user access to the workspace automatically.
func (m *WorkspaceMutator) mutateManagers(ctx context.Context, oldWorkspace, newWorkspace *v1.Workspace) error {
	var currentManagers []string
	if oldWorkspace != nil {
		currentManagers = oldWorkspace.Spec.Managers
	}
	toAddManagers := sliceutil.Difference(newWorkspace.Spec.Managers, currentManagers)
	for _, userId := range toAddManagers {
		user, err := getUser(ctx, m.Client, userId)
		if err != nil {
			if apierrors.IsNotFound(err) {
				newWorkspace.Spec.Managers, _ = sliceutil.RemoveString(newWorkspace.Spec.Managers, userId)
				continue
			}
			return err
		}
		isChanged := false
		if commonuser.AddWorkspace(user, newWorkspace.Name) {
			isChanged = true
		}
		if commonuser.AddManagedWorkspace(user, newWorkspace.Name) {
			isChanged = true
		}
		if isChanged {
			if err = m.Update(ctx, user); err != nil {
				return err
			}
		}
	}
	toDelManagers := sliceutil.Difference(currentManagers, newWorkspace.Spec.Managers)
	for _, userId := range toDelManagers {
		user, err := getUser(ctx, m.Client, userId)
		if err != nil {
			if apierrors.IsNotFound(err) {
				continue
			}
			return err
		}
		if commonuser.RemoveManagedWorkspace(user, newWorkspace.Name) {
			if err = m.Update(ctx, user); err != nil {
				return err
			}
		}
	}
	return nil
}

// WorkspaceValidator validates Workspace resources on create and update operations.
type WorkspaceValidator struct {
	client.Client
	decoder admission.Decoder
}

// Handle validates workspace resources on create, update, and delete operations.
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

// validateOnCreation validates workspace required params, volumes and related resources on creation.
func (v *WorkspaceValidator) validateOnCreation(ctx context.Context, workspace *v1.Workspace) error {
	if err := v.validateCommon(ctx, workspace, nil); err != nil {
		return err
	}
	return nil
}

// validateOnUpdate validates immutable fields, common params, node actions and volume changes on update.
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
	if err := v.validateVolumeRemoved(ctx, newWorkspace, oldWorkspace); err != nil {
		return err
	}
	return nil
}

// validateCommon validates required params, volumes, display name and related resources.
func (v *WorkspaceValidator) validateCommon(ctx context.Context, newWorkspace, oldWorkspace *v1.Workspace) error {
	if err := v.validateRequiredParams(newWorkspace); err != nil {
		return err
	}
	if err := v.validateVolumes(newWorkspace, oldWorkspace); err != nil {
		return err
	}
	if err := validateDNSName(v1.GetDisplayName(newWorkspace), ""); err != nil {
		return err
	}
	if oldWorkspace == nil || newWorkspace.Spec.Replica > oldWorkspace.Spec.Replica {
		if err := v.validateRelatedResource(ctx, newWorkspace); err != nil {
			return err
		}
	}
	return nil
}

// validateRequiredParams ensures cluster, queue policy, workspace name and display name are valid.
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

// validateRelatedResource ensures the node flavor and cluster referenced by the workspace exist.
func (v *WorkspaceValidator) validateRelatedResource(ctx context.Context, workspace *v1.Workspace) error {
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

// validateVolumes validates volume types, capacity, access modes and ensures immutable fields are not changed.
func (v *WorkspaceValidator) validateVolumes(newWorkspace, oldWorkspace *v1.Workspace) error {
	oldVolumeMap := make(map[string]v1.WorkspaceVolume)
	if oldWorkspace != nil {
		for _, vol := range oldWorkspace.Spec.Volumes {
			oldVolumeMap[vol.GenFullVolumeId()] = vol
		}
	}
	supportedTypes := []v1.WorkspaceVolumeType{v1.HOSTPATH, v1.PFS}
	supportedAccessMode := []corev1.PersistentVolumeAccessMode{
		corev1.ReadWriteOnce,
		corev1.ReadWriteMany, corev1.ReadOnlyMany, corev1.ReadWriteOncePod,
	}

	for _, vol := range newWorkspace.Spec.Volumes {
		if vol.MountPath == "" {
			return fmt.Errorf("the mountPath of volume is required")
		}
		if !sliceutil.Contains(supportedTypes, vol.Type) {
			return fmt.Errorf("invalid volume storage type. only %v supported", supportedTypes)
		}
		if vol.Type == v1.HOSTPATH {
			if vol.HostPath == "" {
				return fmt.Errorf("the hostPath of volume is required for hostpath storage")
			}
			continue
		}

		if vol.StorageClass == "" && len(vol.Selector) == 0 {
			return fmt.Errorf("the storageClass or pv selector is empty")
		}
		if vol.Capacity == "" {
			return fmt.Errorf("the capacity of volume is empty")
		}
		if resp, err := resource.ParseQuantity(vol.Capacity); err != nil {
			return err
		} else if resp.IsZero() {
			return fmt.Errorf("the capacity of volume is zero")
		}

		volumeId := vol.GenFullVolumeId()
		oldVolume, ok := oldVolumeMap[volumeId]
		if ok {
			if oldVolume.StorageClass != vol.StorageClass {
				return fmt.Errorf("the storageClass of volume(%s) can not be changed", volumeId)
			}
			if oldVolume.Capacity != vol.Capacity {
				return fmt.Errorf("the capacity of volume(%s) can not be changed", volumeId)
			}
			if !maps.EqualIgnoreOrder(oldVolume.Selector, vol.Selector) {
				return fmt.Errorf("the pv selector of volume(%s) can not be changed", volumeId)
			}
		}
		if !sliceutil.Contains(supportedAccessMode, vol.AccessMode) {
			return fmt.Errorf("invalid volume access mode. only %v supported", supportedAccessMode)
		}
	}
	return nil
}

// validateImmutableFields ensures cluster and node flavor cannot be modified after creation.
func (v *WorkspaceValidator) validateImmutableFields(newWorkspace, oldWorkspace *v1.Workspace) error {
	if newWorkspace.Spec.Cluster != "" && newWorkspace.Spec.Cluster != oldWorkspace.Spec.Cluster {
		return field.Forbidden(field.NewPath("spec").Key("cluster"), "immutable")
	}
	if newWorkspace.Spec.Replica > 0 {
		if oldWorkspace.Spec.NodeFlavor != "" && newWorkspace.Spec.NodeFlavor != "" {
			if newWorkspace.Spec.NodeFlavor != oldWorkspace.Spec.NodeFlavor {
				return field.Forbidden(field.NewPath("spec").Key("nodeFlavor"), "immutable")
			}
		}
	}
	return nil
}

// validateVolumeRemoved ensures PVC volumes in use by workloads are not removed.
// Note: hostPath volumes are ignored in this check.
func (v *WorkspaceValidator) validateVolumeRemoved(ctx context.Context, newWorkspace, oldWorkspace *v1.Workspace) error {
	newVolumeSet := sets.NewSet()
	for _, vol := range newWorkspace.Spec.Volumes {
		if vol.Type == v1.HOSTPATH {
			continue
		}
		newVolumeSet.Insert(vol.GenFullVolumeId())
	}

	volumeId := ""
	for _, vol := range oldWorkspace.Spec.Volumes {
		if vol.Type == v1.HOSTPATH {
			continue
		}
		id := vol.GenFullVolumeId()
		if newVolumeSet.Has(id) {
			continue
		}
		volumeId = id
		break
	}
	if volumeId == "" {
		return nil
	}

	filterFunc := func(w *v1.Workload) bool {
		if w.IsEnd() || !v1.IsWorkloadDispatched(w) {
			return true
		}
		return false
	}
	runningWorkloads, _ := commonworkload.GetWorkloadsOfWorkspace(ctx, v.Client,
		v1.GetClusterId(newWorkspace), []string{newWorkspace.Name}, filterFunc)
	if len(runningWorkloads) > 0 {
		return commonerrors.NewForbidden(fmt.Sprintf("the pvc(%s) is used by workload(%s), "+
			"it can not be removed", volumeId, runningWorkloads[0].Name))
	}
	return nil
}

// validateNodesAction validates node operations ensuring nodes belong to the same cluster.
// It also checks if nodes being bound or unbound have the correct workspace assignment.
func (v *WorkspaceValidator) validateNodesAction(ctx context.Context, newWorkspace, oldWorkspace *v1.Workspace) error {
	oldActions, _ := parseNodesAction(oldWorkspace)
	newActions, err := parseNodesAction(newWorkspace)
	if err != nil {
		return err
	}
	if len(oldActions) > 0 && len(newActions) > 0 && !maps.EqualIgnoreOrder(oldActions, newActions) {
		return commonerrors.NewResourceProcessing(
			fmt.Sprintf("another job(%s) is processing, please wait for it to complete", v1.GetWorkspaceNodesAction(oldWorkspace)))
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

// parseNodesAction parses the workspace nodes action annotation into a map of node names to actions.
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

// validateNodesRemoved ensures no running workloads are using the nodes to be removed.
func (v *WorkspaceValidator) validateNodesRemoved(ctx context.Context, workspace *v1.Workspace, nodeNames []string) error {
	if len(nodeNames) == 0 || v1.HasAnnotation(workspace, v1.WorkspaceForcedAction) {
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

// getWorkspace retrieves a workspace by ID, returning nil for default or empty workspace IDs.
func getWorkspace(ctx context.Context, cli client.Client, workspaceId string) (*v1.Workspace, error) {
	if workspaceId == corev1.NamespaceDefault || workspaceId == "" {
		return nil, nil
	}
	workspace := &v1.Workspace{}
	if err := cli.Get(ctx, client.ObjectKey{Name: workspaceId}, workspace); err != nil {
		return nil, err
	}
	return workspace, nil
}
