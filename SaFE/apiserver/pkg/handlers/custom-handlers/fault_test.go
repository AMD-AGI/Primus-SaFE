/*
 * Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package custom_handlers

import (
	"database/sql"
	"testing"
	"time"

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
