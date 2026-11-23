/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package client

import (
	"context"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"

	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client/model"
)

func TestUpsertWorkspaceUsageSamplesConflictKey(t *testing.T) {
	c, mock := newMockClient(t)
	mock.ExpectQuery(`INSERT INTO "workspace_usage_sample" .* ON CONFLICT \("cluster","workspace","resource","sample_time"\) DO UPDATE SET "quota"="excluded"."quota"`).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(1)))
	err := c.UpsertWorkspaceUsageSamples(context.Background(), []*model.WorkspaceUsageSample{{
		SampleTime: time.Unix(0, 0), Cluster: "c1", Workspace: "ws", Resource: "amd.com/gpu",
	}})
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestUpsertWorkspaceUsageHourlyConflictKey(t *testing.T) {
	c, mock := newMockClient(t)
	mock.ExpectQuery(`INSERT INTO "workspace_usage_hourly" .* ON CONFLICT \("cluster","workspace","resource","hour"\) DO UPDATE SET "quota_avg"="excluded"."quota_avg".*` +
		`"queued_quota_avg"="excluded"."queued_quota_avg".*"queued_other_max"="excluded"."queued_other_max"`).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(1)))
	err := c.UpsertWorkspaceUsageHourly(context.Background(), []*model.WorkspaceUsageHourly{{
		Hour: time.Unix(0, 0), Cluster: "c1", Workspace: "ws", Resource: "amd.com/gpu",
	}})
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestWorkspaceUsageEmptyInputsSkipTheDatabase(t *testing.T) {
	c, mock := newMockClient(t)
	ctx := context.Background()
	require.NoError(t, c.UpsertWorkspaceUsageSamples(ctx, nil))
	require.NoError(t, c.UpsertWorkspaceUsageHourly(ctx, nil))
	none := WorkspaceUsageFilter{Workspaces: []string{}}
	rows, err := c.ListWorkspaceUsageSamples(ctx, none)
	require.NoError(t, err)
	require.Empty(t, rows)
	latest, err := c.ListLatestWorkspaceUsageSamples(ctx, none)
	require.NoError(t, err)
	require.Empty(t, latest)
	hourly, err := c.AggregateWorkspaceUsageHourly(ctx, none, time.Unix(0, 0), time.Hour)
	require.NoError(t, err)
	require.Empty(t, hourly)
	aggregated, err := c.AggregateWorkspaceUsageSamples(ctx, none, time.Unix(0, 0), time.Hour, 5*time.Minute)
	require.NoError(t, err)
	require.Empty(t, aggregated)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestAggregateWorkspaceUsageHourlyBucketsAndClipsInTheDatabase(t *testing.T) {
	c, mock := newMockClient(t)
	origin := time.Date(2026, 11, 10, 0, 0, 0, 0, time.UTC)
	start, end := origin.Add(90*time.Minute), origin.Add(72*time.Hour)
	mock.ExpectQuery(`SELECT cluster, workspace, resource, bucket AS hour, ` +
		`coalesce\(sum\(w \* quota_avg\) / nullif\(sum\(w\), 0\), 0\) AS quota_avg, .*` +
		`max\(cluster_queued_max\) AS cluster_queued_max, .*sum\(part \* allocated_hours\) AS allocated_hours, .*` +
		`FROM \(SELECT \*, part \* coalesce\(nullif\(observed_seconds, 0\), 3600\) AS w ` +
		`FROM \(SELECT \*, .* AS bucket, .* AS part FROM "workspace_usage_hourly" ` +
		`WHERE resource = .* AND workspace IN \(.*\) AND hour >= .* AND hour < .*\) AS clipped\) AS weighted ` +
		`GROUP BY cluster, workspace, resource, bucket ORDER BY bucket`).
		WillReturnRows(sqlmock.NewRows([]string{"hour", "workspace", "observed_seconds"}).AddRow(origin, "a", 1800.0))
	rows, err := c.AggregateWorkspaceUsageHourly(context.Background(), WorkspaceUsageFilter{
		Resource: "amd.com/gpu", Workspaces: []string{"a"}, Start: start, End: end,
	}, origin, 3*time.Hour)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Equal(t, 1800.0, rows[0].ObservedSeconds)
	require.NoError(t, mock.ExpectationsWereMet())

	_, err = c.AggregateWorkspaceUsageHourly(context.Background(), WorkspaceUsageFilter{End: end}, origin, time.Hour)
	require.Error(t, err, "an open range is refused")
}

func TestAggregateWorkspaceUsageSamplesBucketsInTheDatabase(t *testing.T) {
	c, mock := newMockClient(t)
	origin := time.Date(2026, 11, 10, 0, 0, 0, 0, time.UTC)
	start, end := origin.Add(2*time.Hour), origin.Add(72*time.Hour)
	mock.ExpectQuery(`SELECT cluster, workspace, resource, bucket AS hour, ` +
		`coalesce\(sum\(w \* quota\) / nullif\(sum\(w\), 0\), 0\) AS quota_avg, .*` +
		`max\(cluster_queued\) AS cluster_queued_max, .*sum\(w \* allocated\) / 3600 AS allocated_hours, .*` +
		`FROM \(SELECT \*, sum\(queued\) OVER \(PARTITION BY cluster, resource, sample_time\) AS cluster_queued ` +
		`FROM \(SELECT \*, .* AS queued, .* AS bucket, .* AS w FROM "workspace_usage_sample" ` +
		`WHERE resource = .* AND workspace IN \(.*\) AND sample_time >= .* AND sample_time < .*\) AS clipped\) AS weighted ` +
		`GROUP BY cluster, workspace, resource, bucket ORDER BY bucket`).
		WillReturnRows(sqlmock.NewRows([]string{"hour", "workspace", "observed_seconds", "samples"}).AddRow(origin, "a", 3600.0, 12))
	rows, err := c.AggregateWorkspaceUsageSamples(context.Background(), WorkspaceUsageFilter{
		Resource: "amd.com/gpu", Workspaces: []string{"a"}, Start: start, End: end,
	}, origin, 3*time.Hour, 5*time.Minute)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Equal(t, int32(12), rows[0].Samples)
	require.NoError(t, mock.ExpectationsWereMet())

	_, err = c.AggregateWorkspaceUsageSamples(context.Background(), WorkspaceUsageFilter{Start: start, End: end}, origin, time.Hour, 0)
	require.Error(t, err, "an interval is needed")
}

func TestListWorkspaceUsageSamplesFilter(t *testing.T) {
	c, mock := newMockClient(t)
	start, end := time.Unix(100, 0), time.Unix(200, 0)
	mock.ExpectQuery(`SELECT \* FROM "workspace_usage_sample" WHERE resource = \$1 AND cluster = \$2 AND workspace IN \(\$3,\$4\) AND sample_time >= \$5 AND sample_time < \$6 ORDER BY sample_time`).
		WithArgs("amd.com/gpu", "c1", "a", "b", start, end).
		WillReturnRows(sqlmock.NewRows([]string{"id", "workspace"}).AddRow(int64(1), "a"))
	rows, err := c.ListWorkspaceUsageSamples(context.Background(), WorkspaceUsageFilter{
		Resource: "amd.com/gpu", Cluster: "c1", Workspaces: []string{"a", "b"}, Start: start, End: end,
	})
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestListLatestWorkspaceUsageSamplesPicksTheLatestTimePerCluster(t *testing.T) {
	c, mock := newMockClient(t)
	since := time.Unix(100, 0)
	mock.ExpectQuery(`SELECT \* FROM "workspace_usage_sample" WHERE resource = \$1 AND workspace IN \(\$2\) AND sample_time >= \$3 AND \(cluster, sample_time\) IN \(SELECT cluster, max\(sample_time\) FROM "workspace_usage_sample" WHERE resource = \$4 AND sample_time >= \$5 GROUP BY "cluster"\) ORDER BY cluster,workspace`).
		WithArgs("amd.com/gpu", "a", since, "amd.com/gpu", since).
		WillReturnRows(sqlmock.NewRows([]string{"id"}))
	_, err := c.ListLatestWorkspaceUsageSamples(context.Background(), WorkspaceUsageFilter{
		Resource: "amd.com/gpu", Workspaces: []string{"a"}, Start: since,
	})
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestDeleteWorkspaceUsageBefore(t *testing.T) {
	c, mock := newMockClient(t)
	before := time.Unix(100, 0)
	mock.ExpectExec(`DELETE FROM "workspace_usage_sample" WHERE sample_time < \$1`).WithArgs(before).
		WillReturnResult(sqlmock.NewResult(0, 3))
	mock.ExpectExec(`DELETE FROM "workspace_usage_hourly" WHERE hour < \$1`).WithArgs(before).
		WillReturnResult(sqlmock.NewResult(0, 2))
	n, err := c.DeleteWorkspaceUsageSamplesBefore(context.Background(), before)
	require.NoError(t, err)
	require.Equal(t, int64(3), n)
	n, err = c.DeleteWorkspaceUsageHourlyBefore(context.Background(), before)
	require.NoError(t, err)
	require.Equal(t, int64(2), n)
	require.NoError(t, mock.ExpectationsWereMet())
}
