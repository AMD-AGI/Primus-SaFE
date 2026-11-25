/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package workload

import (
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
)

// fixedHostPorts are the host ports SaFE's own hostNetwork workloads always bind on their nodes, with the kinds
// that bind each.
var fixedHostPorts = map[int][]string{
	common.RayJobGcsServerPort: {common.RayJobKind},
	common.RayJobDashboardPort: {common.RayJobKind},
	common.RayJobMetricsPort:   {common.RayJobKind},
	common.MonarchMeshPortNum:  {common.MonarchJob, common.MonarchMesh},
	common.DynamoFrontendPort:  {common.DynamoDeploymentKind, common.InferaDeploymentKind, common.AtomKind},
}

// FixedHostPorts returns, as a new set the caller may add to, the host ports SaFE's own hostNetwork workloads
// always bind on their nodes, which other hostNetwork workloads must not take.
func FixedHostPorts() map[int]struct{} {
	ports := make(map[int]struct{}, len(fixedHostPorts))
	for port := range fixedHostPorts {
		ports[port] = struct{}{}
	}
	return ports
}

// FixedHostPortKinds returns the workload kinds that bind port on their nodes, or nil when port is not one of
// SaFE's fixed host ports.
func FixedHostPortKinds(port int) []string {
	return fixedHostPorts[port]
}
