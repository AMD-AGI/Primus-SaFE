/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package workload

import (
	"sort"
	"testing"

	"gotest.tools/assert"
	corev1 "k8s.io/api/core/v1"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/quantity"
)

// mixedPodsWorkload returns a workload whose pods cover scheduled-running,
// scheduled-pending, scheduled-terminated and unscheduled cases across two
// resource groups, so the NodeUsage aggregate and the legacy Status.Pods path
// can be compared for equivalence.
func mixedPodsWorkload() *v1.Workload {
	return &v1.Workload{
		Spec: v1.WorkloadSpec{
			Resources: []v1.WorkloadResource{
				{CPU: "8", Memory: "10", Replica: 3},
				{CPU: "4", Memory: "6", Replica: 2},
			},
		},
		Status: v1.WorkloadStatus{
			Pods: []v1.WorkloadPod{
				{AdminNodeName: "n1", ResourceId: 0, Phase: corev1.PodRunning},
				{AdminNodeName: "n1", ResourceId: 1, Phase: corev1.PodPending},
				{AdminNodeName: "n2", ResourceId: 0, Phase: corev1.PodRunning},
				// Terminated pod shares n2 with an active pod so the unique-node
				// set is identical on both paths.
				{AdminNodeName: "n2", ResourceId: 0, Phase: corev1.PodSucceeded},
				// Unscheduled but active pod (no admin node yet).
				{AdminNodeName: "", ResourceId: 0, Phase: corev1.PodPending},
			},
		},
	}
}

func TestBuildNodeUsage(t *testing.T) {
	w := mixedPodsWorkload()
	usage := BuildNodeUsage(w)

	byNode := map[string]v1.NodePodUsage{}
	for _, u := range usage {
		byNode[u.Node] = u
	}
	// Terminated pod is excluded; three buckets remain: n1, n2, "".
	assert.Equal(t, len(usage), 3)
	assert.Equal(t, byNode["n1"].Active["0"], 1)
	assert.Equal(t, byNode["n1"].Active["1"], 1)
	assert.Equal(t, byNode["n1"].Running["0"], 1)
	assert.Equal(t, byNode["n1"].Running["1"], 0)
	assert.Equal(t, byNode["n2"].Active["0"], 1)
	assert.Equal(t, byNode["n2"].Running["0"], 1)
	assert.Equal(t, byNode[""].Active["0"], 1)
}

// TestNodeUsageDiscreteResourceIds covers a node holding pods of non-contiguous
// resourceIds (0 and 2, skipping 1). The per-node aggregate iterates only the
// resourceId map keys that exist, so the NodeUsage path must equal the legacy
// per-pod path even with discrete ids.
func TestNodeUsageDiscreteResourceIds(t *testing.T) {
	newWL := func() *v1.Workload {
		return &v1.Workload{
			Spec: v1.WorkloadSpec{
				Resources: []v1.WorkloadResource{
					{CPU: "8", Memory: "10", Replica: 2},  // rid 0
					{CPU: "16", Memory: "20", Replica: 1}, // rid 1 (unused by pods)
					{CPU: "4", Memory: "6", Replica: 1},   // rid 2
				},
			},
			Status: v1.WorkloadStatus{
				Pods: []v1.WorkloadPod{
					{AdminNodeName: "n1", ResourceId: 0, Phase: corev1.PodRunning},
					{AdminNodeName: "n1", ResourceId: 0, Phase: corev1.PodRunning},
					{AdminNodeName: "n1", ResourceId: 2, Phase: corev1.PodRunning},
				},
			},
		}
	}
	wPods := newWL()
	wUsage := newWL()
	wUsage.Status.NodeUsage = BuildNodeUsage(wUsage)

	// Sanity: aggregate keeps discrete rids on the same node.
	assert.Equal(t, wUsage.Status.NodeUsage[0].Active["0"], 2)
	assert.Equal(t, wUsage.Status.NodeUsage[0].Active["2"], 1)
	_, hasRid1 := wUsage.Status.NodeUsage[0].Active["1"]
	assert.Equal(t, hasRid1, false)

	rpnPods, err := GetResourcesPerNode(wPods, "")
	assert.NilError(t, err)
	rpnUsage, err := GetResourcesPerNode(wUsage, "")
	assert.NilError(t, err)
	// n1 = 2*rid0(8cpu) + 1*rid2(4cpu) = 20 cpu, rid1 not counted.
	n1Pods := rpnPods["n1"]
	assert.Equal(t, n1Pods.Cpu().Value(), int64(20))
	assert.Equal(t, quantity.Equal(rpnPods["n1"], rpnUsage["n1"]), true)

	totPods, _, _, err := GetWorkloadResourceUsage(wPods, nil)
	assert.NilError(t, err)
	totUsage, _, _, err := GetWorkloadResourceUsage(wUsage, nil)
	assert.NilError(t, err)
	assert.Equal(t, quantity.Equal(totPods, totUsage), true)
}

func sortedNodes(in []string) []string {
	out := append([]string(nil), in...)
	sort.Strings(out)
	return out
}

// TestNodeUsageDualReadEquivalence verifies the NodeUsage read path returns the
// same results as the legacy Status.Pods path. The guard prefers NodeUsage when
// present, so setting it (built from the same pods) must not change any result.
func TestNodeUsageDualReadEquivalence(t *testing.T) {
	filterNode := func(nodeName string) bool { return nodeName == "" } // filter unscheduled

	wPods := mixedPodsWorkload()
	wUsage := mixedPodsWorkload()
	wUsage.Status.NodeUsage = BuildNodeUsage(wUsage)

	// GetInUseNodeCount
	assert.Equal(t, GetInUseNodeCount(wPods), GetInUseNodeCount(wUsage))

	// GetResourcesPerNode
	rpnPods, err := GetResourcesPerNode(wPods, "")
	assert.NilError(t, err)
	rpnUsage, err := GetResourcesPerNode(wUsage, "")
	assert.NilError(t, err)
	assert.Equal(t, len(rpnPods), len(rpnUsage))
	for node, want := range rpnPods {
		assert.Equal(t, quantity.Equal(want, rpnUsage[node]), true)
	}

	// GetResourcesPerNode with a node filter.
	rpnPodsN1, err := GetResourcesPerNode(wPods, "n1")
	assert.NilError(t, err)
	rpnUsageN1, err := GetResourcesPerNode(wUsage, "n1")
	assert.NilError(t, err)
	assert.Equal(t, len(rpnPodsN1), len(rpnUsageN1))
	assert.Equal(t, quantity.Equal(rpnPodsN1["n1"], rpnUsageN1["n1"]), true)

	// GetWorkloadResourceUsage
	totPods, availPods, nodesPods, err := GetWorkloadResourceUsage(wPods, filterNode)
	assert.NilError(t, err)
	totUsage, availUsage, nodesUsage, err := GetWorkloadResourceUsage(wUsage, filterNode)
	assert.NilError(t, err)
	assert.Equal(t, quantity.Equal(totPods, totUsage), true)
	assert.Equal(t, quantity.Equal(availPods, availUsage), true)
	assert.DeepEqual(t, sortedNodes(uniqueStrings(nodesPods)), sortedNodes(uniqueStrings(nodesUsage)))
}

// TestGetInUseNodeCountExcludesTerminatedOnlyNode verifies a node holding only
// terminated pods is not counted (it no longer occupies the node), and that the
// Status.Pods and NodeUsage paths agree on this. mixedPodsWorkload does not cover
// it because there the terminated pod shares a node with an active one.
func TestGetInUseNodeCountExcludesTerminatedOnlyNode(t *testing.T) {
	w := &v1.Workload{Status: v1.WorkloadStatus{Pods: []v1.WorkloadPod{
		{AdminNodeName: "n1", Phase: corev1.PodRunning},
		{AdminNodeName: "n2", Phase: corev1.PodSucceeded}, // terminated-only node
		{AdminNodeName: "n3", Phase: corev1.PodFailed},    // terminated-only node
	}}}
	// Only n1 holds a non-terminated pod.
	assert.Equal(t, GetInUseNodeCount(w), 1)

	wUsage := w.DeepCopy()
	wUsage.Status.NodeUsage = BuildNodeUsage(wUsage)
	assert.Equal(t, GetInUseNodeCount(wUsage), 1)
	assert.Equal(t, GetInUseNodeCount(w), GetInUseNodeCount(wUsage))
}

// TestNodeUsageNodeSetEquivalence locks the scheduling-relevant equivalence that
// the node-set readers (nodes.GetIdleNodesOfWorkspace, ops_job.excludeBusyNodes,
// workspace webhook) depend on: the set of non-empty nodes in BuildNodeUsage
// equals the set of admin nodes over IsPodRunning (scheduled, non-terminated)
// pods. A node holding only terminated pods is excluded by both paths.
func TestNodeUsageNodeSetEquivalence(t *testing.T) {
	w := &v1.Workload{Status: v1.WorkloadStatus{Pods: []v1.WorkloadPod{
		{AdminNodeName: "n1", Phase: corev1.PodRunning},               // scheduled running
		{AdminNodeName: "n1", Phase: corev1.PodPending},               // scheduled pending (still "running" set)
		{AdminNodeName: "n2", Phase: corev1.PodRunning},               // scheduled running
		{AdminNodeName: "n2", Phase: corev1.PodSucceeded},             // terminated, shares n2
		{AdminNodeName: "n3", Phase: corev1.PodFailed},                // terminated-only node -> excluded
		{AdminNodeName: "", Phase: corev1.PodPending},                 // unscheduled -> no node
	}}}

	legacy := map[string]bool{}
	for i := range w.Status.Pods {
		if v1.IsPodRunning(&w.Status.Pods[i]) {
			legacy[w.Status.Pods[i].AdminNodeName] = true
		}
	}
	usage := map[string]bool{}
	for _, u := range BuildNodeUsage(w) {
		if u.Node != "" {
			usage[u.Node] = true
		}
	}
	// Both paths yield exactly {n1, n2}; n3 (terminated-only) and "" are excluded.
	assert.DeepEqual(t, legacy, usage)
	assert.Equal(t, len(usage), 2)
	assert.Equal(t, usage["n1"] && usage["n2"], true)
	assert.Equal(t, usage["n3"], false)
}

// uniqueStrings de-dupes (the available-node list may repeat a node per pod on
// the legacy path; callers only use the distinct set).
func uniqueStrings(in []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(in))
	for _, s := range in {
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	return out
}

func TestStripOffloadedStatus(t *testing.T) {
	// nil workload is a no-op.
	StripOffloadedStatus(nil)

	// Not offloaded (no NodeUsage) -> pods are kept.
	w := &v1.Workload{Status: v1.WorkloadStatus{
		Pods: []v1.WorkloadPod{{PodId: "p1"}},
	}}
	StripOffloadedStatus(w)
	assert.Equal(t, len(w.Status.Pods), 1)

	// Offloaded (NodeUsage present) -> large arrays are cleared.
	w = &v1.Workload{Status: v1.WorkloadStatus{
		NodeUsage: []v1.NodePodUsage{{Node: "n1"}},
		Pods:      []v1.WorkloadPod{{PodId: "p1", Phase: corev1.PodRunning}},
		Nodes:     [][]string{{"n1"}},
		Ranks:     [][]string{{"0"}},
	}}
	StripOffloadedStatus(w)
	assert.Equal(t, len(w.Status.NodeUsage), 1)
	assert.Assert(t, w.Status.Pods == nil)
	assert.Assert(t, w.Status.Nodes == nil)
	assert.Assert(t, w.Status.Ranks == nil)
}
