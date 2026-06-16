/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package github

import (
	"context"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
)

func TestMatchesRun(t *testing.T) {
	c := &CollectionConfig{
		Enabled:          true,
		GithubOwner:      "owner",
		GithubRepo:       "repo",
		WorkflowPatterns: []string{"ci-*"},
		BranchPatterns:   []string{"main"},
	}
	assert.True(t, c.MatchesRun("owner", "repo", "ci-build", "main"))
	assert.False(t, c.MatchesRun("owner", "repo", "deploy", "main"))
	assert.False(t, c.MatchesRun("owner", "repo", "ci-build", "dev"))
	assert.False(t, c.MatchesRun("other", "repo", "ci-build", "main"))

	// Disabled config never matches.
	c.Enabled = false
	assert.False(t, c.MatchesRun("owner", "repo", "ci-build", "main"))
}

func TestPgArrayToSlice(t *testing.T) {
	assert.Nil(t, pgArrayToSlice("{}"))
	assert.Equal(t, []string{"a", "b"}, pgArrayToSlice(`{"a","b"}`))
	assert.Equal(t, []string{"x"}, pgArrayToSlice("{x}"))
}

func TestSliceToPgArray(t *testing.T) {
	assert.Equal(t, "{}", sliceToPgArray(nil))
	assert.Equal(t, "{a,b}", sliceToPgArray([]string{"a", "b"}))
}

func TestCollectionConfigStoreList(t *testing.T) {
	db, mock, _ := sqlmock.New()
	defer db.Close()
	cols := []string{"id", "name", "github_owner", "github_repo", "workflow_patterns",
		"branch_patterns", "file_patterns", "enabled", "created_by", "created_at", "updated_at"}
	mock.ExpectQuery("SELECT id, name, github_owner").
		WillReturnRows(sqlmock.NewRows(cols).
			AddRow(1, "n", "o", "r", "{ci-*}", "{main}", "{}", true, "user", nil, nil))
	s := NewCollectionConfigStore(db)
	configs, err := s.List(context.Background())
	assert.NoError(t, err)
	assert.Len(t, configs, 1)
	assert.Equal(t, []string{"ci-*"}, configs[0].WorkflowPatterns)
}

func TestCollectionConfigStoreFindMatching(t *testing.T) {
	db, mock, _ := sqlmock.New()
	defer db.Close()
	cols := []string{"id", "name", "github_owner", "github_repo", "workflow_patterns",
		"branch_patterns", "file_patterns", "enabled", "created_by", "created_at", "updated_at"}
	mock.ExpectQuery("SELECT id, name, github_owner").
		WillReturnRows(sqlmock.NewRows(cols).
			AddRow(1, "n", "owner", "repo", "{}", "{}", "{}", true, "user", nil, nil))
	s := NewCollectionConfigStore(db)
	cfg := s.FindMatchingConfig(context.Background(), "owner", "repo", "wf", "main")
	assert.NotNil(t, cfg)
	assert.Equal(t, "owner", cfg.GithubOwner)
}

func TestCollectionConfigStoreCreateDelete(t *testing.T) {
	db, mock, _ := sqlmock.New()
	defer db.Close()
	mock.ExpectExec("INSERT INTO github_collection_configs").WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec("DELETE FROM github_collection_configs").WillReturnResult(sqlmock.NewResult(0, 1))
	s := NewCollectionConfigStore(db)
	assert.NoError(t, s.Create(context.Background(), &CollectionConfig{Name: "n"}))
	assert.NoError(t, s.Delete(context.Background(), 1))
	assert.NoError(t, mock.ExpectationsWereMet())
}
