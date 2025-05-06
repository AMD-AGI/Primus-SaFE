/*
 * Copyright © AMD. 2025-2026. All rights reserved.
 */

package v1

import (
	"strconv"
	"strings"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	PPCount = 8
	TPCount = 4
)

func NewCondition(conditionType, message, reason string) *metav1.Condition {
	result := &metav1.Condition{
		Type:    conditionType,
		Status:  metav1.ConditionTrue,
		Message: message,
		Reason:  reason,
	}
	result.LastTransitionTime = metav1.NewTime(time.Now().UTC())
	return result
}

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

func GetClusterName(obj metav1.Object) string {
	return GetLabel(obj, ClusterNameLabel)
}

func GetDisplayName(obj metav1.Object) string {
	return GetLabel(obj, DisplayNameLabel)
}

func GetTenantId(obj metav1.Object) string {
	return GetLabel(obj, TenantIdLabel)
}

func GetDedicatedTenantId(obj metav1.Object) string {
	if IsPublic(obj) {
		return ""
	}
	return GetLabel(obj, TenantIdLabel)
}

func GetTenantName(obj metav1.Object) string {
	return GetAnnotation(obj, TenantNameAnnotation)
}

func GetUserId(obj metav1.Object) string {
	return GetLabel(obj, UserIdLabel)
}

func GetUserName(obj metav1.Object) string {
	return GetAnnotation(obj, UserNameAnnotation)
}

func GetBindWorkspace(obj metav1.Object) string {
	return GetLabel(obj, NodeBindWorkspaceLabel)
}

func GetBindElasticWorkspace(obj metav1.Object) string {
	return GetLabel(obj, NodeBindElasticWorkspaceLabel)
}

func GetNodeFlavor(obj metav1.Object) string {
	return GetLabel(obj, NodeFlavorLabel)
}

func GetImagePullSecrets(obj metav1.Object) string {
	return GetAnnotation(obj, ImagePullSecretsAnnotation)
}

func IsUncontrolled(obj metav1.Object) bool {
	if obj == nil {
		return false
	}
	_, ok := obj.GetAnnotations()[UncontrolledAnnotation]
	return ok
}

func GetNodesWorkspaceAction(obj metav1.Object) string {
	return GetAnnotation(obj, NodesWorkspaceAction)
}

func GetNodesLabelAction(obj metav1.Object) string {
	return GetAnnotation(obj, NodesLabelAction)
}

func GetNodesAnnotationAction(obj metav1.Object) string {
	return GetAnnotation(obj, NodesAnnotationAction)
}

func GetDescription(obj metav1.Object) string {
	return GetAnnotation(obj, DescriptionAnnotation)
}

func GetWorkloadDispatchCnt(obj metav1.Object) int {
	return atoi(GetLabel(obj, WorkloadDispatchCntLabel))
}

func GetWorkloadMainContainer(obj metav1.Object) string {
	return GetAnnotation(obj, WorkloadMainContainer)
}

func GetCreatorUserId(obj metav1.Object) string {
	return GetLabel(obj, CreatorUserIdLabel)
}

func GetCreatorTenantId(obj metav1.Object) string {
	return GetLabel(obj, CreatorTenantIdLabel)
}

func GetNodeGpuCount(obj metav1.Object) int {
	return atoi(GetLabel(obj, NodeGpuCountLabel))
}

func GetNodeDataDisk(obj metav1.Object) string {
	return GetAnnotation(obj, NodeDataDiskAnnotation)
}

func GetNodeRegionPod(obj metav1.Object) string {
	return GetLabel(obj, NodeRegionPodLabel)
}

func GetWorkloadId(obj metav1.Object) string {
	return GetLabel(obj, WorkloadIdLabel)
}

func GetAlertGroup(obj metav1.Object) string { return GetLabel(obj, AlertGroupLabel) }

func GetPPCount(obj metav1.Object) int {
	n := atoi(GetAnnotation(obj, PPCountAnnotation))
	if n == 0 {
		n = PPCount
	}
	return n
}

func GetTPCount(obj metav1.Object) int {
	n := atoi(GetAnnotation(obj, TPCountAnnotation))
	if n == 0 {
		n = TPCount
	}
	return n
}

func GetSchedulerPolicy(obj metav1.Object) string {
	return GetAnnotation(obj, SchedulerPolicyAnnotation)
}

func GetJobId(obj metav1.Object) string {
	return GetLabel(obj, JobIdLabel)
}

func GetNodeId(obj metav1.Object) string {
	return GetLabel(obj, NodeIdLabel)
}

func GetNodeTemplate(obj metav1.Object) string {
	return GetAnnotation(obj, NodeTemplateAnnotation)
}

func GetNodeStartupTime(obj metav1.Object) string {
	return GetLabel(obj, NodeStartupTimeLabel)
}

func GetNodeInspectionTime(obj metav1.Object) string {
	return GetAnnotation(obj, NodeInspectionTimeAnnotation)
}

func GetFaultIds(obj metav1.Object) string {
	return GetLabel(obj, FaultIDsAnnotation)
}

func GetJobDispatchTime(obj metav1.Object) string {
	return GetAnnotation(obj, JobDispatchTimeAnnotation)
}

func GetNodePrivateIp(obj metav1.Object) string {
	return GetLabel(obj, NodePrivateIpLabel)
}

func GetJobType(obj metav1.Object) string {
	return GetLabel(obj, JobTypeLabel)
}

func GetNodeJobInput(obj metav1.Object) string {
	return GetAnnotation(obj, NodeJobInputAnnotation)
}

func GetJobBatchCount(obj metav1.Object) int {
	return atoi(GetAnnotation(obj, JobBatchCountAnnotation))
}

func GetQueueBalanceTimeout(obj metav1.Object) int {
	return atoi(GetAnnotation(obj, QueueBalanceTimeoutAnnotation))
}

func GetClusterType(obj metav1.Object) string {
	return GetLabel(obj, ClusterTypeLabel)
}

func GetWorkspaceType(obj metav1.Object) string {
	return GetLabel(obj, WorkspaceTypeLabel)
}

func GetWorkspaceId(obj metav1.Object) string {
	return GetLabel(obj, WorkspaceIdLabel)
}

func GetDataPlaneNamespace(obj metav1.Object) string {
	return GetLabel(obj, DataPlaneNamespace)
}

func GetDataPlaneName(obj metav1.Object) string {
	return GetLabel(obj, DataPlaneName)
}

func GetCronScaleInitial(obj metav1.Object) string {
	return GetAnnotation(obj, CronScaleInitialAnnotation)
}

func GetUserEmail(obj metav1.Object) string {
	return GetAnnotation(obj, UserEmailAnnotation)
}

func GetUserAvatarUrl(obj metav1.Object) string {
	return GetAnnotation(obj, UserAvatarUrlAnnotation)
}

func GetRoleExpireTime(obj metav1.Object) int {
	return atoi(GetLabel(obj, UserRoleExpirationLabel))
}

func GetUserEmployeeType(obj metav1.Object) int {
	return atoi(GetLabel(obj, UserEmployeeTypeLabel))
}

func GetWorkloadMaxRuntime(obj metav1.Object) int {
	return atoi(GetAnnotation(obj, WorkloadMaxRuntimeHour))
}

func GetWorkloadRuntime(obj metav1.Object) string {
	return GetAnnotation(obj, WorkloadRunTimeAnnotation)
}

func GetSyncDataPlanes(obj metav1.Object) []string {
	str := GetAnnotation(obj, SyncDataPlanes)
	if str == "" {
		return nil
	}
	return strings.Split(str, ",")
}

func GetKind(obj metav1.Object) string {
	return GetLabel(obj, KindLabel)
}

func GetChipType(obj metav1.Object) string {
	return GetAnnotation(obj, ChipTypeAnnotation)
}

func GetNpuChip(obj metav1.Object) string {
	return GetLabel(obj, NpuChipNameLabel)
}

func GetBackupLabel(obj metav1.Object) string {
	return GetLabel(obj, BackupLabel)
}

func IsWorkloadDispatched(obj metav1.Object) bool {
	if obj == nil || len(obj.GetAnnotations()) == 0 {
		return false
	}
	_, ok := obj.GetAnnotations()[WorkloadDispatchedAnnotation]
	return ok
}

func IsWorkloadScheduled(obj metav1.Object) bool {
	if obj == nil {
		return false
	}
	_, ok := obj.GetAnnotations()[WorkloadScheduledAnnotation]
	return ok
}

func IsWorkloadPreempted(obj metav1.Object) bool {
	if obj == nil {
		return false
	}
	_, ok := obj.GetAnnotations()[WorkloadPreemptedAnnotation]
	return ok
}

func IsWorkloadForcedFailover(obj metav1.Object) bool {
	if obj == nil {
		return false
	}
	_, ok := obj.GetAnnotations()[WorkloadForcedFoAnnotation]
	return ok
}

func IsWorkloadDisableFailover(obj metav1.Object) bool {
	if obj == nil {
		return false
	}
	_, ok := obj.GetAnnotations()[WorkloadDisableFailoverAnnotation]
	return ok
}

func IsWorkloadEnablePreempt(obj metav1.Object) bool {
	if obj == nil {
		return false
	}
	_, ok := obj.GetAnnotations()[WorkloadEnablePreemptAnnotation]
	return ok
}

func IsDisableDFS(obj metav1.Object) bool {
	if obj == nil {
		return false
	}
	_, ok := obj.GetAnnotations()[DisableDFSAnnotation]
	return ok
}

func IsAuthoring(obj metav1.Object) bool {
	if obj == nil {
		return false
	}
	_, ok := obj.GetLabels()[DevelopMachineLabel]
	return ok
}

func IsSafeCreated(obj metav1.Object) bool {
	if obj == nil {
		return false
	}
	_, ok := obj.GetLabels()[SafeCreated]
	return ok
}

func IsEnableHostNetwork(obj metav1.Object) bool {
	return GetAnnotation(obj, EnableHostNetworkAnnotation) == "true"
}

func IsControlPlane(obj metav1.Object) bool {
	if obj == nil {
		return false
	}
	_, ok := obj.GetLabels()[KubernetesControlPlane]
	return ok
}

func IsSecurityUpgrade(obj metav1.Object) bool {
	if obj == nil {
		return false
	}
	_, ok := obj.GetAnnotations()[JobSecurityUpgradeAnnotation]
	return ok
}

func IsClusterProtected(obj metav1.Object) bool {
	if obj == nil {
		return false
	}
	_, ok := obj.GetLabels()[ClusterProtectLabel]
	return ok
}

func IsTenantProtected(obj metav1.Object) bool {
	if obj == nil {
		return false
	}
	_, ok := obj.GetLabels()[TenantProtectLabel]
	return ok
}

func IsScheduledAdvance(obj metav1.Object) bool {
	if obj == nil {
		return false
	}
	_, ok := obj.GetAnnotations()[WorkloadScheduledAdvanceAnnotation]
	return ok
}

func IsEnablePreempt(obj metav1.Object) bool {
	if obj == nil {
		return false
	}
	_, ok := obj.GetLabels()[WorkspaceEnablePreemptLabel]
	return ok
}

func IsPublic(obj metav1.Object) bool {
	if obj == nil {
		return false
	}
	_, ok := obj.GetLabels()[SafePublicLabel]
	return ok
}

func IsBackup(obj metav1.Object) bool {
	if obj == nil {
		return false
	}
	_, ok := obj.GetLabels()[BackupLabel]
	return ok
}

func GetRestricted(obj metav1.Object) int {
	return atoi(GetLabel(obj, RestrictedLabel))
}

func GenAdminName(clusterName, k8sNodeName string) string {
	return clusterName + "-" + k8sNodeName
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
