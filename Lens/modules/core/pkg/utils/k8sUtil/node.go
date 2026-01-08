// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package k8sUtil

import corev1 "k8s.io/api/core/v1"

const (
	NodeStatusReady    = "Ready"
	NodeStatusNotReady = "NotReady"
	NodeStatusUnknown  = "Unknown"
)

func NodeReady(node corev1.Node) bool {
	for _, condition := range node.Status.Conditions {
		if condition.Type == corev1.NodeReady && condition.Status == corev1.ConditionTrue {
			return true
		}
	}
	return false
}

func NodeStatus(node corev1.Node) string {
	for _, condition := range node.Status.Conditions {
		if condition.Type == corev1.NodeReady {
			switch condition.Status {
			case corev1.ConditionTrue:
				return NodeStatusReady
			case corev1.ConditionFalse:
				return NodeStatusNotReady
			case corev1.ConditionUnknown:
				return NodeStatusUnknown
			}
		}
	}
	return NodeStatusUnknown
}
