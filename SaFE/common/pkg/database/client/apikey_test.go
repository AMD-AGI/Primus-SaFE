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

func TestInsertApiKeyNilInput(t *testing.T) {
	client := &Client{}

	err := client.InsertApiKey(context.Background(), nil)
	assert.ErrorContains(t, err, "the input is empty")
}

func TestInsertApiKeyNoDBConnection(t *testing.T) {
	client := &Client{} // No database connection

	apiKey := &ApiKey{
		Name:   "test-key",
		UserId: "user-123",
		ApiKey: "ak-test",
	}

	err := client.InsertApiKey(context.Background(), apiKey)
	assert.ErrorContains(t, err, "db has not been initialized")
}

func TestSelectApiKeysNoDBConnection(t *testing.T) {
	client := &Client{} // No database connection

	query := sqrl.Eq{"user_id": "test-user"}
	_, err := client.SelectApiKeys(context.Background(), query, []string{"id"}, 10, 0)
	assert.ErrorContains(t, err, "db has not been initialized")
}

func TestCountApiKeysNoDBConnection(t *testing.T) {
	client := &Client{} // No database connection

	query := sqrl.Eq{"user_id": "test-user"}
	_, err := client.CountApiKeys(context.Background(), query)
	assert.ErrorContains(t, err, "db has not been initialized")
}

func TestGetApiKeyByIdNoDBConnection(t *testing.T) {
	client := &Client{} // No database connection

	_, err := client.GetApiKeyById(context.Background(), 1)
	assert.ErrorContains(t, err, "db has not been initialized")
}

func TestGetApiKeyByKeyEmptyKey(t *testing.T) {
	client := &Client{}

	_, err := client.GetApiKeyByKey(context.Background(), "")
	assert.ErrorContains(t, err, "api key is empty")
}

func TestGetApiKeyByKeyNoDBConnection(t *testing.T) {
	client := &Client{} // No database connection

	_, err := client.GetApiKeyByKey(context.Background(), "ak-test-key")
	assert.ErrorContains(t, err, "db has not been initialized")
}

func TestSetApiKeyDeletedNoDBConnection(t *testing.T) {
	client := &Client{} // No database connection

	err := client.SetApiKeyDeleted(context.Background(), "user-123", 1)
	assert.ErrorContains(t, err, "db has not been initialized")
}

func TestTApiKeyConstant(t *testing.T) {
	assert.Equal(t, TApiKey, "api_keys")
}
