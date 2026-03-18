// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package clientsets

// ComponentType defines the type of component
type ComponentType int

const (
	// ComponentTypeDataPlane represents data plane components (exporters, jobs, etc.)
	// Data plane components only need access to the current cluster
	ComponentTypeDataPlane ComponentType = iota

	// ComponentTypeControlPlane represents control plane components (api-server, etc.)
	// Control plane components need access to all clusters
	ComponentTypeControlPlane
)

// String returns the string representation of ComponentType
func (ct ComponentType) String() string {
	switch ct {
	case ComponentTypeDataPlane:
		return "DataPlane"
	case ComponentTypeControlPlane:
		return "ControlPlane"
	default:
		return "Unknown"
	}
}

// IsControlPlane returns true if the component is a control plane component
func (ct ComponentType) IsControlPlane() bool {
	return ct == ComponentTypeControlPlane
}

// IsDataPlane returns true if the component is a data plane component
func (ct ComponentType) IsDataPlane() bool {
	return ct == ComponentTypeDataPlane
}

// ComponentDeclaration declares the component type and its dependencies
// Each component should define this in its main.go or bootstrap.go
type ComponentDeclaration struct {
	// Type specifies whether this is a control plane or data plane component
	Type ComponentType

	// RequireK8S indicates whether the component needs K8S client
	RequireK8S bool

	// RequireStorage indicates whether the component needs Storage client (DB, OpenSearch, Prometheus)
	RequireStorage bool
}

// DefaultControlPlaneDeclaration returns a default declaration for control plane components
func DefaultControlPlaneDeclaration() ComponentDeclaration {
	return ComponentDeclaration{
		Type:           ComponentTypeControlPlane,
		RequireK8S:     true,
		RequireStorage: true,
	}
}

// DefaultDataPlaneDeclaration returns a default declaration for data plane components
func DefaultDataPlaneDeclaration() ComponentDeclaration {
	return ComponentDeclaration{
		Type:           ComponentTypeDataPlane,
		RequireK8S:     true,
		RequireStorage: true,
	}
}
