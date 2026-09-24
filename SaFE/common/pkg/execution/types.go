/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

// Package execution holds the client for the external capacity contract 0.2.1, the
// demand/plan/claim protocol SaFE speaks to an external execution controller.
//
// Field sets mirror the published schema exactly. Every wire object declares
// additionalProperties false, so an unknown or misspelled field is rejected rather than
// ignored, and required fields must marshal even at their zero value.
package execution

import (
	"fmt"
	"strings"
	"time"
)

// APIVersion is the contract version this client speaks. The server rejects any other.
const APIVersion = "safe-exec/v1alpha1"

// wireTimeLayout is the timestamp format the schema accepts: UTC, RFC3339, milliseconds.
// Go's default marshalling emits nanoseconds and a numeric offset, which fail the pattern.
const wireTimeLayout = "2006-01-02T15:04:05.000Z"

// Timestamp marshals as the contract timestamp format regardless of the source location.
type Timestamp struct {
	time.Time
}

// NewTimestamp truncates to the precision the wire format carries, so a value survives a
// marshal and unmarshal round trip unchanged.
func NewTimestamp(t time.Time) Timestamp {
	return Timestamp{Time: t.UTC().Truncate(time.Millisecond)}
}

// MarshalJSON renders the UTC millisecond form the schema pattern requires.
func (t Timestamp) MarshalJSON() ([]byte, error) {
	return []byte(`"` + t.UTC().Format(wireTimeLayout) + `"`), nil
}

// UnmarshalJSON accepts the wire form with or without the optional fractional part.
func (t *Timestamp) UnmarshalJSON(data []byte) error {
	raw := strings.Trim(string(data), `"`)
	if raw == "" || raw == "null" {
		return nil
	}
	parsed, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return fmt.Errorf("parse contract timestamp %q: %w", raw, err)
	}
	t.Time = parsed.UTC()
	return nil
}

// ResourceVector is the normalised resource request for a single unit. Every field is
// required on the wire, including the GPU descriptors on CPU-only units.
type ResourceVector struct {
	CPUMillis    int64  `json:"cpu_millis"`
	MemoryBytes  int64  `json:"memory_bytes"`
	ScratchBytes int64  `json:"scratch_bytes"`
	GPUResource  string `json:"gpu_resource"`
	GPUModel     string `json:"gpu_model"`
	GPUCount     int32  `json:"gpu_count"`
}

// Port is a host-network port reservation.
type Port struct {
	Protocol string `json:"protocol"`
	Port     int32  `json:"port"`
}

// PlacementConstraints carries the placement restrictions the contract supports today.
// Anything richer is rejected at admission rather than silently dropped here.
type PlacementConstraints struct {
	NodeSelector     map[string]string `json:"node_selector"`
	AllowedNodeNames []string          `json:"allowed_node_names"`
}

// DemandUnit is one Pod worth of demand. Replicas is fixed at one by the schema: a
// multi-replica workload expands into distinct units with distinct keys.
type DemandUnit struct {
	UnitKey           string               `json:"unit_key"`
	Replicas          int32                `json:"replicas"`
	Resources         ResourceVector       `json:"resources"`
	Ports             []Port               `json:"ports"`
	RequiredRuntimeS  int32                `json:"required_runtime_s"`
	// PIDLimit is the per-task process budget. It is not a ResourceVector field:
	// the provider records it on the hold and enforces it at Prepare. Zero means
	// "declared none" and is refused at Prepare, so the field is always sent.
	PIDLimit          int32                `json:"pid_limit"`
	ImageRef          string               `json:"image_ref"`
	ImageDigest       string               `json:"image_digest,omitempty"`
	Constraints       PlacementConstraints `json:"constraints"`
	ConstraintsDigest string               `json:"constraints_digest"`
}

// Demand reasons. Only InsufficientCapacity asks the provider to acquire capacity; the
// rest explain why a workload waits without implying that more nodes would help.
const (
	ReasonInsufficientCapacity    = "InsufficientCapacity"
	ReasonImagePreparing          = "ImagePreparing"
	ReasonProfileUnvalidated      = "ProfileUnvalidated"
	ReasonConstraintUnsatisfiable = "ConstraintUnsatisfiable"
	ReasonWithdrawn               = "Withdrawn"
	ReasonDependencyNotReady      = "DependencyNotReady"
	ReasonScheduledForFuture      = "ScheduledForFuture"
	ReasonPaused                  = "Paused"
)

// CapacityDemand is the structured projection of one workload's unmet need.
type CapacityDemand struct {
	APIVersion               string       `json:"api_version"`
	RequestID                string       `json:"request_id"`
	DemandID                 string       `json:"demand_id"`
	Revision                 int32        `json:"revision"`
	WorkloadUID              string       `json:"workload_uid"`
	DispatchGeneration       int32        `json:"dispatch_generation"`
	ClusterID                string       `json:"cluster_id"`
	WorkspaceID              string       `json:"workspace_id"`
	ProfileID                string       `json:"profile_id"`
	ProfileRevision          int32        `json:"profile_revision"`
	QueueSnapshotRevision    string       `json:"queue_snapshot_revision"`
	CapacitySnapshotRevision string       `json:"capacity_snapshot_revision"`
	Eligible                 bool         `json:"eligible"`
	Reason                   string       `json:"reason"`
	Units                    []DemandUnit `json:"units"`
	ObservedAt               Timestamp    `json:"observed_at"`
	ExpiresAt                Timestamp    `json:"expires_at"`
}

// ClaimPlacement is one unit's approved seat: which allocation, which node and which
// devices. SaFE passes these back verbatim and never edits them.
type ClaimPlacement struct {
	UnitKey                    string         `json:"unit_key"`
	AllocationID               string         `json:"allocation_id"`
	AllocationGeneration       int32          `json:"allocation_generation"`
	ExpectedAllocationRevision int32          `json:"expected_allocation_revision"`
	NodeName                   string         `json:"node_name"`
	ImageRef                   string         `json:"image_ref"`
	ImageDigest                string         `json:"image_digest"`
	Resources                  ResourceVector `json:"resources"`
	// PIDLimit is copied from the demand unit through the plan onto the hold.
	PIDLimit                   int32          `json:"pid_limit"`
	DeviceIDs                  []string       `json:"device_ids"`
	Ports                      []Port         `json:"ports"`
}

// PlacementPlanRequest asks for candidate placements. The plan reserves nothing.
type PlacementPlanRequest struct {
	APIVersion         string `json:"api_version"`
	RequestID          string `json:"request_id"`
	DemandID           string `json:"demand_id"`
	DemandRevision     int32  `json:"demand_revision"`
	WorkloadUID        string `json:"workload_uid"`
	DispatchGeneration int32  `json:"dispatch_generation"`
	WorkspaceID        string `json:"workspace_id"`
}

// PlacementPlanResponse returns candidates that another claim may take first.
type PlacementPlanResponse struct {
	APIVersion     string           `json:"api_version"`
	RequestID      string           `json:"request_id"`
	DemandID       string           `json:"demand_id"`
	DemandRevision int32            `json:"demand_revision"`
	Placements     []ClaimPlacement `json:"placements"`
}

// ClaimRequest converts a plan into a reservation. It is all or nothing across units.
type ClaimRequest struct {
	APIVersion         string           `json:"api_version"`
	RequestID          string           `json:"request_id"`
	ClaimID            string           `json:"claim_id"`
	WorkloadUID        string           `json:"workload_uid"`
	DispatchGeneration int32            `json:"dispatch_generation"`
	DemandID           string           `json:"demand_id"`
	DemandRevision     int32            `json:"demand_revision"`
	WorkspaceID        string           `json:"workspace_id"`
	Placements         []ClaimPlacement `json:"placements"`
}

// Claim phases. Revoking records that a release was accepted; it does not mean the
// physical resource is free, so the reservation stays charged until Released.
const (
	ClaimPhaseActive   = "Active"
	ClaimPhaseRevoking = "Revoking"
	ClaimPhaseReleased = "Released"
)

// ClaimResponse is the reservation as the provider currently sees it.
type ClaimResponse struct {
	APIVersion         string           `json:"api_version"`
	RequestID          string           `json:"request_id"`
	ClaimID            string           `json:"claim_id"`
	Revision           int32            `json:"revision"`
	Phase              string           `json:"phase"`
	WorkloadUID        string           `json:"workload_uid"`
	DispatchGeneration int32            `json:"dispatch_generation"`
	ClusterID          string           `json:"cluster_id"`
	WorkspaceID        string           `json:"workspace_id"`
	ExpiresAt          Timestamp        `json:"expires_at"`
	Placements         []ClaimPlacement `json:"placements"`
}

// IsActive reports whether the claim may still back a dispatch.
func (c *ClaimResponse) IsActive() bool {
	return c != nil && c.Phase == ClaimPhaseActive
}

// ReleaseRequest withdraws a reservation. The response usually reports Revoking.
type ReleaseRequest struct {
	APIVersion         string `json:"api_version"`
	RequestID          string `json:"request_id"`
	ExpectedRevision   int32  `json:"expected_revision"`
	DispatchGeneration int32  `json:"dispatch_generation"`
	Reason             string `json:"reason"`
}
