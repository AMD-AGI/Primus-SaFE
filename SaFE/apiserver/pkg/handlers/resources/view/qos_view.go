/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package view

import (
	"fmt"
	"strings"

	"k8s.io/apimachinery/pkg/api/resource"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	commonerrors "github.com/AMD-AIG-AIMA/SAFE/common/pkg/errors"
)

// QosResponse is one default ceiling.
type QosResponse struct {
	Name         string          `json:"name"`
	MaxResources *v1.ResourceCap `json:"maxResources,omitempty"`
	MaxRuntime   map[string]int  `json:"maxRuntime,omitempty"`
}

// ListQosResponse is every default ceiling.
type ListQosResponse struct {
	Items []QosResponse `json:"items"`
}

// PutQosRequest replaces one default ceiling. Empty fields clear that dimension.
type PutQosRequest struct {
	MaxResources *v1.ResourceCap `json:"maxResources,omitempty"`
	MaxRuntime   map[string]int  `json:"maxRuntime,omitempty"`
}

// NewQosResponse copies a Qos into the API response.
func NewQosResponse(qos *v1.Qos) QosResponse {
	if qos == nil {
		return QosResponse{}
	}
	return QosResponse{Name: qos.Name, MaxResources: qos.Spec.MaxResources, MaxRuntime: qos.Spec.MaxRuntime}
}

// NormalizeQosResources trims resource fields and stores a bare memory number as Gi.
// An empty cap is returned as nil so an update clears the resource ceiling.
func NormalizeQosResources(cap *v1.ResourceCap) *v1.ResourceCap {
	if cap == nil {
		return nil
	}
	normalized := &v1.ResourceCap{
		CPU:    strings.TrimSpace(cap.CPU),
		Memory: memoryAsGi(strings.TrimSpace(cap.Memory)),
		GPU:    strings.TrimSpace(cap.GPU),
	}
	if normalized.CPU == "" && normalized.Memory == "" && normalized.GPU == "" {
		return nil
	}
	return normalized
}

// memoryAsGi appends Gi when memory has no unit. A value that already has a unit is unchanged.
func memoryAsGi(value string) string {
	if value == "" {
		return ""
	}
	for _, r := range value {
		if (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') {
			return value
		}
	}
	return value + "Gi"
}

// ValidateQosLimits rejects a resource quantity or a non-positive timeout that cannot be enforced.
func ValidateQosLimits(cap *v1.ResourceCap, runtime map[string]int) error {
	if cap != nil {
		for _, item := range []struct{ name, value string }{
			{"cpu", cap.CPU}, {"memory", cap.Memory}, {"gpu", cap.GPU},
		} {
			if strings.TrimSpace(item.value) == "" {
				continue
			}
			quantity, err := resource.ParseQuantity(item.value)
			if err != nil || quantity.Sign() <= 0 {
				return commonerrors.NewBadRequest(fmt.Sprintf("invalid %s limit %q", item.name, item.value))
			}
		}
	}
	for scope, seconds := range runtime {
		if !knownQosScope(scope) || seconds <= 0 {
			return commonerrors.NewBadRequest(fmt.Sprintf("invalid timeout for scope %q", scope))
		}
	}
	return nil
}

// knownQosScope reports whether name is a workspace scope that can carry a timeout.
func knownQosScope(name string) bool {
	switch v1.WorkspaceScope(name) {
	case v1.TrainScope, v1.InferScope, v1.AuthoringScope, v1.CICDScope, v1.RayScope, v1.SandboxScope:
		return true
	default:
		return false
	}
}

// ValidateUserQos checks each per-workspace ceiling. Top-level fields are rejected.
func ValidateUserQos(qos *v1.UserQos) error {
	if qos == nil {
		return nil
	}
	if qos.MaxResources != nil || len(qos.MaxRuntime) > 0 {
		return commonerrors.NewBadRequest("a user qos is set per workspace in workspaces")
	}
	for workspaceID, scope := range qos.Workspaces {
		if strings.TrimSpace(workspaceID) == "" {
			return commonerrors.NewBadRequest("qos workspace id is required")
		}
		if err := ValidateQosLimits(scope.MaxResources, scope.MaxRuntime); err != nil {
			return err
		}
	}
	return nil
}
