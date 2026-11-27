/*
 * Copyright (C) 2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package client

import (
	"context"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// accessRequestRows returns a result set of requests with the given ids and status.
func accessRequestRows(status string, ids ...int64) *sqlmock.Rows {
	rows := sqlmock.NewRows([]string{"id", "workspace", "user_id", "status", "created_at"})
	for _, id := range ids {
		rows.AddRow(id, "ws", "u1", status, time.Now())
	}
	return rows
}

// Creating an empty request fails without touching the database.
func TestCreateWorkspaceAccessRequest(t *testing.T) {
	c, mock := newMockClient(t)
	require.Error(t, c.CreateWorkspaceAccessRequest(context.Background(), nil))
	require.NoError(t, mock.ExpectationsWereMet())
}

// Listing filters on the given fields, newest first, and an empty workspace set matches nothing.
func TestListWorkspaceAccessRequests(t *testing.T) {
	c, mock := newMockClient(t)
	mock.ExpectQuery(`SELECT \* FROM "workspace_access_request" WHERE user_id = \$1 AND workspace IN \(\$2,\$3\) `+
		`AND status = \$4 ORDER BY created_at DESC,id DESC`).
		WithArgs("u1", "ws-a", "ws-b", WorkspaceAccessPending).
		WillReturnRows(accessRequestRows(WorkspaceAccessPending, 2, 1))
	got, err := c.ListWorkspaceAccessRequests(context.Background(), WorkspaceAccessRequestFilter{
		UserID: "u1", Workspaces: []string{"ws-a", "ws-b"}, Status: WorkspaceAccessPending})
	require.NoError(t, err)
	require.Len(t, got, 2)
	require.NoError(t, mock.ExpectationsWereMet())

	got, err = c.ListWorkspaceAccessRequests(context.Background(), WorkspaceAccessRequestFilter{Workspaces: []string{}})
	require.NoError(t, err)
	require.Empty(t, got)
}

// A transition is one conditional update on the from state; no row means not found.
func TestTransitionWorkspaceAccessRequest(t *testing.T) {
	c, mock := newMockClient(t)
	mock.ExpectQuery(`UPDATE workspace_access_request SET status = \$1, decided_by = \$2, decision_comment = \$3, `+
		`decided_at = \$4, updated_at = \$5 WHERE id = \$6 AND status = \$7 RETURNING \*`).
		WithArgs(WorkspaceAccessApproved, "mgr", "ok", sqlmock.AnyArg(), sqlmock.AnyArg(), int64(3), WorkspaceAccessPending).
		WillReturnRows(accessRequestRows(WorkspaceAccessApproved, 3))
	got, err := c.TransitionWorkspaceAccessRequest(context.Background(), 3, WorkspaceAccessTransition{
		From: WorkspaceAccessPending, To: WorkspaceAccessApproved, DecidedBy: "mgr", Comment: "ok"})
	require.NoError(t, err)
	require.Equal(t, WorkspaceAccessApproved, got.Status)

	mock.ExpectQuery(`UPDATE workspace_access_request SET status`).
		WillReturnRows(sqlmock.NewRows([]string{"id"}))
	_, err = c.TransitionWorkspaceAccessRequest(context.Background(), 3, WorkspaceAccessTransition{
		From: WorkspaceAccessPending, To: WorkspaceAccessRejected, DecidedBy: "mgr"})
	require.ErrorIs(t, err, gorm.ErrRecordNotFound)
	require.NoError(t, mock.ExpectationsWereMet())
}

// Auto close is one conditional update of the pending requests created before the cutoff.
func TestAutoCloseWorkspaceAccessRequests(t *testing.T) {
	c, mock := newMockClient(t)
	cutoff := time.Now().Add(-336 * time.Hour)
	mock.ExpectQuery(`UPDATE workspace_access_request SET status = \$1, decided_at = \$2, updated_at = \$3 `+
		`WHERE status = \$4 AND created_at < \$5 RETURNING \*`).
		WithArgs(WorkspaceAccessAutoClosed, sqlmock.AnyArg(), sqlmock.AnyArg(), WorkspaceAccessPending, cutoff).
		WillReturnRows(accessRequestRows(WorkspaceAccessAutoClosed, 4, 5))
	got, err := c.AutoCloseWorkspaceAccessRequests(context.Background(), cutoff)
	require.NoError(t, err)
	require.Len(t, got, 2)
	require.NoError(t, mock.ExpectationsWereMet())
}

// Getting a request reads it by id, and the methods fail without a database.
func TestGetWorkspaceAccessRequest(t *testing.T) {
	c, mock := newMockClient(t)
	mock.ExpectQuery(`SELECT \* FROM "workspace_access_request" WHERE id = \$1`).
		WillReturnRows(accessRequestRows(WorkspaceAccessPending, 9))
	got, err := c.GetWorkspaceAccessRequest(context.Background(), 9)
	require.NoError(t, err)
	require.Equal(t, int64(9), got.ID)

	empty := &Client{}
	_, err = empty.GetWorkspaceAccessRequest(context.Background(), 9)
	require.Error(t, err)
	_, err = empty.AutoCloseWorkspaceAccessRequests(context.Background(), time.Now())
	require.Error(t, err)
}

// Creating a request takes the advisory lock of the workspace and user, checks for a pending request and
// inserts in one transaction; a pending request found rolls back with its id.
func TestCreateWorkspaceAccessRequestChecksPendingUnderTheLock(t *testing.T) {
	c, mock := newMockClient(t)
	key := workspaceAccessLockKey("ws", "u1")
	mock.ExpectBegin()
	mock.ExpectExec(`SELECT pg_advisory_xact_lock\(\$1\)`).WithArgs(key).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery(`SELECT \* FROM "workspace_access_request" WHERE workspace = \$1 AND user_id = \$2 AND status = \$3`).
		WithArgs("ws", "u1", WorkspaceAccessPending, 1).WillReturnRows(sqlmock.NewRows([]string{"id"}))
	mock.ExpectQuery(`INSERT INTO "workspace_access_request"`).WillReturnRows(idRows(7))
	mock.ExpectCommit()
	req := &WorkspaceAccessRequest{Workspace: "ws", UserID: "u1", Status: WorkspaceAccessPending, CreatedAt: time.Now()}
	require.NoError(t, c.CreateWorkspaceAccessRequest(context.Background(), req))
	require.Equal(t, int64(7), req.ID)
	require.NoError(t, mock.ExpectationsWereMet())

	mock.ExpectBegin()
	mock.ExpectExec(`SELECT pg_advisory_xact_lock\(\$1\)`).WithArgs(key).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery(`SELECT \* FROM "workspace_access_request" WHERE workspace = \$1 AND user_id = \$2 AND status = \$3`).
		WillReturnRows(accessRequestRows(WorkspaceAccessPending, 5))
	mock.ExpectRollback()
	err := c.CreateWorkspaceAccessRequest(context.Background(), &WorkspaceAccessRequest{
		Workspace: "ws", UserID: "u1", Status: WorkspaceAccessPending, CreatedAt: time.Now()})
	var dup *PendingAccessRequestError
	require.ErrorAs(t, err, &dup)
	require.Equal(t, int64(5), dup.ID)
	require.NoError(t, mock.ExpectationsWereMet())
}

// Reverting an approval takes the same lock and moves the request back to pending, or to cancelled when the
// user has another pending request for the workspace.
func TestRevertWorkspaceAccessApproval(t *testing.T) {
	for _, tt := range []struct {
		name  string
		other *sqlmock.Rows
		want  string
	}{
		{"no other pending request", sqlmock.NewRows([]string{"id"}), WorkspaceAccessPending},
		{"another pending request", accessRequestRows(WorkspaceAccessPending, 8), WorkspaceAccessCancelled},
	} {
		t.Run(tt.name, func(t *testing.T) {
			c, mock := newMockClient(t)
			mock.ExpectBegin()
			mock.ExpectQuery(`SELECT \* FROM "workspace_access_request" WHERE id = \$1`).
				WillReturnRows(accessRequestRows(WorkspaceAccessApproved, 3))
			mock.ExpectExec(`SELECT pg_advisory_xact_lock\(\$1\)`).WithArgs(workspaceAccessLockKey("ws", "u1")).
				WillReturnResult(sqlmock.NewResult(0, 0))
			mock.ExpectQuery(`SELECT \* FROM "workspace_access_request" WHERE workspace = \$1 AND user_id = \$2 `+
				`AND status = \$3 AND id <> \$4`).
				WithArgs("ws", "u1", WorkspaceAccessPending, int64(3), 1).WillReturnRows(tt.other)
			mock.ExpectQuery(`UPDATE workspace_access_request SET status = \$1`).
				WithArgs(tt.want, "", "", sqlmock.AnyArg(), sqlmock.AnyArg(), int64(3), WorkspaceAccessApproved).
				WillReturnRows(accessRequestRows(tt.want, 3))
			mock.ExpectCommit()
			got, err := c.RevertWorkspaceAccessApproval(context.Background(), 3)
			require.NoError(t, err)
			require.Equal(t, tt.want, got.Status)
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

// The lock key depends on both the workspace and the user.
func TestWorkspaceAccessLockKey(t *testing.T) {
	require.Equal(t, workspaceAccessLockKey("ws", "u1"), workspaceAccessLockKey("ws", "u1"))
	require.NotEqual(t, workspaceAccessLockKey("ws", "u1"), workspaceAccessLockKey("ws", "u2"))
	require.NotEqual(t, workspaceAccessLockKey("ws-a", "b"), workspaceAccessLockKey("ws", "a-b"))
}
