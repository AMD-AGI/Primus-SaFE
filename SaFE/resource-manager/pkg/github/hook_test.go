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
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestOnEphemeralRunnerEventNoMeta(t *testing.T) {
	// No GitHub metadata -> early return, store untouched (nil store is safe).
	tracker := NewWorkflowTracker(nil)
	obj := &unstructured.Unstructured{Object: map[string]interface{}{}}
	tracker.OnEphemeralRunnerEvent(context.Background(), obj, "w1", "c1", false)
}

func TestOnEphemeralRunnerEventWithMeta(t *testing.T) {
	db, mock, _ := sqlmock.New()
	defer db.Close()
	mock.ExpectExec("INSERT INTO github_workflow_runs").WillReturnResult(sqlmock.NewResult(1, 1))

	tracker := NewWorkflowTracker(NewStore(db))
	obj := &unstructured.Unstructured{Object: map[string]interface{}{}}
	obj.SetAnnotations(map[string]string{
		AnnotationRunID:      "10",
		AnnotationWorkflow:   "ci",
		AnnotationRepository: "owner/repo",
	})
	tracker.OnEphemeralRunnerEvent(context.Background(), obj, "w1", "c1", true)
	assert.NoError(t, mock.ExpectationsWereMet())
}
