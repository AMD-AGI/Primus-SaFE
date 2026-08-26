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

func TestParseMigrateAction(t *testing.T) {
	cases := []struct {
		name       string
		action     string
		wantTarget string
		wantOk     bool
	}{
		{name: "migrate with target", action: BuildMigrateAction("ws-b"), wantTarget: "ws-b", wantOk: true},
		{name: "migrate without target", action: NodeActionMigrate + ":", wantOk: false},
		{name: "bare verb is not a migration", action: NodeActionMigrate, wantOk: false},
		{name: "add", action: NodeActionAdd, wantOk: false},
		{name: "remove", action: NodeActionRemove, wantOk: false},
		{name: "empty", action: "", wantOk: false},
		{name: "target keeps everything after the separator", action: "migrate:ws:b", wantTarget: "ws:b", wantOk: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			target, ok := ParseMigrateAction(tc.action)
			if ok != tc.wantOk || target != tc.wantTarget {
				t.Fatalf("ParseMigrateAction(%q) = (%q, %v), want (%q, %v)",
					tc.action, target, ok, tc.wantTarget, tc.wantOk)
			}
		})
	}
}

func TestBuildMigrateActionRoundTrip(t *testing.T) {
	target, ok := ParseMigrateAction(BuildMigrateAction("ws-target"))
	if !ok || target != "ws-target" {
		t.Fatalf("round trip = (%q, %v), want (ws-target, true)", target, ok)
	}
}

func TestNodeMigrateInfoRoundTrip(t *testing.T) {
	start := &metav1.Time{Time: time.Now().UTC().Truncate(time.Second)}
	node := &Node{}
	if !SetNodeMigrateInfo(node, &NodeMigrateInfo{From: "ws-a", Target: "ws-b", StartTime: start}) {
		t.Fatal("SetNodeMigrateInfo reported no change on an empty node")
	}
	info := GetNodeMigrateInfo(node)
	if info == nil {
		t.Fatal("GetNodeMigrateInfo returned nil for a node just marked as migrating")
	}
	if info.From != "ws-a" || info.Target != "ws-b" {
		t.Fatalf("info = %+v, want from ws-a to ws-b", info)
	}
	if info.StartTime == nil || !info.StartTime.Time.Equal(start.Time) {
		t.Fatalf("start time = %v, want %v", info.StartTime, start)
	}
	if SetNodeMigrateInfo(node, &NodeMigrateInfo{From: "ws-a", Target: "ws-b", StartTime: start}) {
		t.Fatal("SetNodeMigrateInfo reported a change when rewriting the same migration")
	}
}

func TestGetNodeMigrateInfoRejectsUnusableValues(t *testing.T) {
	cases := []struct {
		name string
		val  string
	}{
		{name: "absent", val: ""},
		{name: "not json", val: "ws-b"},
		{name: "no target", val: `{"from":"ws-a"}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			node := &Node{}
			if tc.val != "" {
				SetAnnotation(node, NodeMigrateAnnotation, tc.val)
			}
			if info := GetNodeMigrateInfo(node); info != nil {
				t.Fatalf("GetNodeMigrateInfo(%q) = %+v, want nil", tc.val, info)
			}
		})
	}
}

func TestNodeMigrationPredicates(t *testing.T) {
	node := &Node{}
	SetNodeMigrateInfo(node, &NodeMigrateInfo{From: "ws-a", Target: "ws-b"})

	if !IsNodeMigratingTo(node, "ws-b") {
		t.Fatal("a node released for ws-b is not reported as migrating to it")
	}
	if IsNodeMigratingTo(node, "ws-c") {
		t.Fatal("a node released for ws-b is reported as migrating to ws-c")
	}
	if !IsNodeReleasedBy(node, "ws-a", "ws-b") {
		t.Fatal("a node released by ws-a for ws-b is not reported as such")
	}
	// The source has to match too: a node bound to some other workspace is not this
	// migration's node, whatever its target says.
	if IsNodeReleasedBy(node, "ws-c", "ws-b") {
		t.Fatal("a node released by ws-a is reported as released by ws-c")
	}
	if IsNodeMigratingTo(&Node{}, "ws-b") || IsNodeReleasedBy(&Node{}, "ws-a", "ws-b") {
		t.Fatal("a node with no migration is reported as migrating")
	}
}
