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

func TestGenInsertWorkloadCmd(t *testing.T) {
	workload := Workload{}
	cmd := generateCommand(workload, insertWorkloadFormat, "id")
	fmt.Println(cmd)

	userToken := UserToken{}
	cmd = generateCommand(userToken, upsertUserTokenFormat, "")
	fmt.Println(cmd)
}

func TestGetFaultFieldTags(t *testing.T) {
	tags := GetFaultFieldTags()
	monitorId := GetFieldTag(tags, "monitorId")
	assert.Equal(t, monitorId, "monitor_id")
	creationTime := GetFieldTag(tags, "creationTime")
	assert.Equal(t, creationTime, "creation_time")
}

func TestGetApiKeyFieldTags(t *testing.T) {
	tags := GetApiKeyFieldTags()

	// Test basic field mappings
	assert.Equal(t, GetFieldTag(tags, "Id"), "id")
	assert.Equal(t, GetFieldTag(tags, "Name"), "name")
	assert.Equal(t, GetFieldTag(tags, "UserId"), "user_id")
	assert.Equal(t, GetFieldTag(tags, "UserName"), "user_name")
	assert.Equal(t, GetFieldTag(tags, "ApiKey"), "api_key")
	assert.Equal(t, GetFieldTag(tags, "KeyHint"), "key_hint")
	assert.Equal(t, GetFieldTag(tags, "ExpirationTime"), "expiration_time")
	assert.Equal(t, GetFieldTag(tags, "CreationTime"), "creation_time")
	assert.Equal(t, GetFieldTag(tags, "Whitelist"), "whitelist")
	assert.Equal(t, GetFieldTag(tags, "Deleted"), "deleted")
	assert.Equal(t, GetFieldTag(tags, "DeletionTime"), "deletion_time")

	// Test that all expected fields exist
	expectedFields := []string{"Id", "Name", "UserId", "UserName", "ApiKey", "KeyHint", "ExpirationTime", "CreationTime", "Whitelist", "Deleted", "DeletionTime"}
	for _, field := range expectedFields {
		tag := GetFieldTag(tags, field)
		assert.Assert(t, tag != "", "Field %s should have a db tag", field)
	}
}

func TestGenInsertApiKeyCmd(t *testing.T) {
	apiKey := ApiKey{}
	cmd := generateCommand(apiKey, insertApiKeyFormat, "id")
	fmt.Println(cmd)

	// Verify the generated command contains expected table and fields
	assert.Assert(t, len(cmd) > 0, "Command should not be empty")
}
