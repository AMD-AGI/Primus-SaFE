/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package utils

import (
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/lib/pq"
	dto "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	ctrlmetrics "sigs.k8s.io/controller-runtime/pkg/metrics"

	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/metrics"
)

const (
	statusSuccess = "success"
	statusError   = "error"
)

type gormRow struct {
	ID int64 `gorm:"column:id;primaryKey"`
}

func (*gormRow) TableName() string { return "gorm_metrics_test" }

// newMockGorm builds a GORM handle over go-sqlmock with the metrics plugin
// installed, mirroring what ConnectGorm does for a real connection.
func newMockGorm(t *testing.T) (*gorm.DB, sqlmock.Sqlmock) {
	t.Helper()
	sqldb, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqldb.Close() })

	db, err := gorm.Open(postgres.New(postgres.Config{
		Conn:                 sqldb,
		PreferSimpleProtocol: true,
	}), &gorm.Config{SkipDefaultTransaction: true})
	require.NoError(t, err)
	require.NoError(t, db.Use(gormMetricsPlugin{}))
	return db, mock
}

func TestGormQueriesAreInstrumented(t *testing.T) {
	db, mock := newMockGorm(t)
	before := histogramCount(t, metrics.DriverGorm, metrics.OpSelect, statusSuccess)

	mock.ExpectQuery("SELECT").WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(1)))
	var row gormRow
	require.NoError(t, db.First(&row).Error)

	assert.Equal(t, before+1, histogramCount(t, metrics.DriverGorm, metrics.OpSelect, statusSuccess))
}

// An empty result set is reported by GORM as ErrRecordNotFound. It is a normal
// outcome and must not inflate the error counter that alerts are built on.
func TestGormRecordNotFoundIsNotAnError(t *testing.T) {
	db, mock := newMockGorm(t)
	before := counterValue(t, metrics.DriverGorm, metrics.OpSelect, metrics.ErrKindNoRows)

	mock.ExpectQuery("SELECT").WillReturnRows(sqlmock.NewRows([]string{"id"}))
	var row gormRow
	require.ErrorIs(t, db.First(&row).Error, gorm.ErrRecordNotFound)

	assert.Equal(t, before, counterValue(t, metrics.DriverGorm, metrics.OpSelect, metrics.ErrKindNoRows))
}

func TestGormReadOnlyTransactionIsClassified(t *testing.T) {
	db, mock := newMockGorm(t)
	before := counterValue(t, metrics.DriverGorm, metrics.OpInsert, metrics.ErrKindReadOnlyTransaction)

	mock.ExpectQuery("INSERT").
		WillReturnError(&pq.Error{Code: "25006", Message: "cannot execute INSERT in a read-only transaction"})
	require.Error(t, db.Create(&gormRow{ID: 1}).Error)

	assert.Equal(t, before+1, counterValue(t, metrics.DriverGorm, metrics.OpInsert, metrics.ErrKindReadOnlyTransaction))
}

// histogramCount reports the observation count on one series of
// safe_db_operation_duration_seconds.
func histogramCount(t *testing.T, driver, operation, status string) uint64 {
	t.Helper()
	metric := findMetric(t, "safe_db_operation_duration_seconds", map[string]string{
		"driver": driver, "operation": operation, "status": status,
	})
	if metric == nil {
		return 0
	}
	return metric.GetHistogram().GetSampleCount()
}

// counterValue reports the value of one series of safe_db_errors_total.
func counterValue(t *testing.T, driver, operation, kind string) float64 {
	t.Helper()
	metric := findMetric(t, "safe_db_errors_total", map[string]string{
		"driver": driver, "operation": operation, "kind": kind,
	})
	if metric == nil {
		return 0
	}
	return metric.GetCounter().GetValue()
}

// findMetric gathers the default registry and returns the sample carrying
// exactly the given labels, or nil when it has not been emitted yet.
func findMetric(t *testing.T, name string, labels map[string]string) *dto.Metric {
	t.Helper()
	families, err := ctrlmetrics.Registry.Gather()
	require.NoError(t, err)
	for _, family := range families {
		if family.GetName() != name {
			continue
		}
		for _, metric := range family.GetMetric() {
			if labelsMatch(metric, labels) {
				return metric
			}
		}
	}
	return nil
}

func labelsMatch(metric *dto.Metric, want map[string]string) bool {
	got := make(map[string]string, len(metric.GetLabel()))
	for _, label := range metric.GetLabel() {
		got[label.GetName()] = label.GetValue()
	}
	for name, value := range want {
		if got[name] != value {
			return false
		}
	}
	return true
}
