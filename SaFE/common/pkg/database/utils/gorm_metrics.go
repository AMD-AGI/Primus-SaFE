/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package utils

import (
	"errors"
	"time"

	"gorm.io/gorm"

	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/metrics"
)

const (
	gormPluginName = "safe:metrics"
	gormStartKey   = "safe:metrics:start"
)

// gormMetricsPlugin records Prometheus metrics for every statement issued
// through GORM by pairing a before callback that stamps the start time with an
// after callback that reports the latency and error.
type gormMetricsPlugin struct{}

func (gormMetricsPlugin) Name() string { return gormPluginName }

func (gormMetricsPlugin) Initialize(db *gorm.DB) error {
	cb := db.Callback()
	return errors.Join(
		cb.Create().Before("gorm:create").Register(beforeName("create"), gormBefore),
		cb.Create().After("gorm:create").Register(afterName("create"), gormAfter(metrics.OpInsert)),
		cb.Query().Before("gorm:query").Register(beforeName("query"), gormBefore),
		cb.Query().After("gorm:query").Register(afterName("query"), gormAfter(metrics.OpSelect)),
		cb.Update().Before("gorm:update").Register(beforeName("update"), gormBefore),
		cb.Update().After("gorm:update").Register(afterName("update"), gormAfter(metrics.OpUpdate)),
		cb.Delete().Before("gorm:delete").Register(beforeName("delete"), gormBefore),
		cb.Delete().After("gorm:delete").Register(afterName("delete"), gormAfter(metrics.OpDelete)),
		cb.Row().Before("gorm:row").Register(beforeName("row"), gormBefore),
		cb.Row().After("gorm:row").Register(afterName("row"), gormAfter(metrics.OpOther)),
		cb.Raw().Before("gorm:raw").Register(beforeName("raw"), gormBefore),
		cb.Raw().After("gorm:raw").Register(afterName("raw"), gormAfter(metrics.OpOther)),
	)
}

func beforeName(action string) string { return gormPluginName + ":before_" + action }

func afterName(action string) string { return gormPluginName + ":after_" + action }

// gormBefore stamps the statement with its start time.
func gormBefore(db *gorm.DB) {
	db.InstanceSet(gormStartKey, time.Now())
}

// gormAfter reports the outcome of the statement. The SQL verb is preferred
// over the fallback because Raw and Row statements can be anything.
func gormAfter(fallback string) func(*gorm.DB) {
	return func(db *gorm.DB) {
		value, ok := db.InstanceGet(gormStartKey)
		if !ok {
			return
		}
		start, ok := value.(time.Time)
		if !ok {
			return
		}
		operation := fallback
		if statement := db.Statement.SQL.String(); statement != "" {
			operation = metrics.SQLOperation(statement)
		}
		metrics.ObserveDBOperation(metrics.DriverGorm, operation, start, db.Error)
	}
}
