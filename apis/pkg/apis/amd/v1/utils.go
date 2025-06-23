/*
 * Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package v1

import (
	"strconv"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func GetLabel(obj metav1.Object, key string) string {
	if obj == nil || len(obj.GetLabels()) == 0 {
		return ""
	}
	val, ok := obj.GetLabels()[key]
	if !ok {
		return ""
	}
	return val
}

func GetAnnotation(obj metav1.Object, key string) string {
	if obj == nil || len(obj.GetAnnotations()) == 0 {
		return ""
	}
	val, ok := obj.GetAnnotations()[key]
	if !ok {
		return ""
	}
	return val
}

func HasLabel(obj metav1.Object, key string) bool {
	if obj == nil || len(obj.GetLabels()) == 0 {
		return false
	}
	_, ok := obj.GetLabels()[key]
	return ok
}

func HasAnnotation(obj metav1.Object, key string) bool {
	if obj == nil || len(obj.GetAnnotations()) == 0 {
		return false
	}
	_, ok := obj.GetAnnotations()[key]
	return ok
}

func RemoveLabel(obj metav1.Object, key string) bool {
	if obj == nil {
		return false
	}
	if _, ok := obj.GetLabels()[key]; !ok {
		return false
	}
	delete(obj.GetLabels(), key)
	return true
}

func RemoveEmptyLabel(obj metav1.Object, key string) bool {
	if obj == nil {
		return false
	}
	val, ok := obj.GetLabels()[key]
	if ok && val == "" {
		delete(obj.GetLabels(), key)
		return true
	}
	return false
}

func RemoveAnnotation(obj metav1.Object, key string) bool {
	if obj == nil {
		return false
	}
	if _, ok := obj.GetAnnotations()[key]; !ok {
		return false
	}
	delete(obj.GetAnnotations(), key)
	return true
}

func SetLabel(obj metav1.Object, key, val string) bool {
	if obj == nil {
		return false
	}
	if obj.GetLabels() == nil {
		obj.SetLabels(make(map[string]string))
	}
	if currentVal, ok := obj.GetLabels()[key]; ok && currentVal == val {
		return false
	}
	obj.GetLabels()[key] = val
	return true
}

func SetAnnotation(obj metav1.Object, key, val string) bool {
	if obj == nil {
		return false
	}
	if obj.GetAnnotations() == nil {
		obj.SetAnnotations(make(map[string]string))
	}
	if currentVal, ok := obj.GetAnnotations()[key]; ok && currentVal == val {
		return false
	}
	obj.GetAnnotations()[key] = val
	return true
}

func GetNodeGpuCount(obj metav1.Object) int {
	return atoi(GetLabel(obj, NodeGpuCountLabel))
}

func GetNodeStartupTime(obj metav1.Object) string {
	return GetLabel(obj, NodeStartupTimeLabel)
}

func GetClusterId(obj metav1.Object) string {
	return GetLabel(obj, ClusterIdLabel)
}

func GetWorkspaceId(obj metav1.Object) string {
	return GetLabel(obj, WorkspaceIdLabel)
}

func GetNodeId(obj metav1.Object) string {
	return GetLabel(obj, NodeIdLabel)
}

func GetNodeFlavorId(obj metav1.Object) string {
	return GetLabel(obj, NodeFlavorIdLabel)
}

func GetDisplayName(obj metav1.Object) string {
	return GetLabel(obj, DisplayNameLabel)
}

func GetGpuProductName(obj metav1.Object) string {
	return GetAnnotation(obj, GpuProductNameAnnotation)
}

func GetGpuResourceName(obj metav1.Object) string {
	return GetAnnotation(obj, GpuResourceNameAnnotation)
}

func GetNodeLabelAction(obj metav1.Object) string {
	return GetAnnotation(obj, NodeLabelAction)
}

func GetNodeAnnotationAction(obj metav1.Object) string {
	return GetAnnotation(obj, NodeAnnotationAction)
}

func GetWorkspaceNodesAction(obj metav1.Object) string {
	return GetAnnotation(obj, WorkspaceNodesAction)
}

func IsWorkloadDispatched(obj metav1.Object) bool {
	return HasAnnotation(obj, WorkloadDispatchedAnnotation)
}

func IsWorkloadScheduled(obj metav1.Object) bool {
	return HasAnnotation(obj, WorkloadScheduledAnnotation)
}

func IsControlPlane(obj metav1.Object) bool {
	return HasLabel(obj, KubernetesControlPlane)
}

func IsProtected(obj metav1.Object) bool {
	return HasLabel(obj, ProtectLabel)
}

func GetUserName(obj metav1.Object) string {
	return GetAnnotation(obj, UserNameAnnotation)
}

func GetWorkloadDispatchCnt(obj metav1.Object) int {
	return atoi(GetLabel(obj, WorkloadDispatchCntLabel))
}

func GetDescription(obj metav1.Object) string {
	return GetAnnotation(obj, DescriptionAnnotation)
}

func GetMainContainer(obj metav1.Object) string {
	return GetAnnotation(obj, MainContainerAnnotation)
}

func GetWorkloadId(obj metav1.Object) string {
	return GetLabel(obj, WorkloadIdLabel)
}

func IsWorkloadDisableFailover(obj metav1.Object) bool {
	return HasAnnotation(obj, WorkloadDisableFailoverAnnotation)
}

func IsWorkloadReScheduled(obj metav1.Object) bool {
	return HasAnnotation(obj, WorkloadReScheduledAnnotation)
}

func IsEnableHostNetwork(obj metav1.Object) bool {
	return GetAnnotation(obj, EnableHostNetworkAnnotation) == "true"
}

func IsWorkloadEnablePreempt(obj metav1.Object) bool {
	return GetAnnotation(obj, WorkloadEnablePreemptAnnotation) == "true"
}

func IsWorkloadPreempted(obj metav1.Object) bool {
	return HasAnnotation(obj, WorkloadPreemptedAnnotation)
}

func GetRetryCount(obj metav1.Object) int {
	return atoi(GetAnnotation(obj, RetryCountAnnotation))
}

func GetOpsJobId(obj metav1.Object) string {
	return GetLabel(obj, OpsJobIdLabel)
}

func GetOpsJobType(obj metav1.Object) string {
	return GetLabel(obj, OpsJobTypeLabel)
}

func GetOpsJobInput(obj metav1.Object) string {
	return GetAnnotation(obj, OpsJobInputAnnotation)
}

func IsSecurityUpgrade(obj metav1.Object) bool {
	return HasAnnotation(obj, OpsJobSecurityUpgradeAnnotation)
}

func GetOpsJobBatchCount(obj metav1.Object) int {
	return atoi(GetAnnotation(obj, OpsJobBatchCountAnnotation))
}

func IsAuthoring(obj metav1.Object) bool {
	return GetLabel(obj, WorkloadKindLabel) == AuthoringKind
}

func atoi(str string) int {
	if str == "" {
		return 0
	}
	n, err := strconv.Atoi(str)
	if err != nil {
		return 0
	}
	return n
}
