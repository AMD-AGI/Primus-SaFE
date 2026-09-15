/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package client

import (
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestListWorkloadPodsScopesCurrentRun(t *testing.T) {
	c, mock := newMockClient(t)
	mock.ExpectQuery(regexp.QuoteMeta(listWorkloadPodsCmd)).
		WithArgs("w1", "uid-2").
		WillReturnRows(sqlmock.NewRows([]string{
			"workload_id", "workload_uid", "pod_id",
		}).AddRow("w1", "uid-2", "p1"))

	rows, err := c.ListWorkloadPods(t.Context(), "w1", "uid-2")
	if err != nil {
		t.Fatalf("list pods failed: %v", err)
	}
	if len(rows) != 1 || rows[0].WorkloadUId != "uid-2" {
		t.Fatalf("expected current run only, got %+v", rows)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestListWorkloadDispatchNodesScopesCurrentRun(t *testing.T) {
	c, mock := newMockClient(t)
	mock.ExpectQuery(regexp.QuoteMeta(listWorkloadDispatchNodesCmd)).
		WithArgs("w1", "uid-2").
		WillReturnRows(sqlmock.NewRows([]string{
			"workload_id", "workload_uid", "dispatch_index",
		}).AddRow("w1", "uid-2", 0))

	rows, err := c.ListWorkloadDispatchNodes(t.Context(), "w1", "uid-2")
	if err != nil {
		t.Fatalf("list dispatch nodes failed: %v", err)
	}
	if len(rows) != 1 || rows[0].WorkloadUId != "uid-2" {
		t.Fatalf("expected current run only, got %+v", rows)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestListWorkloadRunSQLIncludesLegacyFallback(t *testing.T) {
	sql := listWorkloadRunSQL("workload_pod", "pod_id")
	for _, fragment := range []string{
		"workload_uid = $2",
		"workload_uid = ''",
		"NOT EXISTS",
	} {
		if !regexp.MustCompile(regexp.QuoteMeta(fragment)).MatchString(sql) {
			t.Fatalf("missing %q in run query: %s", fragment, sql)
		}
	}
}
