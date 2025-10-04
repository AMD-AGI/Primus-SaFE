/*
 * Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package custom_handlers

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	commonutils "github.com/AMD-AIG-AIMA/SAFE/common/pkg/utils"
)

func genMockWorkspace(clusterId, nodeFlavorId string) *v1.Workspace {
	name := commonutils.GenerateName("workspace")
	return &v1.Workspace{
		ObjectMeta: metav1.ObjectMeta{
			Name: commonutils.GenerateName(name),
			Labels: map[string]string{
				v1.DisplayNameLabel: name,
				v1.ClusterIdLabel:   clusterId,
				v1.WorkspaceIdLabel: name,
			},
			Annotations: map[string]string{
				v1.DescriptionAnnotation: "test",
			},
		},
		Spec: v1.WorkspaceSpec{
			Cluster:     clusterId,
			Replica:     3,
			NodeFlavor:  nodeFlavorId,
			QueuePolicy: v1.QueueFifoPolicy,
		},
	}
}
