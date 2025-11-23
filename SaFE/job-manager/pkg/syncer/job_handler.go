/*
 * Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package syncer

import (
	"context"
	"fmt"
	"reflect"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
	ctrlruntime "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
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
func (r *SyncerReconciler) handleJob(ctx context.Context, message *resourceMessage, informer *ClusterInformer) (ctrlruntime.Result, error) {
	adminWorkload, err := r.getAdminWorkload(ctx, message.workloadId)
	if adminWorkload == nil {
		return ctrlruntime.Result{}, err
	}
	if !adminWorkload.GetDeletionTimestamp().IsZero() {
		return ctrlruntime.Result{}, nil
	}
	if !v1.IsWorkloadDispatched(adminWorkload) {
		return ctrlruntime.Result{RequeueAfter: time.Second}, nil
	}

	result, err := r.handleJobImpl(ctx, message, adminWorkload, informer)
	if jobutils.IsUnrecoverableError(err) {
		// Errors defined internally are fatal and lead to a terminal state without retry
		err = jobutils.SetWorkloadFailed(ctx, r.Client, adminWorkload, err.Error())
	}
	return result, err
}

// handleJobImpl implements the core logic for handling job resource events.
// Processes resource creation, update, and deletion events.
func (r *SyncerReconciler) handleJobImpl(ctx context.Context, message *resourceMessage,
	adminWorkload *v1.Workload, informer *ClusterInformer) (ctrlruntime.Result, error) {
	if message.action == ResourceDel {
		// wait until all pods are deleted
		if !r.waitAllPodsDeleted(ctx, message, informer) {
			return ctrlruntime.Result{RequeueAfter: time.Second * 3}, nil
		}
	}

	status, err := r.getK8sResourceStatus(ctx, message, informer, adminWorkload)
	if err != nil {
		return ctrlruntime.Result{}, err
	}
	adminWorkload, err = r.updateAdminWorkloadStatus(ctx, adminWorkload, status, message)
	if err != nil {
		klog.ErrorS(err, "failed to update admin workload status")
		return ctrlruntime.Result{}, err
	}

	if message.action == ResourceDel && !adminWorkload.IsEnd() {
		// wait until the job is also deleted
		if !r.waitJobDeleted(ctx, adminWorkload, informer) {
			return ctrlruntime.Result{RequeueAfter: time.Second * 3}, nil
		}
		if err = r.reSchedule(ctx, adminWorkload, message.dispatchCount); err != nil {
			klog.ErrorS(err, "failed to reSchedule", "workload", adminWorkload.Name)
			return ctrlruntime.Result{}, err
		}
	}
	return ctrlruntime.Result{}, nil
}

// getK8sResourceStatus retrieves the status of a Kubernetes resource.
// Extracts phase and message information from the resource.
func (r *SyncerReconciler) getK8sResourceStatus(ctx context.Context, message *resourceMessage,
	clusterInformer *ClusterInformer, adminWorkload *v1.Workload) (*jobutils.K8sResourceStatus, error) {
	if message.action == ResourceDel {
		return &jobutils.K8sResourceStatus{
			Phase:   string(v1.K8sDeleted),
			Message: fmt.Sprintf("%s %s is deleted", message.gvk.Kind, message.name),
		}, nil
	}

	informer, err := clusterInformer.GetResourceInformer(ctx, message.gvk)
	if err != nil {
		klog.ErrorS(err, "failed to get resource informer")
		return nil, err
	}
	k8sObject, err := jobutils.GetObject(informer, message.name, message.namespace)
	if err != nil {
		klog.ErrorS(err, "failed to get k8s object", "name", message.name, "namespace", message.namespace)
		return nil, err
	}
	rt, err := jobutils.GetResourceTemplate(ctx, r.Client, message.gvk)
	if err != nil {
		klog.ErrorS(err, "failed to get resource template", "name", message.name, "kind", message.gvk.Kind)
		return nil, err
	}
	status, err := jobutils.GetK8sResourceStatus(k8sObject, rt)
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
func (r *SyncerReconciler) waitAllPodsDeleted(ctx context.Context, message *resourceMessage, informer *ClusterInformer) bool {
	labelSelector := metav1.LabelSelector{
		MatchLabels: map[string]string{v1.WorkloadIdLabel: message.workloadId},
	}
	listOptions := metav1.ListOptions{
		LabelSelector: metav1.FormatLabelSelector(&labelSelector),
	}
	podList, err := informer.dataClientFactory.ClientSet().CoreV1().Pods(message.namespace).List(ctx, listOptions)
	if err != nil {
		klog.ErrorS(err, "failed to list pods", "workload", message.workloadId, "namespace", message.namespace)
		return false
	}
	if len(podList.Items) == 0 {
		return true
	}
	klog.Warningf("the pods of this workload %s still exists, this will retry again in 3 seconds.", message.workloadId)
	return false
}

func (r *SyncerReconciler) waitJobDeleted(ctx context.Context, adminWorkload *v1.Workload, informer *ClusterInformer) bool {
	obj, err := jobutils.GenObjectReference(ctx, r.Client, adminWorkload)
	if err != nil {
		return apierrors.IsNotFound(err)
	}
	if _, err = jobutils.GetObjectByClientFactory(ctx, informer.ClientFactory(), obj); err != nil {
		if apierrors.IsNotFound(err) {
			return true
		}
	}
	klog.Warningf("the job of this workload %s still exists, this will retry again in 3 seconds.", adminWorkload.Name)
	return false
}

// updateAdminWorkloadStatus updates the admin workload status based on resource status.
// Manages workload phase transitions and condition updates.
func (r *SyncerReconciler) updateAdminWorkloadStatus(ctx context.Context, originalWorkload *v1.Workload,
	status *jobutils.K8sResourceStatus, message *resourceMessage) (*v1.Workload, error) {
	if originalWorkload.IsEnd() || status == nil || status.Phase == "" {
		return originalWorkload, nil
	}
	adminWorkload := originalWorkload.DeepCopy()
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
	adminWorkload.Status.K8sObjectUid = string(message.uid)
	cond := jobutils.NewCondition(status.Phase, status.Message,
		commonworkload.GenerateDispatchReason(message.dispatchCount))
	updateWorkloadCondition(adminWorkload, cond)
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

// updateAdminWorkloadPhase updates the workload phase based on resource status.
func (r *SyncerReconciler) updateAdminWorkloadPhase(adminWorkload *v1.Workload,
	status *jobutils.K8sResourceStatus, message *resourceMessage) {
	switch v1.WorkloadConditionType(status.Phase) {
	case v1.K8sPending:
		adminWorkload.Status.Phase = v1.WorkloadPending
	case v1.K8sSucceeded:
		if isWorkloadEnd(adminWorkload, status, message.dispatchCount) {
			adminWorkload.Status.Phase = v1.WorkloadSucceeded
		}
	case v1.K8sFailed, v1.K8sDeleted:
		if isWorkloadEnd(adminWorkload, status, message.dispatchCount) {
			adminWorkload.Status.Phase = v1.WorkloadFailed
		} else if adminWorkload.IsRunning() && commonworkload.IsApplication(adminWorkload) {
			adminWorkload.Status.Phase = v1.WorkloadNotReady
		}
	case v1.K8sRunning:
		adminWorkload.Status.Phase = v1.WorkloadRunning
	case v1.K8sUpdating:
		// only for deployment/statefulSet
		adminWorkload.Status.Phase = v1.WorkloadUpdating
	case v1.AdminStopped:
		adminWorkload.Status.Phase = v1.WorkloadStopped
	}
}

// reSchedule handles workload rescheduling by resetting status and annotations.
// Clears workload state and marks for rescheduling.
func (r *SyncerReconciler) reSchedule(ctx context.Context, workload *v1.Workload, count int) error {
	originalWorkload := client.MergeFrom(workload.DeepCopy())
	isStatusChanged := false
	if len(workload.Status.Pods) > 0 {
		workload.Status.Pods = nil
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
		if err := r.Status().Patch(ctx, workload, originalWorkload); err != nil {
			return err
		}
	}

	if v1.IsWorkloadDispatched(workload) {
		originalWorkload = client.MergeFrom(workload.DeepCopy())
		annotations := workload.GetAnnotations()
		delete(annotations, v1.WorkloadDispatchedAnnotation)
		delete(annotations, v1.WorkloadScheduledAnnotation)
		// Upon rescheduling, the task is enqueued with high priority
		annotations[v1.WorkloadReScheduledAnnotation] = ""
		workload.SetAnnotations(annotations)
		if err := r.Patch(ctx, workload, originalWorkload); err != nil {
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

// isWorkloadEnd determines if a workload has reached its end state.
// Considers retry limits and failover settings.
func isWorkloadEnd(adminWorkload *v1.Workload, status *jobutils.K8sResourceStatus, count int) bool {
	if commonworkload.IsApplication(adminWorkload) || v1.IsWorkloadPreempted(adminWorkload) {
		return false
	}
	switch v1.WorkloadConditionType(status.Phase) {
	case v1.K8sSucceeded:
		return true
	case v1.K8sFailed, v1.K8sDeleted:
		// Continue retrying until the max retry limit is reached
		if adminWorkload.Spec.MaxRetry <= 0 ||
			count > adminWorkload.Spec.MaxRetry || v1.IsWorkloadDisableFailover(adminWorkload) {
			return true
		}
	}
	return false
}

// getFailedPodInfo extracts information about failed pods for error reporting.
// Collects details about up to 3 failed pods.
func getFailedPodInfo(workload *v1.Workload) string {
	type FailedPodInfo struct {
		Pod       string       `json:"pod"`
		Node      string       `json:"node"`
		Container v1.Container `json:"container"`
	}
	var result []FailedPodInfo
	i := 0
	for _, pod := range workload.Status.Pods {
		if pod.Phase != corev1.PodFailed {
			continue
		}
		info := FailedPodInfo{
			Pod:  pod.PodId,
			Node: pod.K8sNodeName,
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
