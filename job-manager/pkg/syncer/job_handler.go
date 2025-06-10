/*
 * Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package syncer

import (
	"context"
	"fmt"
	"reflect"
	"sort"
	"strconv"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/controller"
	commonerrors "github.com/AMD-AIG-AIMA/SAFE/common/pkg/errors"
	commonworkload "github.com/AMD-AIG-AIMA/SAFE/common/pkg/workload"
	jobutils "github.com/AMD-AIG-AIMA/SAFE/job-manager/pkg/utils"
	jsonutils "github.com/AMD-AIG-AIMA/SAFE/utils/pkg/json"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/sets"
)

func (r *SyncerReconciler) handleJob(ctx context.Context, msg *resourceMessage, informer *ClusterInformer) (controller.Result, error) {
	adminWorkload, err := r.getAdminWorkload(ctx, msg.workloadId)
	if adminWorkload == nil {
		return controller.Result{}, err
	}
	if !adminWorkload.GetDeletionTimestamp().IsZero() {
		return controller.Result{}, nil
	}
	if !v1.IsWorkloadDispatched(adminWorkload) {
		return controller.Result{RequeueAfter: time.Second}, nil
	}

	result, err := r._handleJob(ctx, msg, adminWorkload, informer)
	if jobutils.IsUnRecoverableError(err) {
		// Errors defined internally are fatal and lead to a terminal state without retry
		err = jobutils.SetWorkloadFailed(ctx, r.Client, adminWorkload, err.Error())
	}
	return result, err
}

func (r *SyncerReconciler) _handleJob(ctx context.Context, msg *resourceMessage,
	adminWorkload *v1.Workload, informer *ClusterInformer) (controller.Result, error) {
	if msg.action == ResourceDel {
		klog.Infof("delete resource. name: %s/%s, kind: %s, dispatchCount: %d",
			msg.namespace, msg.name, msg.gvk.Kind, msg.dispatchCount)
		// wait until all pods are deleted
		if !r.waitAllPodsDeleted(ctx, msg, informer) {
			return controller.Result{RequeueAfter: time.Second * 3}, nil
		}
	}

	status, err := r.getK8sResourceStatus(ctx, msg, informer, adminWorkload)
	if err != nil {
		return controller.Result{}, err
	}

	var isNeedRetry bool
	adminWorkload, isNeedRetry, err = r.updateAdminWorkloadStatus(ctx, adminWorkload, status, msg)
	if isNeedRetry {
		return controller.Result{RequeueAfter: time.Second}, nil
	}
	if err != nil {
		klog.ErrorS(err, "failed to update admin workload status")
		return controller.Result{}, err
	}

	if msg.action == ResourceDel && !adminWorkload.IsEnd() {
		if err = r.reSchedule(ctx, adminWorkload, msg.dispatchCount); err != nil {
			klog.ErrorS(err, "failed to reSchedule", "workload", adminWorkload.Name)
			return controller.Result{}, err
		}
	}
	return controller.Result{}, nil
}

func (r *SyncerReconciler) getK8sResourceStatus(ctx context.Context, msg *resourceMessage,
	clusterInformer *ClusterInformer, adminWorkload *v1.Workload) (*jobutils.K8sResourceStatus, error) {
	if msg.action == ResourceDel {
		return &jobutils.K8sResourceStatus{
			Phase:   string(v1.K8sDeleted),
			Message: fmt.Sprintf("%s %s is deleted", msg.gvk.Kind, msg.name),
			Reason:  "ResourceDeleted",
		}, nil
	}

	informer, err := clusterInformer.GetResourceInformer(ctx, msg.gvk)
	if err != nil {
		klog.ErrorS(err, "failed to get resource informer")
		return nil, err
	}
	k8sObject, err := jobutils.GetObject(informer, msg.name, msg.namespace)
	if err != nil {
		klog.ErrorS(err, "failed to get k8s object", "name", msg.name, "namespace", msg.namespace)
		return nil, err
	}
	rt, err := jobutils.GetResourceTemplate(ctx, r.Client, msg.gvk)
	if err != nil {
		klog.ErrorS(err, "failed to get resource template", "name", msg.name, "kind", msg.gvk.Kind)
		return nil, err
	}
	status, err := jobutils.GetK8sResourceStatus(k8sObject, rt)
	if err != nil {
		klog.ErrorS(err, "failed to get phase", "name", msg.name, "namespace", msg.namespace)
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

func (r *SyncerReconciler) waitAllPodsDeleted(ctx context.Context, msg *resourceMessage, informer *ClusterInformer) bool {
	labelSelector := metav1.LabelSelector{
		MatchLabels: map[string]string{v1.WorkloadIdLabel: msg.workloadId},
	}
	listOptions := metav1.ListOptions{
		LabelSelector: metav1.FormatLabelSelector(&labelSelector),
	}
	podList, err := informer.dataClientFactory.ClientSet().CoreV1().Pods(msg.namespace).List(ctx, listOptions)
	if err != nil {
		klog.ErrorS(err, "failed to list pods", "workload", msg.workloadId, "namespace", msg.namespace)
		return false
	}
	if len(podList.Items) == 0 {
		return true
	}
	return false
}

func (r *SyncerReconciler) updateAdminWorkloadStatus(ctx context.Context, originWorkload *v1.Workload,
	status *jobutils.K8sResourceStatus, msg *resourceMessage) (*v1.Workload, bool, error) {
	if originWorkload.IsEnd() || status == nil {
		return originWorkload, false, nil
	}
	adminWorkload := originWorkload.DeepCopy()
	r.updateAdminWorkloadPhase(adminWorkload, status, msg)

	switch {
	case adminWorkload.IsPending():
		if status.Phase != string(v1.K8sDeleted) &&
			status.Phase != string(v1.K8sFailed) && originWorkload.IsPending() {
			return originWorkload, false, nil
		}
	case adminWorkload.IsRunning():
		if isNeedRetry := r.updateAdminWorkloadNodes(adminWorkload, msg); isNeedRetry {
			return originWorkload, true, nil
		}
	case adminWorkload.IsEnd():
		if adminWorkload.Status.EndTime == nil {
			adminWorkload.Status.EndTime = &metav1.Time{Time: time.Now().UTC()}
		}
		r.updateAdminWorkloadNodes(adminWorkload, msg)
	}

	if adminWorkload.Status.StartTime == nil {
		adminWorkload.Status.StartTime = &metav1.Time{Time: time.Now().UTC()}
	}
	adminWorkload.Status.Message = ""
	buildWorkloadCondition(adminWorkload, status, msg.dispatchCount)
	if reflect.DeepEqual(adminWorkload.Status, originWorkload.Status) {
		return originWorkload, false, nil
	}
	if err := r.Status().Update(ctx, adminWorkload); err != nil {
		return nil, false, err
	}
	klog.Infof("update workload status, name: %s, phase: %s, dispatchCount: %d, k8s.status: %s",
		adminWorkload.Name, adminWorkload.Status.Phase, msg.dispatchCount, jsonutils.MarshalSilently(status))
	return adminWorkload, false, nil
}

func (r *SyncerReconciler) updateAdminWorkloadPhase(adminWorkload *v1.Workload,
	status *jobutils.K8sResourceStatus, msg *resourceMessage) {
	switch v1.WorkloadConditionType(status.Phase) {
	case v1.K8sPending:
		adminWorkload.Status.Phase = v1.WorkloadPending
	case v1.K8sSucceeded:
		if isWorkloadEnd(adminWorkload, status, msg.dispatchCount) {
			adminWorkload.Status.Phase = v1.WorkloadSucceeded
		}
	case v1.K8sFailed, v1.K8sDeleted:
		if isWorkloadEnd(adminWorkload, status, msg.dispatchCount) {
			adminWorkload.Status.Phase = v1.WorkloadFailed
		} else if adminWorkload.IsRunning() && commonworkload.IsApplication(adminWorkload) {
			adminWorkload.Status.Phase = v1.WorkloadNotReady
		}
	case v1.K8sRunning:
		if !adminWorkload.IsStopping() {
			adminWorkload.Status.Phase = v1.WorkloadRunning
		}
	case v1.K8sUpdating:
		// only for deployment/statefulSet
		adminWorkload.Status.Phase = v1.WorkloadUpdating
	case v1.AdminStopped:
		adminWorkload.Status.Phase = v1.WorkloadStopped
	}
}

func (r *SyncerReconciler) reSchedule(ctx context.Context, workload *v1.Workload, count int) error {
	patch := client.MergeFrom(workload.DeepCopy())
	isStatusChanged := false
	if len(workload.Status.Pods) > 0 {
		workload.Status.Pods = nil
		isStatusChanged = true
	}
	if workload.Status.Phase != v1.WorkloadPending {
		workload.Status.Phase = v1.WorkloadPending
		isStatusChanged = true
	}
	if len(workload.Status.Nodes) < count {
		workload.Status.Nodes = append(workload.Status.Nodes, []string{})
		isStatusChanged = true
	}
	if isStatusChanged {
		if err := r.Status().Patch(ctx, workload, patch); err != nil {
			return err
		}
	}

	if v1.IsWorkloadDispatched(workload) {
		patch = client.MergeFrom(workload.DeepCopy())
		annotations := workload.GetAnnotations()
		delete(annotations, v1.WorkloadDispatchedAnnotation)
		delete(annotations, v1.WorkloadScheduledAnnotation)
		// Upon rescheduling, the task is enqueued with high priority
		annotations[v1.WorkloadReScheduledAnnotation] = ""
		workload.SetAnnotations(annotations)
		if err := r.Patch(ctx, workload, patch); err != nil {
			return err
		}
	}
	klog.Infof("reSchedule workload, name: %s, dispatchCount: %d", workload.Name, count)
	return nil
}

func (r *SyncerReconciler) updateAdminWorkloadNodes(adminWorkload *v1.Workload, msg *resourceMessage) bool {
	totalNodeCount := adminWorkload.Spec.Resource.Replica
	if commonworkload.IsJob(adminWorkload) {
		if adminWorkload.Spec.Resource.Replica != len(adminWorkload.Status.Pods) {
			return true
		}
		// the nodes of admin workload are already updated
		if len(adminWorkload.Status.Nodes) == msg.dispatchCount {
			return false
		}
	} else {
		if totalNodeCount > len(adminWorkload.Status.Pods) {
			return true
		}
	}
	sortWorkloadPods(adminWorkload)

	nodeNames := make([]string, 0, len(adminWorkload.Status.Pods))
	nodeNameSet := sets.NewSet()
	for _, p := range adminWorkload.Status.Pods {
		if !nodeNameSet.Has(p.K8sNodeName) {
			nodeNames = append(nodeNames, p.K8sNodeName)
			nodeNameSet.Insert(p.K8sNodeName)
		}
	}
	if len(adminWorkload.Status.Nodes) < msg.dispatchCount {
		adminWorkload.Status.Nodes = append(adminWorkload.Status.Nodes, nodeNames)
	} else if msg.dispatchCount > 0 {
		adminWorkload.Status.Nodes[msg.dispatchCount-1] = nodeNames
	}
	return false
}

func sortWorkloadPods(adminWorkload *v1.Workload) {
	sort.Slice(adminWorkload.Status.Pods, func(i, j int) bool {
		if adminWorkload.Status.Pods[i].HostIp == adminWorkload.Status.Pods[j].HostIp {
			return adminWorkload.Status.Pods[i].PodId < adminWorkload.Status.Pods[j].PodId
		}
		return adminWorkload.Status.Pods[i].HostIp < adminWorkload.Status.Pods[j].HostIp
	})
}

func buildWorkloadCondition(adminWorkload *v1.Workload, status *jobutils.K8sResourceStatus, dispatchCount int) {
	if adminWorkload.IsStopping() {
		return
	}
	if commonworkload.IsApplication(adminWorkload) {
		cond := jobutils.NewCondition(status.Phase, status.Message, commonworkload.GenerateDispatchReason(dispatchCount))
		if cond2 := adminWorkload.GetLastCondition(); cond2 != nil && cond.Type == cond2.Type {
			return
		}
		adminWorkload.Status.Conditions = append(adminWorkload.Status.Conditions, *cond)
		// Only keep the latest 30 conditions
		maxReserved := 30
		if l := len(adminWorkload.Status.Conditions); l > maxReserved {
			begin := l - maxReserved
			conditions := make([]metav1.Condition, 0, maxReserved)
			for i := begin; i < l; i++ {
				conditions = append(conditions, adminWorkload.Status.Conditions[i])
			}
			adminWorkload.Status.Conditions = conditions
		}
	} else {
		cond := jobutils.NewCondition(status.Phase, status.Message, commonworkload.GenerateDispatchReason(dispatchCount))
		if jobutils.FindCondition(adminWorkload, cond) == nil {
			adminWorkload.Status.Conditions = append(adminWorkload.Status.Conditions, *cond)
		}
	}
}

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

func getFailedPodInfo(workload *v1.Workload) string {
	result := ""
	i := 0
	type FailedPodInfo struct {
		Pod  string `json:"pod"`
		Node string `json:"node"`
		*v1.PodFailedMessage
	}

	for _, pod := range workload.Status.Pods {
		if pod.Phase != corev1.PodFailed {
			continue
		}
		if result != "" {
			result += "; "
		}
		i++
		info := FailedPodInfo{
			Pod:              pod.PodId,
			Node:             pod.K8sNodeName,
			PodFailedMessage: pod.Message,
		}
		result += "(" + strconv.Itoa(i) + ") " + string(jsonutils.MarshalSilently(&info))
		if i >= 3 {
			break
		}
	}
	return result
}
