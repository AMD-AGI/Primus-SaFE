/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package client

import (
	"context"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	sqrl "github.com/Masterminds/squirrel"
	"github.com/stretchr/testify/assert"
)

func TestApiKeyCRUD(t *testing.T) {
	c, mock := newMockClient(t)
	ctx := context.Background()

	// InsertApiKey nil input
	assert.Error(t, c.InsertApiKey(ctx, nil))

	// InsertApiKey happy path (NamedQueryContext -> RETURNING id)
	mock.ExpectQuery("INSERT INTO api_keys").WillReturnRows(idRows(7))
	ak := &ApiKey{Name: "k", UserId: "u"}
	assert.NoError(t, c.InsertApiKey(ctx, ak))
	assert.Equal(t, int64(7), ak.Id)

	// SelectApiKeys
	mock.ExpectQuery("SELECT \\* FROM api_keys").WillReturnRows(idRows(1))
	keys, err := c.SelectApiKeys(ctx, sqrl.Eq{"deleted": false}, []string{"id"}, 10, 0)
	assert.NoError(t, err)
	assert.Len(t, keys, 1)

	// CountApiKeys
	mock.ExpectQuery("SELECT COUNT").WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(3))
	cnt, err := c.CountApiKeys(ctx, sqrl.Eq{"deleted": false})
	assert.NoError(t, err)
	assert.Equal(t, 3, cnt)

	// GetApiKeyById
	mock.ExpectQuery("SELECT \\* FROM api_keys WHERE id").WithArgs(int64(5)).WillReturnRows(idRows(5))
	k, err := c.GetApiKeyById(ctx, 5)
	assert.NoError(t, err)
	assert.Equal(t, int64(5), k.Id)

	// GetApiKeyByKey empty + happy
	_, err = c.GetApiKeyByKey(ctx, "")
	assert.Error(t, err)
	mock.ExpectQuery("WHERE api_key").WithArgs("abc").WillReturnRows(idRows(1))
	_, err = c.GetApiKeyByKey(ctx, "abc")
	assert.NoError(t, err)

	// GetPlatformKeyByUserId empty + happy
	_, err = c.GetPlatformKeyByUserId(ctx, "")
	assert.Error(t, err)
	mock.ExpectQuery("key_type='platform'").WithArgs("u1").WillReturnRows(idRows(1))
	_, err = c.GetPlatformKeyByUserId(ctx, "u1")
	assert.NoError(t, err)

	// SetApiKeyDeleted: success (rows affected 1)
	mock.ExpectExec("UPDATE api_keys SET deleted").WillReturnResult(sqlmock.NewResult(0, 1))
	assert.NoError(t, c.SetApiKeyDeleted(ctx, "u1", 5))

	// SetApiKeyDeleted: not found (rows affected 0)
	mock.ExpectExec("UPDATE api_keys SET deleted").WillReturnResult(sqlmock.NewResult(0, 0))
	assert.Error(t, c.SetApiKeyDeleted(ctx, "u1", 99))
}

func TestGetDBNotInitialized(t *testing.T) {
	c := &Client{}
	_, err := c.GetApiKeyById(context.Background(), 1)
	assert.Error(t, err)
}
