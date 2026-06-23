/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package image_handlers

import (
	"database/sql"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	dbclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
)

// TestConvertOpsJobToExportedImageList verifies inputs/outputs/conditions parsing.
func TestConvertOpsJobToExportedImageList(t *testing.T) {
	jobs := []*dbclient.OpsJob{
		{
			JobId:        "job-1",
			Phase:        sql.NullString{String: "Succeeded", Valid: true},
			Inputs:       []byte("{workload:wl-1,label:custom}"),
			Outputs:      sql.NullString{String: `[{"name":"target","value":"harbor.io/p/app:tag"}]`, Valid: true},
			Conditions:   sql.NullString{String: `[{"type":"Ready","status":"True","message":"done"}]`, Valid: true},
		},
	}
	result := convertOpsJobToExportedImageList(jobs)
	require.Len(t, result, 1)
	assert.Equal(t, "job-1", result[0].JobId)
	assert.Equal(t, "wl-1", result[0].Workload)
	assert.Equal(t, "custom", result[0].Label)
	assert.Equal(t, "harbor.io/p/app:tag", result[0].ImageName)
	assert.Equal(t, "done", result[0].Log)
}

// TestConvertOpsJobToExportedImageListEmpty verifies empty/zero-field jobs are handled.
func TestConvertOpsJobToExportedImageList_Empty(t *testing.T) {
	result := convertOpsJobToExportedImageList(nil)
	assert.Len(t, result, 0)

	result = convertOpsJobToExportedImageList([]*dbclient.OpsJob{{JobId: "j"}})
	require.Len(t, result, 1)
	assert.Equal(t, "j", result[0].JobId)
}