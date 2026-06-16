/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package github

import (
	"context"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
)

func TestStoreUpsertWorkflowRun(t *testing.T) {
	db, mock, err := sqlmock.New()
	assert.NoError(t, err)
	defer db.Close()

	mock.ExpectExec("INSERT INTO github_workflow_runs").
		WillReturnResult(sqlmock.NewResult(1, 1))

	s := NewStore(db)
	now := time.Now()
	err = s.UpsertWorkflowRun(context.Background(), &WorkflowRunRecord{
		WorkloadID:  "w1",
		GithubRunID: 10,
		StartedAt:   &now,
	})
	assert.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestStoreMarkRunCompleted(t *testing.T) {
	db, mock, _ := sqlmock.New()
	defer db.Close()
	mock.ExpectExec("UPDATE github_workflow_runs").WillReturnResult(sqlmock.NewResult(0, 1))
	s := NewStore(db)
	assert.NoError(t, s.MarkRunCompleted(context.Background(), 10, "success", time.Now()))
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestStoreGetUnsyncedRuns(t *testing.T) {
	db, mock, _ := sqlmock.New()
	defer db.Close()
	cols := []string{"id", "workload_id", "cluster", "github_run_id", "github_job_id",
		"workflow_name", "github_owner", "github_repo", "head_branch", "head_sha", "status", "sync_status"}
	mock.ExpectQuery("SELECT id, workload_id").
		WillReturnRows(sqlmock.NewRows(cols).
			AddRow(1, "w1", "c1", 10, 20, "ci", "o", "r", "main", "sha", "running", "pending"))
	s := NewStore(db)
	runs, err := s.GetUnsyncedRuns(context.Background(), 5)
	assert.NoError(t, err)
	assert.Len(t, runs, 1)
	assert.Equal(t, "w1", runs[0].WorkloadID)
}

func TestStoreMarkSynced(t *testing.T) {
	db, mock, _ := sqlmock.New()
	defer db.Close()
	mock.ExpectExec("UPDATE github_workflow_runs SET sync_status").WillReturnResult(sqlmock.NewResult(0, 1))
	s := NewStore(db)
	assert.NoError(t, s.MarkSynced(context.Background(), 1))
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestStoreUpsertRunDetails(t *testing.T) {
	db, mock, _ := sqlmock.New()
	defer db.Close()
	mock.ExpectExec("INSERT INTO github_workflow_run_details").WillReturnResult(sqlmock.NewResult(1, 1))
	s := NewStore(db)
	err := s.UpsertRunDetails(context.Background(), 1, 10, "url", "j", "l", "push", "actor", 0, "path", []byte("{}"))
	assert.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestStoreInsertMetrics(t *testing.T) {
	db, mock, _ := sqlmock.New()
	defer db.Close()
	mock.ExpectExec("INSERT INTO github_workflow_metrics").WillReturnResult(sqlmock.NewResult(1, 1))
	s := NewStore(db)
	now := time.Now()
	err := s.InsertMetrics(context.Background(), 1, 2, &now, []byte("{}"), []byte("{}"), []byte("{}"))
	assert.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestStoreInsertMetricRow(t *testing.T) {
	db, mock, _ := sqlmock.New()
	defer db.Close()
	mock.ExpectExec("INSERT INTO github_workflow_metrics").WillReturnResult(sqlmock.NewResult(1, 1))
	s := NewStore(db)
	err := s.InsertMetricRow(context.Background(), 1, 2, "file.csv", []byte("{}"))
	assert.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestStoreUpsertStep(t *testing.T) {
	db, mock, _ := sqlmock.New()
	defer db.Close()
	mock.ExpectExec("INSERT INTO github_workflow_steps").WillReturnResult(sqlmock.NewResult(1, 1))
	s := NewStore(db)
	now := time.Now()
	err := s.UpsertStep(context.Background(), 1, 1, "name", "completed", "success", &now, &now, 5)
	assert.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestStoreUpsertCommit(t *testing.T) {
	db, mock, _ := sqlmock.New()
	defer db.Close()
	mock.ExpectExec("INSERT INTO github_workflow_commits").WillReturnResult(sqlmock.NewResult(1, 1))
	s := NewStore(db)
	now := time.Now()
	err := s.UpsertCommit(context.Background(), "sha", "o", "r", "msg", "an", "ae", &now, 1, 2, 3)
	assert.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}
