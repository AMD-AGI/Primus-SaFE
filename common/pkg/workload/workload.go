/*
 * Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
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
	commonerrors "github.com/AMD-AIG-AIMA/SAFE/common/pkg/errors"
	commonnodes "github.com/AMD-AIG-AIMA/SAFE/common/pkg/nodes"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/quantity"
	commonutils "github.com/AMD-AIG-AIMA/SAFE/common/pkg/utils"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/concurrent"
)

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

// Statistics of the resources requested by a workload on each node
// If the input nodeName is not empty, only resources on the specified node are counted.
func GetResourcesPerNode(workload *v1.Workload, adminNodeName string) (map[string]corev1.ResourceList, error) {
	if workload.Spec.Resource.Replica == 0 {
		return nil, nil
	}
	podResources, err := GetPodResources(workload)
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

// Returns the total resource consumption of the workload, filtering out stopped pods and applying node-level filters as specified.
func GetActiveResources(workload *v1.Workload, filterNode func(nodeName string) bool) (corev1.ResourceList, error) {
	if workload.Spec.Resource.Replica == 0 || len(workload.Status.Pods) == 0 {
		return nil, nil
	}
	podResources, err := GetPodResources(workload)
	if err != nil {
		return nil, err
	}

	type podWrapper struct {
		i   int
		pod *v1.WorkloadPod
	}
	count := len(workload.Status.Pods)
	podUsedResources := make([]*corev1.ResourceList, count)
	ch := make(chan *podWrapper, count)
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
		return nil, err
	}
	result := make(corev1.ResourceList)
	for i := range podUsedResources {
		if podUsedResources[i] == nil {
			continue
		}
		result = quantity.AddResource(result, *podUsedResources[i])
	}
	return result, nil
}

func CvtToResourceList(w *v1.Workload) (corev1.ResourceList, error) {
	res := &w.Spec.Resource
	result, err := quantity.CvtToResourceList(res.CPU, res.Memory, res.GPU,
		res.GPUName, res.EphemeralStorage, res.RdmaResource, int64(res.Replica))
	if err != nil {
		return nil, commonerrors.NewBadRequest(err.Error())
	}
	return result, nil
}

func GetPodResources(w *v1.Workload) (corev1.ResourceList, error) {
	res := &w.Spec.Resource
	result, err := quantity.CvtToResourceList(res.CPU, res.Memory, res.GPU,
		res.GPUName, res.EphemeralStorage, res.RdmaResource, 1)
	if err != nil {
		return nil, commonerrors.NewBadRequest(err.Error())
	}
	return result, nil
}

func GetScope(w *v1.Workload) v1.WorkspaceScope {
	switch w.SpecKind() {
	case common.PytorchJobKind:
		return v1.TrainScope
	case common.DeploymentKind, common.StatefulSetKind:
		return v1.InferScope
	case common.AuthoringKind:
		return v1.AuthoringScope
	default:
		return ""
	}
}

func IsApplication(w *v1.Workload) bool {
	if w.SpecKind() == common.DeploymentKind ||
		w.SpecKind() == common.StatefulSetKind {
		return true
	}
	return false
}

func IsJob(w *v1.Workload) bool {
	if w.SpecKind() == common.PytorchJobKind || w.SpecKind() == common.AuthoringKind {
		return true
	}
	return false
}

func IsAuthoring(w *v1.Workload) bool {
	if w.SpecKind() == common.AuthoringKind {
		return true
	}
	return false
}

func IsOpsJob(w *v1.Workload) bool {
	return v1.GetOpsJobId(w) != ""
}

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

func GenerateDispatchReason(count int) string {
	return "run_" + strconv.Itoa(count) + "_times"
}

func GeneratePriorityClass(workload *v1.Workload) string {
	clusterId := v1.GetClusterId(workload)
	strPriority := ""
	switch workload.Spec.Priority {
	case common.HighPriorityInt:
		strPriority = common.HighPriority
	case common.MedPriorityInt:
		strPriority = common.MedPriority
	default:
		strPriority = common.LowPriority
	}
	return commonutils.GeneratePriorityClass(clusterId, strPriority)
}

func GetFailedMessage(workload *v1.Workload) string {
	switch workload.Status.Phase {
	case v1.WorkloadFailed:
		for _, pod := range workload.Status.Pods {
			if pod.FailedMessage == nil {
				continue
			}
			return pod.FailedMessage.Message
		}
		return "unknown failed reason"
	case v1.WorkloadStopped:
		return "workload is stopped"
	case v1.WorkloadSucceeded:
		return ""
	default:
		if !workload.GetDeletionTimestamp().IsZero() {
			return "workload is stopped"
		}
		return ""
	}
}
