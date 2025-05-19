/*
 * Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package workload

import (
	"context"
	"fmt"
	"strconv"

	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/concurrent"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	commonerrors "github.com/AMD-AIG-AIMA/SAFE/common/pkg/errors"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/quantity"
)

func GetTotalCount(w *v1.Workload) int {
	n := 0
	for _, res := range w.Spec.Resources {
		n += res.Replica
	}
	return n
}

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

func GetTemplateConfig(ctx context.Context, cli client.Client, kind, resourceName string) (*corev1.ConfigMap, error) {
	if resourceName == "" {
		resourceName = common.AmdGpu
	}
	selector := labels.SelectorFromSet(map[string]string{v1.KindLabel: kind})
	listOptions := &client.ListOptions{LabelSelector: selector, Namespace: common.PrimusSafeNamespace}
	configmapList := &corev1.ConfigMapList{}
	if err := cli.List(ctx, configmapList, listOptions); err != nil {
		return nil, err
	}
	for i := range configmapList.Items {
		if v1.GetGpuResourceName(&configmapList.Items[i]) == resourceName {
			return &configmapList.Items[i], nil
		}
	}
	return nil, commonerrors.NewInternalError(
		fmt.Sprintf("fail to find configmap. kind: %s, resourceName: %s", kind, resourceName))
}

// Statistics of the resources requested by a workload on each node
// If the input nodeName is not empty, only resources on the specified node are counted.
func GetResourcePerNode(workload *v1.Workload, adminNodeName string) (map[string]corev1.ResourceList, error) {
	if len(workload.Spec.Resources) == 0 {
		return nil, nil
	}
	p := &workload.Spec.Resources[0]
	podResource, err := quantity.CvtToResourceList(p.CPU, p.Memory, p.GPU, p.GPUName, p.EphemeralStorage, 1)
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
			result[pod.AdminNodeName] = quantity.AddResource(resList, podResource)
		} else {
			result[pod.AdminNodeName] = podResource
		}
	}
	return result, nil
}

// Returns the total resource consumption of the workload, filtering out stopped pods and applying node-level filters as specified.
func GetActiveResource(workload *v1.Workload, filterNode func(nodeName string) bool) (corev1.ResourceList, error) {
	if len(workload.Spec.Resources) == 0 || len(workload.Status.Pods) == 0 {
		return nil, nil
	}
	p := &workload.Spec.Resources[0]
	podResource, err := quantity.CvtToResourceList(p.CPU, p.Memory, p.GPU, p.GPUName, p.EphemeralStorage, 1)
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
		podUsedResources[wrapper.i] = &podResource
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
	var result corev1.ResourceList
	for _, res := range w.Spec.Resources {
		rl, err := quantity.CvtToResourceList(res.CPU, res.Memory, res.GPU,
			res.GPUName, res.EphemeralStorage, int64(res.Replica))
		if err != nil {
			return nil, err
		}
		result = quantity.AddResource(result, rl)
	}
	return result, nil
}

func GetScope(w *v1.Workload) v1.WorkspaceScope {
	switch w.Spec.Kind {
	case common.PytorchJobKind:
		return v1.TrainScope
	case common.DeploymentKind, common.StatefulSetKind:
		return v1.InferScope
	default:
		return ""
	}
}

func IsApplication(w *v1.Workload) bool {
	if w.Spec.Kind == common.DeploymentKind || w.Spec.Kind == common.StatefulSetKind {
		return true
	}
	return false
}

func IsResourceEqual(workload1, workload2 *v1.Workload) bool {
	if len(workload1.Spec.Resources) != len(workload2.Spec.Resources) {
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

func GenerateCondReason(count int) string {
	return "run_" + strconv.Itoa(count) + "_times"
}
