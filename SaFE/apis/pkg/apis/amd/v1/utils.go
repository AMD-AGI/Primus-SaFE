/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package v1

import (
	"encoding/json"
	"strconv"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// GetLabel retrieves the value of a label by key from a Kubernetes object.
// Returns an empty string if the object is nil, has no labels, or the key doesn't exist.
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

// GetAnnotation retrieves the value of an annotation by key from a Kubernetes object.
// Returns an empty string if the object is nil, has no annotations, or the key doesn't exist.
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

// HasLabel checks if a label key exists on a Kubernetes object.
// Returns false if the object is nil or has no labels.
func HasLabel(obj metav1.Object, key string) bool {
	if obj == nil || len(obj.GetLabels()) == 0 {
		return false
	}
	_, ok := obj.GetLabels()[key]
	return ok
}

// HasAnnotation checks if an annotation key exists on a Kubernetes object.
// Returns false if the object is nil or has no annotations.
func HasAnnotation(obj metav1.Object, key string) bool {
	if obj == nil || len(obj.GetAnnotations()) == 0 {
		return false
	}
	_, ok := obj.GetAnnotations()[key]
	return ok
}

// RemoveLabel removes a label from a Kubernetes object.
// Returns true if the label was removed, false if the object is nil or the label didn't exist.
func RemoveLabel(obj metav1.Object, key string) bool {
	if obj == nil || len(obj.GetLabels()) == 0 {
		return false
	}
	if _, ok := obj.GetLabels()[key]; !ok {
		return false
	}
	delete(obj.GetLabels(), key)
	return true
}

// RemoveEmptyLabel removes a label from a Kubernetes object if its value is empty.
// Returns true if the label was removed, false otherwise.
func RemoveEmptyLabel(obj metav1.Object, key string) bool {
	if obj == nil || len(obj.GetLabels()) == 0 {
		return false
	}
	val, ok := obj.GetLabels()[key]
	if ok && val == "" {
		delete(obj.GetLabels(), key)
		return true
	}
	return false
}

// RemoveAnnotation removes an annotation from a Kubernetes object.
// Returns true if the annotation was removed, false if the object is nil or the annotation didn't exist.
func RemoveAnnotation(obj metav1.Object, key string) bool {
	if obj == nil || len(obj.GetAnnotations()) == 0 {
		return false
	}
	if _, ok := obj.GetAnnotations()[key]; !ok {
		return false
	}
	delete(obj.GetAnnotations(), key)
	return true
}

// SetLabel sets a label on a Kubernetes object, creating the labels map if needed.
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

// SetAnnotation sets an annotation on a Kubernetes object, creating the annotations map if needed.
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

// GetNodeGpuCount retrieves the GPU count from a node's labels.
func GetNodeGpuCount(obj metav1.Object) int {
	return atoi(GetLabel(obj, NodeGpuCountLabel))
}

// GetNodeStartupTime retrieves the node startup timestamp from labels.
func GetNodeStartupTime(obj metav1.Object) string {
	return GetLabel(obj, NodeStartupTimeLabel)
}

// GetClusterId returns the cluster ID from the request.
func GetClusterId(obj metav1.Object) string {
	return GetLabel(obj, ClusterIdLabel)
}

// GetWorkspaceId returns the workspace ID from the request.
func GetWorkspaceId(obj metav1.Object) string {
	return GetLabel(obj, WorkspaceIdLabel)
}

// GetNodeId retrieves the node ID from a resource.
func GetNodeId(obj metav1.Object) string {
	return GetLabel(obj, NodeIdLabel)
}

// GetNodeFlavorId retrieves the node flavor ID from a resource's labels.
func GetNodeFlavorId(obj metav1.Object) string {
	return GetLabel(obj, NodeFlavorIdLabel)
}

// GetDisplayName retrieves the display name label from a resource.
func GetDisplayName(obj metav1.Object) string {
	return GetLabel(obj, DisplayNameLabel)
}

// GetGpuResourceName retrieves the GPU resource name from annotations.
func GetGpuResourceName(obj metav1.Object) string {
	return GetAnnotation(obj, GpuResourceNameAnnotation)
}

// GetNodeLabelAction retrieves the node label action from annotations.
func GetNodeLabelAction(obj metav1.Object) string {
	return GetAnnotation(obj, NodeLabelAction)
}

// GetNodeAnnotationAction retrieves the node annotation action from annotations.
func GetNodeAnnotationAction(obj metav1.Object) string {
	return GetAnnotation(obj, NodeAnnotationAction)
}

// IsNodeTemplateInstalled checks if the node template has been installed.
func IsNodeTemplateInstalled(obj metav1.Object) bool {
	return GetAnnotation(obj, NodeTemplateInstalledAnnotation) == TrueStr
}

// GetWorkspaceNodesAction retrieves the workspace nodes action from annotations.
func GetWorkspaceNodesAction(obj metav1.Object) string {
	return GetAnnotation(obj, WorkspaceNodesAction)
}

// IsWorkloadDispatched checks if a workload has been dispatched for execution.
func IsWorkloadDispatched(obj metav1.Object) bool {
	return HasAnnotation(obj, WorkloadDispatchedAnnotation)
}

// IsWorkloadScheduled checks if a workload has been scheduled.
func IsWorkloadScheduled(obj metav1.Object) bool {
	return HasAnnotation(obj, WorkloadScheduledAnnotation)
}

// IsControlPlane checks if a node is a control plane node.
func IsControlPlane(obj metav1.Object) bool {
	return HasLabel(obj, KubernetesControlPlane)
}

// IsProtected checks if a resource is protected from deletion.
func IsProtected(obj metav1.Object) bool {
	return HasLabel(obj, ProtectLabel)
}

// GetUserName retrieves the username annotation from a resource.
func GetUserName(obj metav1.Object) string {
	return GetAnnotation(obj, UserNameAnnotation)
}

// GetUserEmail retrieves the user email from annotations.
func GetUserEmail(obj metav1.Object) string {
	return GetAnnotation(obj, UserEmailAnnotation)
}

// GetUserAvatarUrl retrieves the user avatar URL from annotations.
func GetUserAvatarUrl(obj metav1.Object) string {
	return GetAnnotation(obj, UserAvatarUrlAnnotation)
}

// GetUserId retrieves the user ID label from a resource.
func GetUserId(obj metav1.Object) string {
	return GetLabel(obj, UserIdLabel)
}

// GetWorkloadDispatchCnt returns the number of times a workload has been dispatched.
func GetWorkloadDispatchCnt(obj metav1.Object) int {
	return atoi(GetLabel(obj, WorkloadDispatchCntLabel))
}

// GetDescription retrieves the description annotation from a resource.
func GetDescription(obj metav1.Object) string {
	return GetAnnotation(obj, DescriptionAnnotation)
}

// GetMainContainer returns the name of the main container from a workload.
func GetMainContainer(obj metav1.Object) string {
	return GetAnnotation(obj, MainContainerAnnotation)
}

// GetCICDRunnerScaleSetId returns the cicd runner scale set id
func GetCICDRunnerScaleSetId(obj metav1.Object) string {
	return GetAnnotation(obj, CICDScaleSetIdAnnotation)
}

// GetWorkloadId retrieves the workload ID from a resource's labels.
func GetWorkloadId(obj metav1.Object) string {
	return GetLabel(obj, WorkloadIdLabel)
}

// IsWorkloadDisableFailover checks if failover is disabled for a workload.
func IsWorkloadDisableFailover(obj metav1.Object) bool {
	return HasAnnotation(obj, WorkloadDisableFailoverAnnotation)
}

// IsWorkloadReScheduled checks if a workload has been rescheduled.
func IsWorkloadReScheduled(obj metav1.Object) bool {
	return HasAnnotation(obj, WorkloadReScheduledAnnotation)
}

// IsWorkloadEnablePreempt checks if preemption is enabled for a workload.
func IsWorkloadEnablePreempt(obj metav1.Object) bool {
	return GetAnnotation(obj, WorkloadEnablePreemptAnnotation) == TrueStr
}

// IsWorkloadPreempted checks if a workload has been preempted.
func IsWorkloadPreempted(obj metav1.Object) bool {
	return HasAnnotation(obj, WorkloadPreemptedAnnotation)
}

// GetRetryCount retrieves the retry count from annotations.
func GetRetryCount(obj metav1.Object) int {
	return atoi(GetAnnotation(obj, RetryCountAnnotation))
}

// GetOpsJobId retrieves the operations job ID from a resource's labels.
func GetOpsJobId(obj metav1.Object) string {
	return GetLabel(obj, OpsJobIdLabel)
}

// GetOpsJobType retrieves the operations job type from labels.
func GetOpsJobType(obj metav1.Object) string {
	return GetLabel(obj, OpsJobTypeLabel)
}

// IsSecurityUpgrade checks if an operations job is a security upgrade.
func IsSecurityUpgrade(obj metav1.Object) bool {
	return HasAnnotation(obj, OpsJobSecurityUpgradeAnnotation)
}

// GetOpsJobBatchCount retrieves the batch count for an operations job.
func GetOpsJobBatchCount(obj metav1.Object) int {
	return atoi(GetAnnotation(obj, OpsJobBatchCountAnnotation))
}

// GetOpsJobAvailRatio retrieves the availability ratio for an operations job.
func GetOpsJobAvailRatio(obj metav1.Object) float64 {
	return atof(GetAnnotation(obj, OpsJobAvailRatioAnnotation))
}

// GetSecretType retrieves the secret type from a resource's labels.
func GetSecretType(obj metav1.Object) string {
	return GetLabel(obj, SecretTypeLabel)
}

// GetCronjobTimestamp retrieves the cronjob timestamp from annotations.
func GetCronjobTimestamp(obj metav1.Object) string {
	return GetAnnotation(obj, CronJobTimestampAnnotation)
}

func GetEnvToBeRemoved(obj metav1.Object) []string {
	str := GetAnnotation(obj, EnvToBeRemovedAnnotation)
	if str == "" {
		return nil
	}
	var result []string
	if json.Unmarshal([]byte(str), &result) != nil {
		return nil
	}
	return result
}

func GetGithubSecretId(obj metav1.Object) string {
	return GetAnnotation(obj, GithubSecretIdAnnotation)
}

func GetAdminControlPlane(obj metav1.Object) string {
	return GetAnnotation(obj, AdminControlPlaneAnnotation)
}

func IsRequireNodeSpread(obj metav1.Object) bool {
	return GetAnnotation(obj, RequireNodeSpreadAnnotation) == TrueStr
}

func GetRootWorkloadId(obj metav1.Object) string {
	return GetLabel(obj, RootWorkloadIdLabel)
}

func GetResourceId(obj metav1.Object) (int, bool) {
	str := GetAnnotation(obj, ResourceIdAnnotation)
	if str == "" {
		return 0, false
	}
	n, err := strconv.Atoi(str)
	if err != nil {
		return 0, false
	}
	return n, true
}

// atoi converts a string to an integer, returning 0 if conversion fails.
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

// atof converts a string to a float64, returning 0.0 if conversion fails.
func atof(str string) float64 {
	if str == "" {
		return 0
	}
	f, err := strconv.ParseFloat(str, 64)
	if err != nil {
		return 0
	}
	return f
}
