/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package v1

import (
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// fixedNow pins the clock so the freshness boundaries can be exercised exactly, and
// restores it afterwards so the package's other tests keep the real one.
func fixedNow(t *testing.T, now time.Time) {
	t.Helper()
	previous := nowFunc
	nowFunc = func() time.Time { return now }
	t.Cleanup(func() { nowFunc = previous })
}

func externalNode(observedAt, validUntil *metav1.Time) *Node {
	node := &Node{
		Spec: NodeSpec{
			LifecycleMode: NodeLifecycleExternal,
			ExternalRef: &NodeExternalRef{
				Provider:     "spur",
				AllocationId: "alloc-1",
				Generation:   1,
				HostKey:      "host-1",
			},
		},
	}
	SetLabel(node, ClusterIdLabel, "crusoe")
	if observedAt != nil || validUntil != nil {
		node.Status.External = &NodeExternalStatus{
			ObservedAt: observedAt,
			ValidUntil: validUntil,
		}
	}
	return node
}

func TestExternalNodeReadinessTracksObservationFreshness(t *testing.T) {
	now := time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC)
	fixedNow(t, now)
	stamp := func(d time.Duration) *metav1.Time {
		value := metav1.NewTime(now.Add(d))
		return &value
	}

	cases := []struct {
		name       string
		observedAt *metav1.Time
		validUntil *metav1.Time
		wantReady  bool
	}{
		{"fresh observation with room left", stamp(-10 * time.Second), stamp(time.Minute), true},
		{"validity just expired", stamp(-10 * time.Second), stamp(-time.Second), false},
		{"observation older than the backstop", stamp(-DefaultExternalObservationMaxAge - time.Second), stamp(time.Hour), false},
		{"observation exactly at the backstop", stamp(-DefaultExternalObservationMaxAge), stamp(time.Hour), false},
		{"provider has not reported at all", nil, nil, false},
		{"validity without an observation time", nil, stamp(time.Hour), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			node := externalNode(tc.observedAt, tc.validUntil)
			if got := node.IsMachineReady(); got != tc.wantReady {
				t.Fatalf("IsMachineReady() = %t, want %t", got, tc.wantReady)
			}
		})
	}
}

// A far future validUntil must not keep a node alive once the provider goes quiet: that is
// the case the backstop exists for, and it is how a crashed provider would otherwise leave
// its nodes advertised indefinitely.
func TestExternalObservationBackstopOverridesLongValidity(t *testing.T) {
	now := time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC)
	fixedNow(t, now)
	observed := metav1.NewTime(now.Add(-time.Hour))
	valid := metav1.NewTime(now.Add(24 * time.Hour))
	node := externalNode(&observed, &valid)
	if node.IsMachineReady() {
		t.Fatal("a node whose provider stopped reporting an hour ago must not be ready")
	}
}

func TestExternalNodeManagedOnlyNeedsClusterOwnership(t *testing.T) {
	observed := metav1.NewTime(time.Now())
	valid := metav1.NewTime(time.Now().Add(time.Minute))
	node := externalNode(&observed, &valid)
	// ClusterStatus is never written for a virtual node, because it never joins through
	// kubespray. Ownership of a cluster is all it can report.
	if !node.IsManaged() {
		t.Fatal("expected an external node labelled with a cluster to be managed")
	}
	RemoveLabel(node, ClusterIdLabel)
	if node.IsManaged() {
		t.Fatal("expected an external node without a cluster to be unmanaged")
	}
}

func TestNativeNodePredicatesAreUnchanged(t *testing.T) {
	node := &Node{}
	SetLabel(node, ClusterIdLabel, "global")
	node.Status.MachineStatus.Phase = NodeReady
	node.Status.ClusterStatus.Phase = NodeManaged
	if node.IsExternal() {
		t.Fatal("a node without a lifecycle mode must not be external")
	}
	if !node.IsMachineReady() || !node.IsManaged() {
		t.Fatal("expected the native predicates to keep reading the status phases")
	}
	// Freshness must play no part on the native path: a physical node reports through
	// MachineStatus and has no provider observation at all.
	node.Status.External = nil
	if !node.IsMachineReady() {
		t.Fatal("a native node must stay ready without a provider observation")
	}
}

func TestExternalNodePhaseReportsStaleRatherThanEmpty(t *testing.T) {
	now := time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC)
	fixedNow(t, now)
	expired := metav1.NewTime(now.Add(-time.Hour))
	node := externalNode(&expired, &expired)
	if got := node.GetPhase(); got != NodeExternalStale {
		t.Fatalf("GetPhase() = %q, want %q", got, NodeExternalStale)
	}

	observed := metav1.NewTime(now)
	valid := metav1.NewTime(now.Add(time.Minute))
	fresh := externalNode(&observed, &valid)
	if got := fresh.GetPhase(); got != NodeReady {
		t.Fatalf("GetPhase() = %q, want %q", got, NodeReady)
	}
}

func TestExternalNodeIsUnavailableWhenObservationGoesStale(t *testing.T) {
	now := time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC)
	fixedNow(t, now)
	observed := metav1.NewTime(now)
	valid := metav1.NewTime(now.Add(time.Minute))
	node := externalNode(&observed, &valid)
	if ok, reason := node.CheckAvailable(false); !ok {
		t.Fatalf("expected a freshly observed node to be available, got %q", reason)
	}

	// Everything else about the node is unchanged; only the clock moved. Capacity has to
	// leave the aggregate on that alone, because an unreachable provider produces no event.
	fixedNow(t, now.Add(2*time.Minute))
	if ok, _ := node.CheckAvailable(false); ok {
		t.Fatal("expected the node to become unavailable once its observation expired")
	}
}
