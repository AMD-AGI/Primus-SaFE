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
