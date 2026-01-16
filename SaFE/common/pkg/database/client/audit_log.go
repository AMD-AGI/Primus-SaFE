/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package client

import (
	"context"
	"fmt"

	commonerrors "github.com/AMD-AIG-AIMA/SAFE/common/pkg/errors"
	sqrl "github.com/Masterminds/squirrel"
)

const (
	TPAuditLog = "audit_logs"
)

var (
	insertAuditLogFormat = `INSERT INTO ` + TPAuditLog + ` (%s) VALUES (%s);`
)

// InsertAuditLog inserts a new audit log entry into the database.
func (c *Client) InsertAuditLog(ctx context.Context, auditLog *AuditLog) error {
	if auditLog == nil {
		return commonerrors.NewBadRequest("the input is empty")
	}
	db, err := c.getDB()
	if err != nil {
		return err
	}

	_, err = db.NamedExecContext(ctx, generateCommand(*auditLog, insertAuditLogFormat, "id"), auditLog)
	if err != nil {
		return fmt.Errorf("failed to insert audit_log to db: %v", err)
	}
	return nil
}

// SelectAuditLogs retrieves audit logs based on query conditions.
func (c *Client) SelectAuditLogs(ctx context.Context, query sqrl.Sqlizer, orderBy []string, limit, offset int) ([]*AuditLog, error) {
	db, err := c.getDB()
	if err != nil {
		return nil, err
	}

	builder := sqrl.StatementBuilder.PlaceholderFormat(sqrl.Dollar).
		Select("*").From(TPAuditLog)

	if query != nil {
		builder = builder.Where(query)
	}
	for _, order := range orderBy {
		builder = builder.OrderBy(order)
	}
	if limit > 0 {
		builder = builder.Limit(uint64(limit))
	}
	if offset > 0 {
		builder = builder.Offset(uint64(offset))
	}

	sql, args, err := builder.ToSql()
	if err != nil {
		return nil, fmt.Errorf("failed to build select audit_logs query: %v", err)
	}

	var auditLogs []*AuditLog
	err = db.SelectContext(ctx, &auditLogs, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to select audit_logs from db: %v", err)
	}
	return auditLogs, nil
}

// CountAuditLogs counts audit logs based on query conditions.
func (c *Client) CountAuditLogs(ctx context.Context, query sqrl.Sqlizer) (int, error) {
	db, err := c.getDB()
	if err != nil {
		return 0, err
	}

	builder := sqrl.StatementBuilder.PlaceholderFormat(sqrl.Dollar).
		Select("COUNT(*)").From(TPAuditLog)

	if query != nil {
		builder = builder.Where(query)
	}

	sql, args, err := builder.ToSql()
	if err != nil {
		return 0, fmt.Errorf("failed to build count audit_logs query: %v", err)
	}

	var count int
	err = db.GetContext(ctx, &count, sql, args...)
	if err != nil {
		return 0, fmt.Errorf("failed to count audit_logs from db: %v", err)
	}
	return count, nil
}
