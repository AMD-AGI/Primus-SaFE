/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package view

import (
	"sort"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
)

// TestCreateWorkloadRequestGetNodesAffinity covers both the nil and set cases.
func TestCreateWorkloadRequestGetNodesAffinity(t *testing.T) {
	req := &CreateWorkloadRequest{}
	if got := req.GetNodesAffinity(); got != "" {
		t.Errorf("expected empty affinity, got %q", got)
	}
	affinity := "required"
	req.NodesAffinity = &affinity
	if got := req.GetNodesAffinity(); got != "required" {
		t.Errorf("expected required, got %q", got)
	}
}

func makeWorkload(name string, ts time.Time) v1.Workload {
	return v1.Workload{
		ObjectMeta: metav1.ObjectMeta{
			Name:              name,
			CreationTimestamp: metav1.NewTime(ts),
		},
	}
}

// TestWorkloadSliceSort verifies Len, Swap and Less via sort.Sort.
func TestWorkloadSliceSort(t *testing.T) {
	base := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	ws := WorkloadSlice{
		makeWorkload("b", base.Add(time.Hour)),
		makeWorkload("a", base),
		makeWorkload("c", base), // same time as "a", sorted by name
	}
	if ws.Len() != 3 {
		t.Fatalf("Len = %d, want 3", ws.Len())
	}

	sort.Sort(ws)

	if ws[0].Name != "a" || ws[1].Name != "c" || ws[2].Name != "b" {
		t.Errorf("unexpected order: %s, %s, %s", ws[0].Name, ws[1].Name, ws[2].Name)
	}

	// Direct Swap check.
	ws.Swap(0, 2)
	if ws[0].Name != "b" || ws[2].Name != "a" {
		t.Errorf("Swap failed: %s, %s", ws[0].Name, ws[2].Name)
	}

	// Less returns false when neither earlier-time nor smaller-name holds.
	if WorkloadSlice([]v1.Workload{makeWorkload("z", base.Add(time.Hour)), makeWorkload("a", base)}).Less(0, 1) {
		t.Error("expected Less to be false for later timestamp")
	}
}
