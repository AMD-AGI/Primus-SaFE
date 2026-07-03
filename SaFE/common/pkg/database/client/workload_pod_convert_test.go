/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package client

import (
	"reflect"
	"testing"

	corev1 "k8s.io/api/core/v1"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
)

func TestWorkloadPodConvertRoundTrip(t *testing.T) {
	in := v1.WorkloadPod{
		PodId:         "pod-1",
		ResourceId:    2,
		AdminNodeName: "node-a",
		Phase:         corev1.PodRunning,
		HostIp:        "10.0.0.1",
		PodIp:         "10.1.0.5",
		Rank:          "3",
		StartTime:     "2026-06-30T00:00:00Z",
		EndTime:       "",
		FailedMessage: "",
		GroupId:       1,
		Containers: []v1.Container{
			{Name: "main", Message: "OOMKilled", ExitCode: 137},
			{Name: "sidecar", ExitCode: 0},
		},
	}
	row := WorkloadPodFromV1("wl-1", 4, &in)
	if row.WorkloadId != "wl-1" || row.DispatchCount != 4 {
		t.Fatalf("unexpected row identity: %+v", row)
	}
	out := row.ToV1()
	if !reflect.DeepEqual(in, out) {
		t.Fatalf("round-trip mismatch:\n in=%+v\nout=%+v", in, out)
	}
}

func TestWorkloadPodConvertEmptyContainers(t *testing.T) {
	in := v1.WorkloadPod{PodId: "p", AdminNodeName: "n"}
	out := WorkloadPodFromV1("wl", 0, &in).ToV1()
	if len(out.Containers) != 0 {
		t.Fatalf("expected no containers, got %+v", out.Containers)
	}
	if out.PodId != "p" || out.AdminNodeName != "n" {
		t.Fatalf("scalar mismatch: %+v", out)
	}
}

func TestDispatchNodesConvert(t *testing.T) {
	nodes := [][]string{{"n1", "n2"}, {"n3"}}
	ranks := [][]string{{"0", "1"}, {"0"}}
	rows := WorkloadDispatchNodesFromV1("wl", nodes, ranks)
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(rows))
	}
	got := DispatchNodesToV1(rows)
	if !reflect.DeepEqual(got, nodes) {
		t.Fatalf("dispatch nodes round-trip mismatch: %+v vs %+v", got, nodes)
	}
	latest := LatestDispatchNodes(rows)
	if !reflect.DeepEqual(latest, []string{"n3"}) {
		t.Fatalf("latest dispatch nodes mismatch: %+v", latest)
	}
}
