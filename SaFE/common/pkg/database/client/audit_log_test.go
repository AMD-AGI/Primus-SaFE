/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package client

import (
	"context"
	"testing"

	sqrl "github.com/Masterminds/squirrel"
	"gotest.tools/assert"
)

func TestInsertAuditLogNilInput(t *testing.T) {
	client := &Client{}

	err := client.InsertAuditLog(context.Background(), nil)
	assert.ErrorContains(t, err, "the input is empty")
}

func TestInsertAuditLogNoDBConnection(t *testing.T) {
	client := &Client{}

	auditLog := &AuditLog{
		UserId:         "user-123",
		HttpMethod:     "POST",
		RequestPath:    "/api/v1/workloads",
		ResponseStatus: 200,
	}

	err := client.InsertAuditLog(context.Background(), auditLog)
	assert.ErrorContains(t, err, "db has not been initialized")
}

func TestSelectAuditLogsNoDBConnection(t *testing.T) {
	client := &Client{}

	query := sqrl.Eq{"user_id": "test-user"}
	_, err := client.SelectAuditLogs(context.Background(), query, []string{"id"}, 10, 0)
	assert.ErrorContains(t, err, "db has not been initialized")
}

func TestCountAuditLogsNoDBConnection(t *testing.T) {
	client := &Client{}

	query := sqrl.Eq{"user_id": "test-user"}
	_, err := client.CountAuditLogs(context.Background(), query)
	assert.ErrorContains(t, err, "db has not been initialized")
}

func TestTPAuditLogConstant(t *testing.T) {
	assert.Equal(t, TPAuditLog, "audit_logs")
}

func TestGetAuditLogFieldTags(t *testing.T) {
	tags := GetAuditLogFieldTags()

	assert.Equal(t, "id", tags["id"])
	assert.Equal(t, "user_id", tags["userid"])
	assert.Equal(t, "user_name", tags["username"])
	assert.Equal(t, "user_type", tags["usertype"])
	assert.Equal(t, "client_ip", tags["clientip"])
	assert.Equal(t, "http_method", tags["httpmethod"])
	assert.Equal(t, "request_path", tags["requestpath"])
	assert.Equal(t, "resource_type", tags["resourcetype"])
	assert.Equal(t, "action", tags["action"])
	assert.Equal(t, "request_body", tags["requestbody"])
	assert.Equal(t, "response_status", tags["responsestatus"])
	assert.Equal(t, "response_body", tags["responsebody"])
	assert.Equal(t, "latency_ms", tags["latencyms"])
	assert.Equal(t, "trace_id", tags["traceid"])
	assert.Equal(t, "create_time", tags["createtime"])
}
