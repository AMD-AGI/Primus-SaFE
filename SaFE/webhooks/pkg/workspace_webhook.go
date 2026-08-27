/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package webhooks

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

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
	commonutils "github.com/AMD-AIG-AIMA/SAFE/common/pkg/utils"
	commonworkload "github.com/AMD-AIG-AIMA/SAFE/common/pkg/workload"
	jsonutils "github.com/AMD-AIG-AIMA/SAFE/utils/pkg/json"
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
	// First, ahead of every mutation, and that placement is the point.
	//
	// A withdrawal is recognised by its shape, and the validating webhook has to recognise the
	// same write from the same shape. It is handed whatever the mutator leaves behind, so any
	// field a mutation touches on the way here is a field the two of them could read
	// differently -- mutateByNodeFlavor zeroes Spec.Replica for a workspace whose flavor has
	// gone, and Spec.Replica is one of the fields the shape is made of. Deciding before
	// anything runs makes the object both webhooks judge byte-for-byte the one the controller
	// sent, so the question of what ran in between does not arise.
	//
	// Nothing to do once it is recognised, either. Left to mutateNodesAction the write would
	// have its reason annotation stripped and the surviving entries counted into Spec.Replica
	// a second time; the rest of mutateCommon is idempotent over an object that already went
	// through it, so skipping it costs nothing.
	if v1.GetWorkspaceNodesAction(oldWorkspace) != v1.GetWorkspaceNodesAction(newWorkspace) &&
		isNodesActionWithdrawal(oldWorkspace, newWorkspace) {
		return nil
	}
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

// isNodesActionWithdrawal recognises a write that takes entries back out of a nodes-action
// request that is already in flight, and gives back the Spec.Replica those entries were
// counted into. Anything else is false.
//
// This is the shape WorkspaceReconciler.dropRefusedActions writes when it gives up on an
// entry. Recognising it does two things: the write is let past mutateNodesAction and past the
// in-flight check in validateNodesAction, both of which would otherwise turn away the one
// write that ends the request, and it is let past validateScaleDown, which is not what this
// is -- see validateOnUpdate.
//
// Both webhooks call this and they must agree, which holds only while nothing between them
// alters what the predicate reads. Mutating admission runs first and its result is what
// validating admission is handed, so a field the mutator writes would be read differently by
// the two of them. This is why mutateOnUpdate asks before it mutates anything at all rather
// than at the point in its sequence where the question belongs: no mutation has run when the
// mutator answers, and none has run when the validator answers either, because answering yes
// is what makes the mutator return.
//
// The shape is narrow on purpose: entries may only leave, never arrive and never change
// value, the reason annotation must come with it, and Spec.Replica must land on exactly the
// value the withdrawal implies -- not merely a smaller one. A request being shrunk by its own
// author while the controller is part way through binding it is not this, and still gets
// turned away by the in-flight check; an author who forges the rest of the shape still cannot
// use it to move Spec.Replica anywhere of their choosing.
func isNodesActionWithdrawal(oldWorkspace, newWorkspace *v1.Workspace) bool {
	if v1.GetAnnotation(newWorkspace, v1.WorkspaceNodesActionError) == "" ||
		v1.GetAnnotation(oldWorkspace, v1.WorkspaceNodesActionError) ==
			v1.GetAnnotation(newWorkspace, v1.WorkspaceNodesActionError) {
		return false
	}
	oldActions, err := commonnodes.ParseAction(oldWorkspace)
	if err != nil || len(oldActions) == 0 {
		return false
	}
	newActions, err := commonnodes.ParseAction(newWorkspace)
	if err != nil {
		return false
	}
	for key, val := range newActions {
		if oldVal, ok := oldActions[key]; !ok || oldVal != val {
			return false
		}
	}
	if len(newActions) >= len(oldActions) {
		return false
	}
	return newWorkspace.Spec.Replica == commonnodes.WithdrawnReplica(oldWorkspace.Spec.Replica, oldActions, newActions)
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
	if err := m.mutateGpuProduct(ctx, newWorkspace); err != nil {
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
	if v1.GetWorkspaceNodesAction(newWorkspace) == "" {
		return nil
	}
	// A new request supersedes whatever the last one failed with. This is the other half of
	// the lifecycle dropRefusedActions starts when it records a reason; without it a
	// workspace that was ever turned down carries that reason for good.
	//
	// After the check above, not before it: the controller clears the annotation on its way
	// out of a request as well, and one of those exits comes straight after writing the
	// reason that a part of the request was withdrawn.
	v1.RemoveAnnotation(newWorkspace, v1.WorkspaceNodesActionError)

	currentActions, err := commonnodes.ParseAction(newWorkspace)
	if err != nil {
		return err
	}
	// Two passes over a sorted key list, and both halves of that matter.
	//
	// Sorted, because a map range is in a random order and this loop can return an error. With
	// two entries wrong in different ways, whichever the range reached first decided what the
	// caller was told -- the same request answered with a different message each time it was
	// sent, which is a fault report that cannot be acted on.
	//
	// Two passes, because the arithmetic below used to be inside this loop and read
	// Spec.Replica as it went. From zero the first add both set the flavor and took the count
	// to one, and everything after it took a different branch -- so a request mixing an add
	// with a remove landed on a different replica depending on which the range happened to
	// visit first, and a request whose adds disagreed about flavor rejected whichever one the
	// range visited second. Judging every entry first and moving the count once afterwards
	// gives one answer, and it is the arithmetic one.
	keys := make([]string, 0, len(currentActions))
	for key := range currentActions {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	newActions := make(map[string]string)
	// The flavor of each accepted entry, in key order, for the pass below.
	accepted := make([]string, 0, len(keys))
	for _, key := range keys {
		val := currentActions[key]
		n, _ := getNode(ctx, m.Client, key)
		if n == nil {
			klog.ErrorS(err, "failed to get node")
			return commonerrors.NewNotFound(v1.NodeKind, key)
		}
		// Add only, and ahead of the cluster check, for the reasons validateNodesAction spells
		// out: an unmanaged node has no cluster label yet, so the check below would turn it
		// away reporting a cluster mismatch that does not exist.
		//
		// Turned down here rather than left to the validator for the reason the bound-node
		// check further down gives: everything past this point moves Spec.Replica, and an
		// entry that cannot succeed must not move it.
		if val == v1.NodeActionAdd && !n.IsManaged() {
			return commonerrors.NewResourceProcessing(fmt.Sprintf(
				"the node(%s) is not managed yet(phase %q, cluster %q). it can't be added",
				key, n.Status.ClusterStatus.Phase, v1.GetClusterId(n)))
		}
		if v1.GetClusterId(n) != newWorkspace.Spec.Cluster {
			err = fmt.Errorf("the cluster(%s) of the operation and the workspace's"+
				" cluster do not match", v1.GetClusterId(n))
			return err
		}
		if val == v1.NodeActionAdd {
			if n.GetSpecWorkspace() == newWorkspace.Name {
				continue
			}
			// Turned down here, not left for the validator. Everything past this point moves
			// Spec.Replica, and an entry that cannot succeed must not move it. The validator
			// refusing the request afterwards does discard the mutation -- but only for as
			// long as it is there to do it, and a mutator that depends on a second webhook to
			// keep its own arithmetic honest is one misconfiguration away from persisting a
			// replica count nobody asked for.
			//
			// Same wording as validateNodesAction on purpose: one condition, one message,
			// whichever webhook reaches it first.
			if bound := n.GetSpecWorkspace(); bound != "" {
				return commonerrors.NewConflict(fmt.Sprintf(
					"the node(%s) is bound for %s. it can't be added", key, bound))
			}
		} else if val == v1.NodeActionRemove {
			if n.GetSpecWorkspace() == "" {
				continue
			}
			// The mirror of the case above, and the more damaging of the two if it gets
			// through: releasing a node that belongs to someone else decrements a replica
			// count that was never counting it.
			if n.GetSpecWorkspace() != newWorkspace.Name {
				return commonerrors.NewConflict(fmt.Sprintf(
					"the node(%s) belongs to workspace(%s). it can't be removed",
					key, n.GetSpecWorkspace()))
			}
		} else {
			continue
		}
		accepted = append(accepted, v1.GetNodeFlavorId(n))
		newActions[key] = val
	}

	if err := m.applyNodesActionReplica(newWorkspace, keys, newActions, accepted); err != nil {
		return err
	}

	oldActions, _ := commonnodes.ParseAction(oldWorkspace)
	if len(newActions) == 0 {
		if len(oldActions) == 0 {
			v1.RemoveAnnotation(newWorkspace, v1.WorkspaceNodesAction)
			v1.RemoveAnnotation(newWorkspace, v1.WorkspaceForcedAction)
		} else {
			// No effective node ops in this request; keep the previous workspace node-action state.
			v1.SetAnnotation(newWorkspace, v1.WorkspaceNodesAction, v1.GetWorkspaceNodesAction(oldWorkspace))
		}
	} else {
		if len(oldActions) > 0 && !maps.EqualIgnoreOrder(oldActions, newActions) {
			return commonerrors.NewResourceProcessing(fmt.Sprintf("another job(%s) is processing,"+
				" please wait for it to complete", v1.GetWorkspaceNodesAction(oldWorkspace)))
		}
		if len(newActions) != len(currentActions) {
			v1.SetAnnotation(newWorkspace, v1.WorkspaceNodesAction, string(jsonutils.MarshalSilently(newActions)))
		}
	}
	return nil
}

// applyNodesActionReplica moves Spec.Replica by what the accepted entries add up to, and
// settles the flavor while it is there. keys is in sorted order and flavors is parallel to it,
// skipping the keys that are not in accepted.
//
// A workspace with no flavor yet takes it from the first add in key order -- an empty
// Spec.Replica is the only state in which a nodes-action may name the flavor rather than be
// checked against it, and picking by key order rather than by map order is what makes a
// two-add request that disagrees about flavor reject the same one every time.
func (m *WorkspaceMutator) applyNodesActionReplica(newWorkspace *v1.Workspace,
	keys []string, accepted map[string]string, flavors []string) error {
	i, adds, removes := 0, 0, 0
	for _, key := range keys {
		val, ok := accepted[key]
		if !ok {
			continue
		}
		flavor := flavors[i]
		i++
		if val == v1.NodeActionAdd {
			adds++
			if newWorkspace.Spec.NodeFlavor == "" {
				newWorkspace.Spec.NodeFlavor = flavor
			}
		} else {
			removes++
		}
		// After the bootstrap above, so the first add is checked against the flavor it just
		// set and every later entry against the same one. A remove is checked too: releasing
		// a node of the wrong flavor would take the count down by something it never counted.
		if newWorkspace.Spec.NodeFlavor != "" && flavor != newWorkspace.Spec.NodeFlavor {
			return commonerrors.NewConflict(fmt.Sprintf(
				"the flavor(%s) of the operation and the workspace's flavor do not match", flavor))
		}
	}
	replica := newWorkspace.Spec.Replica + adds - removes
	// A request may not take a workspace past empty. Reachable only if Spec.Replica is already
	// behind what the workspace holds -- the entries themselves were each checked against the
	// node's claim, so a remove here is a node this workspace really has.
	if replica < 0 {
		replica = 0
	}
	newWorkspace.Spec.Replica = replica
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
		// Short, so something the workspace is counted as holding is not a node it can
		// release: the nodes are busy, or the status count this arithmetic came from is
		// ahead of what the workspace still holds. Building the request anyway would put a
		// node that is not short into the request in its place, and release a machine that
		// was never the one to give back.
		//
		// This catches the shortfall, not every disagreement. count comes from
		// Status.AvailableReplica + AbnormalReplica, which syncWorkspace recomputes from the
		// claim on every reconcile, and the candidates come from a live read of the same
		// field -- so the two differ only for as long as a reconcile the controller has
		// already been woken for takes to run, and only a difference that leaves the
		// candidate list exactly count long slips past this. Closing that last sliver means
		// counting the held nodes here instead, which means a second copy of syncWorkspace's
		// flavor-filtered arithmetic living in a webhook, kept in step by hand. The sliver
		// is cheaper than the copy.
		return commonerrors.NewInternalError(fmt.Sprintf("only %d of the %d nodes to scale "+
			"down are free to release. please retry", len(nodes), count))
	}
	nodeNames := make([]string, 0, count)
	for _, n := range nodes {
		nodeNames = append(nodeNames, n.Name)
	}
	action := commonnodes.BuildAction(v1.NodeActionRemove, nodeNames...)
	v1.SetAnnotation(newWorkspace, v1.WorkspaceNodesAction, action)
	// A new request supersedes whatever the last one failed with, and this is a new request
	// -- the same lifecycle mutateNodesAction applies to an explicit one. Without it, a
	// reason recorded by dropRefusedActions outlives the request it describes: nothing else
	// clears the annotation, so an add that was turned down once stays on display through
	// every unrelated scale-down that follows, reported by every operator and UI reading it
	// as a binding failure that is happening now.
	v1.RemoveAnnotation(newWorkspace, v1.WorkspaceNodesActionError)
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
			if v1.RemoveAnnotation(w, v1.RetryOnOriginalNodesAnnotation) {
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

// mutateDefaultWorkspaceUsers adds workspace access to all users when marked as default.
func (m *WorkspaceMutator) mutateGpuProduct(ctx context.Context, workspace *v1.Workspace) error {
	if workspace.Spec.NodeFlavor == "" || v1.HasAnnotation(workspace, v1.GpuProductAnnotation) {
		return nil
	}
	nf := &v1.NodeFlavor{}
	if err := m.Get(ctx, client.ObjectKey{Name: workspace.Spec.NodeFlavor}, nf); err != nil {
		return err
	}
	if nf.HasGpu() {
		v1.SetAnnotation(workspace, v1.GpuProductAnnotation, string(nf.Spec.Gpu.Product))
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
	// Decided once and handed to both consumers. Two evaluations of the same predicate over
	// the same pair of objects cannot disagree today, but the property that keeps them
	// agreeing is that nothing in between writes what it reads -- and that is an invariant
	// about the whole validate path, which is harder to keep than a single local.
	isWithdrawal := isNodesActionWithdrawal(oldWorkspace, newWorkspace)
	if err := v.validateNodesAction(ctx, newWorkspace, oldWorkspace, isWithdrawal); err != nil {
		return err
	}
	if err := v.validateVolumeRemoved(ctx, newWorkspace, oldWorkspace); err != nil {
		return err
	}
	// A withdrawal lowers Spec.Replica and is not a scale-down. validateScaleDown exists to
	// stop a workspace built from a running workload from shedding capacity that workload is
	// using; the count being given back here was added moments ago for a bind that never
	// happened, so there is no node under it for anything to be running on. Left in the path
	// it would reject every withdrawal on a workload-sourced workspace and strand the request.
	if !isWithdrawal {
		if err := v.validateScaleDown(ctx, newWorkspace, oldWorkspace); err != nil {
			return err
		}
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

// validateScaleDown ensuring that workspaces created from
// running workloads cannot be scale-down until the source workload completes.
func (v *WorkspaceValidator) validateScaleDown(ctx context.Context, newWorkspace, oldWorkspace *v1.Workspace) error {
	if oldWorkspace.Spec.Replica <= newWorkspace.Spec.Replica {
		return nil
	}
	if sourceWorkloadId := v1.GetSourceWorkloadId(newWorkspace); sourceWorkloadId != "" {
		workload, err := getWorkload(ctx, v.Client, sourceWorkloadId)
		if err == nil && !workload.IsEnd() {
			return commonerrors.NewConflict(
				fmt.Sprintf("Scaling down is not allowed before the workload(%s) finishes.", sourceWorkloadId))
		}
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
		return commonerrors.NewConflict(fmt.Sprintf("the pvc(%s) is used by workload(%s), "+
			"it can not be removed", volumeId, runningWorkloads[0].Name))
	}
	return nil
}

// validateNodesAction validates node operations ensuring nodes belong to the same cluster.
// It also checks if nodes being bound or unbound have the correct workspace assignment.
func (v *WorkspaceValidator) validateNodesAction(ctx context.Context, newWorkspace,
	oldWorkspace *v1.Workspace, isWithdrawal bool) error {
	oldActions, _ := commonnodes.ParseAction(oldWorkspace)
	newActions, err := commonnodes.ParseAction(newWorkspace)
	if err != nil {
		return err
	}
	// The controller taking entries back out of a request it cannot carry out. Judging it
	// against the in-flight check below would reject the one write that ends the request.
	// Decided by the caller so the same answer reaches validateScaleDown -- see validateOnUpdate.
	if isWithdrawal {
		return nil
	}
	if len(oldActions) > 0 && len(newActions) > 0 && !maps.EqualIgnoreOrder(oldActions, newActions) {
		return commonerrors.NewResourceProcessing(
			fmt.Sprintf("another job(%s) is processing, please wait for it to complete", v1.GetWorkspaceNodesAction(oldWorkspace)))
	}
	// Nothing new was asked for, so there is nothing to judge. Every update to a Workspace
	// carrying an in-flight request comes through here -- a replica edit, a volume, a label --
	// and re-running the checks against node state that has moved on since the request was
	// accepted turns any of them into a rejection of an unrelated write. The controller is
	// what decides what becomes of a request once it has been accepted.
	if maps.EqualIgnoreOrder(oldActions, newActions) {
		return nil
	}
	var toRemoveNodes []string
	for key, val := range newActions {
		n, _ := getNode(ctx, v.Client, key)
		if n == nil {
			return commonerrors.NewNotFound(v1.NodeKind, key)
		}
		// Onboarding has to finish before a node can be handed to a workspace, and admission
		// is the only place that can say so where the caller sees it.
		//
		// Ahead of the cluster check on purpose: the write in NodeReconciler.manage that
		// stamps ClusterIdLabel is the same one that sets Managed, so an unmanaged node has
		// no cluster label and would fail that check instead -- reporting a cluster mismatch
		// that does not exist, about a node that is simply not done onboarding.
		//
		// Add only. A remove has to stay possible for a node that ended up bound and then
		// lost its managed state, which is exactly when it needs releasing.
		if val == v1.NodeActionAdd && !n.IsManaged() {
			// Both halves of IsManaged in the message: a node can sit in phase Managed with
			// no cluster label, and reporting only the phase reads as a contradiction.
			return commonerrors.NewResourceProcessing(fmt.Sprintf(
				"the node(%s) is not managed yet(phase %q, cluster %q). it can't be added",
				key, n.Status.ClusterStatus.Phase, v1.GetClusterId(n)))
		}
		if v1.GetClusterId(n) != newWorkspace.Spec.Cluster {
			return fmt.Errorf("the node %s and workspace %s are not in the same cluster", n.Name, newWorkspace.Name)
		}
		if val == v1.NodeActionAdd {
			if bound := n.GetSpecWorkspace(); bound != "" {
				return commonerrors.NewConflict(fmt.Sprintf(
					"the node(%s) is bound for %s. it can't be added", key, bound))
			}
		} else if val == v1.NodeActionRemove {
			if n.GetSpecWorkspace() != newWorkspace.Name {
				return commonerrors.NewConflict(fmt.Sprintf(
					"the node(%s) belongs to workspace(%s). it can't be removed",
					key, n.GetSpecWorkspace()))
			}
			toRemoveNodes = append(toRemoveNodes, key)
		}
	}
	if err = v.validateNodesRemoved(ctx, newWorkspace, toRemoveNodes); err != nil {
		return err
	}
	return nil
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
	if err != nil {
		return err
	}

	for _, workload := range runningWorkloads {
		// Dual-read: prefer the etcd NodePodUsage aggregate; fall back to Status.Pods.
		if len(workload.Status.NodeUsage) > 0 {
			for _, u := range workload.Status.NodeUsage {
				if !nodeNamesSet.Has(u.Node) {
					continue
				}
				return commonerrors.NewForbidden(fmt.Sprintf("the node(%s) is currently in use by"+
					" the workload(%s) and cannot be removed. alternatively, you can force the unbinding.", u.Node, workload.Name))
			}
			continue
		}
		for _, p := range workload.Status.Pods {
			if !nodeNamesSet.Has(p.AdminNodeName) {
				continue
			}
			return commonerrors.NewForbidden(fmt.Sprintf("the node(%s) is currently in use by"+
				" the workload(%s) and cannot be removed. alternatively, you can force the unbinding.", p.AdminNodeName, workload.Name))
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
