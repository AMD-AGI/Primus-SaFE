/*
 * Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package syncer

import (
	"bufio"
	"bytes"
	"context"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"
	ctrlruntime "sigs.k8s.io/controller-runtime"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	commonworkload "github.com/AMD-AIG-AIMA/SAFE/common/pkg/workload"
	jobutils "github.com/AMD-AIG-AIMA/SAFE/job-manager/pkg/utils"
	jsonutils "github.com/AMD-AIG-AIMA/SAFE/utils/pkg/json"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/timeutil"
	unstructuredutils "github.com/AMD-AIG-AIMA/SAFE/utils/pkg/unstructured"
)

const (
	ForceDeleteDelaySeconds = 20
	LogTailLines            = 1000
)

// handlePod processes Pod resource events (add, update, delete)
// Manages the synchronization of pod status between data plane and admin plane
// Parameters:
//   - ctx: The context for the operation
//   - message: The resource message containing pod event details
//   - clusterInformer: The cluster informer for accessing resources
//
// Returns:
//   - ctrlruntime.Result: The result of the handling
//   - error: Any error encountered during processing
func (r *SyncerReconciler) handlePod(ctx context.Context, message *resourceMessage, clusterInformer *ClusterInformer) (ctrlruntime.Result, error) {
	if message.action == ResourceDel {
		return ctrlruntime.Result{}, r.removeWorkloadPod(ctx, message)
	}
	informer, err := clusterInformer.GetResourceInformer(ctx, message.gvk)
	if err != nil {
		return ctrlruntime.Result{}, err
	}
	obj, err := jobutils.GetObject(informer, message.name, message.namespace)
	if err != nil {
		return ctrlruntime.Result{}, err
	}
	if !obj.GetDeletionTimestamp().IsZero() {
		if err = r.removeWorkloadPod(ctx, message); err != nil {
			return ctrlruntime.Result{}, err
		}
		return r.deletePod(ctx, obj, clusterInformer)
	}
	return r.updateWorkloadPod(ctx, obj, clusterInformer, message.workloadId)
}

// deletePod forcefully deletes a pod from the data plane
// Implements a delayed force deletion strategy to avoid premature deletion
// Parameters:
//   - ctx: The context for the operation
//   - obj: The unstructured pod object to delete
//   - clusterInformer: The cluster informer for accessing the data plane client
//
// Returns:
//   - ctrlruntime.Result: The result of the deletion
//   - error: Any error encountered during deletion
func (r *SyncerReconciler) deletePod(ctx context.Context,
	obj *unstructured.Unstructured, clusterInformer *ClusterInformer) (ctrlruntime.Result, error) {
	nowTime := time.Now().Unix()
	if nowTime-obj.GetDeletionTimestamp().Unix() < ForceDeleteDelaySeconds {
		return ctrlruntime.Result{RequeueAfter: time.Second * 3}, nil
	}

	// Specify the delete options (force delete)
	gracePeriodSeconds := int64(0)
	deleteOptions := metav1.DeleteOptions{
		GracePeriodSeconds: &gracePeriodSeconds,
	}
	err := clusterInformer.dataClientFactory.ClientSet().CoreV1().
		Pods(obj.GetNamespace()).Delete(ctx, obj.GetName(), deleteOptions)
	if err != nil {
		if !apierrors.IsNotFound(err) {
			klog.ErrorS(err, "failed to delete pod", "name", obj.GetName())
		} else {
			err = nil
		}
		return ctrlruntime.Result{}, err
	}
	klog.Infof("force to delete pod, namespace: %s, name: %s, generation: %d",
		obj.GetNamespace(), obj.GetName(), obj.GetGeneration())
	return ctrlruntime.Result{}, nil
}

// updateWorkloadPod updates the workload status based on pod information
// Synchronizes pod details like phase, node assignment, and container status
// Parameters:
//   - ctx: The context for the operation
//   - obj: The unstructured pod object
//   - clusterInformer: The cluster informer for accessing resources
//   - workloadId: The ID of the associated workload
//
// Returns:
//   - ctrlruntime.Result: The result of the update
//   - error: Any error encountered during update
func (r *SyncerReconciler) updateWorkloadPod(ctx context.Context, obj *unstructured.Unstructured,
	clusterInformer *ClusterInformer, workloadId string) (ctrlruntime.Result, error) {
	pod := &corev1.Pod{}
	err := unstructuredutils.ConvertUnstructuredToObject(obj, pod)
	if err != nil {
		// This error cannot be resolved by retrying, so it is ignored by returning nil.
		klog.ErrorS(err, "failed to convert object to pod", "data", obj)
		return ctrlruntime.Result{}, nil
	}
	if pod.Status.Phase == corev1.PodFailed {
		klog.Infof("pod(%s) is failed. reason: %s, message: %s, container: %s",
			pod.Name, pod.Status.Reason, pod.Status.Message, string(jsonutils.MarshalSilently(pod.Status.ContainerStatuses)))
	}
	adminWorkload, err := r.getAdminWorkload(ctx, workloadId)
	if adminWorkload == nil {
		return ctrlruntime.Result{}, err
	}

	if !v1.IsWorkloadDispatched(adminWorkload) {
		return ctrlruntime.Result{RequeueAfter: time.Second}, nil
	}
	k8sNode := &corev1.Node{}
	if pod.Spec.NodeName != "" {
		if k8sNode, err = clusterInformer.dataClientFactory.ClientSet().
			CoreV1().Nodes().Get(ctx, pod.Spec.NodeName, metav1.GetOptions{}); err != nil {
			klog.ErrorS(err, "failed to get k8s node")
			return ctrlruntime.Result{}, err
		}
	}

	id := -1
	for i, p := range adminWorkload.Status.Pods {
		if p.PodId != pod.Name {
			continue
		}
		id = i
		if p.Phase == pod.Status.Phase && p.K8sNodeName == pod.Spec.NodeName && p.StartTime != "" {
			return ctrlruntime.Result{}, nil
		}
		break
	}

	workloadPod := v1.WorkloadPod{
		PodId:         pod.Name,
		K8sNodeName:   pod.Spec.NodeName,
		AdminNodeName: v1.GetNodeId(k8sNode),
		Phase:         pod.Status.Phase,
		HostIp:        pod.Status.HostIP,
		PodIp:         pod.Status.PodIP,
		Rank:          getMainContainerRank(adminWorkload, pod),
	}
	if pod.Status.StartTime != nil && !pod.Status.StartTime.IsZero() {
		workloadPod.StartTime = timeutil.FormatRFC3339(pod.Status.StartTime.Time)
	}
	buildPodTerminatedInfo(ctx,
		clusterInformer.dataClientFactory.ClientSet(), adminWorkload, pod, &workloadPod)
	if id >= 0 {
		adminWorkload.Status.Pods[id].K8sNodeName = workloadPod.K8sNodeName
		adminWorkload.Status.Pods[id].AdminNodeName = workloadPod.AdminNodeName
		adminWorkload.Status.Pods[id].Phase = workloadPod.Phase
		adminWorkload.Status.Pods[id].HostIp = workloadPod.HostIp
		adminWorkload.Status.Pods[id].PodIp = workloadPod.PodIp
		adminWorkload.Status.Pods[id].StartTime = workloadPod.StartTime
		adminWorkload.Status.Pods[id].EndTime = workloadPod.EndTime
		adminWorkload.Status.Pods[id].Containers = workloadPod.Containers
		adminWorkload.Status.Pods[id].Rank = workloadPod.Rank
	} else {
		adminWorkload.Status.Pods = append(adminWorkload.Status.Pods, workloadPod)
	}
	return ctrlruntime.Result{}, r.Status().Update(ctx, adminWorkload)
}

// getMainContainerRank retrieves the rank value from the main container's environment variables
// Used for distributed training workloads to identify process rank
// Parameters:
//   - adminWorkload: The workload containing main container information
//   - pod: The pod to extract rank from
//
// Returns:
//   - string: The rank value, or empty string if not found
func getMainContainerRank(adminWorkload *v1.Workload, pod *corev1.Pod) string {
	for _, container := range pod.Spec.Containers {
		if container.Name != v1.GetMainContainer(adminWorkload) {
			continue
		}
		for _, env := range container.Env {
			if env.Name == "RANK" {
				return env.Value
			}
		}
	}
	return ""
}

// removeWorkloadPod removes a pod entry from the workload status
// Called when a pod is deleted to clean up the workload's pod list
// Parameters:
//   - ctx: The context for the operation
//   - message: The resource message containing pod deletion details
//
// Returns:
//   - error: Any error encountered during removal
func (r *SyncerReconciler) removeWorkloadPod(ctx context.Context, message *resourceMessage) error {
	if message.workloadId == "" {
		return nil
	}
	adminWorkload, err := r.getAdminWorkload(ctx, message.workloadId)
	if adminWorkload == nil {
		return err
	}
	if adminWorkload.IsEnd() || commonworkload.IsJob(adminWorkload) {
		return nil
	}

	id := -1
	for i, p := range adminWorkload.Status.Pods {
		if p.PodId == message.name {
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

// buildPodTerminatedInfo constructs termination information for a pod
// Extracts container termination details and finished time for completed pods
// Parameters:
//   - ctx: The context for the operation
//   - clientSet: The Kubernetes client set for log access
//   - adminWorkload: The associated workload
//   - pod: The pod to extract termination info from
//   - workloadPod: The workload pod to populate with termination info
func buildPodTerminatedInfo(ctx context.Context,
	clientSet kubernetes.Interface, adminWorkload *v1.Workload, pod *corev1.Pod, workloadPod *v1.WorkloadPod) {
	if pod.Status.Phase != corev1.PodSucceeded && pod.Status.Phase != corev1.PodFailed {
		return
	}

	var finishedTime *metav1.Time
	for i, container := range pod.Status.ContainerStatuses {
		terminated := container.State.Terminated
		if terminated == nil {
			continue
		}
		if finishedTime == nil || terminated.FinishedAt.After(finishedTime.Time) {
			finishedTime = &pod.Status.ContainerStatuses[i].State.Terminated.FinishedAt
		}
		c := v1.Container{
			Name:     container.Name,
			Reason:   terminated.Reason,
			ExitCode: terminated.ExitCode,
			Message:  terminated.Message,
		}
		if commonworkload.IsOpsJob(adminWorkload) {
			message := getPodLog(ctx, clientSet, pod, v1.GetMainContainer(adminWorkload))
			c.Message = message
		}
		workloadPod.Containers = append(workloadPod.Containers, c)
	}
	if finishedTime != nil && !finishedTime.IsZero() {
		workloadPod.EndTime = timeutil.FormatRFC3339(finishedTime.Time)
	}
}

// getPodLog retrieves and filters logs from a pod's main container
// Extracts lines containing ERROR or SUCCESS markers for OpsJob workloads
// Parameters:
//   - ctx: The context for the operation
//   - clientSet: The Kubernetes client set for log access
//   - pod: The pod to retrieve logs from
//   - mainContainerName: The name of the main container
//
// Returns:
//   - string: Filtered log content as JSON, or empty string if no relevant logs
func getPodLog(ctx context.Context, clientSet kubernetes.Interface, pod *corev1.Pod, mainContainerName string) string {
	var tailLine int64 = LogTailLines
	opt := &corev1.PodLogOptions{
		Container: mainContainerName,
		TailLines: &tailLine,
	}
	data, err := clientSet.CoreV1().Pods(pod.Namespace).GetLogs(pod.Name, opt).DoRaw(ctx)
	if err != nil {
		klog.ErrorS(err, "failed to get log of pod", "namespace", pod.Namespace, "podName", pod.Name)
		return ""
	}

	// Scanner and bytes.Reader do not require explicit closing
	scanner := bufio.NewScanner(bytes.NewReader(data))
	var lines []string
	for scanner.Scan() {
		line := scanner.Text()
		if strings.Contains(line, "[ERROR]") || strings.Contains(line, "[SUCCESS]") {
			lines = append(lines, line)
		}
	}
	if err = scanner.Err(); err != nil {
		klog.ErrorS(err, "failed to read pod log lines")
	}
	if len(lines) == 0 {
		return ""
	}
	return string(jsonutils.MarshalSilently(lines))
}
