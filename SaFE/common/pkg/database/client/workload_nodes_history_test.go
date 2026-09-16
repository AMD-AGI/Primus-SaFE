/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package client

import (
	"errors"
	"fmt"
	"reflect"
	"regexp"
	"strings"
	"testing"

	dbutils "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/utils"
	"github.com/DATA-DOG/go-sqlmock"
)

func TestBuildWorkloadNodesHistoryEntry(t *testing.T) {
	t.Parallel()
	old := &Workload{
		WorkloadId:    "w1",
		DispatchCount: 2,
		Phase:         dbutils.NullString("Failed"),
		Nodes:         dbutils.NullString(`[["legacy-1"]]`),
	}
	rows := WorkloadDispatchNodesFromV1(
		"w1", "old-uid",
		[][]string{{"n1", "n2"}, {"n3"}},
		[][]string{{"0", "1"}, {"0"}},
	)

	entry, err := buildWorkloadNodesHistoryEntry(old, rows)
	if err != nil {
		t.Fatalf("build entry failed: %v", err)
	}
	if entry == nil {
		t.Fatal("expected an entry built from the dispatch rows")
	}
	if !reflect.DeepEqual(entry.Nodes, [][]string{{"n1", "n2"}, {"n3"}}) {
		t.Errorf("dispatch rows must win over the nodes column, got %+v", entry.Nodes)
	}
	if !reflect.DeepEqual(entry.Ranks, [][]string{{"0", "1"}, {"0"}}) {
		t.Errorf("dispatch ranks must be archived with nodes, got %+v", entry.Ranks)
	}
	if entry.DispatchCount != 2 || entry.Phase != "Failed" {
		t.Errorf("run context not carried, got %+v", entry)
	}

	entry, err = buildWorkloadNodesHistoryEntry(old, nil)
	if err != nil {
		t.Fatalf("build fallback entry failed: %v", err)
	}
	if entry == nil || !reflect.DeepEqual(entry.Nodes, [][]string{{"legacy-1"}}) {
		t.Errorf("expected the nodes column as fallback, got %+v", entry)
	}
	emptyRows := WorkloadDispatchNodesFromV1("w1", "old-uid", [][]string{{}}, nil)
	entry, err = buildWorkloadNodesHistoryEntry(old, emptyRows)
	if err != nil {
		t.Fatalf("build empty-row fallback entry failed: %v", err)
	}
	if entry == nil || !reflect.DeepEqual(entry.Nodes, [][]string{{"legacy-1"}}) {
		t.Errorf("empty dispatch rows must fall back to the nodes column, got %+v", entry)
	}

	entry, err = buildWorkloadNodesHistoryEntry(&Workload{WorkloadId: "w1"}, nil)
	if err != nil {
		t.Fatalf("build empty entry failed: %v", err)
	}
	if entry != nil {
		t.Errorf("a run without nodes has nothing to archive, got %+v", entry)
	}

	entry, err = buildWorkloadNodesHistoryEntry(&Workload{
		WorkloadId: "w1",
		Nodes:      dbutils.NullString("not json"),
	}, nil)
	if err == nil {
		t.Fatal("corrupt legacy nodes must fail the archive")
	}
	if entry != nil {
		t.Errorf("corrupt legacy nodes must not produce an entry, got %+v", entry)
	}
}

func TestAppendWorkloadNodesHistoryKeepsLastRuns(t *testing.T) {
	t.Parallel()
	raw := ""
	total := maxWorkloadNodesHistory + 2
	for i := 0; i < total; i++ {
		next, _, err := appendWorkloadNodesHistory(raw, &WorkloadNodesHistoryEntry{
			DispatchCount: i,
			Nodes:         [][]string{{fmt.Sprintf("n%d", i)}},
		})
		if err != nil {
			t.Fatalf("append failed: %v", err)
		}
		raw = next
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

func TestAppendWorkloadNodesHistoryRejectsCorruptJSON(t *testing.T) {
	t.Parallel()
	_, _, err := appendWorkloadNodesHistory("not json", &WorkloadNodesHistoryEntry{
		DispatchCount: 1,
		Nodes:         [][]string{{"n1"}},
	})
	if !errors.Is(err, errWorkloadNodesHistoryCorrupt) {
		t.Fatalf("corrupt history must not be overwritten, got %v", err)
	}
}

func TestAppendWorkloadNodesHistoryRetainsOversizedLatestRun(t *testing.T) {
	t.Parallel()
	raw, dropped, err := appendWorkloadNodesHistory("", &WorkloadNodesHistoryEntry{
		DispatchCount: 1,
		Nodes:         [][]string{{strings.Repeat("n", maxWorkloadNodesHistoryBytes+1)}},
	})
	if err != nil {
		t.Fatalf("oversized latest run must be retained: %v", err)
	}
	if dropped != 0 || len(raw) <= maxWorkloadNodesHistoryBytes {
		t.Fatalf("unexpected oversized latest run result: dropped=%d bytes=%d", dropped, len(raw))
	}
}

func TestAppendWorkloadNodesHistoryDropsOlderRunsToFit(t *testing.T) {
	t.Parallel()
	entrySize := maxWorkloadNodesHistoryBytes/2 + 1
	first, _, err := appendWorkloadNodesHistory("", &WorkloadNodesHistoryEntry{
		DispatchCount: 1,
		Nodes:         [][]string{{strings.Repeat("a", entrySize)}},
	})
	if err != nil {
		t.Fatal(err)
	}
	raw, dropped, err := appendWorkloadNodesHistory(first, &WorkloadNodesHistoryEntry{
		DispatchCount: 2,
		Nodes:         [][]string{{strings.Repeat("b", entrySize)}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if dropped != 1 {
		t.Fatalf("expected one older run to be dropped, got %d", dropped)
	}
	entries := DecodeWorkloadNodesHistory(raw)
	if len(entries) != 1 || entries[0].DispatchCount != 2 {
		t.Fatalf("expected only the latest run, got %+v", entries)
	}
}

func TestWorkloadUpsertExcludesNodesHistory(t *testing.T) {
	t.Parallel()
	if strings.Contains(getWorkloadCmd, "*") {
		t.Errorf("upsert existence query must not load the nodes history payload: %s", getWorkloadCmd)
	}
	if strings.Contains(updateWorkloadCmd, "nodes_history") {
		t.Errorf("ordinary exporter updates must not rewrite nodes history: %s", updateWorkloadCmd)
	}
}

func TestArchiveWorkloadNodesForResume(t *testing.T) {
	c, mock := newMockClient(t)
	mock.ExpectQuery(regexp.QuoteMeta(selectWorkloadForNodesArchiveCmd)).
		WithArgs("w1").
		WillReturnRows(sqlmock.NewRows([]string{
			"workload_id", "nodes_history",
		}).AddRow("w1", nil))
	mock.ExpectExec(regexp.QuoteMeta(updateWorkloadNodesHistoryCmd)).
		WithArgs(sqlmock.AnyArg(), "w1").
		WillReturnResult(sqlmock.NewResult(0, 1))

	previous := &Workload{
		WorkloadId:    "w1",
		DispatchCount: 2,
		Phase:         dbutils.NullString("Failed"),
	}
	rows := WorkloadDispatchNodesFromV1("w1", "old-uid", [][]string{{"n1"}}, [][]string{{"0"}})
	if err := c.ArchiveWorkloadNodesForResume(t.Context(), previous, "new-uid", rows); err != nil {
		t.Fatalf("archive failed: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestArchiveWorkloadNodesForResumeSkipsWhenNoNodes(t *testing.T) {
	c, mock := newMockClient(t)
	previous := &Workload{
		WorkloadId: "w1",
	}
	if err := c.ArchiveWorkloadNodesForResume(t.Context(), previous, "new-uid", nil); err != nil {
		t.Fatalf("archive failed: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestArchiveWorkloadNodesForResumeKeepsHistoryOnCorruptJSON(t *testing.T) {
	c, mock := newMockClient(t)
	mock.ExpectQuery(regexp.QuoteMeta(selectWorkloadForNodesArchiveCmd)).
		WithArgs("w1").
		WillReturnRows(sqlmock.NewRows([]string{
			"workload_id", "nodes_history",
		}).AddRow("w1", "not json"))
	previous := &Workload{WorkloadId: "w1", DispatchCount: 1}
	rows := WorkloadDispatchNodesFromV1("w1", "old-uid", [][]string{{"n1"}}, nil)
	if err := c.ArchiveWorkloadNodesForResume(
		t.Context(), previous, "new-uid", rows,
	); !errors.Is(err, errWorkloadNodesHistoryCorrupt) {
		t.Fatalf("expected corrupt history error, got %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestDeleteWorkloadDispatchNodesNotInEmptyKeep(t *testing.T) {
	c, mock := newMockClient(t)
	if err := c.DeleteWorkloadDispatchNodesNotIn(t.Context(), "w1", "uid-1", nil); err != nil {
		t.Fatalf("empty keep must not delete current dispatch rows: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestDeleteWorkloadDispatchNodesNotInKeepIndexes(t *testing.T) {
	c, mock := newMockClient(t)
	mock.ExpectExec("DELETE FROM workload_dispatch_node").
		WithArgs("w1", "uid-1", 0, 1).
		WillReturnResult(sqlmock.NewResult(0, 1))
	if err := c.DeleteWorkloadDispatchNodesNotIn(t.Context(), "w1", "uid-1", []int{0, 1}); err != nil {
		t.Fatalf("keep indexes delete failed: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
