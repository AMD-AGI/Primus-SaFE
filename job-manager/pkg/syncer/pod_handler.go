/*
 * Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package syncer

import (
	"context"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/klog/v2"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/controller"
	commonworkload "github.com/AMD-AIG-AIMA/SAFE/common/pkg/workload"
	jobutils "github.com/AMD-AIG-AIMA/SAFE/job-manager/pkg/utils"
	jsonutils "github.com/AMD-AIG-AIMA/SAFE/utils/pkg/json"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/timeutil"
	unstructuredutils "github.com/AMD-AIG-AIMA/SAFE/utils/pkg/unstructured"
)

func (r *SyncerReconciler) handlePod(ctx context.Context, msg *resourceMessage, clusterInformer *ClusterInformer) (controller.Result, error) {
	if msg.action == ResourceDel {
		return controller.Result{}, r.removeWorkloadPod(ctx, msg)
	}
	informer, err := clusterInformer.GetResourceInformer(ctx, msg.gvk)
	if err != nil {
		return controller.Result{}, err
	}
	obj, err := jobutils.GetObject(informer, msg.name, msg.namespace)
	if err != nil {
		return controller.Result{}, err
	}
	if !obj.GetDeletionTimestamp().IsZero() {
		if err = r.removeWorkloadPod(ctx, msg); err != nil {
			return controller.Result{}, err
		}
		return r.deletePod(ctx, obj, clusterInformer)
	}
	return r.updateWorkloadPod(ctx, obj, clusterInformer, msg.workloadId)
}

func (r *SyncerReconciler) deletePod(ctx context.Context,
	obj *unstructured.Unstructured, clusterInformer *ClusterInformer) (controller.Result, error) {
	nowTime := time.Now().Unix()
	if nowTime-obj.GetDeletionTimestamp().Unix() < 20 {
		return controller.Result{RequeueAfter: time.Second * 3}, nil
	}

	// Specify the delete options (force delete)
	deletePolicy := metav1.DeletePropagationForeground
	gracePeriodSeconds := int64(0)
	deleteOptions := metav1.DeleteOptions{
		GracePeriodSeconds: &gracePeriodSeconds,
		PropagationPolicy:  &deletePolicy,
	}
	err := clusterInformer.dataClientFactory.ClientSet().CoreV1().
		Pods(obj.GetNamespace()).Delete(ctx, obj.GetName(), deleteOptions)
	if err != nil {
		if !apierrors.IsNotFound(err) {
			klog.ErrorS(err, "failed to delete pod", "name", obj.GetName())
		} else {
			err = nil
		}
		return controller.Result{}, err
	}
	klog.Infof("force to delete pod, namespace: %s, name: %s, generation: %d",
		obj.GetNamespace(), obj.GetName(), obj.GetGeneration())
	return controller.Result{}, nil
}

func (r *SyncerReconciler) updateWorkloadPod(ctx context.Context, obj *unstructured.Unstructured,
	clusterInformer *ClusterInformer, workloadId string) (controller.Result, error) {
	pod := &corev1.Pod{}
	err := unstructuredutils.ConvertUnstructuredToObject(obj, pod)
	if err != nil {
		// This error cannot be resolved by retrying, so it is ignored by returning nil.
		klog.ErrorS(err, "failed to convert object to pod", "data", obj)
		return controller.Result{}, nil
	}
	if pod.Status.Phase == corev1.PodFailed {
		klog.Infof("pod(%s) is failed. reason: %s, message: %s, container: %s",
			pod.Name, pod.Status.Reason, pod.Status.Message, string(jsonutils.MarshalSilently(pod.Status.ContainerStatuses)))
	}
	adminWorkload, err := r.getAdminWorkload(ctx, workloadId)
	if adminWorkload == nil {
		return controller.Result{}, err
	}

	if !v1.IsWorkloadDispatched(adminWorkload) {
		return controller.Result{RequeueAfter: time.Second}, nil
	}
	k8sNode := &corev1.Node{}
	if pod.Spec.NodeName != "" {
		if k8sNode, err = clusterInformer.dataClientFactory.ClientSet().
			CoreV1().Nodes().Get(ctx, pod.Spec.NodeName, metav1.GetOptions{}); err != nil {
			klog.ErrorS(err, "failed to get k8s node")
			return controller.Result{}, err
		}
	}

	id := -1
	for i, p := range adminWorkload.Status.Pods {
		if p.PodId != pod.Name {
			continue
		}
		id = i
		if p.Phase == pod.Status.Phase && p.K8sNodeName == pod.Spec.NodeName && p.StartTime != "" {
			return controller.Result{}, nil
		}
		break
	}

	workloadPod := v1.WorkloadPod{
		PodId:         pod.Name,
		K8sNodeName:   pod.Spec.NodeName,
		AdminNodeName: v1.GetNodeId(k8sNode),
		Phase:         pod.Status.Phase,
		HostIp:        pod.Status.HostIP,
	}
	if !pod.Status.StartTime.IsZero() {
		workloadPod.StartTime = timeutil.FormatRFC3339(&pod.Status.StartTime.Time)
	}
	buildPodTerminatedInfo(pod, &workloadPod)
	if workloadPod.Message != nil {
		klog.Infof("pod(%s) exited abnormally. message: %s",
			pod.Name, string(jsonutils.MarshalSilently(workloadPod.Message)))
	}

	if id >= 0 {
		adminWorkload.Status.Pods[id].K8sNodeName = workloadPod.K8sNodeName
		adminWorkload.Status.Pods[id].AdminNodeName = workloadPod.AdminNodeName
		adminWorkload.Status.Pods[id].Phase = workloadPod.Phase
		adminWorkload.Status.Pods[id].HostIp = workloadPod.HostIp
		adminWorkload.Status.Pods[id].StartTime = workloadPod.StartTime
		adminWorkload.Status.Pods[id].EndTime = workloadPod.EndTime
		adminWorkload.Status.Pods[id].Message = workloadPod.Message
	} else {
		adminWorkload.Status.Pods = append(adminWorkload.Status.Pods, workloadPod)
	}
	if err = r.Status().Update(ctx, adminWorkload); err != nil {
		return controller.Result{}, err
	}
	return controller.Result{}, nil
}

func (r *SyncerReconciler) removeWorkloadPod(ctx context.Context, msg *resourceMessage) error {
	if msg.workloadId == "" {
		return nil
	}
	adminWorkload, err := r.getAdminWorkload(ctx, msg.workloadId)
	if adminWorkload == nil {
		return err
	}
	if adminWorkload.IsEnd() || commonworkload.IsJob(adminWorkload) {
		return nil
	}

	id := -1
	for i, p := range adminWorkload.Status.Pods {
		if p.PodId == msg.name {
			id = i
			break
		}
	}
	if id < 0 {
		return nil
	}
	newPods := append(adminWorkload.Status.Pods[:id], adminWorkload.Status.Pods[id+1:]...)
	adminWorkload.Status.Pods = newPods
	if err = r.Status().Update(ctx, adminWorkload); err != nil {
		klog.ErrorS(err, "failed to update workload status", "name", adminWorkload.Name)
		return err
	}
	return nil
}

func buildPodTerminatedInfo(p *corev1.Pod, workloadPod *v1.WorkloadPod) {
	if p.Status.Phase == corev1.PodFailed {
		workloadPod.Message = new(v1.PodFailedMessage)
		if p.Status.Message != "" {
			workloadPod.Message.Message = p.Status.Message
		}
	} else if p.Status.Phase != corev1.PodSucceeded {
		return
	}

	var finishedTime *metav1.Time
	for i, container := range p.Status.ContainerStatuses {
		terminated := container.State.Terminated
		if terminated == nil {
			continue
		}
		if finishedTime == nil || terminated.FinishedAt.After(finishedTime.Time) {
			finishedTime = &p.Status.ContainerStatuses[i].State.Terminated.FinishedAt
		}
		exitCode := terminated.ExitCode
		if exitCode == 0 || p.Status.Phase != corev1.PodFailed {
			continue
		}
		containerMsg := v1.ContainerFailedMessage{
			Name:     container.Name,
			Reason:   terminated.Reason,
			ExitCode: exitCode,
			Signal:   terminated.Signal,
			Message:  terminated.Message,
		}
		workloadPod.Message.Containers = append(workloadPod.Message.Containers, containerMsg)
	}
	if finishedTime != nil && !finishedTime.IsZero() {
		workloadPod.EndTime = timeutil.FormatRFC3339(&finishedTime.Time)
	}
}
