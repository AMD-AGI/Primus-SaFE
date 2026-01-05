/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package client

import (
	"context"
	"fmt"

	sqrl "github.com/Masterminds/squirrel"
	"k8s.io/klog/v2"

	commonerrors "github.com/AMD-AIG-AIMA/SAFE/common/pkg/errors"
)

const (
	TPlaygroundSession = "playground_session"
)

var (
	insertPlaygroundSessionFormat = `INSERT INTO ` + TPlaygroundSession + ` (%s) VALUES (%s) RETURNING id`
	updatePlaygroundSessionCmd    = fmt.Sprintf(`UPDATE %s 
		SET model_name = :model_name,
		    display_name = :display_name,
		    system_prompt = :system_prompt,
		    messages = :messages,
		    update_time = :update_time
		WHERE id = :id`, TPlaygroundSession)
)

// InsertPlaygroundSession inserts a new playground session and returns the auto-generated ID.
func (c *Client) InsertPlaygroundSession(ctx context.Context, session *PlaygroundSession) error {
	if session == nil {
		return commonerrors.NewBadRequest("the input is empty")
	}
	db, err := c.getDB()
	if err != nil {
		return err
	}

	rows, err := db.NamedQueryContext(ctx, generateCommand(*session, insertPlaygroundSessionFormat, "id"), session)
	if err != nil {
		klog.ErrorS(err, "failed to insert playground session db")
		return err
	}
	defer rows.Close()

	// Scan the returned ID
	if rows.Next() {
		if err = rows.Scan(&session.Id); err != nil {
			klog.ErrorS(err, "failed to scan returned id")
			return err
		}
	}

	return nil
}

// UpdatePlaygroundSession updates an existing playground session.
func (c *Client) UpdatePlaygroundSession(ctx context.Context, session *PlaygroundSession) error {
	if session == nil {
		return commonerrors.NewBadRequest("the input is empty")
	}
	db, err := c.getDB()
	if err != nil {
		return err
	}

	_, err = db.NamedExecContext(ctx, updatePlaygroundSessionCmd, session)
	if err != nil {
		klog.ErrorS(err, "failed to update playground session db", "id", session.Id)
	}
	return err
}

// SelectPlaygroundSessions retrieves multiple playground session records.
func (c *Client) SelectPlaygroundSessions(ctx context.Context, query sqrl.Sqlizer, orderBy []string, limit, offset int) ([]*PlaygroundSession, error) {
	db, err := c.getDB()
	if err != nil {
		return nil, err
	}

	sql, args, err := sqrl.Select("*").PlaceholderFormat(sqrl.Dollar).
		From(TPlaygroundSession).
		Where(query).
		OrderBy(orderBy...).
		Limit(uint64(limit)).
		Offset(uint64(offset)).ToSql()
	if err != nil {
		return nil, err
	}

	var sessions []*PlaygroundSession
	if c.RequestTimeout > 0 {
		ctx2, cancel := context.WithTimeout(ctx, c.RequestTimeout)
		defer cancel()
		err = db.SelectContext(ctx2, &sessions, sql, args...)
	} else {
		err = db.SelectContext(ctx, &sessions, sql, args...)
	}
	return sessions, err
}

// CountPlaygroundSessions returns the total count of playground sessions matching the criteria.
func (c *Client) CountPlaygroundSessions(ctx context.Context, query sqrl.Sqlizer) (int, error) {
	db, err := c.getDB()
	if err != nil {
		return 0, err
	}
	sql, args, err := sqrl.Select("COUNT(*)").PlaceholderFormat(sqrl.Dollar).From(TPlaygroundSession).Where(query).ToSql()
	if err != nil {
		return 0, err
	}
	var cnt int
	err = db.GetContext(ctx, &cnt, sql, args...)
	return cnt, err
}

// SetPlaygroundSessionDeleted marks a playground session as deleted in the database.
func (c *Client) SetPlaygroundSessionDeleted(ctx context.Context, id int64) error {
	db, err := c.getDB()
	if err != nil {
		return err
	}
	cmd := fmt.Sprintf(`UPDATE %s SET is_deleted=true WHERE id=$1`, TPlaygroundSession)
	_, err = db.ExecContext(ctx, cmd, id)
	if err != nil {
		klog.ErrorS(err, "failed to update playground session db", "id", id)
		return err
	}
	return nil
}

// GetPlaygroundSession retrieves a playground session by ID.
func (c *Client) GetPlaygroundSession(ctx context.Context, id int64) (*PlaygroundSession, error) {
	if id <= 0 {
		return nil, commonerrors.NewBadRequest("id is invalid")
	}
	dbTags := GetPlaygroundSessionFieldTags()
	dbSql := sqrl.And{
		sqrl.Eq{GetFieldTag(dbTags, "IsDeleted"): false},
		sqrl.Eq{GetFieldTag(dbTags, "Id"): id},
	}
	sessions, err := c.SelectPlaygroundSessions(ctx, dbSql, nil, 1, 0)
	if err != nil {
		klog.ErrorS(err, "failed to select playground session", "id", id)
		return nil, err
	}
	if len(sessions) == 0 {
		return nil, commonerrors.NewNotFound("PlaygroundSession", fmt.Sprintf("%d", id))
	}
	return sessions[0], nil
}
