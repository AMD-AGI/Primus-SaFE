/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package syncer

import (
	"bufio"
	"bytes"
	"context"
	"sort"
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
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/netutil"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/sets"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/timeutil"
	unstructuredutils "github.com/AMD-AIG-AIMA/SAFE/utils/pkg/unstructured"
)

const (
	ForceDeleteDelaySeconds = 20
	LogTailLines            = 1000

	appComponent     = "app.kubernetes.io/component"
	scaleSetListener = "runner-scale-set-listener"
)

// handlePod processes Pod resource events (add, update, delete).
// Manages the synchronization of pod status between data plane and admin plane.
func (r *SyncerReconciler) handlePod(ctx context.Context,
	message *resourceMessage, clusterClientSets *ClusterClientSets) (ctrlruntime.Result, error) {
	if message.action == ResourceDel {
		return ctrlruntime.Result{}, r.removeWorkloadPod(ctx, message)
	}
	informer, err := clusterClientSets.GetResourceInformer(ctx, message.gvk)
	if err != nil {
		return ctrlruntime.Result{}, err
	}
	obj, err := jobutils.GetObjectByInformer(informer, message.name, message.namespace)
	if err != nil {
		return ctrlruntime.Result{}, err
	}
	if !obj.GetDeletionTimestamp().IsZero() {
		if err = r.removeWorkloadPod(ctx, message); err != nil {
			return ctrlruntime.Result{}, err
		}
		return r.deletePod(ctx, obj, clusterClientSets)
	}
	return r.updateWorkloadPod(ctx, obj, clusterClientSets, message)
}

// deletePod forcefully deletes a pod from the data plane.
// Implements a delayed force deletion strategy to avoid premature deletion.
func (r *SyncerReconciler) deletePod(ctx context.Context,
	obj *unstructured.Unstructured, clusterClientSets *ClusterClientSets) (ctrlruntime.Result, error) {
	nowTime := time.Now().Unix()
	if nowTime-obj.GetDeletionTimestamp().Unix() < ForceDeleteDelaySeconds {
		return ctrlruntime.Result{RequeueAfter: time.Second * 3}, nil
	}

	// Specify the delete options (force delete)
	gracePeriodSeconds := int64(0)
	deleteOptions := metav1.DeleteOptions{
		GracePeriodSeconds: &gracePeriodSeconds,
	}
	err := clusterClientSets.dataClientFactory.ClientSet().CoreV1().
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

// updateWorkloadPod updates the workload status based on pod information.
// Synchronizes pod details like phase, node assignment, and container status.
func (r *SyncerReconciler) updateWorkloadPod(ctx context.Context, obj *unstructured.Unstructured,
	clientSets *ClusterClientSets, message *resourceMessage) (ctrlruntime.Result, error) {
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
	adminWorkload, err := r.getAdminWorkload(ctx, message.workloadId)
	if adminWorkload == nil {
		return ctrlruntime.Result{}, err
	}
	if !v1.IsWorkloadDispatched(adminWorkload) {
		return ctrlruntime.Result{RequeueAfter: time.Second}, nil
	}

	id := -1
	for i, p := range adminWorkload.Status.Pods {
		if p.PodId != pod.Name {
			continue
		}
		id = i
		if p.Phase == pod.Status.Phase && p.K8sNodeName == pod.Spec.NodeName &&
			p.StartTime != "" && p.HostIp == pod.Status.HostIP {
			return ctrlruntime.Result{}, nil
		}
		break
	}

	k8sNode := &corev1.Node{}
	if pod.Spec.NodeName != "" {
		if k8sNode, err = clientSets.dataClientFactory.ClientSet().
			CoreV1().Nodes().Get(ctx, pod.Spec.NodeName, metav1.GetOptions{}); err != nil {
			klog.ErrorS(err, "failed to get k8s node")
			return ctrlruntime.Result{}, err
		}
	}

	resourceId, _ := v1.GetResourceId(pod)
	workloadPod := v1.WorkloadPod{
		PodId:         pod.Name,
		ResourceId:    resourceId,
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
		clientSets.dataClientFactory.ClientSet(), adminWorkload, pod, &workloadPod)
	shouldUpdateNodes := false
	if id >= 0 {
		if adminWorkload.Status.Pods[id].K8sNodeName != workloadPod.K8sNodeName ||
			adminWorkload.Status.Pods[id].HostIp != workloadPod.HostIp ||
			adminWorkload.Status.Pods[id].Rank != workloadPod.Rank {
			shouldUpdateNodes = true
		}
		adminWorkload.Status.Pods[id] = workloadPod
	} else {
		adminWorkload.Status.Pods = append(adminWorkload.Status.Pods, workloadPod)
		shouldUpdateNodes = true
	}
	if shouldUpdateNodes {
		r.updateWorkloadNodes(adminWorkload, message)
	}
	if commonworkload.IsCICDScalingRunnerSet(adminWorkload) {
		updateCICDScalingRunnerSetPhase(adminWorkload, pod)
	}
	return ctrlruntime.Result{}, r.Status().Update(ctx, adminWorkload)
}

// updateCICDScalingRunnerSetPhase updates the workload phase for CICD scaling runner sets
// based on the phase of its listener pod, since these workloads don't have inherent status.
// Running pods result in WorkloadRunning status, pending pods result in WorkloadPending,
// and all other pod phases result in WorkloadNotReady status.
func updateCICDScalingRunnerSetPhase(adminWorkload *v1.Workload, pod *corev1.Pod) {
	val, ok := pod.Labels[appComponent]
	if !ok || val != scaleSetListener {
		return
	}
	switch pod.Status.Phase {
	case corev1.PodRunning:
		adminWorkload.Status.Message = ""
		adminWorkload.Status.Phase = v1.WorkloadRunning
	case corev1.PodPending:
		adminWorkload.Status.Phase = v1.WorkloadPending
	default:
		adminWorkload.Status.Message = ""
		adminWorkload.Status.Phase = v1.WorkloadNotReady
	}
}

// updateWorkloadNodes updates the node information for a workload.
// Collects node assignments from workload pods.
func (r *SyncerReconciler) updateWorkloadNodes(adminWorkload *v1.Workload, message *resourceMessage) {
	sortWorkloadPods(adminWorkload)

	nodeNames := make([]string, 0, len(adminWorkload.Status.Pods))
	ranks := make([]string, 0, len(adminWorkload.Status.Pods))
	nodeNameSet := sets.NewSet()
	for _, p := range adminWorkload.Status.Pods {
		if !nodeNameSet.Has(p.K8sNodeName) {
			nodeNames = append(nodeNames, p.K8sNodeName)
			ranks = append(ranks, p.Rank)
			nodeNameSet.Insert(p.K8sNodeName)
		}
	}
	if len(adminWorkload.Status.Nodes) < message.dispatchCount {
		adminWorkload.Status.Nodes = append(adminWorkload.Status.Nodes, nodeNames)
		adminWorkload.Status.Ranks = append(adminWorkload.Status.Ranks, ranks)
	} else if message.dispatchCount > 0 {
		adminWorkload.Status.Nodes[message.dispatchCount-1] = nodeNames
		adminWorkload.Status.Ranks[message.dispatchCount-1] = ranks
	}
}

// getMainContainerRank retrieves the rank value from the main container's environment variables.
// Used for distributed training workloads to identify process rank.
func getMainContainerRank(adminWorkload *v1.Workload, pod *corev1.Pod) string {
	mainContainerName := getMainContainerName(adminWorkload, pod)
	for _, container := range pod.Spec.Containers {
		if mainContainerName != "" && container.Name != mainContainerName {
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

// removeWorkloadPod removes a pod entry from the workload status.
// Called when a pod is deleted to clean up the workload's pod list.
func (r *SyncerReconciler) removeWorkloadPod(ctx context.Context, message *resourceMessage) error {
	if message.workloadId == "" {
		return nil
	}
	adminWorkload, err := r.getAdminWorkload(ctx, message.workloadId)
	if adminWorkload == nil || adminWorkload.IsEnd() {
		return err
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
	if commonworkload.IsApplication(adminWorkload) {
		r.updateWorkloadNodes(adminWorkload, message)
	}
	if err = r.Status().Update(ctx, adminWorkload); err != nil {
		klog.ErrorS(err, "failed to update workload status", "name", adminWorkload.Name)
		return err
	}
	return nil
}

// buildPodTerminatedInfo constructs termination information for a pod.
// Extracts container termination details and finished time for completed pods.
func buildPodTerminatedInfo(ctx context.Context,
	clientSet kubernetes.Interface, adminWorkload *v1.Workload, pod *corev1.Pod, workloadPod *v1.WorkloadPod) {
	if pod.Status.Phase == corev1.PodFailed {
		if pod.Status.Reason != "" {
			workloadPod.FailedMessage += pod.Status.Reason
		}
		if pod.Status.Message != "" {
			if workloadPod.FailedMessage != "" {
				workloadPod.FailedMessage += ", "
			}
			workloadPod.FailedMessage += pod.Status.Message
		}
	} else if pod.Status.Phase != corev1.PodSucceeded {
		return
	}

	var finishedTime *metav1.Time
	mainContainerName := getMainContainerName(adminWorkload, pod)
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
		if mainContainerName == "" {
			mainContainerName = c.Name
		}
		if commonworkload.IsOpsJob(adminWorkload) && c.Name == mainContainerName {
			message := getPodLog(ctx, clientSet, pod, mainContainerName)
			c.Message = message
		}
		workloadPod.Containers = append(workloadPod.Containers, c)
	}

	if finishedTime != nil && !finishedTime.IsZero() {
		workloadPod.EndTime = timeutil.FormatRFC3339(finishedTime.Time)
	}
}

// getPodLog retrieves and filters logs from a pod's main container.
// Extracts lines containing ERROR or SUCCESS markers for OpsJob workloads.
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

// sortWorkloadPods sorts workload pods by host IP and pod ID.
// Ensures consistent ordering of pods for node assignment tracking.
func sortWorkloadPods(adminWorkload *v1.Workload) {
	sort.Slice(adminWorkload.Status.Pods, func(i, j int) bool {
		if adminWorkload.Status.Pods[i].HostIp == adminWorkload.Status.Pods[j].HostIp {
			return adminWorkload.Status.Pods[i].PodId < adminWorkload.Status.Pods[j].PodId
		}
		return netutil.ConvertIpToInt(adminWorkload.Status.Pods[i].HostIp) <
			netutil.ConvertIpToInt(adminWorkload.Status.Pods[j].HostIp)
	})
}

// getMainContainerName get main container name of pod
func getMainContainerName(adminWorkload *v1.Workload, pod *corev1.Pod) string {
	mainContainerName := v1.GetMainContainer(pod)
	if mainContainerName == "" {
		// TODO: Keep old logic for compatibility; remove it later.
		mainContainerName = v1.GetMainContainer(adminWorkload)
	}
	return mainContainerName
}
