/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package client

import (
	"strings"
	"testing"

	sqrl "github.com/Masterminds/squirrel"
	"gotest.tools/v3/assert"
)

func TestNewPosttrainRunViewQuery_DefaultOrderDoesNotDuplicateDESC(t *testing.T) {
	query := newPosttrainRunViewQuery(&PosttrainRunFilter{
		Limit:  20,
		Offset: 0,
	})

	sqlStr, _, err := query.PlaceholderFormat(sqrl.Dollar).ToSql()
	assert.NilError(t, err)
	assert.Assert(t, strings.Contains(sqlStr, "ORDER BY COALESCE(w.creation_time, p.created_at) DESC"))
	assert.Assert(t, !strings.Contains(sqlStr, "DESC DESC"))
}

func TestNewPosttrainRunViewQuery_SortByStatusOrderAsc(t *testing.T) {
	query := newPosttrainRunViewQuery(&PosttrainRunFilter{
		SortBy: "status",
		Order:  ASC,
		Limit:  20,
		Offset: 0,
	})

	sqlStr, _, err := query.PlaceholderFormat(sqrl.Dollar).ToSql()
	assert.NilError(t, err)
	assert.Assert(t, strings.Contains(sqlStr, "ORDER BY COALESCE(w.phase, p.status) ASC"))
}
