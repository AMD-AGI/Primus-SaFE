/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package syncer

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	apitypes "k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog/v2"
	ctrlruntime "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	commonconfig "github.com/AMD-AIG-AIMA/SAFE/common/pkg/config"
	commonfaults "github.com/AMD-AIG-AIMA/SAFE/common/pkg/faults"
	commonworkload "github.com/AMD-AIG-AIMA/SAFE/common/pkg/workload"
	jobutils "github.com/AMD-AIG-AIMA/SAFE/job-manager/pkg/utils"
	jsonutils "github.com/AMD-AIG-AIMA/SAFE/utils/pkg/json"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/netutil"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/sets"
	sliceutil "github.com/AMD-AIG-AIMA/SAFE/utils/pkg/slice"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/timeutil"
	unstructuredutils "github.com/AMD-AIG-AIMA/SAFE/utils/pkg/unstructured"
)

const (
	ForceDeleteDelaySeconds = 60
	MaxRayJobWaitTime       = 3600
	LogTailLines            = 1000

	appComponent     = "app.kubernetes.io/component"
	scaleSetListener = "runner-scale-set-listener"
	monarchMeshLabel = "monarch.pytorch.org/mesh-name"
)

// handlePod processes Pod resource events (add, update, delete).
// Manages the synchronization of pod status between data plane and admin plane.
func (r *SyncerReconciler) handlePod(ctx context.Context,
	message *resourceMessage, clusterClientSets *ClusterClientSets) (ctrlruntime.Result, error) {
	informer, err := clusterClientSets.GetResourceInformer(ctx, message.gvk)
	if err != nil {
		return ctrlruntime.Result{}, err
	}
	obj, err := jobutils.GetObjectByInformer(informer, message.name, message.namespace)
	if err != nil {
		return ctrlruntime.Result{}, err
	}
	if obj != nil && message.uid != "" && obj.GetUID() != message.uid {
		klog.V(4).InfoS("ignore pod event for a replaced object",
			"namespace", message.namespace, "pod", message.name,
			"eventUID", message.uid, "currentUID", obj.GetUID())
		return ctrlruntime.Result{}, nil
	}
	if obj == nil || !obj.GetDeletionTimestamp().IsZero() {
		if obj != nil && message.meshName == "" {
			message.meshName = obj.GetLabels()[monarchMeshLabel]
		}
		if err = r.removeWorkloadPod(ctx, clusterClientSets, message); err != nil {
			return ctrlruntime.Result{}, err
		}
		return r.deletePod(ctx, obj, clusterClientSets)
	}
	return r.updateAdminWorkloadByPod(ctx, clusterClientSets, obj, message)
}

// deletePod forcefully deletes a pod from the data plane.
// Implements a delayed force deletion strategy to avoid premature deletion.
func (r *SyncerReconciler) deletePod(ctx context.Context,
	obj *unstructured.Unstructured, clusterClientSets *ClusterClientSets) (ctrlruntime.Result, error) {
	if obj == nil || obj.GetDeletionTimestamp().IsZero() {
		return ctrlruntime.Result{}, nil
	}
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

// updateAdminWorkloadByPod updates the workload status based on pod information.
// Synchronizes pod details like phase, node assignment, and container status.
func (r *SyncerReconciler) updateAdminWorkloadByPod(ctx context.Context, clientSets *ClusterClientSets,
	obj *unstructured.Unstructured, message *resourceMessage) (ctrlruntime.Result, error) {
	pod := convertPodFromUnstructured(obj)
	if pod == nil {
		return ctrlruntime.Result{}, nil
	}

	unlock, err := r.lockPodStatusForPod(ctx, clientSets, pod, message)
	if err != nil {
		return ctrlruntime.Result{}, err
	}
	defer unlock()

	adminWorkload, err := r.getAdminWorkloadAndSyncPod(ctx, clientSets, pod, message)
	if adminWorkload == nil || err != nil {
		return ctrlruntime.Result{}, err
	}
	if !v1.IsWorkloadDispatched(adminWorkload) {
		return ctrlruntime.Result{RequeueAfter: time.Second}, nil
	}

	// Submitter pod is handled as an independent module, no further updates needed
	if ok, err := r.handleRaySubmitterTimeout(ctx, adminWorkload, pod); err != nil || ok {
		return ctrlruntime.Result{}, err
	}

	k8sNode, err := r.getK8sNode(ctx, clientSets, pod.Spec.NodeName)
	if err != nil {
		return ctrlruntime.Result{}, err
	}

	podInfo, oldPodPhase, isUpdated := r.updateWorkloadNodeAndPods(ctx, clientSets, adminWorkload, pod, k8sNode)
	if !isUpdated {
		// This pod's detail is already current, but the aggregate built from the
		// whole set may still be behind a lost patch race, and no other writer
		// rebuilds it. Repair it alone rather than falling into the full status
		// write, which would rewrite the DB from this snapshot.
		_, err = r.repairNodeUsage(ctx, adminWorkload)
		return ctrlruntime.Result{}, err
	}

	if isAllPodsAssigned(adminWorkload) {
		if err = r.createStickyNodeFaults(ctx, adminWorkload); err != nil {
			return ctrlruntime.Result{}, err
		}
	}
	// CICD scaling runner sets are the only pod-driven case that owns the
	// workload phase; carry it in the field patch so it is not dropped.
	var extraStatusFields map[string]any
	if commonworkload.IsCICDScalingRunnerSet(adminWorkload) {
		updateCICDScalingRunnerSetPhase(adminWorkload, pod)
		extraStatusFields = map[string]any{"phase": adminWorkload.Status.Phase}
	}
	if err = r.patchWorkloadPodStatus(ctx, adminWorkload, extraStatusFields); err != nil {
		klog.ErrorS(err, "failed to update admin workload status", "name", adminWorkload.Name)
		return ctrlruntime.Result{}, err
	}
	if oldPodPhase != podInfo.Phase {
		if commonworkload.IsRayJob(adminWorkload) && podInfo.ResourceId == 0 && v1.IsPodTerminated(&podInfo) {
			return ctrlruntime.Result{RequeueAfter: MaxRayJobWaitTime * time.Second}, nil
		}
	}
	return ctrlruntime.Result{}, nil
}

func convertPodFromUnstructured(obj *unstructured.Unstructured) *corev1.Pod {
	pod := &corev1.Pod{}
	err := unstructuredutils.ConvertUnstructuredToObject(obj, pod)
	if err != nil {
		// This error cannot be resolved by retrying, so it is ignored by returning nil.
		klog.ErrorS(err, "failed to convert object to pod", "data", obj)
		return nil
	}
	if pod.Status.Phase == corev1.PodFailed {
		klog.Infof("pod(%s) is failed. reason: %s, message: %s, container: %s",
			pod.Name, pod.Status.Reason, pod.Status.Message, string(jsonutils.MarshalSilently(pod.Status.ContainerStatuses)))
	}
	return pod
}

func (r *SyncerReconciler) lockPodStatusForPod(ctx context.Context, clientSets *ClusterClientSets,
	pod *corev1.Pod, message *resourceMessage) (func(), error) {
	if meshName := pod.GetLabels()[monarchMeshLabel]; meshName != "" {
		message.meshName = meshName
		message.namespace = pod.GetNamespace()
		if v1.GetWorkloadId(pod) == "" {
			meshObj, err := r.getMonarchMesh(ctx, clientSets, meshName, pod.GetNamespace())
			if err != nil {
				return nil, err
			}
			message.workloadId = v1.GetWorkloadId(meshObj)
			if err = r.persistMeshPodOwnership(ctx, clientSets, pod, message.workloadId,
				v1.GetLabel(meshObj, v1.GroupIdLabel),
				v1.GetAnnotation(meshObj, v1.ResourceIdAnnotation),
				v1.GetAnnotation(meshObj, v1.MainContainerAnnotation)); err != nil {
				return nil, err
			}
		} else {
			message.workloadId = v1.GetWorkloadId(pod)
		}
	}
	id, err := r.resolvePodWorkloadID(ctx, clientSets, message)
	if err != nil {
		return nil, err
	}
	return r.lockPodStatus(id), nil
}

func (r *SyncerReconciler) resolvePodWorkloadID(ctx context.Context, clientSets *ClusterClientSets,
	message *resourceMessage) (string, error) {
	if message.workloadId != "" {
		return message.workloadId, nil
	}
	if message.meshName == "" || clientSets == nil {
		return "", nil
	}
	meshObj, err := r.getMonarchMesh(ctx, clientSets, message.meshName, message.namespace)
	if err != nil {
		return "", err
	}
	return v1.GetWorkloadId(meshObj), nil
}

func (r *SyncerReconciler) getAdminWorkloadAndSyncPod(ctx context.Context,
	clientSets *ClusterClientSets, pod *corev1.Pod, message *resourceMessage) (*v1.Workload, error) {
	var adminWorkload *v1.Workload
	var err error
	if meshName := pod.GetLabels()[monarchMeshLabel]; meshName != "" {
		workloadID := v1.GetWorkloadId(pod)
		if workloadID == "" {
			var meshObj *unstructured.Unstructured
			meshObj, err = r.getMonarchMesh(ctx, clientSets, meshName, pod.GetNamespace())
			if err != nil {
				return nil, err
			}
			workloadID = v1.GetWorkloadId(meshObj)
			groupID := v1.GetLabel(meshObj, v1.GroupIdLabel)
			resourceID := v1.GetAnnotation(meshObj, v1.ResourceIdAnnotation)
			mainContainer := v1.GetAnnotation(meshObj, v1.MainContainerAnnotation)
			if err = r.persistMeshPodOwnership(ctx, clientSets, pod,
				workloadID, groupID, resourceID, mainContainer); err != nil {
				return nil, err
			}
		}
		adminWorkload, err = r.getAdminWorkload(ctx, workloadID)
	} else {
		adminWorkload, err = r.getAdminWorkload(ctx, message.workloadId)
	}
	if err != nil || adminWorkload == nil {
		return nil, err
	}
	v1.SetLabel(adminWorkload, v1.WorkloadDispatchCntLabel, strconv.Itoa(message.dispatchCount))
	return adminWorkload, nil
}

// persistMeshPodOwnership stores stable ownership on a generated mesh pod.
func (r *SyncerReconciler) persistMeshPodOwnership(ctx context.Context, clientSets *ClusterClientSets,
	pod *corev1.Pod, workloadID, groupID, resourceID, mainContainer string) error {
	labelsPatch := map[string]any{v1.WorkloadIdLabel: workloadID}
	annotationsPatch := map[string]any{}
	if groupID != "" {
		labelsPatch[v1.GroupIdLabel] = groupID
	}
	if resourceID != "" {
		annotationsPatch[v1.ResourceIdAnnotation] = resourceID
	}
	if mainContainer != "" {
		annotationsPatch[v1.MainContainerAnnotation] = mainContainer
	}
	metadataPatch := map[string]any{"labels": labelsPatch}
	if pod.ResourceVersion != "" {
		// Guard against stamping ownership from an old event onto a same-name
		// replacement pod.
		metadataPatch["resourceVersion"] = pod.ResourceVersion
	}
	patch := map[string]any{"metadata": metadataPatch}
	if len(annotationsPatch) > 0 {
		metadataPatch["annotations"] = annotationsPatch
	}
	_, err := clientSets.dataClientFactory.ClientSet().CoreV1().Pods(pod.Namespace).Patch(
		ctx, pod.Name, apitypes.MergePatchType, jsonutils.MarshalSilently(patch), metav1.PatchOptions{})
	if err != nil && !apierrors.IsNotFound(err) {
		return err
	}
	v1.SetLabel(pod, v1.WorkloadIdLabel, workloadID)
	if groupID != "" {
		v1.SetLabel(pod, v1.GroupIdLabel, groupID)
	}
	if resourceID != "" {
		v1.SetAnnotation(pod, v1.ResourceIdAnnotation, resourceID)
	}
	if mainContainer != "" {
		v1.SetAnnotation(pod, v1.MainContainerAnnotation, mainContainer)
	}
	return nil
}

func (r *SyncerReconciler) getK8sNode(ctx context.Context, clientSets *ClusterClientSets, nodeName string) (*corev1.Node, error) {
	k8sNode := &corev1.Node{}
	if nodeName == "" {
		return k8sNode, nil
	}
	var err error
	if k8sNode, err = clientSets.dataClientFactory.ClientSet().CoreV1().Nodes().Get(ctx, nodeName, metav1.GetOptions{}); err != nil {
		klog.ErrorS(err, "failed to get k8s node", "name", nodeName)
		return nil, err
	}
	return k8sNode, nil
}

// isNodeUsageStale reports whether the offloaded aggregate in etcd disagrees
// with the per-pod detail the workload carries, which is hydrated from the DB.
// A concurrent status writer can win the patch race and leave the aggregate
// behind, and the pod sync is the only writer that rebuilds it.
//
// The condition mirrors the one patchWorkloadPodStatus writes the aggregate
// under. The offload annotation alone does not imply the aggregate is
// maintained: the webhook stamps it on every workload it creates, while the
// aggregate is only written when the DB is configured. Comparing without the DB
// would report every aggregate as stale, since none is ever published.
func (r *SyncerReconciler) isNodeUsageStale(w *v1.Workload) bool {
	if !commonconfig.IsDBEnable() || r.dbClient == nil || !v1.IsWorkloadStatusOffloadEnabled(w) {
		return false
	}
	return !commonworkload.NodeUsageEquivalent(commonworkload.BuildNodeUsage(w), w.Status.NodeUsage)
}

// repairNodeUsage republishes the offloaded aggregate when it no longer matches
// the pod detail hydrated from the DB, and reports whether it patched.
//
// Only the aggregate is written. The DB already holds the detail this rebuild
// comes from, so it needs no correction, and a full status write would carry
// this snapshot's pod set into DeleteWorkloadPodsNotIn: events are keyed per
// pod, so a worker handling a sibling pod of the same workload runs
// concurrently, and its committed rows are absent from a snapshot taken before
// it wrote.
func (r *SyncerReconciler) repairNodeUsage(ctx context.Context, w *v1.Workload) (bool, error) {
	if !r.isNodeUsageStale(w) {
		return false, nil
	}
	w.Status.NodeUsage = commonworkload.BuildNodeUsage(w)
	if err := jobutils.PatchWorkloadStatusFields(ctx, r.Client, w,
		map[string]any{"nodeUsage": w.Status.NodeUsage}); err != nil {
		return false, err
	}
	klog.Infof("repaired stale node usage of workload %s, nodes: %d",
		w.Name, len(w.Status.NodeUsage))
	return true, nil
}

func (r *SyncerReconciler) updateWorkloadNodeAndPods(ctx context.Context, clientSets *ClusterClientSets,
	adminWorkload *v1.Workload, pod *corev1.Pod, k8sNode *corev1.Node) (v1.WorkloadPod, corev1.PodPhase, bool) {
	id := -1
	for i, p := range adminWorkload.Status.Pods {
		if p.PodId != pod.Name {
			continue
		}
		id = i
		//
		if p.Phase == pod.Status.Phase && p.AdminNodeName == v1.GetNodeId(k8sNode) &&
			p.StartTime != "" && p.HostIp == pod.Status.HostIP {
			// Return early if no critical changes detected
			return v1.WorkloadPod{}, "", false
		}
		break
	}

	podInfo := r.buildWorkloadPodInfo(ctx, clientSets, adminWorkload, pod, k8sNode)
	var oldPhase corev1.PodPhase
	needUpdateNode := false
	if id >= 0 {
		oldPhase = adminWorkload.Status.Pods[id].Phase
		if adminWorkload.Status.Pods[id].AdminNodeName != podInfo.AdminNodeName ||
			adminWorkload.Status.Pods[id].HostIp != podInfo.HostIp ||
			adminWorkload.Status.Pods[id].Rank != podInfo.Rank {
			needUpdateNode = true
		}
		adminWorkload.Status.Pods[id] = podInfo
	} else {
		adminWorkload.Status.Pods = append(adminWorkload.Status.Pods, podInfo)
		needUpdateNode = true
	}
	if commonworkload.IsRayJob(adminWorkload) {
		prunedPods := pruneStaleRayJobPods(adminWorkload.Status.Pods)
		if len(prunedPods) != len(adminWorkload.Status.Pods) {
			adminWorkload.Status.Pods = prunedPods
			needUpdateNode = true
		}
	}
	if needUpdateNode {
		r.updateWorkloadNodes(adminWorkload)
	}
	return podInfo, oldPhase, true

}

func (r *SyncerReconciler) buildWorkloadPodInfo(ctx context.Context, clientSets *ClusterClientSets,
	adminWorkload *v1.Workload, pod *corev1.Pod, k8sNode *corev1.Node) v1.WorkloadPod {
	resourceId, _ := v1.GetResourceId(pod)
	mainContainerName := getMainContainerName(adminWorkload, pod)
	groupId := -1
	if groupIdStr := v1.GetGroupId(pod); groupIdStr != "" {
		var err error
		if groupId, err = strconv.Atoi(groupIdStr); err != nil {
			groupId = -1
		}
	}

	workloadPod := v1.WorkloadPod{
		PodId:         pod.Name,
		ResourceId:    int8(resourceId),
		AdminNodeName: v1.GetNodeId(k8sNode),
		Phase:         pod.Status.Phase,
		HostIp:        pod.Status.HostIP,
		PodIp:         pod.Status.PodIP,
		Rank:          getMainContainerRank(mainContainerName, pod),
		GroupId:       int8(groupId),
	}
	if pod.Status.StartTime != nil && !pod.Status.StartTime.IsZero() {
		workloadPod.StartTime = timeutil.FormatRFC3339(pod.Status.StartTime.Time)
	}
	buildPodTerminatedInfo(ctx, clientSets.dataClientFactory.ClientSet(),
		adminWorkload, pod, &workloadPod, mainContainerName)

	return workloadPod
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
		adminWorkload.Status.Phase = v1.WorkloadRunning
	case corev1.PodPending:
		adminWorkload.Status.Phase = v1.WorkloadPending
	default:
		// The listener pod left Running/Pending; the runner set becomes NotReady.
		// This is the usual precursor to failover/deletion, so log the transition.
		if adminWorkload.Status.Phase != v1.WorkloadNotReady {
			klog.Infof("CICD scaling runner set %s phase -> NotReady (listener pod %s phase: %s)",
				adminWorkload.Name, pod.Name, pod.Status.Phase)
		}
		adminWorkload.Status.Phase = v1.WorkloadNotReady
	}
}

// updateWorkloadNodes updates the node information for a workload.
// Collects node assignments from workload pods.
func (r *SyncerReconciler) updateWorkloadNodes(adminWorkload *v1.Workload) {
	sortWorkloadPods(adminWorkload)

	nodeNames := make([]string, 0, len(adminWorkload.Status.Pods))
	ranks := make([]string, 0, len(adminWorkload.Status.Pods))
	nodeNameSet := sets.NewSet()
	for i := range adminWorkload.Status.Pods {
		if !nodeNameSet.Has(adminWorkload.Status.Pods[i].AdminNodeName) {
			nodeNames = append(nodeNames, adminWorkload.Status.Pods[i].AdminNodeName)
			if !commonworkload.IsTorchFT(adminWorkload) && !commonworkload.IsMonarchJob(adminWorkload) {
				ranks = append(ranks, adminWorkload.Status.Pods[i].Rank)
			}
			nodeNameSet.Insert(adminWorkload.Status.Pods[i].AdminNodeName)
		}
	}
	dispatchCount := v1.GetWorkloadDispatchCnt(adminWorkload)
	if len(adminWorkload.Status.Nodes) < dispatchCount {
		adminWorkload.Status.Nodes = append(adminWorkload.Status.Nodes, nodeNames)
		adminWorkload.Status.Ranks = append(adminWorkload.Status.Ranks, ranks)
	} else if dispatchCount > 0 {
		adminWorkload.Status.Nodes[dispatchCount-1] = nodeNames
		adminWorkload.Status.Ranks[dispatchCount-1] = ranks
	}
}

// getMainContainerRank retrieves the rank value from the main container's environment variables.
// Used for distributed training workloads to identify process rank.
func getMainContainerRank(mainContainerName string, pod *corev1.Pod) string {
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
func (r *SyncerReconciler) removeWorkloadPod(ctx context.Context, clientSets *ClusterClientSets,
	message *resourceMessage) error {
	workloadID, err := r.resolvePodWorkloadID(ctx, clientSets, message)
	if err != nil || workloadID == "" {
		return err
	}
	unlock := r.lockPodStatus(workloadID)
	defer unlock()
	adminWorkload, err := r.getAdminWorkload(ctx, workloadID)
	if adminWorkload == nil {
		return err
	}

	id := indexOfPod(adminWorkload.Status.Pods, message.name)
	if id < 0 || !r.applyPodGone(adminWorkload, id) {
		// A prior attempt may have written the DB (Stopped or dropped the row)
		// and then lost the etcd CAS. Hydration already shows the pod gone, so
		// there is no pod field to patch; the aggregate still needs publishing.
		_, err = r.repairNodeUsage(ctx, adminWorkload)
		return err
	}

	if err = r.patchWorkloadPodStatus(ctx, adminWorkload, nil); err != nil {
		klog.ErrorS(err, "failed to update workload status", "name", adminWorkload.Name)
		return err
	}
	return nil
}

// handleMonarchMesh releases one mesh group's pod records after its pods are gone.
func (r *SyncerReconciler) handleMonarchMesh(ctx context.Context, clientSets *ClusterClientSets,
	message *resourceMessage) (ctrlruntime.Result, error) {
	mesh, err := r.getMonarchMesh(ctx, clientSets, message.name, message.namespace)
	if message.action != ResourceDeleting && message.action != ResourceDel {
		if err != nil {
			return ctrlruntime.Result{}, err
		}
		return ctrlruntime.Result{}, ensureMonarchMeshFinalizer(ctx, clientSets, mesh)
	}
	if err == nil && mesh.GetUID() != message.uid {
		klog.InfoS("skip stale mesh delete event after same-name mesh replacement",
			"namespace", message.namespace, "mesh", message.name,
			"deletedUID", message.uid, "currentUID", mesh.GetUID())
		return ctrlruntime.Result{}, nil
	}
	if err != nil && !apierrors.IsNotFound(err) {
		return ctrlruntime.Result{}, err
	}

	remain, known, err := meshPodsRemain(ctx, clientSets, message.namespace, message.name)
	if err != nil {
		return ctrlruntime.Result{}, err
	}
	if !known || remain {
		return ctrlruntime.Result{RequeueAfter: time.Second}, nil
	}

	groupID, err := strconv.Atoi(message.groupId)
	if err != nil {
		return ctrlruntime.Result{}, fmt.Errorf("invalid MonarchMesh group id %q: %w", message.groupId, err)
	}
	unlock := r.lockPodStatus(message.workloadId)
	defer unlock()
	adminWorkload, err := r.getAdminWorkload(ctx, message.workloadId)
	if err != nil {
		return ctrlruntime.Result{}, err
	}
	if adminWorkload == nil {
		return ctrlruntime.Result{}, removeMonarchMeshFinalizer(ctx, clientSets, mesh)
	}
	if !commonworkload.IsMonarchJob(adminWorkload) {
		return ctrlruntime.Result{}, fmt.Errorf("workload %s is not a MonarchJob", adminWorkload.Name)
	}

	changed := false
	for i := range adminWorkload.Status.Pods {
		pod := &adminWorkload.Status.Pods[i]
		if pod.ResourceId <= 0 || int(pod.GroupId) != groupID {
			continue
		}
		if r.applyPodGone(adminWorkload, i) {
			changed = true
		}
	}
	if !changed {
		_, err = r.repairNodeUsage(ctx, adminWorkload)
	} else {
		err = r.patchWorkloadPodStatus(ctx, adminWorkload, nil)
	}
	if err != nil {
		return ctrlruntime.Result{}, err
	}
	return ctrlruntime.Result{}, removeMonarchMeshFinalizer(ctx, clientSets, mesh)
}

// ensureMonarchMeshFinalizer keeps a mesh readable until pod status cleanup.
func ensureMonarchMeshFinalizer(ctx context.Context, clientSets *ClusterClientSets,
	mesh *unstructured.Unstructured) error {
	if mesh == nil || controllerutil.ContainsFinalizer(mesh, v1.MonarchMeshFinalizer) {
		return nil
	}
	finalizers := append([]string(nil), mesh.GetFinalizers()...)
	finalizers = append(finalizers, v1.MonarchMeshFinalizer)
	return patchMonarchMeshFinalizers(ctx, clientSets, mesh, finalizers)
}

// removeMonarchMeshFinalizer allows a mesh to disappear after cleanup.
func removeMonarchMeshFinalizer(ctx context.Context, clientSets *ClusterClientSets,
	mesh *unstructured.Unstructured) error {
	if mesh == nil || !controllerutil.ContainsFinalizer(mesh, v1.MonarchMeshFinalizer) {
		return nil
	}
	finalizers := make([]string, 0, len(mesh.GetFinalizers())-1)
	for _, finalizer := range mesh.GetFinalizers() {
		if finalizer != v1.MonarchMeshFinalizer {
			finalizers = append(finalizers, finalizer)
		}
	}
	return patchMonarchMeshFinalizers(ctx, clientSets, mesh, finalizers)
}

func patchMonarchMeshFinalizers(ctx context.Context, clientSets *ClusterClientSets,
	mesh *unstructured.Unstructured, finalizers []string) error {
	metadata := map[string]any{"finalizers": finalizers}
	if mesh.GetResourceVersion() != "" {
		metadata["resourceVersion"] = mesh.GetResourceVersion()
	}
	return jobutils.PatchObject(ctx, clientSets.ClientFactory(), mesh,
		jsonutils.MarshalSilently(map[string]any{"metadata": metadata}))
}

// meshPodsRemain checks the synced pod cache for pods owned by a mesh name.
func meshPodsRemain(ctx context.Context, clientSets *ClusterClientSets,
	namespace, meshName string) (bool, bool, error) {
	informer, err := clientSets.GetResourceInformer(ctx, podResourceGVK)
	if err != nil || !informer.Informer().HasSynced() {
		return false, false, nil
	}
	selector := labels.SelectorFromSet(labels.Set{monarchMeshLabel: meshName})
	pods, err := informer.Lister().ByNamespace(namespace).List(selector)
	if err != nil {
		return false, false, err
	}
	return len(pods) > 0, true, nil
}

// applyPodGone records in the workload status that one of its pods no longer
// exists, and reports whether the status changed.
//
// Shared by the delete-event path and reconcileVanishedPods so both treat a
// vanished pod the same way.
func (r *SyncerReconciler) applyPodGone(adminWorkload *v1.Workload, id int) bool {
	// CICD keeps no per-pod history either, but is grouped here rather than in
	// IsApplication, which describes the create-once + sync update dispatch
	// lifecycle a runner set does not follow.
	if commonworkload.IsApplication(adminWorkload) || commonworkload.IsCICD(adminWorkload) {
		// Drop the pod entry and refresh node assignment so the status tracks the
		// current replica set.
		adminWorkload.Status.Pods = append(adminWorkload.Status.Pods[:id], adminWorkload.Status.Pods[id+1:]...)
		r.updateWorkloadNodes(adminWorkload)
		return true
	}
	// Other kinds keep the entry as history and only flip a still-live pod to
	// Stopped; a terminal phase is left as is, and IsPodTerminated counts Stopped
	// among them.
	p := &adminWorkload.Status.Pods[id]
	if v1.IsPodTerminated(p) {
		return false
	}
	p.Phase = corev1.PodPhase(v1.WorkloadStopped)
	return true
}

// podResourceGVK identifies the pod informer whose cache says which pods exist.
var podResourceGVK = schema.GroupVersionKind{Version: "v1", Kind: common.PodKind}

// vanishedPodGracePeriod is the minimum age of a pod record before it may be
// released.
const vanishedPodGracePeriod = 5 * time.Minute

// reconcileVanishedPods releases the pod records of a dispatched workload whose
// pods no longer exist, so they stop counting toward the workspace's resource
// usage. Existence is read from the pod informer's cache rather than the API,
// except for a mesh workload, whose pods are attributed through a live Get of the
// mesh objects their records name.
//
// Runs at most once per workload per process: within one process the delete events
// that release a record are reliable, so the drift it repairs can only be left
// behind by a process that ended.
func (r *SyncerReconciler) reconcileVanishedPods(ctx context.Context, clientSets *ClusterClientSets,
	adminWorkload *v1.Workload, message *resourceMessage) error {
	if adminWorkload == nil || len(adminWorkload.Status.Pods) == 0 {
		return nil
	}
	// An ended workload's records no longer count toward usage, and teardown deletes
	// its pods on purpose, so neither is a case for releasing records. Dropping the
	// entry keeps the map to the unfinished set, and gives the round that follows a
	// re-schedule -- which tears the old objects down the same way -- its own pass.
	if adminWorkload.IsEnd() || message.action == ResourceDel || message.action == ResourceDeleting {
		r.vanishedPodsChecked.Delete(adminWorkload.Name)
		return nil
	}
	// Dispatch is a precondition rather than an assumption about the caller: its
	// timestamp is what ages a record whose pod never started, and reSchedule drops
	// the annotation while it retries. Dropping the entry gives the round that
	// follows a re-schedule its own pass.
	if !v1.IsWorkloadDispatched(adminWorkload) {
		r.vanishedPodsChecked.Delete(adminWorkload.Name)
		return nil
	}
	if _, done := r.vanishedPodsChecked.LoadOrStore(adminWorkload.Name, struct{}{}); done {
		return nil
	}

	// Re-read rather than work from the caller's copy: this event may have written the
	// status through a deep copy, leaving that copy a version behind. Patching from it
	// would rewrite the database from a stale pod list -- writeWorkloadStatusToDB ends
	// with DeleteWorkloadPodsNotIn -- and then have the guarded etcd patch rejected, so
	// the NodeUsage aggregate the scheduler reads would never land. The Get also
	// re-hydrates the pod list and gives IsEnd a current phase.
	// Hold the name separately: the short declaration below assigns to the parameter
	// instead of shadowing it, and getAdminWorkload yields nil both on a failed Get and
	// on a workload that no longer exists.
	name := adminWorkload.Name
	fresh, err := r.getAdminWorkload(ctx, name)
	if err != nil || fresh == nil || len(fresh.Status.Pods) == 0 {
		r.vanishedPodsChecked.Delete(name)
		return err
	}
	adminWorkload = fresh
	if adminWorkload.IsEnd() {
		r.vanishedPodsChecked.Delete(adminWorkload.Name)
		return nil
	}

	live, ok := r.livePodNames(ctx, clientSets, adminWorkload)
	if !ok {
		// Unknown answer: nothing may be concluded from a record's absence. Re-armed
		// so the next event retries.
		r.vanishedPodsChecked.Delete(adminWorkload.Name)
		return nil
	}

	gone := vanishedPodIds(adminWorkload, live)
	if len(gone) == 0 {
		return nil
	}

	released := 0
	for _, podId := range gone {
		// Resolved per pod: applyPodGone removes entries for some workload kinds,
		// which invalidates any index taken before it ran.
		id := indexOfPod(adminWorkload.Status.Pods, podId)
		if id < 0 {
			continue
		}
		if r.applyPodGone(adminWorkload, id) {
			released++
		}
	}
	if released == 0 {
		return nil
	}
	// The count is the signal that pod delete events are being lost, so it counts the
	// records this pass actually changed rather than the candidates it considered.
	klog.Infof("released %d vanished pod record(s) of workload %s, live pods: %d",
		released, adminWorkload.Name, len(live))
	if err := r.patchWorkloadPodStatus(ctx, adminWorkload, nil); err != nil {
		// A conflict is expected when this event also wrote the status, since the
		// copy in hand is then a version behind. Logged as information, because the
		// re-armed entry means the next event retries with a fresh copy.
		klog.V(2).Infof("deferred releasing vanished pod records of workload %s: %v",
			adminWorkload.Name, err)
		r.vanishedPodsChecked.Delete(adminWorkload.Name)
		return err
	}
	return nil
}

// livePodNames returns the names of the workload's pods that currently exist, and
// whether that could be established.
//
// False means the answer is unknown, and must never be read as "no pods exist": an
// unsynced cache reports empty rather than reporting nothing.
func (r *SyncerReconciler) livePodNames(ctx context.Context, clientSets *ClusterClientSets,
	adminWorkload *v1.Workload) (map[string]struct{}, bool) {
	if clientSets == nil {
		return nil, false
	}

	if clusterId := v1.GetClusterId(adminWorkload); clusterId != clientSets.name {
		klog.Errorf("workload %s belongs to cluster %q, not %q: not comparing its pods",
			adminWorkload.Name, clusterId, clientSets.name)
		return nil, false
	}
	informer, err := clientSets.GetResourceInformer(ctx, podResourceGVK)
	if err != nil {
		klog.V(2).Infof("no pod informer to reconcile workload %s against: %v", adminWorkload.Name, err)
		return nil, false
	}
	if !informer.Informer().HasSynced() {
		return nil, false
	}
	// Selected by the workload id label, and across all namespaces. Both are load
	// bearing and easy to undo by accident:
	//
	// A pod carries primus-safe.workload.id = the root workload id, and its own
	// workload's name under primus-safe.k8s.object.id (buildPodLabels ->
	// buildObjectLabels -> getRootWorkloadId). Pod records are attached to whichever
	// workload GetWorkloadId names, which is that same label, so the owner of a
	// record and this selector are the same value by construction. Selecting on
	// k8s.object.id instead would release a child workload's records while its pods
	// run.
	//
	// The namespace is not the workspace either: a CICD runner set owns the ARC
	// listener pod, which runs in the controller's namespace.
	objs, err := informer.Lister().List(
		labels.SelectorFromSet(labels.Set{v1.WorkloadIdLabel: adminWorkload.Name}))
	if err != nil {
		klog.ErrorS(err, "failed to list pods from informer cache", "name", adminWorkload.Name)
		return nil, false
	}
	live := make(map[string]struct{}, len(objs))
	if !addPodNames(objs, live, adminWorkload.Name) {
		return nil, false
	}
	// A mesh pod gets the workload label after its first successful sync. Pods not
	// yet observed by this process still need ownership resolved from the mesh.
	if commonworkload.IsMonarchJob(adminWorkload) &&
		!r.addMeshPodNames(ctx, clientSets, informer.Lister(), adminWorkload, live) {
		return nil, false
	}
	return live, true
}

// addPodNames adds the names of the given cache objects to live, and reports whether
// all of them could be read. An object that cannot be read leaves the set incomplete,
// and an incomplete set reads as a vanished pod, so it is not a partial answer.
func addPodNames(objs []runtime.Object, live map[string]struct{}, workloadName string) bool {
	for _, obj := range objs {
		pod, ok := obj.(*unstructured.Unstructured)
		if !ok {
			klog.Errorf("unexpected object type %T in the pod informer cache of workload %s",
				obj, workloadName)
			return false
		}
		live[pod.GetName()] = struct{}{}
	}
	return true
}

// addMeshPodNames adds the workload's Monarch mesh pods to live, and reports whether
// their ownership could be established.
//
// Persisted workload labels are authoritative for one pod generation. Unlabeled
// pods are resolved from their live mesh. Only a pod that a record names is
// resolved, and each mesh is read once.
func (r *SyncerReconciler) addMeshPodNames(ctx context.Context, clientSets *ClusterClientSets,
	lister cache.GenericLister, adminWorkload *v1.Workload, live map[string]struct{}) bool {
	selector, err := labels.Parse(monarchMeshLabel)
	if err != nil {
		return false
	}
	objs, err := lister.List(selector)
	if err != nil {
		klog.ErrorS(err, "failed to list mesh pods from informer cache", "name", adminWorkload.Name)
		return false
	}
	recorded := make(map[string]struct{}, len(adminWorkload.Status.Pods))
	for i := range adminWorkload.Status.Pods {
		recorded[adminWorkload.Status.Pods[i].PodId] = struct{}{}
	}
	ownedByWorkload := make(map[string]bool, len(objs))
	for _, obj := range objs {
		pod, ok := obj.(*unstructured.Unstructured)
		if !ok {
			klog.Errorf("unexpected object type %T in the pod informer cache of workload %s",
				obj, adminWorkload.Name)
			return false
		}
		if _, hasRecord := recorded[pod.GetName()]; !hasRecord {
			continue
		}
		if workloadID := v1.GetWorkloadId(pod); workloadID != "" {
			if workloadID == adminWorkload.Name {
				live[pod.GetName()] = struct{}{}
			}
			continue
		}
		meshName := pod.GetLabels()[monarchMeshLabel]
		meshKey := pod.GetNamespace() + "/" + meshName
		owned, resolved := ownedByWorkload[meshKey]
		if !resolved {
			meshObj, err := r.getMonarchMesh(ctx, clientSets, meshName, pod.GetNamespace())
			if err != nil || meshObj == nil {
				// A mesh that cannot be read may be this workload's own.
				klog.V(2).Infof("cannot resolve mesh %s while reconciling workload %s: %v",
					meshKey, adminWorkload.Name, err)
				return false
			}
			owned = v1.GetWorkloadId(meshObj) == adminWorkload.Name
			ownedByWorkload[meshKey] = owned
		}
		if owned {
			live[pod.GetName()] = struct{}{}
		}
	}
	return true
}

// vanishedPodIds returns the ids of the non-terminal pod records whose pod is not
// among the live ones and whose age is past the grace period.
//
// Age comes from the record's own start time, or from the workload's dispatch when
// the pod never started. The grace period covers a record hydrated from the
// database, which may describe a pod this process has not observed yet.
func vanishedPodIds(adminWorkload *v1.Workload, live map[string]struct{}) []string {
	now := time.Now().UTC()
	cutoff := now.Add(-vanishedPodGracePeriod)
	dispatchedBeforeCutoff := false
	if dispatchedAt, err := timeutil.CvtStrToRFC3339Milli(
		v1.GetAnnotation(adminWorkload, v1.WorkloadDispatchedAnnotation)); err == nil {
		dispatchedBeforeCutoff = dispatchedAt.Before(cutoff)
	}

	gone := make([]string, 0)
	for i := range adminWorkload.Status.Pods {
		p := &adminWorkload.Status.Pods[i]
		if p.PodId == "" || v1.IsPodTerminated(p) {
			continue
		}
		if _, ok := live[p.PodId]; ok {
			continue
		}
		// A start time is written by FormatRFC3339, which drops the zone, while
		// metav1.Time has already converted the instant to local -- so east of UTC the
		// record parses as starting in the future. Such a time says nothing about age,
		// and the workload's dispatch, written in UTC, is used instead.
		startedAt, err := timeutil.CvtStrToRFC3339Milli(p.StartTime)
		usable := err == nil && !startedAt.After(now)
		switch {
		case usable && startedAt.After(cutoff):
			continue
		case !usable && !dispatchedBeforeCutoff:
			continue
		}
		gone = append(gone, p.PodId)
	}
	return gone
}

// indexOfPod locates a pod record by id, or -1.
func indexOfPod(pods []v1.WorkloadPod, podId string) int {
	for i := range pods {
		if pods[i].PodId == podId {
			return i
		}
	}
	return -1
}

// createReservedFaults creates fault to reserve nodes for the workload
// This ensures that after failover, the workload can still use the same nodes
func (r *SyncerReconciler) createStickyNodeFaults(ctx context.Context, adminWorkload *v1.Workload) error {
	count := v1.GetWorkloadDispatchCnt(adminWorkload)
	if !v1.IsRetryingOnOriginal(adminWorkload) || count <= 0 || shouldWorkloadStopRetry(adminWorkload, count) {
		return nil
	}
	var toAddNodes, toDelNodes []string
	if count >= 2 {
		toAddNodes = sliceutil.Difference(adminWorkload.Status.Nodes[count-1], adminWorkload.Status.Nodes[count-2])
		toDelNodes = sliceutil.Difference(adminWorkload.Status.Nodes[count-2], adminWorkload.Status.Nodes[count-1])
	} else {
		toAddNodes = adminWorkload.Status.Nodes[count-1]
	}

	for _, n := range toAddNodes {
		fault, err := generateStickyFault(adminWorkload, n, r.Client.Scheme())
		if err != nil {
			return err
		}
		if fault == nil {
			continue
		}
		if err = r.Create(ctx, fault); err != nil && !apierrors.IsAlreadyExists(err) {
			klog.ErrorS(err, "failed to create sticky node fault", "name", fault.Name)
			return err
		}
	}
	for _, n := range toDelNodes {
		faultId := commonfaults.GenerateFaultId(n, v1.StickyNodesMonitorId)
		if err := r.Delete(ctx, &v1.Fault{ObjectMeta: metav1.ObjectMeta{Name: faultId}}); err != nil && !apierrors.IsNotFound(err) {
			klog.ErrorS(err, "failed to delete sticky node fault", "name", faultId)
			return err
		}
	}
	klog.Infof("Create sticky nodes faults for the workload %s.", adminWorkload.Name)
	return nil
}

func (r *SyncerReconciler) handleRaySubmitterTimeout(ctx context.Context, adminWorkload *v1.Workload, pod *corev1.Pod) (bool, error) {
	if !commonworkload.IsRayJob(adminWorkload) {
		return false, nil
	}
	id := -1
	for i, p := range adminWorkload.Status.Pods {
		if p.PodId != pod.Name {
			continue
		}
		id = i
		break
	}
	if id < 0 || adminWorkload.Status.Pods[id].ResourceId > 0 || adminWorkload.Status.Pods[id].EndTime == "" {
		return false, nil
	}
	endTime, err := time.Parse(timeutil.TimeRFC3339Short, adminWorkload.Status.Pods[id].EndTime)
	if err != nil {
		return false, nil
	}
	if time.Since(endTime) < MaxRayJobWaitTime*time.Second {
		return false, nil
	}
	return true, jobutils.SetWorkloadFailed(ctx, r.Client, adminWorkload, "rayJob submitter has timed out")
}

func (r *SyncerReconciler) getMonarchMesh(ctx context.Context,
	clusterClientSets *ClusterClientSets, name, namespace string) (*unstructured.Unstructured, error) {
	meshGvk := commonworkload.MonarchMeshWorkloadGVK()
	rt, err := commonworkload.GetResourceTemplateByGVK(ctx, r.Client, meshGvk)
	if err != nil {
		return nil, err
	}
	meshObject, err := jobutils.GetObject(ctx, clusterClientSets.ClientFactory(), name, namespace, rt.ToSchemaGVK())
	if err != nil {
		return nil, err
	}
	return meshObject, nil
}

func generateStickyFault(adminWorkload *v1.Workload,
	adminNodeId string, scheme *runtime.Scheme) (*v1.Fault, error) {
	if adminNodeId == "" {
		return nil, nil
	}
	fault := &v1.Fault{
		ObjectMeta: metav1.ObjectMeta{
			Name: commonfaults.GenerateFaultId(adminNodeId, v1.StickyNodesMonitorId),
			Labels: map[string]string{
				v1.WorkloadIdLabel: adminWorkload.Name,
				v1.NodeIdLabel:     adminNodeId,
			},
		},
		Spec: v1.FaultSpec{
			MonitorId: v1.StickyNodesMonitorId,
			Message:   fmt.Sprintf("sticky node for workload %s", adminWorkload.Name),
			Action:    common.TaintAction,
			Node: &v1.FaultNode{
				ClusterName: v1.GetClusterId(adminWorkload),
				AdminName:   adminNodeId,
			},
		},
	}
	err := controllerutil.SetControllerReference(adminWorkload, fault, scheme)
	if err != nil {
		return nil, err
	}
	return fault, err
}

// buildPodTerminatedInfo constructs termination information for a pod.
// Extracts container termination details and finished time for completed pods.
func buildPodTerminatedInfo(ctx context.Context, clientSet kubernetes.Interface,
	adminWorkload *v1.Workload, pod *corev1.Pod, workloadPod *v1.WorkloadPod, mainContainerName string) {
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
		// The preflight results are handled by job self-parse.
		if commonworkload.IsOpsJob(adminWorkload) && c.Name == mainContainerName &&
			v1.GetOpsJobType(adminWorkload) != string(v1.OpsJobPreflightType) {
			message := getPodLog(ctx, clientSet, pod, mainContainerName)
			c.Message = message
		}
		workloadPod.Containers = append(workloadPod.Containers, c)
	}

	if finishedTime != nil && !finishedTime.IsZero() {
		workloadPod.EndTime = timeutil.FormatRFC3339(finishedTime.Time)
	} else {
		workloadPod.EndTime = timeutil.FormatRFC3339(time.Now())
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

// sortWorkloadPods sorts workload pods by host IP and pod ID to maintain consistent ordering.
// For TorchFT workloads, pods are first sorted by GroupId, then by host IP and pod ID within the same group.
// For RayJob workloads, pods are sorted by role: submitter (no -head-/-worker-) first, then head, then worker (by name).
// For regular workloads, pods are sorted directly by host IP and pod ID.
// This ensures consistent ordering of pods for node assignment tracking.
func sortWorkloadPods(adminWorkload *v1.Workload) {
	pods := adminWorkload.Status.Pods

	if commonworkload.IsTorchFT(adminWorkload) {
		// For TorchFT workloads, sort by GroupId first, then by host IP and pod ID within the same group
		sort.Slice(pods, func(i, j int) bool {
			if pods[i].GroupId == pods[j].GroupId {
				return comparePodsByIPAndID(pods[i], pods[j])
			}
			return pods[i].GroupId < pods[j].GroupId
		})
	} else if commonworkload.IsMonarchJob(adminWorkload) {
		sort.Slice(pods, func(i, j int) bool {
			if pods[i].ResourceId == pods[j].ResourceId {
				if pods[i].GroupId == pods[j].GroupId {
					return comparePodsByIPAndID(pods[i], pods[j])
				}
				return pods[i].GroupId < pods[j].GroupId
			}
			return pods[i].ResourceId < pods[j].ResourceId
		})
	} else if commonworkload.IsRayJob(adminWorkload) {
		// For RayJob: submitter first, then head, then worker (by name)
		sort.Slice(pods, func(i, j int) bool {
			tierI := getRayJobPodTier(pods[i].PodId)
			tierJ := getRayJobPodTier(pods[j].PodId)
			if tierI != tierJ {
				return tierI < tierJ
			}
			return pods[i].PodId < pods[j].PodId
		})
	} else {
		// For regular workloads, sort directly by host IP and pod ID
		sort.Slice(pods, func(i, j int) bool {
			return comparePodsByIPAndID(pods[i], pods[j])
		})
	}
}

// getRayJobPodTier returns sort tier for RayJob pods: 0=submitter, 1=head, 2=worker
func getRayJobPodTier(podId string) int {
	if strings.Contains(podId, "-head-") {
		return 1
	}
	if strings.Contains(podId, "-worker-") {
		return 2
	}
	return 0 // submitter or other
}

// getRayJobPodSlotKey returns the stable slot key for a RayJob pod.
// Head and submitter are single-instance per generation, so they share a fixed
// slot to dedup stale pods across RayJob restarts. Worker pods, however, may
// have multiple concurrent replicas within one worker group (e.g. ray.io/group
// "1" with 8 replicas, all named "<cluster>-1-worker-<random>"); keying them by
// the group index would collapse those distinct replicas into one slot and drop
// all but one from the status. Each worker pod therefore gets its own slot
// (full pod name); stale workers from a previous generation are pruned via pod
// deletion events (removeWorkloadPod), not by this slot dedup.
func getRayJobPodSlotKey(podId string) string {
	if strings.Contains(podId, "-head-") {
		return "head"
	}
	if strings.Contains(podId, "-worker-") {
		return podId
	}
	return "submitter"
}

// pruneStaleRayJobPods keeps only the current pod for each RayJob role slot.
func pruneStaleRayJobPods(pods []v1.WorkloadPod) []v1.WorkloadPod {
	if len(pods) <= 1 {
		return pods
	}
	slots := make(map[string][]v1.WorkloadPod)
	for _, pod := range pods {
		key := getRayJobPodSlotKey(pod.PodId)
		slots[key] = append(slots[key], pod)
	}
	result := make([]v1.WorkloadPod, 0, len(slots))
	for _, group := range slots {
		result = append(result, selectCurrentRayJobPod(group))
	}
	return result
}

// selectCurrentRayJobPod picks the active pod from candidates sharing the same RayJob slot.
func selectCurrentRayJobPod(pods []v1.WorkloadPod) v1.WorkloadPod {
	if len(pods) == 1 {
		return pods[0]
	}
	candidates := pods
	active := make([]v1.WorkloadPod, 0, len(pods))
	for i := range pods {
		if !v1.IsPodTerminated(&pods[i]) {
			active = append(active, pods[i])
		}
	}
	if len(active) > 0 {
		candidates = active
	}
	sort.Slice(candidates, func(i, j int) bool {
		return compareRayJobPodPriority(candidates[i], candidates[j]) > 0
	})
	return candidates[0]
}

// compareRayJobPodPriority returns positive when pod a is preferred over pod b.
func compareRayJobPodPriority(a, b v1.WorkloadPod) int {
	if phaseDiff := rayJobPodPhasePriority(a.Phase) - rayJobPodPhasePriority(b.Phase); phaseDiff != 0 {
		return phaseDiff
	}
	timeA, hasTimeA := parseRayJobPodStartTime(a.StartTime)
	timeB, hasTimeB := parseRayJobPodStartTime(b.StartTime)
	if hasTimeA && hasTimeB && !timeA.Equal(timeB) {
		if timeA.After(timeB) {
			return 1
		}
		return -1
	}
	if hasTimeA != hasTimeB {
		if hasTimeA {
			return 1
		}
		return -1
	}
	if a.PodId > b.PodId {
		return 1
	}
	if a.PodId < b.PodId {
		return -1
	}
	return 0
}

func rayJobPodPhasePriority(phase corev1.PodPhase) int {
	switch phase {
	case corev1.PodRunning:
		return 3
	case corev1.PodPending:
		return 2
	case corev1.PodUnknown:
		return 1
	default:
		return 0
	}
}

func parseRayJobPodStartTime(startTime string) (time.Time, bool) {
	if startTime == "" {
		return time.Time{}, false
	}
	parsed, err := time.Parse(timeutil.TimeRFC3339Short, startTime)
	if err != nil {
		return time.Time{}, false
	}
	return parsed, true
}

// comparePodsByIPAndID sort by hostIp and podId
func comparePodsByIPAndID(podI, podJ v1.WorkloadPod) bool {
	if podI.HostIp == podJ.HostIp {
		return podI.PodId < podJ.PodId
	}

	ipI := netutil.ConvertIpToInt(podI.HostIp)
	ipJ := netutil.ConvertIpToInt(podJ.HostIp)
	return ipI < ipJ
}

// getMainContainerName get main container name of pod
func getMainContainerName(adminWorkload *v1.Workload, pod *corev1.Pod) string {
	mainContainerName := v1.GetMainContainer(pod)
	if mainContainerName == "" {
		// TODO: Keep old logic for compatibility; remove it later.
		resourceId, _ := v1.GetResourceId(pod)
		mainContainerName = commonworkload.GetMainContainer(adminWorkload, adminWorkload.SpecKind(), resourceId)
	}
	return mainContainerName
}

// isAllPodsAssigned checks if all pods in the workload are in Running or Termination phase
func isAllPodsAssigned(workload *v1.Workload) bool {
	if commonworkload.IsRayJob(workload) {
		// For RayJob, the ray-job-submitter pod is automatically created as the management pod
		if len(workload.Status.Pods) != commonworkload.GetTotalReplica(workload)+1 {
			return false
		}
	} else if len(workload.Status.Pods) != commonworkload.GetTotalReplica(workload) {
		return false
	}
	for _, p := range workload.Status.Pods {
		if p.Phase == corev1.PodPending || p.AdminNodeName == "" {
			return false
		}
	}
	return true
}

func isAllPodRunning(workload *v1.Workload) bool {
	for _, p := range workload.Status.Pods {
		if p.Phase != corev1.PodRunning {
			return false
		}
	}
	return true
}
