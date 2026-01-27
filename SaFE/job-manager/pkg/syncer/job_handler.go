/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package syncer

import (
	"context"
	"fmt"
	"reflect"
	"strconv"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
	ctrlruntime "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	commonerrors "github.com/AMD-AIG-AIMA/SAFE/common/pkg/errors"
	commonworkload "github.com/AMD-AIG-AIMA/SAFE/common/pkg/workload"
	jobutils "github.com/AMD-AIG-AIMA/SAFE/job-manager/pkg/utils"
	jsonutils "github.com/AMD-AIG-AIMA/SAFE/utils/pkg/json"
)

const (
	MaxFailedPodsToShow = 3
	MaxConditionHistory = 30
)

// handleJob processes job resource events and synchronizes status between data plane and admin plane.
// Manages the lifecycle of workload resources and handles failure scenarios.
func (r *SyncerReconciler) handleJob(ctx context.Context,
	message *resourceMessage, clientSets *ClusterClientSets) (ctrlruntime.Result, error) {
	adminWorkload, err := r.getAdminWorkload(ctx, message.workloadId)
	if err != nil || adminWorkload == nil {
		return ctrlruntime.Result{}, err
	}
	if message.namespace != adminWorkload.Spec.Workspace {
		return ctrlruntime.Result{}, nil
	}
	if commonworkload.IsCICDScalingRunnerSet(adminWorkload) && message.gvk.Kind != common.CICDScaleRunnerSetKind {
		return ctrlruntime.Result{}, nil
	}
	if !v1.IsWorkloadDispatched(adminWorkload) {
		return ctrlruntime.Result{RequeueAfter: time.Second}, nil
	}

	result, err := r.handleJobImpl(ctx, message, adminWorkload, clientSets)
	if jobutils.IsUnrecoverableError(err) {
		// Errors defined internally are fatal and lead to a terminal state without retry
		err = jobutils.SetWorkloadFailed(ctx, r.Client, adminWorkload, err.Error())
	}
	return result, err
}

// handleJobImpl implements the core logic for handling job resource events.
// Processes resource creation, update, and deletion events.
func (r *SyncerReconciler) handleJobImpl(ctx context.Context, message *resourceMessage,
	adminWorkload *v1.Workload, clientSets *ClusterClientSets) (ctrlruntime.Result, error) {
	if message.action == ResourceDel {
		// wait until all pods are deleted
		if !r.waitAllPodsDeleted(ctx, message, clientSets) {
			return ctrlruntime.Result{RequeueAfter: time.Second * 3}, nil
		}
	}

	if !adminWorkload.IsEnd() {
		status, err := r.getK8sObjectStatus(ctx, message, clientSets, adminWorkload)
		if err != nil {
			return ctrlruntime.Result{}, err
		}
		adminWorkload, err = r.updateAdminWorkloadStatus(ctx, adminWorkload, status, message)
		if err != nil {
			klog.ErrorS(err, "failed to update admin workload status")
			return ctrlruntime.Result{}, err
		}
	}

	// Check if the resource is being deleted OR if the workload was preempted and dispatched
	// The additional IsWorkloadPreempted check prevents issues when the service restarts during
	// the preemption process, which could cause message loss and prevent proper cleanup/re-scheduling
	if message.action == ResourceDel ||
		(v1.IsWorkloadPreempted(adminWorkload) && !v1.IsWorkloadDisableFailover(adminWorkload)) {
		// wait until the job is also deleted
		if !r.waitJobDeleted(ctx, message, clientSets) {
			// If this is not a deletion message, return without retrying
			if message.action != ResourceDel {
				return ctrlruntime.Result{}, nil
			}
			return ctrlruntime.Result{RequeueAfter: time.Second * 3}, nil
		}
		if !adminWorkload.IsEnd() {
			// TorchFT workloads consist of multiple objects (groups), while other workloads have a single object.
			// For TorchFT: wait until ALL objects in the group are deleted before triggering reSchedule
			// For other workloads: single job deletion is sufficient to trigger reSchedule
			if commonworkload.IsTorchFT(adminWorkload) {
				unstructuredObjs, err := jobutils.ListObjectsByWorkload(ctx, r.Client, clientSets.ClientFactory(), adminWorkload)
				if err != nil {
					klog.ErrorS(err, "failed to list objects by workload", "workload", adminWorkload.Name)
					return ctrlruntime.Result{}, err
				}
				if len(unstructuredObjs) != 0 {
					return ctrlruntime.Result{}, nil
				}
			}
			if err := r.reSchedule(ctx, adminWorkload, message.dispatchCount); err != nil {
				klog.ErrorS(err, "failed to reSchedule", "workload", adminWorkload.Name)
				return ctrlruntime.Result{}, err
			}
		}
	}
	return ctrlruntime.Result{}, nil
}

// getK8sObjectStatus retrieves the status of a Kubernetes object in data plane.
// Extracts phase and message information from the object.
func (r *SyncerReconciler) getK8sObjectStatus(ctx context.Context, message *resourceMessage,
	clientSets *ClusterClientSets, adminWorkload *v1.Workload) (*jobutils.K8sObjectStatus, error) {
	if message.action == ResourceDel {
		return &jobutils.K8sObjectStatus{
			Phase:   string(v1.K8sDeleted),
			Message: fmt.Sprintf("%s %s is deleted", message.gvk.Kind, message.name),
		}, nil
	}
	k8sObject, err := jobutils.GetObject(ctx, clientSets.ClientFactory(), message.name, message.namespace, message.gvk)
	if err != nil {
		klog.ErrorS(err, "failed to get k8s object", "name", message.name, "namespace", message.namespace)
		return nil, err
	}

	var rt *v1.ResourceTemplate
	if commonworkload.IsTorchFT(adminWorkload) {
		// For TorchFT workloads, the Kubernetes object GVK matches the workload GVK directly
		// So we can retrieve the resource template using the message GVK (which is the actual object GVK)
		rt, err = commonworkload.GetResourceTemplateByGVK(ctx, r.Client, message.gvk)
	} else {
		// For other workloads, the resource template is associated with the workload GVK
		rt, err = commonworkload.GetResourceTemplate(ctx, r.Client, adminWorkload)
	}

	if err != nil {
		klog.ErrorS(err, "failed to get resource template", "workload", adminWorkload.Name, "kind", message.gvk.Kind)
		return nil, err
	}
	status, err := jobutils.GetK8sObjectStatus(k8sObject, rt)
	if err != nil {
		klog.ErrorS(err, "failed to get phase", "name", message.name, "namespace", message.namespace)
		return nil, commonerrors.NewInternalError(err.Error())
	}
	if status == nil {
		return nil, nil
	}

	// Obtain detailed failure information from the Pod upon failure
	if status.Phase == string(v1.K8sFailed) {
		if failedPodInfo := getFailedPodInfo(adminWorkload); failedPodInfo != "" {
			status.Message += ", details: " + failedPodInfo
		}
	}
	return status, nil
}

// waitAllPodsDeleted checks if all pods associated with a workload have been deleted.
func (r *SyncerReconciler) waitAllPodsDeleted(ctx context.Context, message *resourceMessage, clientSets *ClusterClientSets) bool {
	labelSelector := metav1.LabelSelector{
		MatchLabels: map[string]string{v1.K8sObjectIdLabel: message.name},
	}
	// TODO: Keep old logic for compatibility; remove it later.
	if len(message.selectorLabels) > 0 {
		if _, ok := message.selectorLabels[v1.WorkloadIdLabel]; ok {
			labelSelector.MatchLabels = map[string]string{v1.WorkloadIdLabel: message.workloadId}
		}
	}
	listOptions := metav1.ListOptions{
		LabelSelector: metav1.FormatLabelSelector(&labelSelector),
	}
	klog.Infof("wait for all pods to be deleted, workload: %s, match labels: %v", message.workloadId, labelSelector.MatchLabels)
	podList, err := clientSets.dataClientFactory.ClientSet().CoreV1().Pods(message.namespace).List(ctx, listOptions)
	if err != nil {
		klog.ErrorS(err, "failed to list pods", "workload", message.workloadId, "namespace", message.namespace)
		return false
	}
	if len(podList.Items) == 0 {
		klog.Infof("all pods are deleted, workload: %s", message.workloadId)
		return true
	}
	return false
}

func (r *SyncerReconciler) waitJobDeleted(ctx context.Context, message *resourceMessage, clientSets *ClusterClientSets) bool {
	obj, err := jobutils.GetObject(ctx, clientSets.ClientFactory(), message.name, message.namespace, message.gvk)
	if err != nil {
		return apierrors.IsNotFound(err)
	}
	if ts := obj.GetDeletionTimestamp(); ts != nil && !ts.IsZero() && time.Since(ts.Time) >= 1*time.Minute {
		patchObj := map[string]any{
			"metadata": map[string]any{
				"finalizers": []string{},
			},
		}
		p := jsonutils.MarshalSilently(patchObj)
		if patchErr := jobutils.PatchObject(ctx, clientSets.ClientFactory(), obj, p); patchErr != nil {
			return apierrors.IsNotFound(patchErr)
		}
	}
	return false
}

// updateAdminWorkloadStatus updates the admin workload status based on resource status.
// Manages workload phase transitions and condition updates.
func (r *SyncerReconciler) updateAdminWorkloadStatus(ctx context.Context, originalWorkload *v1.Workload,
	status *jobutils.K8sObjectStatus, message *resourceMessage) (*v1.Workload, error) {
	if status == nil {
		return originalWorkload, nil
	}
	adminWorkload := originalWorkload.DeepCopy()
	if commonworkload.IsCICDScalingRunnerSet(adminWorkload) && status.RunnerScaleSetId != "" {
		if adminWorkload.Status.RunnerScaleSetId != status.RunnerScaleSetId {
			patch := client.MergeFrom(originalWorkload)
			adminWorkload.Status.RunnerScaleSetId = status.RunnerScaleSetId
			if err := r.Status().Patch(ctx, adminWorkload, patch); err != nil {
				return nil, err
			}
		}
		return adminWorkload, nil
	}

	r.updateAdminWorkloadPhase(adminWorkload, status, message)
	if adminWorkload.Status.StartTime == nil {
		adminWorkload.Status.StartTime = &metav1.Time{Time: time.Now().UTC()}
	}
	if adminWorkload.IsEnd() && adminWorkload.Status.EndTime == nil {
		adminWorkload.Status.EndTime = &metav1.Time{Time: time.Now().UTC()}
	}
	if !status.IsPending() {
		adminWorkload.Status.Message = ""
	}
	if !commonworkload.IsTorchFT(adminWorkload) ||
		adminWorkload.Status.Phase != originalWorkload.Status.Phase || isTorchFTGroupFailed(adminWorkload) {
		cond := jobutils.NewCondition(status.Phase, status.Message,
			commonworkload.GenerateDispatchReason(message.dispatchCount))
		updateWorkloadCondition(adminWorkload, cond)
	}
	if reflect.DeepEqual(adminWorkload.Status, originalWorkload.Status) {
		return originalWorkload, nil
	}
	if err := r.Status().Update(ctx, adminWorkload); err != nil {
		return nil, err
	}
	klog.Infof("update workload status, name: %s, phase: %s, dispatchCount: %d, k8s.status: %s",
		adminWorkload.Name, adminWorkload.Status.Phase, message.dispatchCount, jsonutils.MarshalSilently(status))
	return adminWorkload, nil
}

// updateAdminWorkloadPhase updates the workload phase based on k8s object status.
// It was previously determined that the workload has not reached a terminal state.
func (r *SyncerReconciler) updateAdminWorkloadPhase(adminWorkload *v1.Workload,
	status *jobutils.K8sObjectStatus, message *resourceMessage) {
	phase := v1.WorkloadConditionType(status.Phase)
	switch phase {
	case v1.K8sPending:
		adminWorkload.Status.Phase = v1.WorkloadPending
	case v1.K8sSucceeded:
		if !commonworkload.IsTorchFT(adminWorkload) ||
			handleTorchFTGroupStatus(adminWorkload, message.groupId, v1.WorkloadSucceeded) == v1.WorkloadSucceeded {
			adminWorkload.Status.Phase = v1.WorkloadSucceeded
		}
	case v1.K8sFailed:
		if commonworkload.IsTorchFT(adminWorkload) {
			if handleTorchFTGroupStatus(adminWorkload, message.groupId, v1.WorkloadFailed) != v1.WorkloadFailed {
				break
			}
		}
		if shouldTerminateWorkload(adminWorkload, status, message.dispatchCount) {
			adminWorkload.Status.Phase = v1.WorkloadFailed
		} else if commonworkload.IsApplication(adminWorkload) {
			adminWorkload.Status.Phase = v1.WorkloadNotReady
		}
	case v1.K8sDeleted:
		if commonworkload.IsTorchFT(adminWorkload) {
			if handleTorchFTGroupStatus(adminWorkload, message.groupId, v1.WorkloadStopped) != v1.WorkloadStopped {
				break
			}
		}
		if shouldTerminateWorkload(adminWorkload, status, message.dispatchCount) {
			if commonworkload.IsCICDEphemeralRunner(adminWorkload) {
				// Currently, when an EphemeralRunner successfully completes,
				// it does not set a success status but is instead deleted directly.
				// refer: actions-runner-controller/ephemeralrunner_controller.go: 374-381
				adminWorkload.Status.Phase = v1.WorkloadSucceeded
			} else {
				adminWorkload.Status.Phase = v1.WorkloadStopped
			}
		} else if commonworkload.IsApplication(adminWorkload) {
			adminWorkload.Status.Phase = v1.WorkloadNotReady
		}
	case v1.K8sRunning:
		if !commonworkload.IsTorchFT(adminWorkload) ||
			handleTorchFTGroupStatus(adminWorkload, message.groupId, v1.WorkloadRunning) == v1.WorkloadRunning {
			adminWorkload.Status.Phase = v1.WorkloadRunning
		}
	case v1.K8sUpdating:
		// only for deployment/statefulSet
		adminWorkload.Status.Phase = v1.WorkloadUpdating
	}
}

// reSchedule handles workload rescheduling by resetting status and annotations.
// Clears workload state and marks for rescheduling.
func (r *SyncerReconciler) reSchedule(ctx context.Context, workload *v1.Workload, count int) error {
	isStatusChanged := false
	if len(workload.Status.Pods) > 0 {
		workload.Status.Pods = nil
		isStatusChanged = true
	}
	if len(workload.Status.TorchFTPhase) > 0 {
		workload.Status.TorchFTPhase = nil
		isStatusChanged = true
	}
	if workload.Status.Phase != v1.WorkloadPending {
		workload.Status.Phase = v1.WorkloadPending
		reason := commonworkload.GenerateDispatchReason(v1.GetWorkloadDispatchCnt(workload) + 1)
		cond := jobutils.NewCondition(string(v1.AdminScheduling), "the workload is re-scheduling", reason)
		updateWorkloadCondition(workload, cond)
		isStatusChanged = true
	}
	if len(workload.Status.Nodes) < count {
		workload.Status.Nodes = append(workload.Status.Nodes, []string{})
		isStatusChanged = true
	}
	if len(workload.Status.Ranks) < count {
		workload.Status.Ranks = append(workload.Status.Ranks, []string{})
		isStatusChanged = true
	}
	if isStatusChanged {
		if err := r.Status().Update(ctx, workload); err != nil {
			return err
		}
	}

	if v1.IsWorkloadDispatched(workload) {
		patch := client.MergeFrom(workload.DeepCopy())
		v1.RemoveAnnotation(workload, v1.WorkloadScheduledAnnotation)
		v1.RemoveAnnotation(workload, v1.WorkloadDispatchedAnnotation)
		v1.SetAnnotation(workload, v1.WorkloadReScheduledAnnotation, "")
		if err := r.Patch(ctx, workload, patch); err != nil {
			return err
		}
	}
	klog.Infof("reSchedule workload, name: %s, dispatchCount: %d", workload.Name, count)
	return nil
}

// updateWorkloadCondition updates workload conditions based on resource status.
// Manages condition history and ensures proper condition tracking.
func updateWorkloadCondition(adminWorkload *v1.Workload, newCondition *metav1.Condition) {
	if commonworkload.IsApplication(adminWorkload) {
		lastCondition := adminWorkload.GetLastCondition()
		if lastCondition != nil && newCondition.Type == lastCondition.Type && newCondition.Reason == lastCondition.Reason {
			*lastCondition = *newCondition
			return
		}
		adminWorkload.Status.Conditions = append(adminWorkload.Status.Conditions, *newCondition)
		// Only keep the latest 30 conditions
		maxReserved := MaxConditionHistory
		if l := len(adminWorkload.Status.Conditions); l > maxReserved {
			begin := l - maxReserved
			conditions := make([]metav1.Condition, 0, maxReserved)
			for i := begin; i < l; i++ {
				conditions = append(conditions, adminWorkload.Status.Conditions[i])
			}
			adminWorkload.Status.Conditions = conditions
		}
	} else {
		currentCondition := jobutils.FindCondition(adminWorkload, newCondition)
		if currentCondition != nil {
			if currentCondition.Status != newCondition.Status ||
				currentCondition.Message != newCondition.Message {
				*currentCondition = *newCondition
			}
		} else {
			adminWorkload.Status.Conditions = append(adminWorkload.Status.Conditions, *newCondition)
		}
	}
}

// shouldTerminateWorkload determines if a workload has reached its end state.
// Considers retry limits and failover settings.
func shouldTerminateWorkload(adminWorkload *v1.Workload, status *jobutils.K8sObjectStatus, count int) bool {
	if commonworkload.IsApplication(adminWorkload) || v1.IsWorkloadPreempted(adminWorkload) {
		return false
	}

	switch v1.WorkloadConditionType(status.Phase) {
	case v1.K8sSucceeded:
		return true
	case v1.K8sFailed, v1.K8sDeleted:
		if shouldWorkloadStopRetry(adminWorkload, count) {
			return true
		}
	}
	return false
}

// shouldWorkloadStopRetry determines if a workload should stop retrying based on its retry limit.
func shouldWorkloadStopRetry(adminWorkload *v1.Workload, count int) bool {
	if v1.IsWorkloadDisableFailover(adminWorkload) {
		return true
	}
	// Continue retrying until the max retry limit is reached
	if adminWorkload.Spec.MaxRetry <= 0 || count > adminWorkload.Spec.MaxRetry {
		return true
	}
	return false
}

// handleTorchFTGroupStatus handles status updates for TorchFT workload groups.
// groupId: 0 lighthouse and [1,totalGroups] workers.
// For failure status: available worker groups < minGroup. lighthouse status is temporarily not considered.
// For succeed status: all workers and lighthouse are succeed.
// For other status: returns status only when ALL groups have the same status.
// otherwise: returns empty string.
func handleTorchFTGroupStatus(adminWorkload *v1.Workload, groupIdStr string, phase v1.WorkloadPhase) v1.WorkloadPhase {
	// Get total group count
	totalGroups, err := commonworkload.GetReplicaGroup(adminWorkload, common.ReplicaGroup)
	if err != nil || totalGroups <= 0 {
		// If we can't get total groups, treat as single group
		return phase
	}
	groupId, err := strconv.Atoi(groupIdStr)
	if err != nil {
		// If we can't get group id, treat as single group
		return phase
	}
	// TorchFT job indices are 1 to totalGroups, index > totalGroups is invalid
	if groupId > totalGroups {
		return ""
	}

	minGroups, err := commonworkload.GetReplicaGroup(adminWorkload, common.MinReplicaGroup)
	if err != nil || minGroups <= 0 {
		// If we can't get total groups, treat as single group
		return phase
	}

	// Initialize TorchFTPhase map if nil
	if adminWorkload.Status.TorchFTPhase == nil {
		adminWorkload.Status.TorchFTPhase = make(map[string]v1.WorkloadPhase)
	}

	// Update current group phase
	adminWorkload.Status.TorchFTPhase[groupIdStr] = phase

	// Special handling for WorkloadFailed: only fail if remaining groups < minGroups
	if phase == v1.WorkloadFailed {
		if isTorchFTGroupFailed(adminWorkload) {
			return v1.WorkloadFailed
		}
		return ""
	}

	// Check if all groups have the same phase
	if len(adminWorkload.Status.TorchFTPhase) > totalGroups {
		allSamePhase := true
		beginId := 0
		if phase == v1.WorkloadSucceeded {
			beginId = 1
		}
		for i := beginId; i <= totalGroups; i++ {
			p := adminWorkload.Status.TorchFTPhase[strconv.Itoa(i)]
			if p != phase {
				allSamePhase = false
				break
			}
		}
		if allSamePhase {
			return phase
		}
	}

	// Not all groups have the same phase yet
	return ""
}

// isTorchFTGroupFailed checks if the TorchFT workload should be considered as failed
// if the number of remaining available worker groups falls below the minimum required groups
func isTorchFTGroupFailed(adminWorkload *v1.Workload) bool {
	totalGroups, _ := commonworkload.GetReplicaGroup(adminWorkload, common.ReplicaGroup)
	minGroups, _ := commonworkload.GetReplicaGroup(adminWorkload, common.MinReplicaGroup)

	failedCount := 0
	for i := 1; i <= totalGroups; i++ {
		p := adminWorkload.Status.TorchFTPhase[strconv.Itoa(i)]
		if p == v1.WorkloadFailed || p == v1.WorkloadStopped {
			failedCount++
		}
	}
	// Remaining groups = totalGroups - failedCount
	// If remaining < minGroups, the workload should fail
	if totalGroups-failedCount < minGroups {
		return true
	}
	return false
}

// getFailedPodInfo extracts information about failed pods for error reporting.
// Collects details about up to 3 failed pods.
func getFailedPodInfo(workload *v1.Workload) string {
	type FailedPodInfo struct {
		Pod       string       `json:"pod"`
		Node      string       `json:"node"`
		Message   string       `json:"message,omitempty"`
		Container v1.Container `json:"container"`
	}
	var result []FailedPodInfo
	i := 0
	for _, pod := range workload.Status.Pods {
		if pod.Phase != corev1.PodFailed {
			continue
		}
		info := FailedPodInfo{
			Pod:     pod.PodId,
			Node:    pod.K8sNodeName,
			Message: pod.FailedMessage,
		}
		for _, c := range pod.Containers {
			if c.ExitCode == int32(0) {
				continue
			}
			info.Container = c
			break
		}
		result = append(result, info)
		i++
		if i >= MaxFailedPodsToShow {
			break
		}
	}
	if len(result) == 0 {
		return ""
	}
	return string(jsonutils.MarshalSilently(result))
}
