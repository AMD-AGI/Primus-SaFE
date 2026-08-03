/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package client

import (
	"context"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/lib/pq"
	dto "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	ctrlmetrics "sigs.k8s.io/controller-runtime/pkg/metrics"

	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/metrics"
)

// The status label values are part of the metric contract that alert rules are
// written against, so they are spelled out here rather than imported.
const (
	statusSuccess = "success"
	statusError   = "error"
)

func TestSqlxQueriesAreInstrumented(t *testing.T) {
	c, mock := newMockClient(t)
	before := histogramCount(t, metrics.DriverSqlx, metrics.OpInsert, statusSuccess)

	mock.ExpectExec("INSERT INTO user_token").WillReturnResult(sqlmock.NewResult(1, 1))
	require.NoError(t, c.UpsertUserToken(context.Background(), &UserToken{UserId: "u", SessionId: "s"}))

	assert.Equal(t, before+1, histogramCount(t, metrics.DriverSqlx, metrics.OpInsert, statusSuccess))
}

// A read-only transaction rejection is what a PostgreSQL replica returns while
// it is being promoted. It must reach the metrics as its own error kind so that
// it can be alerted on separately from ordinary query failures.
func TestSqlxReadOnlyTransactionIsClassified(t *testing.T) {
	c, mock := newMockClient(t)
	before := counterValue(t, metrics.DriverSqlx, metrics.OpInsert, metrics.ErrKindReadOnlyTransaction)

	mock.ExpectExec("INSERT INTO user_token").
		WillReturnError(&pq.Error{Code: "25006", Message: "cannot execute INSERT in a read-only transaction"})
	require.Error(t, c.UpsertUserToken(context.Background(), &UserToken{UserId: "u", SessionId: "s"}))

	assert.Equal(t, before+1, counterValue(t, metrics.DriverSqlx, metrics.OpInsert, metrics.ErrKindReadOnlyTransaction))
	assert.Positive(t, histogramCount(t, metrics.DriverSqlx, metrics.OpInsert, statusError))
}

func TestSqlxSelectIsInstrumented(t *testing.T) {
	c, mock := newMockClient(t)
	before := histogramCount(t, metrics.DriverSqlx, metrics.OpSelect, statusSuccess)

	mock.ExpectQuery("SELECT").WillReturnRows(sqlmock.NewRows([]string{"user_id"}).AddRow("u"))
	_, err := c.SelectUserTokens(context.Background(), nil, nil, 1, 0)
	require.NoError(t, err)

	assert.Equal(t, before+1, histogramCount(t, metrics.DriverSqlx, metrics.OpSelect, statusSuccess))
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
