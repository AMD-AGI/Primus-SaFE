/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package client

import (
	"context"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestPostTrainingTemplateAndExperimentCRUD(t *testing.T) {
	c, mock := newLooseMockClient(t)
	arm(mock, 40)
	ctx := context.Background()

	_ = c.CreatePostTrainingTemplate(ctx, &PostTrainingTemplate{TemplateID: "tpl-1", Workspace: "ws"})
	_, _ = c.GetPostTrainingTemplate(ctx, "tpl-1")
	_, _, _ = c.ListPostTrainingTemplates(ctx, PostTrainingTemplateFilter{Workspace: "ws", TrainType: "sft",
		Search: "x", Limit: 10, Offset: 10})
	_ = c.SetPostTrainingTemplateDeleted(ctx, "tpl-1")
	_, _ = c.CountPostTrainingRunsByTemplate(ctx, []string{"tpl-1"})
	_ = c.CreatePostTrainingExperiment(ctx, &PostTrainingExperiment{ExperimentID: "exp-1", Workspace: "ws"})
	_, _ = c.GetPostTrainingExperiment(ctx, "exp-1")
	_, _, _ = c.ListPostTrainingExperiments(ctx, PostTrainingExperimentFilter{Workspace: "ws",
		ExperimentIDs: []string{"exp-1"}, Limit: 5, Offset: 5})
	_ = c.SetPostTrainingExperimentDeleted(ctx, "exp-1")
}

func TestPostTrainingTemplateInputs(t *testing.T) {
	c := &Client{}
	ctx := context.Background()
	require.Error(t, c.CreatePostTrainingTemplate(ctx, nil))
	require.Error(t, c.CreatePostTrainingExperiment(ctx, nil))
	counts, err := c.CountPostTrainingRunsByTemplate(ctx, nil)
	require.NoError(t, err, "no templates need no query")
	require.Empty(t, counts)
	_, err = c.GetPostTrainingTemplate(ctx, "tpl-1")
	require.Error(t, err, "expected error when gorm not initialized")
}

func TestUpdatePostTrainingTemplateBumpsVersion(t *testing.T) {
	c, mock := newMockClient(t)
	ctx := context.Background()
	mock.ExpectExec(`UPDATE "post_training_template" SET .*"version"=version \+ 1.* WHERE .*AND version = \$\d+`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	version, err := c.UpdatePostTrainingTemplate(ctx, "tpl-1", 3, map[string]interface{}{"name": "renamed"})
	require.NoError(t, err)
	require.Equal(t, 4, version, "the version written is the one after the version that was read")

	mock.ExpectExec(`UPDATE "post_training_template" SET`).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery(`SELECT count\(\*\) FROM "post_training_template"`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	_, err = c.UpdatePostTrainingTemplate(ctx, "tpl-1", 2, map[string]interface{}{"name": "renamed"})
	require.ErrorIs(t, err, ErrStaleVersion, "someone else saved the template since version 2 was read")

	mock.ExpectExec(`UPDATE "post_training_template" SET`).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery(`SELECT count\(\*\) FROM "post_training_template"`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	_, err = c.UpdatePostTrainingTemplate(ctx, "missing", 1, nil)
	require.ErrorIs(t, err, gorm.ErrRecordNotFound)
	require.NoError(t, mock.ExpectationsWereMet())
}
