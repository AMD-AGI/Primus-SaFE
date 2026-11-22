/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package usage

import (
	"sort"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client/model"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/qos"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/quantity"
	commonworkload "github.com/AMD-AIG-AIMA/SAFE/common/pkg/workload"
)

// The reasons a queued request is split by.
const (
	ReasonQuota    = "queued_quota"
	ReasonUserQos  = "queued_user_qos"
	ReasonExternal = "queued_external"
	ReasonOther    = "queued_other"
)

// DefaultResource is the resource the usage statistics describe unless asked otherwise.
const DefaultResource = common.AmdGpu

// Pending reasons that job-manager writes to a workload's status. Its module is not shared with
// resource-manager and the apiserver, so the prefixes are repeated here.
const (
	insufficientReasonPrefix = "In queue - insufficient resources"
	earlierWorkloadsPrefix   = "In queue - waiting for earlier or higher priority workloads"
)

// gpuResources are the GPU resource names a workspace quota and a workload request can hold.
var gpuResources = []string{common.AmdGpu, common.NvidiaGpu}

// IsGPUResource reports whether name is a GPU resource.
func IsGPUResource(name string) bool {
	for _, gpu := range gpuResources {
		if name == gpu {
			return true
		}
	}
	return false
}

// isTracked reports whether name is a resource the statistics keep.
func isTracked(name string) bool {
	return name == string(corev1.ResourceCPU) || name == string(corev1.ResourceMemory) || IsGPUResource(name)
}

// ClassifyQueueReason maps the pending reason of a workload to the queued column it counts in.
func ClassifyQueueReason(message string) string {
	switch {
	case qos.IsUserQuotaReason(message):
		return ReasonUserQos
	case commonworkload.IsExternalWaitReason(message):
		return ReasonExternal
	case strings.HasPrefix(message, insufficientReasonPrefix), strings.HasPrefix(message, earlierWorkloadsPrefix):
		return ReasonQuota
	default:
		return ReasonOther
	}
}

// pendingReason is the status message, or the message of the latest AdminScheduling condition
// when the status has none.
func pendingReason(w *v1.Workload) string {
	if w.Status.Message != "" {
		return w.Status.Message
	}
	for i := len(w.Status.Conditions) - 1; i >= 0; i-- {
		if w.Status.Conditions[i].Type == string(v1.AdminScheduling) {
			return w.Status.Conditions[i].Message
		}
	}
	return ""
}

// waitsForNodes reports whether the unbound part of a workload counts as queued.
func waitsForNodes(w *v1.Workload) bool {
	return w.IsPending() || w.Status.Phase == v1.WorkloadNotReady
}

// WorkloadUsage is what one unfinished workload holds on nodes and what it still waits for,
// by resource name.
type WorkloadUsage struct {
	Bound  map[string]float64
	Queued map[string]float64
	// Reason is the queued column of Queued, empty when nothing is queued.
	Reason string
}

// ComputeWorkload returns the usage of a workload, or nil when it has finished. Bound counts the
// requests of the pods on a node that have not terminated. Queued is the rest of the request of a
// workload that is pending or not ready; a running workload queues nothing.
func ComputeWorkload(w *v1.Workload) (*WorkloadUsage, error) {
	if w == nil || w.IsEnd() {
		return nil, nil
	}
	perNode, err := commonworkload.GetResourcesPerNode(w, "")
	if err != nil {
		return nil, err
	}
	bound := corev1.ResourceList{}
	for _, list := range perNode {
		bound = quantity.AddResource(bound, list)
	}
	result := &WorkloadUsage{Bound: toFloats(bound), Queued: map[string]float64{}}
	if !waitsForNodes(w) {
		return result, nil
	}
	total, err := commonworkload.GetTotalResourceList(w)
	if err != nil {
		return nil, err
	}
	for name, value := range toFloats(total) {
		if rest := value - result.Bound[name]; rest > 0 {
			result.Queued[name] = rest
		}
	}
	if len(result.Queued) > 0 {
		result.Reason = ClassifyQueueReason(pendingReason(w))
	}
	return result, nil
}

// toFloats keeps the tracked resources of a list as numbers: cores, bytes and cards.
func toFloats(list corev1.ResourceList) map[string]float64 {
	result := make(map[string]float64, len(list))
	for name, value := range list {
		if isTracked(string(name)) {
			result[string(name)] = value.AsApproximateFloat64()
		}
	}
	return result
}

// Key identifies the usage of one resource of one workspace.
type Key struct {
	Cluster   string
	Workspace string
	Resource  string
}

// GPUWorkload is a workload holding GPUs of one resource on its nodes.
type GPUWorkload struct {
	UID  string
	Key  Key
	GPUs float64
}

// Snapshot is the usage of every workspace at one collector tick.
type Snapshot struct {
	Rows map[Key]*model.WorkspaceUsageSample
	// GPUWorkloads by cluster, whose utilization decides busy.
	GPUWorkloads map[string][]GPUWorkload
	// Skipped counts the workloads whose request could not be read.
	Skipped int
}

// NewSnapshot computes the usage rows of every workspace at sampleTime. Every workspace has a
// cpu, memory and default GPU row, and a row for any other GPU resource it has quota, allocation
// or a queued request of. Workloads of a workspace that is not listed are left out.
func NewSnapshot(sampleTime time.Time, workspaces []v1.Workspace, workloads []v1.Workload) *Snapshot {
	s := &Snapshot{
		Rows:         map[Key]*model.WorkspaceUsageSample{},
		GPUWorkloads: map[string][]GPUWorkload{},
	}
	clusters := make(map[string]string, len(workspaces))
	for i := range workspaces {
		ws := &workspaces[i]
		clusters[ws.Name] = ws.Spec.Cluster
		for _, name := range []string{string(corev1.ResourceCPU), string(corev1.ResourceMemory), DefaultResource} {
			s.row(sampleTime, Key{Cluster: ws.Spec.Cluster, Workspace: ws.Name, Resource: name})
		}
		for name, value := range toFloats(ws.Status.TotalResources) {
			s.row(sampleTime, Key{Cluster: ws.Spec.Cluster, Workspace: ws.Name, Resource: name}).Quota = value
		}
	}
	for i := range workloads {
		w := &workloads[i]
		cluster, ok := clusters[w.Spec.Workspace]
		if !ok {
			continue
		}
		u, err := ComputeWorkload(w)
		if err != nil {
			s.Skipped++
			continue
		}
		if u == nil {
			continue
		}
		for name, value := range u.Bound {
			if value <= 0 {
				continue
			}
			key := Key{Cluster: cluster, Workspace: w.Spec.Workspace, Resource: name}
			row := s.row(sampleTime, key)
			row.Allocated += value
			row.RunningWorkloads++
			if IsGPUResource(name) {
				s.GPUWorkloads[cluster] = append(s.GPUWorkloads[cluster], GPUWorkload{UID: string(w.UID), Key: key, GPUs: value})
			}
		}
		for name, value := range u.Queued {
			row := s.row(sampleTime, Key{Cluster: cluster, Workspace: w.Spec.Workspace, Resource: name})
			addQueued(row, u.Reason, value)
			row.QueuedWorkloads++
		}
	}
	return s
}

// row returns the row of key, created empty on first use.
func (s *Snapshot) row(sampleTime time.Time, key Key) *model.WorkspaceUsageSample {
	if row, ok := s.Rows[key]; ok {
		return row
	}
	row := &model.WorkspaceUsageSample{
		SampleTime: sampleTime,
		Cluster:    key.Cluster,
		Workspace:  key.Workspace,
		Resource:   key.Resource,
	}
	s.Rows[key] = row
	return row
}

// addQueued adds value to the queued column of reason.
func addQueued(row *model.WorkspaceUsageSample, reason string, value float64) {
	switch reason {
	case ReasonQuota:
		row.QueuedQuota += value
	case ReasonUserQos:
		row.QueuedUserQos += value
	case ReasonExternal:
		row.QueuedExternal += value
	default:
		row.QueuedOther += value
	}
}

// Clusters returns the clusters that have rows, sorted.
func (s *Snapshot) Clusters() []string {
	seen := map[string]bool{}
	var result []string
	for key := range s.Rows {
		if !seen[key.Cluster] {
			seen[key.Cluster] = true
			result = append(result, key.Cluster)
		}
	}
	sort.Strings(result)
	return result
}

// GPUWorkloadUIDs returns the distinct uids of the workloads holding GPUs in cluster.
func (s *Snapshot) GPUWorkloadUIDs(cluster string) []string {
	seen := map[string]bool{}
	var result []string
	for _, g := range s.GPUWorkloads[cluster] {
		if !seen[g.UID] {
			seen[g.UID] = true
			result = append(result, g.UID)
		}
	}
	return result
}

// ApplyGPUUtilization sets busy and coverage on the GPU rows of cluster. utilization holds the
// average utilization (percent) of each workload uid that has data. A nil map means Robust could
// not be asked. A row without allocated GPUs is not busy and has no coverage. A row whose
// allocated GPUs have no data at all has an unknown busy, never a busy of zero.
func (s *Snapshot) ApplyGPUUtilization(cluster string, utilization map[string]float64) {
	busy := map[Key]float64{}
	covered := map[Key]float64{}
	for _, g := range s.GPUWorkloads[cluster] {
		if value, ok := utilization[g.UID]; ok {
			busy[g.Key] += g.GPUs * value / 100
			covered[g.Key] += g.GPUs
		}
	}
	for key, row := range s.Rows {
		if key.Cluster != cluster || !IsGPUResource(key.Resource) {
			continue
		}
		row.Busy, row.UtilCoverage = nil, nil
		switch {
		case row.Allocated <= 0:
			row.Busy = float64Ptr(0)
		case utilization == nil:
		case covered[key] <= 0:
			row.UtilCoverage = float64Ptr(0)
		default:
			row.Busy = float64Ptr(busy[key])
			row.UtilCoverage = float64Ptr(min(1, covered[key]/row.Allocated))
		}
	}
}

// SortedRows returns the rows ordered by cluster, workspace and resource.
func (s *Snapshot) SortedRows() []*model.WorkspaceUsageSample {
	result := make([]*model.WorkspaceUsageSample, 0, len(s.Rows))
	for _, row := range s.Rows {
		result = append(result, row)
	}
	sort.Slice(result, func(i, j int) bool {
		a, b := result[i], result[j]
		if a.Cluster != b.Cluster {
			return a.Cluster < b.Cluster
		}
		if a.Workspace != b.Workspace {
			return a.Workspace < b.Workspace
		}
		return a.Resource < b.Resource
	})
	return result
}

func float64Ptr(v float64) *float64 {
	return &v
}
