/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package syncer

import (
	"testing"

	"gotest.tools/assert"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
)

func TestPodWrapperSort(t *testing.T) {
	workload := &v1.Workload{
		Status: v1.WorkloadStatus{
			Pods: []v1.WorkloadPod{{
				HostIp: "127.0.0.1",
				PodId:  "p1",
			}, {
				HostIp: "127.0.0.2",
				PodId:  "p2",
			}, {
				HostIp: "127.0.0.1",
				PodId:  "p3",
			}},
		},
	}
	sortWorkloadPods(workload)
	assert.Equal(t, workload.Status.Pods[0].PodId, "p2")
	assert.Equal(t, workload.Status.Pods[1].PodId, "p1")
	assert.Equal(t, workload.Status.Pods[2].PodId, "p3")
}
