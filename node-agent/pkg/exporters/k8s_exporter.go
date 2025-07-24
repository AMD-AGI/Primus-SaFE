/*
 * Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package exporters

import (
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"

	commonfaults "github.com/AMD-AIG-AIMA/SAFE/common/pkg/faults"
	"github.com/AMD-AIG-AIMA/SAFE/node-agent/pkg/node"
	"github.com/AMD-AIG-AIMA/SAFE/node-agent/pkg/types"
)

type K8sExporter struct {
	node *node.Node
}

func (ke *K8sExporter) Handle(msg *types.MonitorMessage) error {
	var conditions []corev1.NodeCondition
	var isChanged bool
	if msg.StatusCode != types.StatusError {
		conditions, isChanged = genDeleteConditions(ke.node.GetK8sNode(), msg)
	} else {
		conditions, isChanged = genAddConditions(ke.node.GetK8sNode(), msg)
	}
	if !isChanged {
		return nil
	}
	err := ke.node.UpdateConditions(conditions)
	if err == nil {
		klog.Infof("Update conditions for node %s successfully", ke.node.GetK8sNode().Name)
	}
	return err
}

func (ke *K8sExporter) Name() string {
	return "k8sExporter"
}

func genAddConditions(node *corev1.Node, msg *types.MonitorMessage) ([]corev1.NodeCondition, bool) {
	results := make([]corev1.NodeCondition, 0, len(node.Status.Conditions)+1)
	isFound := false
	key := commonfaults.GenerateTaintKey(msg.Id)
	for _, cond := range node.Status.Conditions {
		if string(cond.Type) == key {
			if cond.Status == corev1.ConditionTrue {
				return nil, false
			}
			cond.Status = corev1.ConditionTrue
			cond.Message = msg.Value
			cond.LastTransitionTime = metav1.NewTime(time.Now().UTC())
			isFound = true
		}
		results = append(results, cond)
	}
	if !isFound {
		results = append(results, corev1.NodeCondition{
			Type:               corev1.NodeConditionType(key),
			Status:             corev1.ConditionTrue,
			LastTransitionTime: metav1.NewTime(time.Now().UTC()),
			Message:            msg.Value,
		})
	}
	klog.Infof("gen add condition. key: %s, message: %s", key, msg.Value)
	return results, true
}

func genDeleteConditions(node *corev1.Node, msg *types.MonitorMessage) ([]corev1.NodeCondition, bool) {
	results := make([]corev1.NodeCondition, 0, len(node.Status.Conditions))
	key := commonfaults.GenerateTaintKey(msg.Id)
	for i, cond := range node.Status.Conditions {
		if string(cond.Type) != key {
			results = append(results, node.Status.Conditions[i])
		} else {
			klog.Infof("gen deleting condition. key: %s, message: %s", cond.Type, cond.Message)
		}
	}
	if len(results) == len(node.Status.Conditions) {
		return nil, false
	}
	return results, true
}
