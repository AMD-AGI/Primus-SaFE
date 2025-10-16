/*
 * Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package client

import (
	"context"
	"fmt"
	"time"

	commonerrors "github.com/AMD-AIG-AIMA/SAFE/common/pkg/errors"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"

	dbutils "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/utils"
)

const (
	TPSshSessionRecord = "ssh_session_records"
)

var (
	insertSshSessionRecordFormat = `INSERT INTO ` + TPSshSessionRecord + ` (%s) VALUES (%s) RETURNING id;`
)

func (c *Client) InsertSshSessionRecord(ctx context.Context, record *SshSessionRecords) (int64, error) {
	var insertId int64
	if record == nil {
		return insertId, commonerrors.NewBadRequest("the input is empty")
	}
	db, err := c.getDB()
	if err != nil {
		return insertId, err
	}

	rows, err := db.NamedQueryContext(ctx, genInsertCommand(*record, insertSshSessionRecordFormat, "id"), record)
	if err != nil {
		return insertId, fmt.Errorf("failed to insert ssh_session_records db err: %v", err)
	}
	defer rows.Close()

	if rows.Next() {
		if err = rows.Scan(&insertId); err != nil {
			return insertId, fmt.Errorf("scan id err: %v", err)
		}
	}
	return insertId, err
}

func (c *Client) SetSshDisconnect(ctx context.Context, id int64, disconnectReason string) error {
	db, err := c.getDB()
	if err != nil {
		return err
	}
	nowTime := dbutils.NullMetaV1Time(&metav1.Time{Time: time.Now().UTC()})
	cmd := fmt.Sprintf(`UPDATE %s SET disconnect_reason=$1, disconnect_time=$2 WHERE id=$3`,
		TPSshSessionRecord)
	_, err = db.ExecContext(ctx, cmd, disconnectReason, nowTime, id)
	if err != nil {
		klog.ErrorS(err, "failed to update publicKey db. ", "id", id)
		return err
	}
	return nil
}
