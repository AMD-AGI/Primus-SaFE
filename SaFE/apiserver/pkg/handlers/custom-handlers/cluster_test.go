/*
 * Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package custom_handlers

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/custom-handlers/types"
)

// TestCvtToClusterResponseItem tests conversion from v1.Cluster to ClusterResponseItem
func TestCvtToClusterResponseItem(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name    string
		cluster *v1.Cluster
		want    types.ClusterResponseItem
	}{
		{
			name: "basic cluster",
			cluster: &v1.Cluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "test-cluster",
					CreationTimestamp: metav1.NewTime(now),
					Labels: map[string]string{
						v1.UserIdLabel: "user-123",
					},
				},
				Status: v1.ClusterStatus{
					ControlPlaneStatus: v1.ControlPlaneStatus{
						Phase: v1.ReadyPhase,
					},
				},
			},
			want: types.ClusterResponseItem{
				ClusterId:    "test-cluster",
				UserId:       "user-123",
				Phase:        string(v1.ReadyPhase),
				IsProtected:  false,
				CreationTime: now.Format("2006-01-02T15:04:05"),
			},
		},
		{
			name: "protected cluster",
			cluster: &v1.Cluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "prod-cluster",
					CreationTimestamp: metav1.NewTime(now),
					Labels: map[string]string{
						v1.UserIdLabel:  "admin-user",
						v1.ProtectLabel: "",
					},
				},
				Status: v1.ClusterStatus{
					ControlPlaneStatus: v1.ControlPlaneStatus{
						Phase: v1.ReadyPhase,
					},
				},
			},
			want: types.ClusterResponseItem{
				ClusterId:    "prod-cluster",
				UserId:       "admin-user",
				Phase:        string(v1.ReadyPhase),
				IsProtected:  true,
				CreationTime: now.Format("2006-01-02T15:04:05"),
			},
		},
		{
			name: "cluster without user",
			cluster: &v1.Cluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "no-user-cluster",
					CreationTimestamp: metav1.NewTime(now),
				},
				Status: v1.ClusterStatus{
					ControlPlaneStatus: v1.ControlPlaneStatus{
						Phase: v1.CreatingPhase,
					},
				},
			},
			want: types.ClusterResponseItem{
				ClusterId:    "no-user-cluster",
				UserId:       "",
				Phase:        string(v1.CreatingPhase),
				IsProtected:  false,
				CreationTime: now.Format("2006-01-02T15:04:05"),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := cvtToClusterResponseItem(tt.cluster)
			assert.Equal(t, tt.want.ClusterId, result.ClusterId)
			assert.Equal(t, tt.want.UserId, result.UserId)
			assert.Equal(t, tt.want.Phase, result.Phase)
			assert.Equal(t, tt.want.IsProtected, result.IsProtected)
			// Time comparison - format should match
			assert.Contains(t, result.CreationTime, now.Format("2006-01-02"))
		})
	}
}

// TestParseProcessNodesRequest tests parsing of process nodes request
func TestParseProcessNodesRequest(t *testing.T) {
	tests := []struct {
		name    string
		action  string
		nodeIds []string
		wantErr bool
	}{
		{
			name:    "add nodes",
			action:  "add",
			nodeIds: []string{"node1", "node2", "node3"},
			wantErr: false,
		},
		{
			name:    "remove nodes",
			action:  "remove",
			nodeIds: []string{"node1"},
			wantErr: false,
		},
		{
			name:    "empty node list",
			action:  "add",
			nodeIds: []string{},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := &types.ProcessNodesRequest{
				Action:  tt.action,
				NodeIds: tt.nodeIds,
			}

			assert.Equal(t, tt.action, req.Action)
			assert.Equal(t, len(tt.nodeIds), len(req.NodeIds))
		})
	}
}
