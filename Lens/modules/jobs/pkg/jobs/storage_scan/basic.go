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

// HealthLevel 简化的健康状态。
type HealthLevel string

const (
	HealthOK       HealthLevel = "ok"
	HealthDegraded HealthLevel = "degraded"
	HealthWarn     HealthLevel = "warn"
	HealthError    HealthLevel = "error"
	HealthUnknown  HealthLevel = "unknown"
)

// CapacityInfo 统一容量/配额统计。
type CapacityInfo struct {
	TotalBytes    *int64 `json:"totalBytes,omitempty"`
	UsedBytes     *int64 `json:"usedBytes,omitempty"`
	FreeBytes     *int64 `json:"freeBytes,omitempty"`
	InodesTotal   *int64 `json:"inodesTotal,omitempty"`
	InodesUsed    *int64 `json:"inodesUsed,omitempty"`
	IOPSCapacity  *int64 `json:"iopsCapacity,omitempty"`  // 可选
	BWCapacityBps *int64 `json:"bwCapacityBps,omitempty"` // 可选
}

// MountPoint 描述一次挂载/使用关系（PVC/Pod/Node）。
type MountPoint struct {
	Namespace  string `json:"namespace"`
	PVC        string `json:"pvc"`
	PV         string `json:"pv"`
	Pod        string `json:"pod,omitempty"`
	Node       string `json:"node,omitempty"`
	AccessMode string `json:"accessMode,omitempty"`
}

// LeakOrphan 描述泄漏资源（如孤儿 PV、陈旧挂载）。
type LeakOrphan struct {
	Kind   string `json:"kind"` // e.g. "OrphanPV", "StaleMount"
	Name   string `json:"name"`
	Detail string `json:"detail"`
}
type BackendReport struct {
	Cluster      string               `json:"cluster"`
	BackendKind  BackendKind          `json:"backendKind"`
	BackendName  string               `json:"backendName"` // 逻辑名，如 storageClass 或文件系统名
	Health       HealthLevel          `json:"health"`
	Capacity     CapacityInfo         `json:"capacity"`
	Mounts       []MountPoint         `json:"mounts,omitempty"`
	Leaks        []LeakOrphan         `json:"leaks,omitempty"`
	TopologyHint map[string]string    `json:"topologyHint,omitempty"` // zone/rack/fs-id 等
	Raw          map[string]any       `json:"raw,omitempty"`
	MetaSecret   types.NamespacedName `json:"metaSecret,omitempty"` // 可选，元数据/控制面访问 Secret
}

type DriverContext struct {
	Cluster string
	Kube    kubernetes.Interface
	Extra   map[string]string
}

type Driver interface {
	// Name 返回 Driver 名称（用于注册/日志）。
	Name() string
	// Detect 返回该后端在集群中的候选实体数量（>0 表示存在）。
	Detect(ctx context.Context, dctx DriverContext) (int, error)
	// ListBackends 列出可被扫描的逻辑后端（e.g. 多个文件系统/多个SC）。
	ListBackends(ctx context.Context, dctx DriverContext) ([]string, error)
	// Collect 对指定 backend 执行采集并返回 BackendReport（不需要填 Cluster 字段）。
	Collect(ctx context.Context, dctx DriverContext, backend string) (BackendReport, error)
}
