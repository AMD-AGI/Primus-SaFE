/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package workload

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/sets"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/selection"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	commonerrors "github.com/AMD-AIG-AIMA/SAFE/common/pkg/errors"
	commonnodes "github.com/AMD-AIG-AIMA/SAFE/common/pkg/nodes"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/quantity"
	commonutils "github.com/AMD-AIG-AIMA/SAFE/common/pkg/utils"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/concurrent"
	sliceutil "github.com/AMD-AIG-AIMA/SAFE/utils/pkg/slice"
)

// GetTotalReplica returns the total replica count across all resources in the workload
func GetTotalReplica(w *v1.Workload) int {
	n := 0
	for _, res := range w.Spec.Resources {
		n += res.Replica
	}
	return n
}

// GetTotalNodeCount returns the total number of unique nodes where the workload's pods are running.
func GetTotalNodeCount(w *v1.Workload) int {
	// Prefer the etcd NodePodUsage aggregate; fall back to Status.Pods when it is
	// absent.
	if len(w.Status.NodeUsage) > 0 {
		return totalNodeCountFromUsage(w.Status.NodeUsage)
	}
	uniqNodeSet := sets.NewSet()
	for _, p := range w.Status.Pods {
		uniqNodeSet.Insert(p.AdminNodeName)
	}
	return uniqNodeSet.Len()
}

// GetWorkloadsOfWorkspace retrieves workloads belonging to specified workspace(s) and cluster.
func GetWorkloadsOfWorkspace(ctx context.Context, cli client.Client, clusterName string, workspaceNames []string,
	filterFunc func(*v1.Workload) bool) ([]*v1.Workload, error) {
	var labelSelector = labels.NewSelector()
	if clusterName != "" {
		req, _ := labels.NewRequirement(v1.ClusterIdLabel, selection.Equals, []string{clusterName})
		labelSelector = labelSelector.Add(*req)
	}
	if len(workspaceNames) != 0 {
		req, _ := labels.NewRequirement(v1.WorkspaceIdLabel, selection.In, workspaceNames)
		labelSelector = labelSelector.Add(*req)
	}
	listOptions := &client.ListOptions{LabelSelector: labelSelector}
	workloadList := &v1.WorkloadList{}
	if err := cli.List(ctx, workloadList, listOptions); err != nil {
		return nil, err
	}
	result := make([]*v1.Workload, 0, len(workloadList.Items))
	for i, w := range workloadList.Items {
		if filterFunc != nil && filterFunc(&w) {
			continue
		}
		result = append(result, &workloadList.Items[i])
	}
	return result, nil
}

// GetWorkloadsOfK8sNode retrieves workload names running on a specific Kubernetes node.
func GetWorkloadsOfK8sNode(ctx context.Context, k8sClient kubernetes.Interface, k8sNodeName, namespace string) ([]string, error) {
	if namespace == "" {
		return nil, nil
	}
	pods, err := commonnodes.ListPods(ctx, k8sClient, []string{k8sNodeName}, namespace)
	if err != nil {
		return nil, err
	}
	var results []string
	for _, p := range pods {
		name, ok := p.GetLabels()[v1.WorkloadIdLabel]
		if ok && name != "" {
			results = append(results, name)
		}
	}
	return results, nil
}

// GetResourcesPerNode calculates resource usage per node for a workload.
func GetResourcesPerNode(workload *v1.Workload, adminNodeName string) (map[string]corev1.ResourceList, error) {
	if GetTotalReplica(workload) == 0 {
		return nil, nil
	}
	allPodResources, err := toPodResourceLists(workload)
	if err != nil {
		return nil, err
	}

	// Dual-read: prefer the etcd NodePodUsage aggregate; fall back to Status.Pods.
	if len(workload.Status.NodeUsage) > 0 {
		return resourcesPerNodeFromUsage(workload.Status.NodeUsage, allPodResources, adminNodeName), nil
	}

	result := map[string]corev1.ResourceList{}
	for _, pod := range workload.Status.Pods {
		if !v1.IsPodRunning(&pod) || pod.ResourceId >= int8(len(allPodResources)) {
			continue
		}
		if adminNodeName != "" && adminNodeName != pod.AdminNodeName {
			continue
		}
		podResources := allPodResources[pod.ResourceId]
		resList, ok := result[pod.AdminNodeName]
		if ok {
			result[pod.AdminNodeName] = quantity.AddResource(resList, podResources)
		} else {
			result[pod.AdminNodeName] = podResources
		}
	}
	return result, nil
}

func GetMainContainer(obj metav1.Object, kind string, resourceId int) string {
	if kind == common.RayJobKind && resourceId == 0 {
		return common.RayJobSubmitterName
	}
	return v1.GetMainContainer(obj)
}

func GetMainContainerByPod(obj metav1.Object, kind, podName string) string {
	if kind == common.RayJobKind && podName != "" {
		if !strings.Contains(podName, "-head-") && !strings.Contains(podName, "-worker-") {
			return common.RayJobSubmitterName
		}
	}
	return v1.GetMainContainer(obj)
}

// GetWorkloadResourceUsage retrieves active resources based on the input workload.
// It filters out terminated pods and applies node filtering criteria.
func GetWorkloadResourceUsage(workload *v1.Workload, filterNode func(nodeName string) bool) (
	corev1.ResourceList, corev1.ResourceList, []string, error) {
	if GetTotalReplica(workload) == 0 ||
		(len(workload.Status.Pods) == 0 && len(workload.Status.NodeUsage) == 0) {
		return nil, nil, nil, nil
	}
	allPodResources, err := toPodResourceLists(workload)
	if err != nil {
		return nil, nil, nil, err
	}

	// Dual-read: prefer the etcd NodePodUsage aggregate; fall back to Status.Pods.
	if len(workload.Status.NodeUsage) > 0 {
		total, avail, nodes := workloadResourceUsageFromUsage(workload.Status.NodeUsage, allPodResources, filterNode)
		return total, avail, nodes, nil
	}

	type input struct {
		id  int
		pod *v1.WorkloadPod
	}
	type output struct {
		isFiltered   bool
		isTerminated bool
		resourceId   int
	}

	count := len(workload.Status.Pods)
	outputs := make([]output, count)
	ch := make(chan *input, count)
	defer close(ch)
	for i := range workload.Status.Pods {
		ch <- &input{
			id:  i,
			pod: &workload.Status.Pods[i],
		}
	}

	concurrent.Exec(count, func() error {
		in := <-ch
		if v1.IsPodTerminated(in.pod) {
			outputs[in.id].isTerminated = true
			return nil
		}
		outputs[in.id].resourceId = int(in.pod.ResourceId)
		if filterNode != nil && filterNode(in.pod.AdminNodeName) {
			outputs[in.id].isFiltered = true
			return nil
		}
		return nil
	})
	totalResource := make(corev1.ResourceList)
	availableResource := make(corev1.ResourceList)
	availableNodes := make([]string, 0, count)
	for i := range outputs {
		resourceId := outputs[i].resourceId
		if outputs[i].isTerminated || resourceId >= len(allPodResources) {
			continue
		}
		podResource := allPodResources[resourceId]
		totalResource = quantity.AddResource(totalResource, podResource)
		if !outputs[i].isFiltered {
			availableResource = quantity.AddResource(availableResource, podResource)
			availableNodes = append(availableNodes, workload.Status.Pods[i].AdminNodeName)
		}
	}
	return totalResource, availableResource, availableNodes, nil
}

// GetTotalResourceList converts workload resources to total resource list by summing up all workload resources.
func GetTotalResourceList(workload *v1.Workload) (corev1.ResourceList, error) {
	result := make(corev1.ResourceList)
	for _, res := range workload.Spec.Resources {
		resourceList, err := quantity.CvtToResourceList(res.CPU, res.Memory, res.GPU,
			res.GPUName, res.EphemeralStorage, res.RdmaResource, float64(res.Replica))
		if err != nil {
			return nil, commonerrors.NewBadRequest(err.Error())
		}
		result = quantity.AddResource(result, resourceList)
	}
	return result, nil
}

// GetPodResources converts workload resource specification to per-pod ResourceList
func GetPodResourceList(res *v1.WorkloadResource) (corev1.ResourceList, error) {
	if res == nil {
		return nil, fmt.Errorf("the input resource is empty")
	}
	result, err := quantity.CvtToResourceList(res.CPU, res.Memory, res.GPU,
		res.GPUName, res.EphemeralStorage, res.RdmaResource, 1)
	if err != nil {
		return nil, commonerrors.NewBadRequest(err.Error())
	}
	return result, nil
}

// toPodResourceLists converts workload resources to a list of resource lists
func toPodResourceLists(workload *v1.Workload) ([]corev1.ResourceList, error) {
	result := make([]corev1.ResourceList, len(workload.Spec.Resources))
	for i, res := range workload.Spec.Resources {
		var err error
		if result[i], err = GetPodResourceList(&res); err != nil {
			return nil, err
		}
	}
	return result, nil
}

// GetScope determines the workspace scope based on workload kind.
func GetScope(w *v1.Workload) v1.WorkspaceScope {
	switch w.SpecKind() {
	case common.PytorchJobKind, common.UnifiedJobKind, common.JobKind, common.TorchFTKind, common.MonarchJob:
		return v1.TrainScope
	case common.DeploymentKind, common.StatefulSetKind, common.DynamoDeploymentKind, common.OptimusDeploymentKind:
		return v1.InferScope
	case common.AuthoringKind:
		return v1.AuthoringScope
	case common.CICDScaleRunnerSetKind, common.CICDEphemeralRunnerKind:
		return v1.CICDScope
	case common.RayJobKind:
		return v1.RayScope
	case common.SandboxKind:
		return v1.SandboxScope
	default:
		return ""
	}
}

// IsApplication returns true if the workload follows the "create-once + sync update"
// lifecycle handled by syncWorkloadToObject (Deployment, StatefulSet, DynamoDeployment).
// DynamoDeployment is included because its underlying DGD CR is reconciled by the
// Dynamo operator and supports in-place spec updates without re-creation.
func IsApplication(w *v1.Workload) bool {
	if w.SpecKind() == common.DeploymentKind ||
		w.SpecKind() == common.StatefulSetKind ||
		w.SpecKind() == common.DynamoDeploymentKind ||
		w.SpecKind() == common.OptimusDeploymentKind {
		return true
	}
	return false
}

// IsAuthoring returns true if the workload is authoring type.
func IsAuthoring(w *v1.Workload) bool {
	if w.SpecKind() == common.AuthoringKind {
		return true
	}
	return false
}

// IsCICD returns true if workload is about cicd
func IsCICD(w *v1.Workload) bool {
	if w.SpecKind() == common.CICDScaleRunnerSetKind || w.SpecKind() == common.CICDEphemeralRunnerKind {
		return true
	}
	return false
}

// IsCICDScalingRunnerSet returns true if the workload is an AutoscalingRunnerSet type.
func IsCICDScalingRunnerSet(w *v1.Workload) bool {
	if w.SpecKind() == common.CICDScaleRunnerSetKind {
		return true
	}
	return false
}

// IsCICDEphemeralRunner returns true if the workload is an EphemeralRunner type.
func IsCICDEphemeralRunner(w *v1.Workload) bool {
	if w.SpecKind() == common.CICDEphemeralRunnerKind {
		return true
	}
	return false
}

// IsTorchFT returns true if the workload is an TorchFT type.
func IsTorchFT(w *v1.Workload) bool {
	if w.SpecKind() == common.TorchFTKind {
		return true
	}
	return false
}

// IsRayJob returns true if the workload is an RayJob type.
func IsRayJob(w *v1.Workload) bool {
	if w.SpecKind() == common.RayJobKind {
		return true
	}
	return false
}

// IsDynamoDeployment returns true if the workload is a DynamoDeployment type.
// DynamoDeployment wraps the upstream DynamoGraphDeployment (DGD) CR managed by
// the Dynamo operator. See plan Phase 2 for the full design.
func IsDynamoDeployment(w *v1.Workload) bool {
	return w.SpecKind() == common.DynamoDeploymentKind
}

// GetDynamoServiceRoles returns the parsed service roles for a DynamoDeployment.
// Each element corresponds positionally to one Workload.Spec.Resources entry.
//
// Source of truth:
//  1. annotation primus-safe.dynamo.service-roles (comma separated, e.g. "frontend,prefill,decode,planner")
//  2. fallback inference based on len(Resources):
//     - 2 -> ["frontend", "worker"]              (aggregated minimal)
//     - 3 -> ["frontend", "worker", "planner"]   (aggregated + planner)
//     - other counts -> nil (the webhook should reject these; defensive return)
//
// Returns nil for non-DynamoDeployment workloads.
func GetDynamoServiceRoles(w *v1.Workload) []string {
	if !IsDynamoDeployment(w) {
		return nil
	}
	val := v1.GetAnnotation(w, v1.DynamoServiceRolesAnnotation)
	if val != "" {
		roles := make([]string, 0, 4)
		for _, r := range strings.Split(val, ",") {
			if r = strings.TrimSpace(r); r != "" {
				roles = append(roles, r)
			}
		}
		return roles
	}
	switch len(w.Spec.Resources) {
	case 2:
		return []string{common.DynamoRoleFrontend, common.DynamoRoleWorker}
	case 3:
		return []string{common.DynamoRoleFrontend, common.DynamoRoleWorker, common.DynamoRolePlanner}
	default:
		return nil
	}
}

// GetDynamoKVTransferBackend returns the KV transfer backend for disaggregated
// serving (nixl / mori / mooncake). Returns the default (nixl) when annotation
// is missing or empty.
func GetDynamoKVTransferBackend(w *v1.Workload) string {
	val := v1.GetAnnotation(w, v1.DynamoKVTransferBackendAnnotation)
	if val == "" {
		return common.DynamoDefaultKVBackend
	}
	return val
}

// GetDynamoMultinodeRoles returns the set of service roles that run as a
// multi-node LeaderWorkerSet, parsed from the multinode-roles annotation
// (comma-separated). For a role in this set the node count is its
// Resources[i].Replica. Returns nil when the annotation is absent.
func GetDynamoMultinodeRoles(w *v1.Workload) []string {
	val := v1.GetAnnotation(w, v1.DynamoMultinodeRolesAnnotation)
	if val == "" {
		return nil
	}
	roles := make([]string, 0, 2)
	for _, r := range strings.Split(val, ",") {
		if r = strings.TrimSpace(r); r != "" {
			roles = append(roles, r)
		}
	}
	return roles
}

// IsDynamoMultinodeRole reports whether the given role is configured to run as
// a multi-node LeaderWorkerSet. When true, the role's Resources[i].Replica is
// the LeaderWorkerSet node count rather than a Deployment replica count.
func IsDynamoMultinodeRole(w *v1.Workload, role string) bool {
	for _, r := range GetDynamoMultinodeRoles(w) {
		if r == role {
			return true
		}
	}
	return false
}

// GetDynamoBackendFramework returns the chosen backend framework
// (sglang / vllm / trtllm). Returns the default (sglang) when annotation
// is missing or empty.
func GetDynamoBackendFramework(w *v1.Workload) string {
	val := v1.GetAnnotation(w, v1.DynamoBackendFrameworkAnnotation)
	if val == "" {
		return common.DynamoDefaultBackendFramework
	}
	return val
}

// IsOptimusDeployment returns true if the workload is an OptimusDeployment.
// OptimusDeployment renders a rocserve.amd.com/v1alpha1 RocServeDeployment
// reconciled by the standalone RocServe operator (the Optimus analogue of
// DynamoDeployment). Roles/KV-backend/framework reuse the Dynamo constants.
func IsOptimusDeployment(w *v1.Workload) bool {
	return w.SpecKind() == common.OptimusDeploymentKind
}

// GetOptimusServiceRoles returns the parsed service roles for an
// OptimusDeployment, positionally matching Workload.Spec.Resources.
// Source: annotation primus-safe.optimus.service-roles (comma-separated);
// fallback by len(Resources): 2 -> [frontend,worker], 3 -> [frontend,prefill,decode].
// Returns nil for non-OptimusDeployment workloads.
func GetOptimusServiceRoles(w *v1.Workload) []string {
	if !IsOptimusDeployment(w) {
		return nil
	}
	val := v1.GetAnnotation(w, v1.OptimusServiceRolesAnnotation)
	if val != "" {
		roles := make([]string, 0, 4)
		for _, r := range strings.Split(val, ",") {
			if r = strings.TrimSpace(r); r != "" {
				roles = append(roles, r)
			}
		}
		return roles
	}
	switch len(w.Spec.Resources) {
	case 2:
		return []string{common.DynamoRoleFrontend, common.DynamoRoleWorker}
	case 3:
		return []string{common.DynamoRoleFrontend, common.DynamoRolePrefill, common.DynamoRoleDecode}
	default:
		return nil
	}
}

// GetOptimusKVTransferBackend returns the KV transfer backend (nixl/mori/
// mooncake) for disaggregated serving; default nixl when annotation absent.
func GetOptimusKVTransferBackend(w *v1.Workload) string {
	val := v1.GetAnnotation(w, v1.OptimusKVTransferBackendAnnotation)
	if val == "" {
		return common.OptimusDefaultKVBackend
	}
	return val
}

// GetOptimusMultinodeRoles returns the roles that run as a multi-node
// LeaderWorkerSet (node count = that role's Resources[i].Replica).
func GetOptimusMultinodeRoles(w *v1.Workload) []string {
	val := v1.GetAnnotation(w, v1.OptimusMultinodeRolesAnnotation)
	if val == "" {
		return nil
	}
	roles := make([]string, 0, 2)
	for _, r := range strings.Split(val, ",") {
		if r = strings.TrimSpace(r); r != "" {
			roles = append(roles, r)
		}
	}
	return roles
}

// IsOptimusMultinodeRole reports whether the given role runs as a multi-node
// LeaderWorkerSet.
func IsOptimusMultinodeRole(w *v1.Workload, role string) bool {
	for _, r := range GetOptimusMultinodeRoles(w) {
		if r == role {
			return true
		}
	}
	return false
}

// GetOptimusBackendFramework returns the chosen backend framework
// (sglang/vllm); default sglang when annotation absent.
func GetOptimusBackendFramework(w *v1.Workload) string {
	val := v1.GetAnnotation(w, v1.OptimusBackendFrameworkAnnotation)
	if val == "" {
		return common.OptimusDefaultBackendFramework
	}
	return val
}

func IsMonarchJob(w *v1.Workload) bool {
	if w.SpecKind() == common.MonarchJob {
		return true
	}
	return false
}

func IsMonarchMesh(w *v1.Workload) bool {
	if w.SpecKind() == common.MonarchMesh {
		return true
	}
	return false
}

func IsSandBox(w *v1.Workload) bool {
	if w.SpecKind() == common.SandboxKind {
		return true
	}
	return false
}

// IsOpsJob returns true if the workload is about ops job
func IsOpsJob(w *v1.Workload) bool {
	return v1.GetOpsJobId(w) != ""
}

// IsResourceEqual compares the resource specifications of two workloads.
// Returns true if both workloads have the same resource including replica counts
func IsResourceEqual(workload1, workload2 *v1.Workload) bool {
	if GetTotalReplica(workload1) != GetTotalReplica(workload2) ||
		len(workload1.Spec.Resources) != len(workload2.Spec.Resources) {
		return false
	}
	resourceList1, err1 := GetTotalResourceList(workload1)
	if err1 != nil {
		return false
	}
	resourceList2, err2 := GetTotalResourceList(workload2)
	if err2 != nil {
		return false
	}
	return quantity.Equal(resourceList1, resourceList2)
}

// GenerateDispatchReason generates a dispatch reason string based on count.
func GenerateDispatchReason(count int) string {
	return "run_" + strconv.Itoa(count) + "_times"
}

// GeneratePriorityClass generates priority class name for a workload.
func GeneratePriorityClass(workload *v1.Workload) string {
	clusterId := v1.GetClusterId(workload)
	strPriority := GeneratePriority(workload.Spec.Priority)
	return commonutils.GenerateClusterPriorityClass(clusterId, strPriority)
}

// GeneratePriority converts integer priority to string representation.
func GeneratePriority(priority int) string {
	strPriority := ""
	switch priority {
	case common.HighPriorityInt:
		strPriority = common.HighPriority
	case common.MedPriorityInt:
		strPriority = common.MedPriority
	default:
		strPriority = common.LowPriority
	}
	return strPriority
}

// GetWorkloadTemplate retrieves the ConfigMap template for a workload based on its version and kind.
// Note that this GVK must be a workload GVK.
func GetWorkloadTemplate(ctx context.Context, cli client.Client, gvk schema.GroupVersionKind) (*corev1.ConfigMap, error) {
	selector := labels.SelectorFromSet(map[string]string{
		v1.WorkloadVersionLabel: gvk.Version, v1.WorkloadKindLabel: gvk.Kind})
	listOptions := &client.ListOptions{LabelSelector: selector, Namespace: common.PrimusSafeNamespace}
	configmapList := &corev1.ConfigMapList{}
	if err := cli.List(ctx, configmapList, listOptions); err != nil {
		return nil, err
	}
	if len(configmapList.Items) > 0 {
		return &configmapList.Items[0], nil
	}
	return nil, commonerrors.NewInternalError(
		fmt.Sprintf("failed to find workload template. gvk: %s", gvk.String()))
}

// GetResourceTemplate Retrieve the corresponding resource_template based on the workload's GVK.
// For TorchFT or Monarch workload: cannot be looked up directly because it corresponds to multiple objects
// For other workloads: the workload GVK can be used directly to find the resource template
// (PyTorchJob and Deployment), so the template lookup needs to be handled differently
func GetResourceTemplate(ctx context.Context, cli client.Client, workload *v1.Workload) (*v1.ResourceTemplate, error) {
	return GetResourceTemplateByGVK(ctx, cli, workload.ToSchemaGVK())
}

// GetResourceTemplateByGVK Retrieve the corresponding resource_template based on the specified GVK.
// Note that the GetResourceTemplate function mentioned above is primarily used and this is specific to TorchFT.
func GetResourceTemplateByGVK(ctx context.Context, cli client.Client, gvk schema.GroupVersionKind) (*v1.ResourceTemplate, error) {
	templateList := &v1.ResourceTemplateList{}
	labelSelector := labels.SelectorFromSet(map[string]string{v1.WorkloadVersionLabel: gvk.Version})
	if err := cli.List(ctx, templateList, &client.ListOptions{LabelSelector: labelSelector}); err != nil {
		return nil, err
	}
	for i, item := range templateList.Items {
		kinds := strings.Split(v1.GetAnnotation(&item, v1.WorkloadKindLabel), ",")
		if sliceutil.Contains(kinds, gvk.Kind) {
			return &templateList.Items[i], nil
		}
	}
	return nil, commonerrors.NewInternalError(
		fmt.Sprintf("the resource template is not found, kind: %s, version: %s", gvk.Kind, gvk.Version))
}

// ConvertResourceToList converts a single workload resource to a list of workload resources
func ConvertResourceToList(workloadResource v1.WorkloadResource, kind string) []v1.WorkloadResource {
	if workloadResource.Replica <= 0 {
		return nil
	}
	result := make([]v1.WorkloadResource, 0, 2)
	if kind == common.PytorchJobKind || kind == common.UnifiedJobKind {
		result = append(result, workloadResource)
		result[0].Replica = 1
		if workloadResource.Replica > 1 {
			result = append(result, workloadResource)
			result[1].Replica = workloadResource.Replica - 1
		}
	} else {
		result = append(result, workloadResource)
	}
	return result
}

// GetReplicaCount retrieves the replica group count from the workload's environment variables.
// The replica group is used for torchFT workload.
// Returns an error if the environment variable is not set or cannot be converted to a valid integer.
func GetReplicaCount(workload *v1.Workload, key string) (int, error) {
	val, ok := workload.Spec.Env[key]
	if !ok || val == "" {
		return 0, fmt.Errorf("the %s of workload environment variables is empty", key)
	}
	n, err := strconv.Atoi(val)
	if err != nil {
		return 0, err
	}
	return n, nil
}

// GetWorkloadGVK returns the GroupVersionKind(s) for a workload
// For TorchFT workloads: returns multiple GVKs since TorchFT consists of multiple resource types
//   - PyTorchJob GVK for the training job components
//   - Deployment GVK for the lighthouse deployment component
//
// For MonarhchJob workloads: returns multiple GVKs since Moranch consists of multiple resource types
//   - MonarchMesh GVK for the training job components
//   - MonarchClient GVK for the client component
//
// For other workloads: returns the single GVK specified in the workload spec
func GetWorkloadGVK(workload *v1.Workload) []schema.GroupVersionKind {
	result := make([]schema.GroupVersionKind, 0, 2)
	if IsTorchFT(workload) {
		result = append(result, schema.GroupVersionKind{
			Group: "kubeflow.org", Version: common.DefaultVersion, Kind: common.PytorchJobKind,
		})
		result = append(result, schema.GroupVersionKind{
			Group: "apps", Version: common.DefaultVersion, Kind: common.DeploymentKind,
		})
	} else if IsMonarchJob(workload) {
		result = append(result, schema.GroupVersionKind{
			Group: "", Version: common.DefaultVersion, Kind: common.MonarchClient,
		})
		result = append(result, MonarchMeshWorkloadGVK())
	} else {
		result = append(result, workload.ToSchemaGVK())
	}
	return result
}

func MonarchMeshWorkloadGVK() schema.GroupVersionKind {
	return schema.GroupVersionKind{
		Group: "monarch.pytorch.org", Version: common.DefaultVersion, Kind: common.MonarchMesh,
	}
}

// SetMainContainerViaTemplate retrieves and sets the main container name for a workload
// Returns false if the workload is TorchFT or already has a main container annotation
// Otherwise, fetches the workload template and sets the main container annotation
func SetMainContainerViaTemplate(ctx context.Context, cli client.Client, workload *v1.Workload) bool {
	if IsTorchFT(workload) || v1.GetMainContainer(workload) != "" {
		return false
	}
	cm, err := GetWorkloadTemplate(ctx, cli, workload.ToSchemaGVK())
	if err == nil {
		v1.SetAnnotation(workload, v1.MainContainerAnnotation, v1.GetMainContainer(cm))
	}
	return true
}

// GetUsedHostPorts returns a set of all host ports currently in use by workloads in the specified cluster.
// It collects: (1) JobPort and SSHPort from RDMA workloads (hostNetwork pods); (2) Service.NodePort;
// (3) GcsServer and Dashboard Port for RayJob; (4) DynamoFrontendPort for DGD when any worker uses RDMA
// (Frontend is promoted to hostNetwork in IsEnabledHostNetwork to dodge the same-node Pod->hostIP hairpin).
// The returned map acts as a set where keys are port numbers and values are empty structs.
func GetUsedHostPorts(ctx context.Context, cli client.Client, clusterId string) map[int]struct{} {
	ports := make(map[int]struct{})
	workloadList := &v1.WorkloadList{}
	labelSelector := labels.SelectorFromSet(map[string]string{v1.ClusterIdLabel: clusterId})
	if cli.List(ctx, workloadList, &client.ListOptions{LabelSelector: labelSelector}) == nil {
		for _, item := range workloadList.Items {
			if item.HasHostNetwork() {
				if item.Spec.JobPort > 0 {
					ports[item.Spec.JobPort] = struct{}{}
				}
				if IsRayJob(&item) {
					ports[common.RayJobDashboardPort] = struct{}{}
					ports[common.RayJobGcsServerPort] = struct{}{}
					ports[common.RayJobMetricsPort] = struct{}{}
				}
				if IsMonarchJob(&item) {
					ports[common.MonarchMeshPortNum] = struct{}{}
				}
				if IsDynamoDeployment(&item) || IsOptimusDeployment(&item) {
					ports[common.DynamoFrontendPort] = struct{}{}
				}
			}
			if item.Spec.Service != nil && item.Spec.Service.NodePort > 0 {
				ports[item.Spec.Service.NodePort] = struct{}{}
			}
		}
	}
	return ports
}

// GetSpecifiedNodes retrieves the list of nodes specified for the workload to run on.
func GetSpecifiedNodes(workload *v1.Workload) []string {
	keys := []string{common.SpecifiedNodes, v1.K8sHostName}
	for _, key := range keys {
		if val, _ := workload.Spec.CustomerLabels[key]; val != "" {
			return strings.Split(val, " ")
		}
	}
	return nil
}

func IsEnabledHostNetwork(workload *v1.Workload, resourceId int) bool {
	if v1.IsForceHostNetwork(workload) {
		return true
	}
	if resourceId >= len(workload.Spec.Resources) || resourceId < 0 {
		return false
	}
	// RayJob submitterPodTemplate is resource index 0; headGroupSpec is index 1.
	// When the head uses hostNetwork (non-empty RdmaResource), keep the submitter Job
	// on hostNetwork too so a co-scheduled submitter can reach the Ray Dashboard without
	// same-node Pod-CIDR -> node-primary-IP hairpin timeouts to :8265.
	if IsRayJob(workload) && resourceId == 0 && len(workload.Spec.Resources) > 1 {
		if workload.Spec.Resources[1].RdmaResource != "" {
			return true
		}
	}
	// DGD: Resources[0] is the Frontend service; Resources[1..N-1] are workers
	// (Worker / PrefillWorker / DecodeWorker / Planner / Epp). When any worker uses
	// hostNetwork (non-empty RdmaResource), keep the Frontend on hostNetwork too so a
	// co-scheduled Frontend can reach worker hostIPs without same-node Pod-CIDR ->
	// node-primary-IP hairpin timeouts on dynamo TCP request plane ports.
	if (IsDynamoDeployment(workload) || IsOptimusDeployment(workload)) && resourceId == 0 && len(workload.Spec.Resources) > 1 {
		for i := 1; i < len(workload.Spec.Resources); i++ {
			if workload.Spec.Resources[i].RdmaResource != "" {
				return true
			}
		}
	}
	return workload.Spec.Resources[resourceId].RdmaResource != ""
}