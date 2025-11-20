/*
 * Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package workload

import (
	"context"
	"fmt"
	"strconv"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	commonconfig "github.com/AMD-AIG-AIMA/SAFE/common/pkg/config"
	commonerrors "github.com/AMD-AIG-AIMA/SAFE/common/pkg/errors"
	commonnodes "github.com/AMD-AIG-AIMA/SAFE/common/pkg/nodes"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/quantity"
	commonutils "github.com/AMD-AIG-AIMA/SAFE/common/pkg/utils"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/concurrent"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/floatutil"
)

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

// GetWorkloadTemplate retrieves the ConfigMap template for a workload based on its version and kind.
func GetWorkloadTemplate(ctx context.Context, cli client.Client, workload *v1.Workload) (*corev1.ConfigMap, error) {
	selector := labels.SelectorFromSet(map[string]string{
		v1.WorkloadVersionLabel: workload.SpecVersion(), v1.WorkloadKindLabel: workload.SpecKind()})
	listOptions := &client.ListOptions{LabelSelector: selector, Namespace: common.PrimusSafeNamespace}
	configmapList := &corev1.ConfigMapList{}
	if err := cli.List(ctx, configmapList, listOptions); err != nil {
		return nil, err
	}
	if len(configmapList.Items) > 0 {
		return &configmapList.Items[0], nil
	}
	return nil, commonerrors.NewInternalError(
		fmt.Sprintf("failed to find configmap. gvk: %s, resourceName: %s",
			workload.Spec.GroupVersionKind.VersionKind(), workload.Spec.Resource.GPUName))
}

// GetResourcesPerNode calculates resource usage per node for a workload.
func GetResourcesPerNode(workload *v1.Workload, adminNodeName string) (map[string]corev1.ResourceList, error) {
	if workload.Spec.Resource.Replica == 0 {
		return nil, nil
	}
	podResources, err := GetPodResources(&workload.Spec.Resource)
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
			result[pod.AdminNodeName] = quantity.AddResource(resList, podResources)
		} else {
			result[pod.AdminNodeName] = podResources
		}
	}
	return result, nil
}

// GetActiveResources retrieves active resources based on the input workload.
// It filters out terminated pods and applies node filtering criteria.
func GetActiveResources(workload *v1.Workload, filterNode func(nodeName string) bool) (corev1.ResourceList, []string, error) {
	if workload.Spec.Resource.Replica == 0 || len(workload.Status.Pods) == 0 {
		return nil, nil, nil
	}
	podResources, err := GetPodResources(&workload.Spec.Resource)
	if err != nil {
		return nil, nil, err
	}

	type podWrapper struct {
		i   int
		pod *v1.WorkloadPod
	}
	count := len(workload.Status.Pods)
	podUsedResources := make([]*corev1.ResourceList, count)
	ch := make(chan *podWrapper, count)
	defer close(ch)
	for i := range workload.Status.Pods {
		ch <- &podWrapper{
			i:   i,
			pod: &workload.Status.Pods[i],
		}
	}

	_, err = concurrent.Exec(count, func() error {
		wrapper := <-ch
		pod := wrapper.pod
		if !v1.IsPodRunning(pod) {
			return nil
		}
		if filterNode != nil && filterNode(pod.AdminNodeName) {
			return nil
		}
		podUsedResources[wrapper.i] = &podResources
		return nil
	})
	if err != nil {
		return nil, nil, err
	}
	resources := make(corev1.ResourceList)
	nodes := make([]string, 0, count)
	for i := range podUsedResources {
		if podUsedResources[i] == nil {
			continue
		}
		resources = quantity.AddResource(resources, *podUsedResources[i])
		nodes = append(nodes, workload.Status.Pods[i].AdminNodeName)
	}
	return resources, nodes, nil
}

// CvtToResourceList converts data to the target format.
func CvtToResourceList(w *v1.Workload) (corev1.ResourceList, error) {
	res := &w.Spec.Resource
	result, err := quantity.CvtToResourceList(res.CPU, res.Memory, res.GPU,
		res.GPUName, res.EphemeralStorage, res.RdmaResource, int64(res.Replica))
	if err != nil {
		return nil, commonerrors.NewBadRequest(err.Error())
	}
	return result, nil
}

// GetPodResources converts workload resource specification to per-pod ResourceList.
func GetPodResources(res *v1.WorkloadResource) (corev1.ResourceList, error) {
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

// GetScope determines the workspace scope based on workload kind.
func GetScope(w *v1.Workload) v1.WorkspaceScope {
	switch w.SpecKind() {
	case common.PytorchJobKind:
		return v1.TrainScope
	case common.DeploymentKind, common.StatefulSetKind:
		return v1.InferScope
	case common.AuthoringKind:
		return v1.AuthoringScope
	case common.CICDScaleSetKind, common.CICDRunnerKind:
		return v1.CICDScope
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

// IsJob returns true if the workload is a job type (PyTorchJob, Authoring, or Job).
func IsJob(w *v1.Workload) bool {
	if w.SpecKind() == common.PytorchJobKind ||
		w.SpecKind() == common.AuthoringKind || w.SpecKind() == common.JobKind {
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
	if w.SpecKind() == common.CICDScaleSetKind || w.SpecKind() == common.CICDRunnerKind {
		return true
	}
	return false
}

// IsJob returns true if the workload is about ops job
func IsOpsJob(w *v1.Workload) bool {
	return v1.GetOpsJobId(w) != ""
}

// IsResourceEqual compares the resource specifications of two workloads.
// Returns true if both workloads have the same replica count and identical resource requirements,
// false otherwise or if there's an error during resource conversion.
func IsResourceEqual(workload1, workload2 *v1.Workload) bool {
	if workload1.Spec.Resource.Replica != workload2.Spec.Resource.Replica {
		return false
	}
	rl1, err1 := CvtToResourceList(workload1)
	if err1 != nil {
		return false
	}
	rl2, err2 := CvtToResourceList(workload2)
	if err2 != nil {
		return false
	}
	return quantity.Equal(rl1, rl2)
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

// GenerateMaxAvailResource generates maximum available resource for workload by NodeFlavor.
func GenerateMaxAvailResource(nf *v1.NodeFlavor) *v1.WorkloadResource {
	nodeResources := nf.ToResourceList(commonconfig.GetRdmaName())
	availResource := quantity.GetAvailableResource(nodeResources)
	if !floatutil.FloatEqual(commonconfig.GetMaxEphemeralStorePercent(), 0) {
		maxEphemeralStoreQuantity, _ := quantity.GetMaxEphemeralStoreQuantity(nodeResources)
		if maxEphemeralStoreQuantity != nil {
			availResource[corev1.ResourceEphemeralStorage] = *maxEphemeralStoreQuantity
		}
	}

	maxAvailCpu, _ := availResource[corev1.ResourceCPU]
	maxAvailMem, _ := availResource[corev1.ResourceMemory]
	maxAvailStorage, _ := quantity.GetMaxEphemeralStoreQuantity(nodeResources)
	result := &v1.WorkloadResource{
		CPU:              maxAvailCpu.String(),
		Memory:           quantity.ToString(maxAvailMem),
		EphemeralStorage: quantity.ToString(*maxAvailStorage),
	}
	if result.Memory == "" {
		result.Memory = "1Mi"
	}
	if result.EphemeralStorage == "" {
		result.EphemeralStorage = "1Mi"
	}
	if nf.HasGpu() {
		result.GPUName = nf.Spec.Gpu.ResourceName
		result.GPU = nf.Spec.Gpu.Quantity.String()
	}
	return result
}
