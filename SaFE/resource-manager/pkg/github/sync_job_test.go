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
	"k8s.io/apimachinery/pkg/runtime"
	ctrlfake "sigs.k8s.io/controller-runtime/pkg/client/fake"

	corev1 "k8s.io/api/core/v1"
	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
)

func TestNewSyncJobDefaults(t *testing.T) {
	j := NewSyncJob(nil, nil, 0, 0)
	assert.Equal(t, 20, j.batchSize)
	assert.Equal(t, 30*time.Second, j.interval)

	j2 := NewSyncJob(nil, nil, 5, time.Minute)
	assert.Equal(t, 5, j2.batchSize)
	assert.Equal(t, time.Minute, j2.interval)
}

func TestSyncBatchNoRuns(t *testing.T) {
	db, mock, _ := sqlmock.New()
	defer db.Close()
	cols := []string{"id", "workload_id", "cluster", "github_run_id", "github_job_id", "workflow_name",
		"github_owner", "github_repo", "head_branch", "head_sha", "status", "sync_status"}
	mock.ExpectQuery("SELECT id, workload_id").WillReturnRows(sqlmock.NewRows(cols))

	j := &SyncJob{store: NewStore(db), batchSize: 20}
	j.syncBatch(context.Background())
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestSyncBatchMarksSyncedForEmptyOwner(t *testing.T) {
	db, mock, _ := sqlmock.New()
	defer db.Close()
	cols := []string{"id", "workload_id", "cluster", "github_run_id", "github_job_id", "workflow_name",
		"github_owner", "github_repo", "head_branch", "head_sha", "status", "sync_status"}
	// Run with empty owner/repo -> immediately MarkSynced.
	mock.ExpectQuery("SELECT id, workload_id").
		WillReturnRows(sqlmock.NewRows(cols).
			AddRow(1, "w1", "c1", 10, 0, "ci", "", "", "", "", "running", "pending"))
	mock.ExpectExec("UPDATE github_workflow_runs SET sync_status").WillReturnResult(sqlmock.NewResult(0, 1))

	j := &SyncJob{store: NewStore(db), batchSize: 20}
	j.syncBatch(context.Background())
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestGetGitHubTokenResolveFails(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)
	cl := ctrlfake.NewClientBuilder().WithScheme(scheme).Build()
	j := &SyncJob{
		credentialResolver: NewGitHubCredentialResolver(cl),
		tokenSource:        NewGitHubTokenSource(),
	}
	// Run with no workload id -> resolve fails -> empty token.
	token := j.getGitHubToken(context.Background(), &WorkflowRunRecord{})
	assert.Equal(t, "", token)
}
