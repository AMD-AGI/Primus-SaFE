/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package resource

import (
	"context"
	"fmt"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"
	ctrlruntime "sigs.k8s.io/controller-runtime"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	commonconfig "github.com/AMD-AIG-AIMA/SAFE/common/pkg/config"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/quantity"
	"github.com/AMD-AIG-AIMA/SAFE/resource-manager/pkg/utils"
)

const (
	// ExternalBudgetMissingReason is the event reason recorded on an external workspace whose
	// namespace has no budget quota.
	ExternalBudgetMissingReason = "ExternalBudgetMissing"

	quotaRequestsPrefix = "requests."
)

// externalBudgetResync is how often an external workspace re-reads its budget quota.
// The quota lives on the data-plane cluster, which this controller does not watch, so a
// change to spec.hard is picked up on the next resync rather than on an event.
func externalBudgetResync() time.Duration {
	return commonconfig.GetExternalWorkspaceResync()
}

// syncExternalWorkspace sets the capacity of an external workspace from the ResourceQuota
// named v1.ExternalBudgetQuotaName in the workspace's namespace on its data-plane cluster.
//
// An external workspace has no nodes of its own to add up: its capacity is supplied on demand,
// so a sum over the nodes it holds is zero whenever nothing is running, and the queue would
// never let the first workload through. The budget is instead the quota's spec.hard, which
// the capacity supplier maintains and the apiserver enforces at Pod creation.
//
// TotalResources and AvailableResources are both set to spec.hard. AvailableResources is the
// capacity the queue may hand out, not what is left of it: the scheduler subtracts every
// workload it has already dispatched (getLeftTotalResources), including ones whose Pods do
// not exist yet. Writing hard minus status.used here would subtract running workloads twice.
//
// No quota means no budget: both are written as empty, which the queue reads as zero, and an
// event on the workspace says why. Any other read failure is returned and the status is left
// as it was -- a quota that could not be read is not a quota that is absent.
func (r *WorkspaceReconciler) syncExternalWorkspace(ctx context.Context, workspace *v1.Workspace) error {
	k8sClients, err := utils.GetK8sClientFactory(r.clientManager, workspace.Spec.Cluster)
	if err != nil {
		return err
	}
	budget, err := readExternalBudget(ctx, k8sClients.ClientSet(), workspace.Name)
	if err != nil {
		return err
	}
	if budget == nil {
		message := fmt.Sprintf("no ResourceQuota %q in namespace %q on cluster %q; "+
			"the workspace budget is treated as zero and nothing will be dispatched",
			v1.ExternalBudgetQuotaName, workspace.Name, workspace.Spec.Cluster)
		klog.Warningf("workspace %s: %s", workspace.Name, message)
		if r.recorder != nil {
			r.recorder.Event(workspace, corev1.EventTypeWarning, ExternalBudgetMissingReason, message)
		}
	}

	isChanged := false
	if !quantity.Equal(budget, workspace.Status.TotalResources) {
		workspace.Status.TotalResources = budget
		isChanged = true
	}
	if !quantity.Equal(budget, workspace.Status.AvailableResources) {
		workspace.Status.AvailableResources = budget.DeepCopy()
		isChanged = true
	}
	if len(workspace.Status.AbnormalResources) > 0 {
		workspace.Status.AbnormalResources = nil
		isChanged = true
	}
	if workspace.Status.AvailableReplica != 0 {
		workspace.Status.AvailableReplica = 0
		isChanged = true
	}
	if workspace.Status.AbnormalReplica != 0 {
		workspace.Status.AbnormalReplica = 0
		isChanged = true
	}
	if isChanged {
		workspace.Status.UpdateTime = &metav1.Time{Time: time.Now().UTC()}
		if err = r.Status().Update(ctx, workspace); err != nil {
			return err
		}
	}
	return nil
}

// readExternalBudget returns the budget quota's spec.hard as workspace resource names, or nil
// when the quota does not exist.
func readExternalBudget(ctx context.Context, clientSet kubernetes.Interface, namespace string) (corev1.ResourceList, error) {
	quota, err := clientSet.CoreV1().ResourceQuotas(namespace).Get(ctx, v1.ExternalBudgetQuotaName, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to read ResourceQuota %s/%s: %w", namespace, v1.ExternalBudgetQuotaName, err)
	}
	return quotaHardToResources(quota.Spec.Hard), nil
}

// quotaHardToResources maps a quota's spec.hard onto the resource names a workspace status
// and a workload request use. "requests.<name>" maps to <name>; the bare cpu, memory and
// ephemeral-storage names are the same limit as their "requests." form, and when both are set
// the smaller one is the one the apiserver enforces. limits.*, object counts and anything
// else do not bound what the queue hands out and are dropped.
func quotaHardToResources(hard corev1.ResourceList) corev1.ResourceList {
	result := corev1.ResourceList{}
	put := func(name corev1.ResourceName, val resource.Quantity) {
		if old, ok := result[name]; ok && old.Cmp(val) <= 0 {
			return
		}
		result[name] = val.DeepCopy()
	}
	for key, val := range hard {
		switch {
		case strings.HasPrefix(string(key), quotaRequestsPrefix):
			put(corev1.ResourceName(strings.TrimPrefix(string(key), quotaRequestsPrefix)), val)
		case key == corev1.ResourceCPU || key == corev1.ResourceMemory || key == corev1.ResourceEphemeralStorage:
			put(key, val)
		}
	}
	return result
}

// finishExternalWorkspace ends a reconcile of an external workspace in place of the scaling
// switch. Its capacity is the budget quota, not Spec.Replica, so there is nothing to scale:
// running the switch would compare Spec.Replica to AvailableReplica+AbnormalReplica and bind
// or release nodes against it. Explicit nodes-action binding is also skipped earlier in
// reconcileWorkspace. The phase still has to advance, because the switch is the only other
// place that sets it, and an external workspace at zero running workloads is not abnormal.
//
// The requeue is what picks up a change to the quota, which no event here reports.
func (r *WorkspaceReconciler) finishExternalWorkspace(ctx context.Context,
	workspace *v1.Workspace, actionResult ctrlruntime.Result) (ctrlruntime.Result, error) {
	if workspace.Status.Phase != v1.WorkspaceRunning {
		if err := r.updatePhase(ctx, workspace, v1.WorkspaceRunning); err != nil {
			return ctrlruntime.Result{}, err
		}
	}
	result := actionResult
	resync := externalBudgetResync()
	if result.RequeueAfter == 0 || result.RequeueAfter > resync {
		result.RequeueAfter = resync
	}
	return result, nil
}
