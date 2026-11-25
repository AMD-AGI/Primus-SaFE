/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package workload

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
)

func TestFixedHostPorts(t *testing.T) {
	ports := FixedHostPorts()
	for _, port := range []int{common.RayJobGcsServerPort, common.RayJobDashboardPort, common.RayJobMetricsPort,
		common.MonarchMeshPortNum, common.DynamoFrontendPort} {
		assert.Contains(t, ports, port)
		assert.NotEmpty(t, FixedHostPortKinds(port), "port %d", port)
	}
	ports[1] = struct{}{}
	assert.NotContains(t, FixedHostPorts(), 1, "each call returns a new set")
	assert.Nil(t, FixedHostPortKinds(12345))
	assert.Equal(t, []string{common.MonarchJob, common.MonarchMesh}, FixedHostPortKinds(common.MonarchMeshPortNum))
}
