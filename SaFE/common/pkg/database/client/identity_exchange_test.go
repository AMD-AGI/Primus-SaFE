/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package client

import (
	"context"
	"database/sql"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
)

func TestInsertIdentityBindingColumns(t *testing.T) {
	c, mock := newMockClient(t)
	mock.ExpectQuery(`INSERT INTO identity_bindings \(client_id, issuer, subject, user_id, creation_time, deleted, deletion_time\)`).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(5))
	b := &IdentityBinding{ClientId: "c", Issuer: "i", Subject: "s", UserId: "u"}
	require.NoError(t, c.InsertIdentityBinding(context.Background(), b))
	require.Equal(t, int64(5), b.Id)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestGetIdentityBindingOnlyActiveRows(t *testing.T) {
	c, mock := newMockClient(t)
	mock.ExpectQuery(`SELECT \* FROM identity_bindings WHERE client_id=\$1 AND issuer=\$2 AND subject=\$3 AND deleted=false`).
		WithArgs("c", "i", "s").
		WillReturnRows(sqlmock.NewRows([]string{"client_id", "user_id"}).AddRow("c", "u"))
	b, err := c.GetIdentityBinding(context.Background(), "c", "i", "s")
	require.NoError(t, err)
	require.Equal(t, "u", b.UserId)

	mock.ExpectQuery(`SELECT \* FROM identity_bindings`).WillReturnError(sql.ErrNoRows)
	_, err = c.GetIdentityBinding(context.Background(), "c", "i", "none")
	require.ErrorIs(t, err, sql.ErrNoRows)
}

func TestDeleteIdentityBindingIsScopedToOwner(t *testing.T) {
	c, mock := newMockClient(t)
	mock.ExpectExec(`UPDATE identity_bindings SET deleted=true, deletion_time=now\(\) WHERE id=\$1 AND user_id=\$2 AND deleted=false`).
		WithArgs(int64(3), "owner").WillReturnResult(sqlmock.NewResult(0, 1))
	ok, err := c.DeleteIdentityBinding(context.Background(), 3, "owner")
	require.NoError(t, err)
	require.True(t, ok)

	mock.ExpectExec(`UPDATE identity_bindings`).WithArgs(int64(3), "intruder").WillReturnResult(sqlmock.NewResult(0, 0))
	ok, err = c.DeleteIdentityBinding(context.Background(), 3, "intruder")
	require.NoError(t, err)
	require.False(t, ok)
}

func TestHasIdentityBindingForUser(t *testing.T) {
	c, mock := newMockClient(t)
	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM identity_bindings WHERE client_id=\$1 AND user_id=\$2 AND deleted=false`).
		WithArgs("c", "u").WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	ok, err := c.HasIdentityBindingForUser(context.Background(), "c", "u")
	require.NoError(t, err)
	require.True(t, ok)
}

func TestInsertServiceClientColumns(t *testing.T) {
	c, mock := newMockClient(t)
	mock.ExpectQuery(`INSERT INTO service_clients \(client_id, name, secret_hash, auth_method, allowed_audiences, allowed_scopes, whitelist, max_ttl_seconds, disabled, created_by, creation_time, public_key\)`).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(1))
	require.NoError(t, c.InsertServiceClient(context.Background(), &ServiceClient{ClientId: "c"}))
}

func TestBatchInsertAuditLogsCarriesActor(t *testing.T) {
	c, mock := newMockClient(t)
	mock.ExpectExec(`INSERT INTO audit_logs \(.*actor_client_id,token_jti\) VALUES`).
		WithArgs(
			"u", sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), "GET", "/p", sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(),
			200, sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(),
			sql.NullString{String: "svc-1", Valid: true}, sql.NullString{String: "jti-1", Valid: true},
		).WillReturnResult(sqlmock.NewResult(0, 1))
	err := c.BatchInsertAuditLogs(context.Background(), []*AuditLog{{
		UserId: "u", HttpMethod: "GET", RequestPath: "/p", ResponseStatus: 200,
		ActorClientId: sql.NullString{String: "svc-1", Valid: true},
		TokenJti:      sql.NullString{String: "jti-1", Valid: true},
	}})
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestAuditLogFieldTagsIncludeActor(t *testing.T) {
	tags := GetAuditLogFieldTags()
	require.Contains(t, tags, "actorclientid")
	require.Contains(t, tags, "tokenjti")
}

func TestServiceClientQueries(t *testing.T) {
	c, mock := newMockClient(t)
	ctx := context.Background()
	mock.ExpectQuery(`SELECT \* FROM service_clients WHERE client_id=\$1`).WithArgs("c1").
		WillReturnRows(sqlmock.NewRows([]string{"client_id", "disabled"}).AddRow("c1", true))
	sc, err := c.GetServiceClient(ctx, "c1")
	require.NoError(t, err)
	require.True(t, sc.Disabled)
	_, err = c.GetServiceClient(ctx, "")
	require.Error(t, err)

	mock.ExpectQuery(`SELECT \* FROM service_clients ORDER BY id`).
		WillReturnRows(sqlmock.NewRows([]string{"client_id"}).AddRow("a").AddRow("b"))
	list, err := c.ListServiceClients(ctx)
	require.NoError(t, err)
	require.Len(t, list, 2)

	mock.ExpectExec(`UPDATE service_clients SET disabled=\$1 WHERE client_id=\$2`).WithArgs(true, "c1").
		WillReturnResult(sqlmock.NewResult(0, 1))
	ok, err := c.SetServiceClientDisabled(ctx, "c1", true)
	require.NoError(t, err)
	require.True(t, ok)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestListIdentityBindingsByUser(t *testing.T) {
	c, mock := newMockClient(t)
	mock.ExpectQuery(`SELECT \* FROM identity_bindings WHERE user_id=\$1 AND deleted=false ORDER BY id`).WithArgs("u").
		WillReturnRows(sqlmock.NewRows([]string{"client_id", "user_id"}).AddRow("c", "u"))
	list, err := c.ListIdentityBindingsByUser(context.Background(), "u")
	require.NoError(t, err)
	require.Len(t, list, 1)
}

func TestIdentityExchangeNilInputs(t *testing.T) {
	c, _ := newMockClient(t)
	require.Error(t, c.InsertServiceClient(context.Background(), nil))
	require.Error(t, c.InsertIdentityBinding(context.Background(), nil))
}
