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
	rows := WorkloadDispatchNodesFromV1("w1", [][]string{{"n1", "n2"}, {"n3"}}, nil)

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

	entry, err = buildWorkloadNodesHistoryEntry(&Workload{WorkloadId: "w1"}, nil)
	if err != nil {
		t.Fatalf("build empty entry failed: %v", err)
	}
	if entry != nil {
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
	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta(selectWorkloadForNodesArchiveCmd)).
		WithArgs("w1").
		WillReturnRows(sqlmock.NewRows([]string{
			"workload_id", "dispatch_count", "phase", "nodes", "nodes_history",
		}).AddRow("w1", 2, "Failed", nil, nil))
	mock.ExpectQuery(regexp.QuoteMeta(listWorkloadDispatchNodesForArchiveCmd)).
		WithArgs("w1").
		WillReturnRows(sqlmock.NewRows([]string{
			"workload_id", "dispatch_index", "nodes", "ranks", "updated_at",
		}).AddRow("w1", 0, `["n1"]`, `["0"]`, nil))
	mock.ExpectExec(regexp.QuoteMeta(updateWorkloadNodesHistoryCmd)).
		WithArgs(sqlmock.AnyArg(), "w1").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(regexp.QuoteMeta(deleteWorkloadDispatchNodesForArchiveCmd)).
		WithArgs("w1").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	if err := c.ArchiveWorkloadNodesForResume(t.Context(), "w1"); err != nil {
		t.Fatalf("archive failed: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestArchiveWorkloadNodesForResumeRollsBackDeleteFailure(t *testing.T) {
	c, mock := newMockClient(t)
	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta(selectWorkloadForNodesArchiveCmd)).
		WithArgs("w1").
		WillReturnRows(sqlmock.NewRows([]string{
			"workload_id", "dispatch_count", "phase", "nodes", "nodes_history",
		}).AddRow("w1", 1, "Succeeded", nil, nil))
	mock.ExpectQuery(regexp.QuoteMeta(listWorkloadDispatchNodesForArchiveCmd)).
		WithArgs("w1").
		WillReturnRows(sqlmock.NewRows([]string{
			"workload_id", "dispatch_index", "nodes", "ranks", "updated_at",
		}).AddRow("w1", 0, `["n1"]`, `["0"]`, nil))
	mock.ExpectExec(regexp.QuoteMeta(updateWorkloadNodesHistoryCmd)).
		WithArgs(sqlmock.AnyArg(), "w1").
		WillReturnResult(sqlmock.NewResult(0, 1))
	deleteErr := errors.New("delete failed")
	mock.ExpectExec(regexp.QuoteMeta(deleteWorkloadDispatchNodesForArchiveCmd)).
		WithArgs("w1").
		WillReturnError(deleteErr)
	mock.ExpectRollback()

	if err := c.ArchiveWorkloadNodesForResume(t.Context(), "w1"); !errors.Is(err, deleteErr) {
		t.Fatalf("expected delete failure, got %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
