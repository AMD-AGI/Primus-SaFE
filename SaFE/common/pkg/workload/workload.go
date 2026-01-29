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

	result := map[string]corev1.ResourceList{}
	for _, pod := range workload.Status.Pods {
		if !v1.IsPodRunning(&pod) {
			continue
		}
		if adminNodeName != "" && adminNodeName != pod.AdminNodeName {
			continue
		}
		resList, ok := result[pod.AdminNodeName]
		if ok {
			result[pod.AdminNodeName] = quantity.AddResource(resList, allPodResources[pod.ResourceId])
		} else {
			result[pod.AdminNodeName] = allPodResources[pod.ResourceId]
		}
	}
	return result, nil
}

// GetWorkloadResourceUsage retrieves active resources based on the input workload.
// It filters out terminated pods and applies node filtering criteria.
func GetWorkloadResourceUsage(workload *v1.Workload, filterNode func(nodeName string) bool) (
	corev1.ResourceList, corev1.ResourceList, []string, error) {
	if GetTotalReplica(workload) == 0 || len(workload.Status.Pods) == 0 {
		return nil, nil, nil, nil
	}
	allPodResources, err := toPodResourceLists(workload)
	if err != nil {
		return nil, nil, nil, err
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
		outputs[in.id].resourceId = in.pod.ResourceId
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
		totalResource = quantity.AddResource(totalResource, allPodResources[resourceId])
		if !outputs[i].isFiltered {
			availableResource = quantity.AddResource(availableResource, allPodResources[resourceId])
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
	case common.PytorchJobKind, common.UnifiedJobKind, common.JobKind, common.TorchFTKind:
		return v1.TrainScope
	case common.DeploymentKind, common.StatefulSetKind:
		return v1.InferScope
	case common.AuthoringKind:
		return v1.AuthoringScope
	case common.CICDScaleRunnerSetKind, common.CICDEphemeralRunnerKind:
		return v1.CICDScope
	case common.RayJobKind:
		return v1.RayScope
	default:
		return ""
	}
}

// IsApplication returns true if the workload is an application type (Deployment or StatefulSet).
func IsApplication(w *v1.Workload) bool {
	if w.SpecKind() == common.DeploymentKind ||
		w.SpecKind() == common.StatefulSetKind {
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
		fmt.Sprintf("failed to find configMap. gvk: %s", gvk.String()))
}

// GetResourceTemplate Retrieve the corresponding resource_template based on the workload's GVK.
// For non-TorchFT workloads: the workload GVK can be used directly to find the resource template
// For TorchFT workloads: cannot be looked up directly because TorchFT corresponds to multiple objects
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

// GetReplicaGroup retrieves the replica process group number from the workload's environment variables.
// The replica process group is used for torchFT workload.
// Returns an error if the environment variable is not set or cannot be converted to a valid integer.
func GetReplicaGroup(workload *v1.Workload, key string) (int, error) {
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
	} else {
		result = append(result, workload.ToSchemaGVK())
	}
	return result
}

// GetWorkloadMainContainer retrieves and sets the main container name for a workload
// Returns false if the workload is TorchFT or already has a main container annotation
// Otherwise, fetches the workload template and sets the main container annotation
func GetWorkloadMainContainer(ctx context.Context, cli client.Client, workload *v1.Workload) bool {
	if IsTorchFT(workload) || v1.GetMainContainer(workload) != "" {
		return false
	}
	cm, err := GetWorkloadTemplate(ctx, cli, workload.ToSchemaGVK())
	if err == nil {
		v1.SetAnnotation(workload, v1.MainContainerAnnotation, v1.GetMainContainer(cm))
	}
	return true
}
