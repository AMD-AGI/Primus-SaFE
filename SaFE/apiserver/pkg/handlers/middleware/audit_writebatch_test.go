/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package middleware

import (
	"errors"
	"testing"

	"github.com/golang/mock/gomock"

	dbclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
	mock_client "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client/mock"
)

func TestWriteBatchSuccess(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	m := mock_client.NewMockInterface(ctrl)
	m.EXPECT().BatchInsertAuditLogs(gomock.Any(), gomock.Any()).Return(nil)

	b := &auditLogBuffer{client: m}
	b.writeBatch([]*dbclient.AuditLog{{UserId: "u1"}})
}

func TestWriteBatchFallbackToIndividual(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	m := mock_client.NewMockInterface(ctrl)
	// Batch insert fails -> fall back to individual inserts (one ok, one error).
	m.EXPECT().BatchInsertAuditLogs(gomock.Any(), gomock.Any()).Return(errors.New("batch failed"))
	m.EXPECT().InsertAuditLog(gomock.Any(), gomock.Any()).Return(nil)
	m.EXPECT().InsertAuditLog(gomock.Any(), gomock.Any()).Return(errors.New("insert failed"))

	b := &auditLogBuffer{client: m}
	b.writeBatch([]*dbclient.AuditLog{{UserId: "u1"}, {UserId: "u2"}})
}

func TestWriteBatchEmpty(t *testing.T) {
	b := &auditLogBuffer{}
	b.writeBatch(nil)
}
