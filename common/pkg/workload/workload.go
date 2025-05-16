/*
 * Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package workload

import (
	"context"
	"fmt"

	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	commonerrors "github.com/AMD-AIG-AIMA/SAFE/common/pkg/errors"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/quantity"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
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

func GetWorkloadTemplateConfig(ctx context.Context, cli client.Client, kind, resourceName string) (*corev1.ConfigMap, error) {
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

func IsApplication(w *v1.Workload) bool {
	if w.Spec.Kind == common.DeploymentKind || w.Spec.Kind == common.StatefulSetKind {
		return true
	}
	return false
}

func CvtToResourceList(resources []v1.WorkloadResource) (corev1.ResourceList, error) {
	var result corev1.ResourceList
	for _, res := range resources {
		rl, err := quantity.CvtToResourceList(res.CPU, res.Memory, res.GPU,
			res.GPUName, res.EphemeralStorage, int64(res.Replica))
		if err != nil {
			return nil, err
		}
		result = quantity.AddResource(result, rl)
	}
	return result, nil
}

func IsResourceListEqual(resources1, resources2 []v1.WorkloadResource) bool {
	if len(resources1) != len(resources2) {
		return false
	}
	rl1, err1 := CvtToResourceList(resources1)
	if err1 != nil {
		return false
	}
	rl2, err2 := CvtToResourceList(resources2)
	if err2 != nil {
		return false
	}
	return quantity.Equal(rl1, rl2)
}

func GetScope(kind string) v1.WorkspaceScope {
	switch kind {
	case common.PytorchJobKind:
		return v1.TrainScope
	case common.DeploymentKind, common.StatefulSetKind:
		return v1.InferScope
	default:
		return ""
	}
}
