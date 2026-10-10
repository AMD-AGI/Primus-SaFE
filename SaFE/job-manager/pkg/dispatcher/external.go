/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package dispatcher

import (
	"fmt"
	"strconv"
	"strings"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	commonworkload "github.com/AMD-AIG-AIMA/SAFE/common/pkg/workload"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// isExternalWorkload reports whether a workload was admitted against external capacity.
func isExternalWorkload(workload *v1.Workload) bool {
	if workload == nil || workload.Status.ExternalExecution == nil {
		return false
	}
	state := workload.Status.ExternalExecution
	if state.PlacementMode == v1.ExternalPlacementKubeScheduler {
		return true
	}
	return state.ClaimId != ""
}

// isKubeSchedulerPlacement reports whether the workload uses the kube-scheduler path.
func isKubeSchedulerPlacement(workload *v1.Workload) bool {
	return workload != nil && workload.Status.ExternalExecution != nil &&
		workload.Status.ExternalExecution.PlacementMode == v1.ExternalPlacementKubeScheduler
}

// inferaUsesK8sDiscovery reports whether an InferaDeployment registers workers
// through the Kubernetes API (needs a projected ServiceAccount token). Matches
// the operator: any discoveryBackend other than "etcd" (including unset).
func inferaUsesK8sDiscovery(workload *v1.Workload, obj *unstructured.Unstructured) bool {
	if workload == nil || !commonworkload.IsInferaDeployment(workload) {
		return false
	}
	if obj != nil {
		backend, found, err := unstructured.NestedString(obj.Object, "spec", "discoveryBackend")
		if err == nil && found && backend == "etcd" {
			return false
		}
	}
	return true
}

// isExternalGang reports whether an admitted external workload has the host-network RDMA
// gang shape.
func isExternalGang(workload *v1.Workload) bool {
	return isExternalWorkload(workload) && commonworkload.IsExternalRDMAGang(workload)
}

// externalHostNetworkEnabled reports whether an external pod template may use
// hostNetwork. ForceHostNetwork always wins. Otherwise whole-node RDMA gangs
// keep the privilege, and external Infera/Dynamo roles follow the same RDMA
// rules as IsEnabledHostNetwork.
func externalHostNetworkEnabled(workload *v1.Workload, resourceId int) bool {
	if workload != nil && v1.IsForceHostNetwork(workload) {
		return true
	}
	if isExternalGang(workload) {
		return true
	}
	if workload == nil || !isExternalWorkload(workload) {
		return false
	}
	if !commonworkload.IsInferaDeployment(workload) && !commonworkload.IsDynamoDeployment(workload) {
		return false
	}
	if resourceId < 0 || resourceId >= len(workload.Spec.Resources) {
		return false
	}
	// Frontend/role0 shares hostNetwork when any worker requests RDMA.
	if resourceId == 0 && len(workload.Spec.Resources) > 1 {
		for i := 1; i < len(workload.Spec.Resources); i++ {
			if workload.Spec.Resources[i].RdmaResource != "" {
				return true
			}
		}
	}
	return workload.Spec.Resources[resourceId].RdmaResource != ""
}

// externalRoleUnitKey returns the unit whose approval a role's pod template carries. Gang
// workers share one template, and the scheduler admits them only with identical approvals.
func externalRoleUnitKey(workload *v1.Workload, resourceId int) string {
	if resourceId > 0 && isExternalGang(workload) {
		return v1.ExternalWorkerUnitKeyPrefix + "0"
	}
	return v1.ExternalSingleUnitKey
}

// externalRoleNodes lists the approved nodes the pods of one role may bind to.
func externalRoleNodes(workload *v1.Workload, resourceId int) []string {
	if !isExternalGang(workload) {
		return externalApprovedNodes(workload)
	}
	wantWorker := resourceId > 0
	var names []string
	for _, p := range workload.Status.ExternalExecution.Placements {
		if p.NodeName != "" && strings.HasPrefix(p.UnitKey, v1.ExternalWorkerUnitKeyPrefix) == wantWorker {
			names = append(names, p.NodeName)
		}
	}
	return names
}

// externalGangProblem reports why the stored placements cannot back a gang dispatch, or ""
// when they can: one placement per unit, each on its own node, all with one vector and image.
func externalGangProblem(workload *v1.Workload) string {
	placements := workload.Status.ExternalExecution.Placements
	expected := 1 + workload.Spec.Resources[1].Replica
	if len(placements) != expected {
		return fmt.Sprintf("claim approved %d units, the gang has %d", len(placements), expected)
	}
	keys := make(map[string]struct{}, expected)
	keys[v1.ExternalSingleUnitKey] = struct{}{}
	for i := 0; i < expected-1; i++ {
		keys[fmt.Sprintf("%s%d", v1.ExternalWorkerUnitKeyPrefix, i)] = struct{}{}
	}
	nodes := make(map[string]struct{}, expected)
	first := placements[0]
	for _, p := range placements {
		if _, ok := keys[p.UnitKey]; !ok {
			return "claim approved unexpected or repeated unit " + p.UnitKey
		}
		delete(keys, p.UnitKey)
		if _, ok := nodes[p.NodeName]; ok || p.NodeName == "" {
			return fmt.Sprintf("claim approved node %q for unit %s, which is empty or shared", p.NodeName, p.UnitKey)
		}
		nodes[p.NodeName] = struct{}{}
		if p.ImageRef != first.ImageRef || p.CPUMillis != first.CPUMillis || p.MemoryBytes != first.MemoryBytes ||
			p.ScratchBytes != first.ScratchBytes || p.GPUResource != first.GPUResource || p.GPUCount != first.GPUCount {
			return "claim approved unit " + p.UnitKey + " differently from unit " + first.UnitKey
		}
	}
	return ""
}

// externalPodAnnotations are identifiers written onto the execution object.
func externalPodAnnotations(workload *v1.Workload) map[string]interface{} {
	state := workload.Status.ExternalExecution
	if state == nil {
		return nil
	}
	result := map[string]interface{}{
		v1.ExternalWorkloadUIDAnnotation: string(workload.UID),
		v1.ExternalDispatchGenAnnotation: strconv.Itoa(int(state.DispatchGeneration)),
	}
	if isExternalGang(workload) {
		result[v1.ExternalGangKeyAnnotation] = string(workload.UID)
	}
	return result
}

// findPlacement returns the approved seat for one unit.
func findPlacement(state *v1.WorkloadExternalExecution, unitKey string) *v1.WorkloadExternalPlacement {
	for i := range state.Placements {
		if state.Placements[i].UnitKey == unitKey {
			return &state.Placements[i]
		}
	}
	return nil
}

// externalApprovedNodes lists the virtual nodes a workload's pods may bind to.
func externalApprovedNodes(workload *v1.Workload) []string {
	state := workload.Status.ExternalExecution
	if state == nil {
		return nil
	}
	names := make([]string, 0, len(state.Placements))
	seen := make(map[string]struct{}, len(state.Placements))
	for i := range state.Placements {
		name := state.Placements[i].NodeName
		if name == "" {
			continue
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		names = append(names, name)
	}
	return names
}

// externalApprovedImage returns the image frozen on the stored placement, if any.
func externalApprovedImage(workload *v1.Workload, unitKey string) string {
	state := workload.Status.ExternalExecution
	if state == nil {
		return ""
	}
	if placement := findPlacement(state, unitKey); placement != nil {
		return placement.ImageRef
	}
	return ""
}
