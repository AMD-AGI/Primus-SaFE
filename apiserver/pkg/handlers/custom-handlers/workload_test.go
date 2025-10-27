/*
 * Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package custom_handlers

import (
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	commonutils "github.com/AMD-AIG-AIMA/SAFE/common/pkg/utils"
)

func genMockWorkload(clusterId, workspaceId string) *v1.Workload {
	return &v1.Workload{
		TypeMeta: metav1.TypeMeta{
			Kind:       v1.WorkloadKind,
			APIVersion: v1.SchemeGroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: commonutils.GenerateName("workload"),
			Labels: map[string]string{
				v1.WorkspaceIdLabel: workspaceId,
				v1.ClusterIdLabel:   clusterId,
			},
			Annotations: map[string]string{
				v1.MainContainerAnnotation:      "main",
				v1.WorkloadDispatchedAnnotation: "",
			},
			CreationTimestamp: metav1.NewTime(time.Now()),
		},
		Spec: v1.WorkloadSpec{
			Workspace:  workspaceId,
			MaxRetry:   3,
			Priority:   1,
			Image:      "image",
			EntryPoint: "sh -c test.sh",
			GroupVersionKind: v1.GroupVersionKind{
				Group:   "kubeflow.org",
				Version: "v1",
				Kind:    "PyTorchJob",
			},
			Resource: v1.WorkloadResource{
				Replica: 2,
				CPU:     "16",
				GPU:     "4",
				GPUName: common.AmdGpu,
				Memory:  "1Gi",
			},
		},
		Status: v1.WorkloadStatus{
			Phase: v1.WorkloadRunning,
		},
	}
}
