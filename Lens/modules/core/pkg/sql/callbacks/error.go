// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package callbacks

import (
	"context"
	errors2 "errors"
	commonContext "github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/context"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/errors"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/sql/metrics"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/trace"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/lib/pq"
	"gorm.io/gorm"
)

const (
	ctxKeyNotAllowRecordNotFound = "_record_not_found_not_allowed"
)

func CreateErrorSolveCallback(f func(ctx context.Context, tableName string, err error) error) func(db *gorm.DB) {
	return func(db *gorm.DB) {
		if db.Error == nil {
			return
		}
		tableName := "unknown"
		if db.Statement != nil && db.Statement.Table != "" {
			tableName = db.Statement.Table
		}
		db.Error = f(db.Statement.Context, tableName, db.Error)
	}
}

func ErrorWithStack(ctx context.Context, tableName string, originErr error) error {
	if ctx != nil {
		_, exist := commonContext.GetValue(ctx, ctxKeyNotAllowRecordNotFound)
		if !exist && errors2.Is(originErr, gorm.ErrRecordNotFound) {
			return nil
		}
	}
	var pqErr *pq.Error
	caller := trace.GetNearestCaller(2)
	var err error
	errMsg := ""
	if errors2.As(originErr, &pqErr) {
		errMsg = pqErr.Message
		err = errors.NewError().WithError(originErr).WithCode(errors.CodeDatabaseError).WithMessage(pqErr.Message)
	} else {
		errMsg = originErr.Error()
		if len(errMsg) > 10 {
			errMsg = errMsg[:10]
		}
		err = errors.NewError().WithError(originErr).WithCode(errors.CodeDatabaseError)
	}
	metrics.RecordSQLError(caller, tableName, errMsg)
	return err
}

func RecordNotFoundNotAllowed(ctx context.Context) context.Context {
	return commonContext.WithObject(ctx, ctxKeyNotAllowRecordNotFound, "")
}

func RestErrorWithStack(ctx context.Context, tableName string, originErr error) error {
	if ctx != nil {
		_, exist := commonContext.GetValue(ctx, ctxKeyNotAllowRecordNotFound)
		if !exist && errors2.Is(originErr, gorm.ErrRecordNotFound) {
			return nil
		}
	}
	var pqErr *pgconn.PgError
	caller := trace.GetNearestCaller(3)
	errMsg := ""
	if errors2.As(originErr, &pqErr) {
		errMsg = pqErr.Message
	} else {
		errMsg = originErr.Error()
		if len(errMsg) > 10 {
			errMsg = errMsg[:10]
		}
	}
	metrics.RecordSQLError(caller, tableName, errMsg)
	err := errors.NewError().WithError(originErr).WithCode(errors.CodeDatabaseError)
	if errors2.Is(originErr, gorm.ErrRecordNotFound) {
		return errors.NewError().WithMessage("Not Existed")
	}
	return errors.WrapError(err, "", errors.CodeDatabaseError)
}
