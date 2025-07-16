/*
 * Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package ops_job

import (
	"context"
	"encoding/json"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	commonnodes "github.com/AMD-AIG-AIMA/SAFE/common/pkg/nodes"
)

type OpsJobInput struct {
	Commands     []OpsJobCommand `json:"commands"`
	DispatchTime int64           `json:"dispatchTime"`
}

type OpsJobCommand struct {
	// the addon name
	Addon string `json:"addon"`
	// the command to be executed by nodeAgent (base64-encoded).
	// Note that only one job command is allowed per node at any given time.
	Action string `json:"action,omitempty"`
	// the command to check the action execution status (base64 encoded).
	// Return 0 on success, 1 on failure.
	Observe string `json:"observe,omitempty"`
	// Determines whether the command should be registered as a service in systemd
	IsSystemd bool `json:"isSystemd,omitempty"`
	// If it is a One-shot Service, the reload operation is not applicable.
	IsOneShotService bool `json:"isOneShotService,omitempty"`
	// target gpu chip(amd or nvidia), If left empty, it applies to all chip.
	GpuChip v1.GpuChipType `json:"gpuChip,omitempty"`
	// target GPU product(case-sensitive), such as the MI300X, If left empty, it applies to all product.
	GpuProduct v1.GpuChipProduct `json:"gpuProduct,omitempty"`
}

func GetOpsJobInput(obj metav1.Object) *OpsJobInput {
	val := v1.GetOpsJobInput(obj)
	if val == "" {
		return nil
	}
	var jobInput OpsJobInput
	if err := json.Unmarshal([]byte(val), &jobInput); err != nil {
		return nil
	}
	return &jobInput
}

func CleanupJobRelatedInfo(ctx context.Context, cli client.Client, opsJobId string) error {
	labelSelector := labels.SelectorFromSet(map[string]string{v1.OpsJobIdLabel: opsJobId})

	nodeList := &v1.NodeList{}
	if err := cli.List(ctx, nodeList, &client.ListOptions{LabelSelector: labelSelector}); err != nil {
		return err
	}
	for _, adminNode := range nodeList.Items {
		patch := client.MergeFrom(adminNode.DeepCopy())
		nodesLabelAction := commonnodes.BuildAction(v1.NodeActionRemove, v1.OpsJobIdLabel, v1.OpsJobTypeLabel)
		nodesAnnotationAction := commonnodes.BuildAction(v1.NodeActionRemove, v1.OpsJobInputAnnotation)
		metav1.SetMetaDataAnnotation(&adminNode.ObjectMeta, v1.NodeLabelAction, nodesLabelAction)
		metav1.SetMetaDataAnnotation(&adminNode.ObjectMeta, v1.NodeAnnotationAction, nodesAnnotationAction)
		if err := cli.Patch(ctx, &adminNode, patch); err != nil {
			klog.ErrorS(err, "failed to patch node")
			return err
		}
	}

	faultList := &v1.FaultList{}
	if err := cli.List(ctx, faultList, &client.ListOptions{LabelSelector: labelSelector}); err != nil {
		return err
	}
	for _, fault := range faultList.Items {
		if err := cli.Delete(ctx, &fault); err != nil {
			klog.Infof("delete addon fault, id: %s", fault.Name)
		}
	}
	return nil
}
