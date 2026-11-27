/*
 * Copyright (C) 2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package client

import (
	"context"
	"fmt"
	"hash/fnv"
	"time"

	"github.com/lib/pq"
	"gorm.io/gorm"

	commonerrors "github.com/AMD-AIG-AIMA/SAFE/common/pkg/errors"
)

const (
	TWorkspaceAccessRequest = "workspace_access_request"

	// The states of a workspace access request. Every state but pending is final.
	WorkspaceAccessPending    = "pending"
	WorkspaceAccessApproved   = "approved"
	WorkspaceAccessRejected   = "rejected"
	WorkspaceAccessCancelled  = "cancelled"
	WorkspaceAccessAutoClosed = "auto_closed"
)

// WorkspaceAccessRequest is a request of a user for direct member access to a workspace.
type WorkspaceAccessRequest struct {
	ID        int64  `gorm:"column:id;primaryKey;autoIncrement" json:"id" db:"id"`
	Workspace string `gorm:"column:workspace" json:"workspace" db:"workspace"`
	UserID    string `gorm:"column:user_id" json:"userId" db:"user_id"`
	UserName  string `gorm:"column:user_name" json:"userName" db:"user_name"`
	Reason    string `gorm:"column:reason" json:"reason" db:"reason"`
	Status    string `gorm:"column:status" json:"status" db:"status"`
	// DecidedBy is the user who approved, rejected or cancelled the request; empty for auto_closed.
	DecidedBy       string      `gorm:"column:decided_by" json:"decidedBy" db:"decided_by"`
	DecisionComment string      `gorm:"column:decision_comment" json:"decisionComment" db:"decision_comment"`
	DecidedAt       pq.NullTime `gorm:"column:decided_at" json:"decidedAt" db:"decided_at"`
	CreatedAt       time.Time   `gorm:"column:created_at" json:"createdAt" db:"created_at"`
	UpdatedAt       pq.NullTime `gorm:"column:updated_at" json:"updatedAt" db:"updated_at"`
}

// TableName sets the SQL table name for WorkspaceAccessRequest.
func (WorkspaceAccessRequest) TableName() string {
	return TWorkspaceAccessRequest
}

// WorkspaceAccessRequestFilter selects requests for ListWorkspaceAccessRequests. Zero-valued fields are
// ignored; a non-nil empty Workspaces matches nothing.
type WorkspaceAccessRequestFilter struct {
	UserID     string
	Workspace  string
	Workspaces []string
	Status     string
}

// WorkspaceAccessTransition moves a request from one state to another. DecidedBy and Comment are stored
// with the new state; moving back to pending clears the decision.
type WorkspaceAccessTransition struct {
	From      string
	To        string
	DecidedBy string
	Comment   string
}

var (
	transitionWorkspaceAccessCmd = `UPDATE ` + TWorkspaceAccessRequest + ` SET status = ?, decided_by = ?, ` +
		`decision_comment = ?, decided_at = ?, updated_at = ? WHERE id = ? AND status = ? RETURNING *`
	autoCloseWorkspaceAccessCmd = `UPDATE ` + TWorkspaceAccessRequest + ` SET status = ?, decided_at = ?, ` +
		`updated_at = ? WHERE status = ? AND created_at < ? RETURNING *`
	workspaceAccessLockCmd = `SELECT pg_advisory_xact_lock(?)`
)

// PendingAccessRequestError reports that the user already has a pending request, ID, for the workspace.
type PendingAccessRequestError struct {
	ID int64
}

// Error returns the message of the error.
func (e *PendingAccessRequestError) Error() string {
	return fmt.Sprintf("a pending access request %d already exists", e.ID)
}

// CreateWorkspaceAccessRequest stores a new pending request and sets its ID. Under the advisory lock of the
// workspace and user, it returns a *PendingAccessRequestError instead when the user already has a pending
// request for the workspace.
func (c *Client) CreateWorkspaceAccessRequest(ctx context.Context, req *WorkspaceAccessRequest) error {
	if req == nil {
		return commonerrors.NewBadRequest("the input is empty")
	}
	db, err := c.GetGormDB()
	if err != nil {
		return err
	}
	return db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		other, err := lockPendingWorkspaceAccess(tx, req.Workspace, req.UserID, 0)
		if err != nil {
			return err
		}
		if other != nil {
			return &PendingAccessRequestError{ID: other.ID}
		}
		return tx.Create(req).Error
	})
}

// RevertWorkspaceAccessApproval moves an approved request back to pending under the advisory lock of its
// workspace and user, or to cancelled when the user has another pending request for the workspace. It
// returns the updated request, or gorm.ErrRecordNotFound when the request does not exist or is not approved.
func (c *Client) RevertWorkspaceAccessApproval(ctx context.Context, id int64) (*WorkspaceAccessRequest, error) {
	db, err := c.GetGormDB()
	if err != nil {
		return nil, err
	}
	var updated *WorkspaceAccessRequest
	err = db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var req WorkspaceAccessRequest
		if err := tx.Where("id = ?", id).First(&req).Error; err != nil {
			return err
		}
		other, err := lockPendingWorkspaceAccess(tx, req.Workspace, req.UserID, id)
		if err != nil {
			return err
		}
		to := WorkspaceAccessPending
		if other != nil {
			to = WorkspaceAccessCancelled
		}
		updated, err = transitionWorkspaceAccess(tx, id, WorkspaceAccessTransition{From: WorkspaceAccessApproved, To: to})
		return err
	})
	if err != nil {
		return nil, err
	}
	return updated, nil
}

// lockPendingWorkspaceAccess takes the transaction-scoped advisory lock of the workspace and user, then
// returns a pending request of the user for the workspace other than except, or nil when there is none.
func lockPendingWorkspaceAccess(tx *gorm.DB, workspace, userId string, except int64) (*WorkspaceAccessRequest, error) {
	if err := tx.Exec(workspaceAccessLockCmd, workspaceAccessLockKey(workspace, userId)).Error; err != nil {
		return nil, err
	}
	query := tx.Where("workspace = ?", workspace).Where("user_id = ?", userId).Where("status = ?", WorkspaceAccessPending)
	if except != 0 {
		query = query.Where("id <> ?", except)
	}
	var reqs []*WorkspaceAccessRequest
	if err := query.Limit(1).Find(&reqs).Error; err != nil {
		return nil, err
	}
	if len(reqs) == 0 {
		return nil, nil
	}
	return reqs[0], nil
}

// workspaceAccessLockKey returns the advisory lock key of the requests of a user for a workspace: the
// 64-bit FNV-1a hash of both, separated by a byte neither contains.
func workspaceAccessLockKey(workspace, userId string) int64 {
	h := fnv.New64a()
	_, _ = h.Write([]byte(workspace + "\x00" + userId))
	return int64(h.Sum64())
}

// GetWorkspaceAccessRequest fetches a request by id. It returns gorm.ErrRecordNotFound when there is none.
func (c *Client) GetWorkspaceAccessRequest(ctx context.Context, id int64) (*WorkspaceAccessRequest, error) {
	db, err := c.GetGormDB()
	if err != nil {
		return nil, err
	}
	var req WorkspaceAccessRequest
	if err = db.WithContext(ctx).Where("id = ?", id).First(&req).Error; err != nil {
		return nil, err
	}
	return &req, nil
}

// ListWorkspaceAccessRequests returns the requests matching filter, newest first.
func (c *Client) ListWorkspaceAccessRequests(ctx context.Context,
	filter WorkspaceAccessRequestFilter) ([]*WorkspaceAccessRequest, error) {
	if filter.Workspaces != nil && len(filter.Workspaces) == 0 {
		return nil, nil
	}
	db, err := c.GetGormDB()
	if err != nil {
		return nil, err
	}
	query := db.WithContext(ctx).Model(&WorkspaceAccessRequest{})
	if filter.UserID != "" {
		query = query.Where("user_id = ?", filter.UserID)
	}
	if filter.Workspace != "" {
		query = query.Where("workspace = ?", filter.Workspace)
	}
	if len(filter.Workspaces) > 0 {
		query = query.Where("workspace IN ?", filter.Workspaces)
	}
	if filter.Status != "" {
		query = query.Where("status = ?", filter.Status)
	}
	var reqs []*WorkspaceAccessRequest
	if err = query.Order("created_at DESC").Order("id DESC").Find(&reqs).Error; err != nil {
		return nil, err
	}
	return reqs, nil
}

// TransitionWorkspaceAccessRequest applies the transition in one conditional update, so only one of
// concurrent transitions from the same state takes effect. It returns the updated request, or
// gorm.ErrRecordNotFound when the request does not exist or is not in the From state.
func (c *Client) TransitionWorkspaceAccessRequest(ctx context.Context, id int64,
	t WorkspaceAccessTransition) (*WorkspaceAccessRequest, error) {
	db, err := c.GetGormDB()
	if err != nil {
		return nil, err
	}
	return transitionWorkspaceAccess(db.WithContext(ctx), id, t)
}

// transitionWorkspaceAccess runs the conditional update of TransitionWorkspaceAccessRequest on db.
func transitionWorkspaceAccess(db *gorm.DB, id int64, t WorkspaceAccessTransition) (*WorkspaceAccessRequest, error) {
	now := time.Now().UTC()
	decidedAt := pq.NullTime{Time: now, Valid: true}
	if t.To == WorkspaceAccessPending {
		decidedAt = pq.NullTime{}
	}
	var rows []*WorkspaceAccessRequest
	err := db.Raw(transitionWorkspaceAccessCmd, t.To, t.DecidedBy, t.Comment, decidedAt,
		pq.NullTime{Time: now, Valid: true}, id, t.From).Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, gorm.ErrRecordNotFound
	}
	return rows[0], nil
}

// AutoCloseWorkspaceAccessRequests sets every request still pending that was created before the given
// time to auto_closed in one conditional update, and returns the requests it closed. A request is
// returned by at most one of concurrent calls.
func (c *Client) AutoCloseWorkspaceAccessRequests(ctx context.Context,
	before time.Time) ([]*WorkspaceAccessRequest, error) {
	db, err := c.GetGormDB()
	if err != nil {
		return nil, err
	}
	now := pq.NullTime{Time: time.Now().UTC(), Valid: true}
	var rows []*WorkspaceAccessRequest
	err = db.WithContext(ctx).Raw(autoCloseWorkspaceAccessCmd, WorkspaceAccessAutoClosed, now, now,
		WorkspaceAccessPending, before).Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	return rows, nil
}
