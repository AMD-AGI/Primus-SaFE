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

// TestListWorkloadRunSQLHasNoLegacyFallback prevents stale rows from reviving.
// TestListWorkloadRunSQLHasNoLegacyFallback prevents stale rows from reviving.
func TestListWorkloadRunSQLHasNoLegacyFallback(t *testing.T) {
	sql := listWorkloadRunSQL("workload_pod", "pod_id")
	if !regexp.MustCompile(regexp.QuoteMeta("workload_uid = $2")).MatchString(sql) {
		t.Fatalf("missing current run predicate: %s", sql)
	}
	for _, forbidden := range []string{"workload_uid = ''", "NOT EXISTS"} {
		if regexp.MustCompile(regexp.QuoteMeta(forbidden)).MatchString(sql) {
			t.Fatalf("legacy fallback %q can resurrect stale rows: %s", forbidden, sql)
		}
	}
}

// TestRunUpsertsKeepRollingUpgradeConflictKeys protects old writers at rollout.
// TestRunUpsertsKeepRollingUpgradeConflictKeys protects old writers at rollout.
func TestRunUpsertsKeepRollingUpgradeConflictKeys(t *testing.T) {
	tests := []struct {
		name string
		sql  string
		key  string
	}{
		{name: "pod", sql: upsertWorkloadPodCmd, key: "ON CONFLICT (workload_id, pod_id)"},
		{name: "dispatch", sql: upsertWorkloadDispatchNodeCmd, key: "ON CONFLICT (workload_id, dispatch_index)"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !regexp.MustCompile(regexp.QuoteMeta(tt.key)).MatchString(tt.sql) {
				t.Fatalf("upsert must retain old conflict key during rollout: %s", tt.sql)
			}
			if !regexp.MustCompile(regexp.QuoteMeta("workload_uid = EXCLUDED.workload_uid")).MatchString(tt.sql) {
				t.Fatalf("upsert must move the row to the current run: %s", tt.sql)
			}
		})
	}
}
