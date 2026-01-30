/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package client

import (
	"fmt"
	"testing"

	"gotest.tools/assert"
)

func TestGetEvaluationTaskFieldTags(t *testing.T) {
	tags := GetEvaluationTaskFieldTags()

	// Test basic field mappings
	assert.Equal(t, GetFieldTag(tags, "TaskId"), "task_id")
	assert.Equal(t, GetFieldTag(tags, "TaskName"), "task_name")
	assert.Equal(t, GetFieldTag(tags, "ServiceId"), "service_id")
	assert.Equal(t, GetFieldTag(tags, "ServiceType"), "service_type")
	assert.Equal(t, GetFieldTag(tags, "ServiceName"), "service_name")
	assert.Equal(t, GetFieldTag(tags, "Benchmarks"), "benchmarks")
	assert.Equal(t, GetFieldTag(tags, "Status"), "status")
	assert.Equal(t, GetFieldTag(tags, "OpsJobId"), "ops_job_id")
	assert.Equal(t, GetFieldTag(tags, "Workspace"), "workspace")
	assert.Equal(t, GetFieldTag(tags, "UserId"), "user_id")
	assert.Equal(t, GetFieldTag(tags, "UserName"), "user_name")
	assert.Equal(t, GetFieldTag(tags, "IsDeleted"), "is_deleted")
	assert.Equal(t, GetFieldTag(tags, "CreationTime"), "creation_time")
	assert.Equal(t, GetFieldTag(tags, "StartTime"), "start_time")
	assert.Equal(t, GetFieldTag(tags, "EndTime"), "end_time")

	// Test judge model fields
	assert.Equal(t, GetFieldTag(tags, "JudgeServiceId"), "judge_service_id")
	assert.Equal(t, GetFieldTag(tags, "JudgeServiceType"), "judge_service_type")
	assert.Equal(t, GetFieldTag(tags, "JudgeServiceName"), "judge_service_name")

	// Test performance fields
	assert.Equal(t, GetFieldTag(tags, "Timeout"), "timeout")
	assert.Equal(t, GetFieldTag(tags, "Concurrency"), "concurrency")
}

func TestGenInsertEvaluationTaskCmd(t *testing.T) {
	task := EvaluationTask{}
	cmd := generateCommand(task, insertEvaluationTaskFormat, "id")
	fmt.Println(cmd)

	// Verify the generated command contains expected table and fields
	assert.Assert(t, len(cmd) > 0, "Command should not be empty")
}

func TestEvaluationTaskStatus(t *testing.T) {
	// Test status constants
	assert.Equal(t, string(EvaluationTaskStatusPending), "Pending")
	assert.Equal(t, string(EvaluationTaskStatusRunning), "Running")
	assert.Equal(t, string(EvaluationTaskStatusSucceeded), "Succeeded")
	assert.Equal(t, string(EvaluationTaskStatusFailed), "Failed")
	assert.Equal(t, string(EvaluationTaskStatusCancelled), "Cancelled")
}

