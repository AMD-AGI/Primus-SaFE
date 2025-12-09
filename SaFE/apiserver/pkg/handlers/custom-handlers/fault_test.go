/*
 * Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package custom_handlers

import (
	"database/sql"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/lib/pq"
	"github.com/stretchr/testify/assert"

	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/custom-handlers/types"
	dbclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
)

// TestCvtToFaultResponseItem tests the conversion from database Fault to response item
func TestCvtToFaultResponseItem(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name     string
		fault    *dbclient.Fault
		validate func(*testing.T, types.FaultResponseItem)
	}{
		{
			name: "complete fault record",
			fault: &dbclient.Fault{
				Uid:          "fault-001",
				Node:         sql.NullString{String: "node-1", Valid: true},
				MonitorId:    "monitor-disk-001",
				Message:      sql.NullString{String: "Disk usage above 90%", Valid: true},
				Action:       sql.NullString{String: "Alert", Valid: true},
				Phase:        sql.NullString{String: "Active", Valid: true},
				Cluster:      sql.NullString{String: "cluster-1", Valid: true},
				CreationTime: pq.NullTime{Time: now, Valid: true},
				DeletionTime: pq.NullTime{Valid: false},
			},
			validate: func(t *testing.T, result types.FaultResponseItem) {
				assert.Equal(t, "fault-001", result.ID)
				assert.Equal(t, "node-1", result.NodeId)
				assert.Equal(t, "monitor-disk-001", result.MonitorId)
				assert.Equal(t, "Disk usage above 90%", result.Message)
				assert.Equal(t, "Alert", result.Action)
				assert.Equal(t, "Active", result.Phase)
				assert.Equal(t, "cluster-1", result.ClusterId)
				assert.NotEmpty(t, result.CreationTime)
				assert.Empty(t, result.DeletionTime)
			},
		},
		{
			name: "fault record with null fields",
			fault: &dbclient.Fault{
				Uid:          "fault-002",
				Node:         sql.NullString{Valid: false},
				MonitorId:    "monitor-memory-002",
				Message:      sql.NullString{Valid: false},
				Action:       sql.NullString{Valid: false},
				Phase:        sql.NullString{Valid: false},
				Cluster:      sql.NullString{Valid: false},
				CreationTime: pq.NullTime{Valid: false},
				DeletionTime: pq.NullTime{Valid: false},
			},
			validate: func(t *testing.T, result types.FaultResponseItem) {
				assert.Equal(t, "fault-002", result.ID)
				assert.Empty(t, result.NodeId)
				assert.Equal(t, "monitor-memory-002", result.MonitorId)
				assert.Empty(t, result.Message)
				assert.Empty(t, result.Action)
				assert.Empty(t, result.Phase)
				assert.Empty(t, result.ClusterId)
				assert.Empty(t, result.CreationTime)
				assert.Empty(t, result.DeletionTime)
			},
		},
		{
			name: "resolved fault with deletion time",
			fault: &dbclient.Fault{
				Uid:          "fault-003",
				Node:         sql.NullString{String: "node-3", Valid: true},
				MonitorId:    "monitor-cpu-003",
				Message:      sql.NullString{String: "CPU usage normalized", Valid: true},
				Action:       sql.NullString{String: "Resolved", Valid: true},
				Phase:        sql.NullString{String: "Resolved", Valid: true},
				Cluster:      sql.NullString{String: "cluster-2", Valid: true},
				CreationTime: pq.NullTime{Time: now.Add(-1 * time.Hour), Valid: true},
				DeletionTime: pq.NullTime{Time: now, Valid: true},
			},
			validate: func(t *testing.T, result types.FaultResponseItem) {
				assert.Equal(t, "fault-003", result.ID)
				assert.Equal(t, "node-3", result.NodeId)
				assert.Equal(t, "Resolved", result.Phase)
				assert.NotEmpty(t, result.CreationTime)
				assert.NotEmpty(t, result.DeletionTime)
			},
		},
		{
			name: "critical fault",
			fault: &dbclient.Fault{
				Uid:          "fault-004",
				Node:         sql.NullString{String: "node-4", Valid: true},
				MonitorId:    "monitor-network-004",
				Message:      sql.NullString{String: "Network interface down", Valid: true},
				Action:       sql.NullString{String: "Critical", Valid: true},
				Phase:        sql.NullString{String: "Critical", Valid: true},
				Cluster:      sql.NullString{String: "prod-cluster", Valid: true},
				CreationTime: pq.NullTime{Time: now, Valid: true},
				DeletionTime: pq.NullTime{Valid: false},
			},
			validate: func(t *testing.T, result types.FaultResponseItem) {
				assert.Equal(t, "fault-004", result.ID)
				assert.Equal(t, "Critical", result.Phase)
				assert.Equal(t, "Network interface down", result.Message)
				assert.Equal(t, "prod-cluster", result.ClusterId)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := cvtToFaultResponseItem(tt.fault)
			tt.validate(t, result)
		})
	}
}

// TestParseListFaultQuery tests parsing of fault list query parameters
func TestParseListFaultQuery(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name        string
		queryParams map[string]string
		validate    func(*testing.T, *types.ListFaultRequest, error)
	}{
		{
			name:        "empty query uses defaults",
			queryParams: map[string]string{},
			validate: func(t *testing.T, query *types.ListFaultRequest, err error) {
				assert.NoError(t, err)
				assert.Equal(t, types.DefaultQueryLimit, query.Limit)
				assert.Equal(t, dbclient.DESC, query.Order)
				assert.NotEmpty(t, query.SortBy)
			},
		},
		{
			name: "custom limit and offset",
			queryParams: map[string]string{
				"limit":  "50",
				"offset": "10",
			},
			validate: func(t *testing.T, query *types.ListFaultRequest, err error) {
				assert.NoError(t, err)
				assert.Equal(t, 50, query.Limit)
				assert.Equal(t, 10, query.Offset)
			},
		},
		{
			name: "sort ascending",
			queryParams: map[string]string{
				"order": "asc",
			},
			validate: func(t *testing.T, query *types.ListFaultRequest, err error) {
				assert.NoError(t, err)
				assert.Equal(t, "asc", query.Order)
			},
		},
		{
			name: "filter by monitor ID",
			queryParams: map[string]string{
				"monitorId": "monitor-001",
			},
			validate: func(t *testing.T, query *types.ListFaultRequest, err error) {
				assert.NoError(t, err)
				assert.Equal(t, "monitor-001", query.MonitorId)
			},
		},
		{
			name: "filter by cluster ID",
			queryParams: map[string]string{
				"clusterId": "cluster-prod",
			},
			validate: func(t *testing.T, query *types.ListFaultRequest, err error) {
				assert.NoError(t, err)
				assert.Equal(t, "cluster-prod", query.ClusterId)
			},
		},
		{
			name: "filter by node ID",
			queryParams: map[string]string{
				"nodeId": "node-1",
			},
			validate: func(t *testing.T, query *types.ListFaultRequest, err error) {
				assert.NoError(t, err)
				assert.Equal(t, "node-1", query.NodeId)
			},
		},
		{
			name: "only open faults",
			queryParams: map[string]string{
				"onlyOpen": "true",
			},
			validate: func(t *testing.T, query *types.ListFaultRequest, err error) {
				assert.NoError(t, err)
				assert.True(t, query.OnlyOpen)
			},
		},
		{
			name: "combined parameters",
			queryParams: map[string]string{
				"limit":     "20",
				"offset":    "5",
				"sortBy":    "creation_time",
				"order":     "desc",
				"monitorId": "monitor-001,monitor-002",
				"clusterId": "cluster-prod",
				"nodeId":    "node-1",
				"onlyOpen":  "true",
			},
			validate: func(t *testing.T, query *types.ListFaultRequest, err error) {
				assert.NoError(t, err)
				assert.Equal(t, 20, query.Limit)
				assert.Equal(t, 5, query.Offset)
				assert.Equal(t, "creation_time", query.SortBy)
				assert.Equal(t, "desc", query.Order)
				assert.Equal(t, "monitor-001,monitor-002", query.MonitorId)
				assert.Equal(t, "cluster-prod", query.ClusterId)
				assert.Equal(t, "node-1", query.NodeId)
				assert.True(t, query.OnlyOpen)
			},
		},
		{
			name: "invalid order value",
			queryParams: map[string]string{
				"order": "invalid",
			},
			validate: func(t *testing.T, query *types.ListFaultRequest, err error) {
				assert.Error(t, err)
			},
		},
		{
			name: "negative limit uses default",
			queryParams: map[string]string{
				"limit": "-1",
			},
			validate: func(t *testing.T, query *types.ListFaultRequest, err error) {
				// Negative limit should be rejected or use default
				if err == nil {
					assert.Equal(t, types.DefaultQueryLimit, query.Limit)
				}
			},
		},
		{
			name: "zero limit uses default",
			queryParams: map[string]string{
				"limit": "0",
			},
			validate: func(t *testing.T, query *types.ListFaultRequest, err error) {
				assert.NoError(t, err)
				assert.Equal(t, types.DefaultQueryLimit, query.Limit)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create test request
			req := httptest.NewRequest(http.MethodGet, "/api/v1/faults", nil)
			q := req.URL.Query()
			for key, val := range tt.queryParams {
				q.Add(key, val)
			}
			req.URL.RawQuery = q.Encode()

			// Create gin context
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = req

			query, err := parseListFaultQuery(c)
			tt.validate(t, query, err)
		})
	}
}

// TestBuildOrderBy tests the order by clause builder
func TestBuildOrderBy_Fault(t *testing.T) {
	dbTags := dbclient.GetFaultFieldTags()

	tests := []struct {
		name     string
		sortBy   string
		order    string
		validate func(*testing.T, []string)
	}{
		{
			name:   "valid sort by and order",
			sortBy: dbclient.GetFieldTag(dbTags, "CreationTime"),
			order:  "desc",
			validate: func(t *testing.T, result []string) {
				assert.Len(t, result, 1)
				assert.Contains(t, result[0], "desc")
			},
		},
		{
			name:   "ascending order",
			sortBy: dbclient.GetFieldTag(dbTags, "CreationTime"),
			order:  "asc",
			validate: func(t *testing.T, result []string) {
				assert.Len(t, result, 1)
				assert.Contains(t, result[0], "asc")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := buildOrderBy(tt.sortBy, tt.order, dbTags)
			tt.validate(t, result)
		})
	}
}
