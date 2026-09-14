/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package client

import (
	"fmt"
	"reflect"
	"testing"

	dbutils "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/utils"
)

func TestIsNewWorkloadRun(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name     string
		oldUid   string
		curUid   string
		expected bool
	}{
		{name: "resumed run", oldUid: "uid-1", curUid: "uid-2", expected: true},
		{name: "same run", oldUid: "uid-1", curUid: "uid-1", expected: false},
		{name: "legacy row without uid", oldUid: "", curUid: "uid-2", expected: false},
		{name: "incoming without uid", oldUid: "uid-1", curUid: "", expected: false},
	}
	for _, tc := range cases {
		old := &Workload{WorkloadUId: dbutils.NullString(tc.oldUid)}
		cur := &Workload{WorkloadUId: dbutils.NullString(tc.curUid)}
		if got := isNewWorkloadRun(old, cur); got != tc.expected {
			t.Errorf("%s: expected %v, got %v", tc.name, tc.expected, got)
		}
	}
}

func TestBuildWorkloadNodesHistoryEntry(t *testing.T) {
	t.Parallel()
	old := &Workload{
		WorkloadId:    "w1",
		DispatchCount: 2,
		Phase:         dbutils.NullString("Failed"),
		Nodes:         dbutils.NullString(`[["legacy-1"]]`),
	}
	rows := WorkloadDispatchNodesFromV1("w1", [][]string{{"n1", "n2"}, {"n3"}}, nil)

	entry := buildWorkloadNodesHistoryEntry(old, rows)
	if entry == nil {
		t.Fatal("expected an entry built from the dispatch rows")
	}
	if !reflect.DeepEqual(entry.Nodes, [][]string{{"n1", "n2"}, {"n3"}}) {
		t.Errorf("dispatch rows must win over the nodes column, got %+v", entry.Nodes)
	}
	if entry.DispatchCount != 2 || entry.Phase != "Failed" {
		t.Errorf("run context not carried, got %+v", entry)
	}

	entry = buildWorkloadNodesHistoryEntry(old, nil)
	if entry == nil || !reflect.DeepEqual(entry.Nodes, [][]string{{"legacy-1"}}) {
		t.Errorf("expected the nodes column as fallback, got %+v", entry)
	}

	if entry = buildWorkloadNodesHistoryEntry(&Workload{WorkloadId: "w1"}, nil); entry != nil {
		t.Errorf("a run without nodes has nothing to archive, got %+v", entry)
	}
}

func TestAppendWorkloadNodesHistoryKeepsLastRuns(t *testing.T) {
	t.Parallel()
	raw := ""
	total := maxWorkloadNodesHistory + 2
	for i := 0; i < total; i++ {
		var err error
		raw, err = appendWorkloadNodesHistory(raw, &WorkloadNodesHistoryEntry{
			DispatchCount: i,
			Nodes:         [][]string{{fmt.Sprintf("n%d", i)}},
		})
		if err != nil {
			t.Fatalf("append failed: %v", err)
		}
	}

	entries := DecodeWorkloadNodesHistory(raw)
	if len(entries) != maxWorkloadNodesHistory {
		t.Fatalf("expected %d entries, got %d", maxWorkloadNodesHistory, len(entries))
	}
	if entries[0].DispatchCount != total-maxWorkloadNodesHistory {
		t.Errorf("oldest runs must be dropped first, got %d", entries[0].DispatchCount)
	}
	if entries[len(entries)-1].DispatchCount != total-1 {
		t.Errorf("newest run must be last, got %d", entries[len(entries)-1].DispatchCount)
	}
}

func TestDecodeWorkloadNodesHistoryMalformed(t *testing.T) {
	t.Parallel()
	if entries := DecodeWorkloadNodesHistory("not json"); entries != nil {
		t.Errorf("expected nil on unreadable history, got %+v", entries)
	}
}
