package storage_scan

import (
	"context"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
)

type BackendKind string

const (
	BackendJuiceFS BackendKind = "juicefs"
	BackendWeka    BackendKind = "weka"
	BackendCeph    BackendKind = "ceph"
	BackendNFS     BackendKind = "nfs"
)

// HealthLevel represents simplified health status.
type HealthLevel string

const (
	HealthOK       HealthLevel = "ok"
	HealthDegraded HealthLevel = "degraded"
	HealthWarn     HealthLevel = "warn"
	HealthError    HealthLevel = "error"
	HealthUnknown  HealthLevel = "unknown"
)

// CapacityInfo represents unified capacity/quota statistics.
type CapacityInfo struct {
	TotalBytes    *int64 `json:"totalBytes,omitempty"`
	UsedBytes     *int64 `json:"usedBytes,omitempty"`
	FreeBytes     *int64 `json:"freeBytes,omitempty"`
	InodesTotal   *int64 `json:"inodesTotal,omitempty"`
	InodesUsed    *int64 `json:"inodesUsed,omitempty"`
	IOPSCapacity  *int64 `json:"iopsCapacity,omitempty"`  // Optional
	BWCapacityBps *int64 `json:"bwCapacityBps,omitempty"` // Optional
}

// MountPoint describes a mount/usage relationship (PVC/Pod/Node).
type MountPoint struct {
	Namespace  string `json:"namespace"`
	PVC        string `json:"pvc"`
	PV         string `json:"pv"`
	Pod        string `json:"pod,omitempty"`
	Node       string `json:"node,omitempty"`
	AccessMode string `json:"accessMode,omitempty"`
}

// LeakOrphan describes leaked resources (such as orphan PVs, stale mounts).
type LeakOrphan struct {
	Kind   string `json:"kind"` // e.g. "OrphanPV", "StaleMount"
	Name   string `json:"name"`
	Detail string `json:"detail"`
}
type BackendReport struct {
	Cluster      string               `json:"cluster"`
	BackendKind  BackendKind          `json:"backendKind"`
	BackendName  string               `json:"backendName"` // Logical name, such as storageClass or filesystem name
	Health       HealthLevel          `json:"health"`
	Capacity     CapacityInfo         `json:"capacity"`
	Mounts       []MountPoint         `json:"mounts,omitempty"`
	Leaks        []LeakOrphan         `json:"leaks,omitempty"`
	TopologyHint map[string]string    `json:"topologyHint,omitempty"` // zone/rack/fs-id, etc.
	Raw          map[string]any       `json:"raw,omitempty"`
	MetaSecret   types.NamespacedName `json:"metaSecret,omitempty"` // Optional, metadata/control plane access Secret
}

type DriverContext struct {
	Cluster string
	Kube    kubernetes.Interface
	Extra   map[string]string
}

type Driver interface {
	// Name returns the Driver name (used for registration/logging).
	Name() string
	// Detect returns the number of candidate entities of this backend in the cluster (>0 means exists).
	Detect(ctx context.Context, dctx DriverContext) (int, error)
	// ListBackends lists logical backends that can be scanned (e.g. multiple filesystems/multiple SCs).
	ListBackends(ctx context.Context, dctx DriverContext) ([]string, error)
	// Collect performs collection for specified backend and returns BackendReport (no need to fill Cluster field).
	Collect(ctx context.Context, dctx DriverContext, backend string) (BackendReport, error)
}
