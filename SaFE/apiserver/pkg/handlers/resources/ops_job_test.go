/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package resources

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/lib/pq"
	"github.com/stretchr/testify/assert"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/resources/view"
	dbclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
)

// TestCvtToOpsJobResponseItem tests conversion from database OpsJob to response item
func TestCvtToOpsJobResponseItem(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name     string
		job      *dbclient.OpsJob
		validate func(*testing.T, view.OpsJobResponseItem)
	}{
		{
			name: "complete ops job",
			job: &dbclient.OpsJob{
				JobId:        "preflight-job-123",
				Cluster:      "test-cluster",
				Workspace:    sql.NullString{String: "test-workspace", Valid: true},
				UserId:       sql.NullString{String: "user-123", Valid: true},
				UserName:     sql.NullString{String: "testuser", Valid: true},
				Type:         string(v1.OpsJobPreflightType),
				Phase:        sql.NullString{String: string(v1.OpsJobRunning), Valid: true},
				CreationTime: pq.NullTime{Time: now, Valid: true},
				StartTime:    pq.NullTime{Time: now.Add(1 * time.Minute), Valid: true},
				EndTime:      pq.NullTime{Time: now.Add(10 * time.Minute), Valid: true},
				Timeout:      600,
			},
			validate: func(t *testing.T, result view.OpsJobResponseItem) {
				assert.Equal(t, "preflight-job-123", result.JobId)
				assert.Equal(t, "test-cluster", result.ClusterId)
				assert.Equal(t, "test-workspace", result.WorkspaceId)
				assert.Equal(t, "user-123", result.UserId)
				assert.Equal(t, "testuser", result.UserName)
				assert.Equal(t, v1.OpsJobPreflightType, result.Type)
				assert.Equal(t, v1.OpsJobRunning, result.Phase)
				assert.Equal(t, 600, result.TimeoutSecond)
				assert.NotEmpty(t, result.CreationTime)
			},
		},
		{
			name: "minimal ops job with null fields",
			job: &dbclient.OpsJob{
				JobId:     "addon-job-456",
				Cluster:   "prod-cluster",
				Type:      string(v1.OpsJobAddonType),
				Workspace: sql.NullString{Valid: false},
				UserId:    sql.NullString{Valid: false},
				UserName:  sql.NullString{Valid: false},
				Phase:     sql.NullString{Valid: false},
				Timeout:   0,
			},
			validate: func(t *testing.T, result view.OpsJobResponseItem) {
				assert.Equal(t, "addon-job-456", result.JobId)
				assert.Equal(t, "prod-cluster", result.ClusterId)
				assert.Equal(t, v1.OpsJobAddonType, result.Type)
				assert.Empty(t, result.WorkspaceId)
				assert.Empty(t, result.UserId)
				assert.Empty(t, result.UserName)
				// Empty phase should default to Pending
				assert.Equal(t, v1.OpsJobPending, result.Phase)
			},
		},
		{
			name: "dumplog job",
			job: &dbclient.OpsJob{
				JobId:        "dumplog-job-789",
				Cluster:      "debug-cluster",
				Type:         string(v1.OpsJobDumpLogType),
				Phase:        sql.NullString{String: string(v1.OpsJobSucceeded), Valid: true},
				CreationTime: pq.NullTime{Time: now, Valid: true},
				EndTime:      pq.NullTime{Time: now.Add(5 * time.Minute), Valid: true},
				Timeout:      300,
			},
			validate: func(t *testing.T, result view.OpsJobResponseItem) {
				assert.Equal(t, "dumplog-job-789", result.JobId)
				assert.Equal(t, v1.OpsJobDumpLogType, result.Type)
				assert.Equal(t, v1.OpsJobSucceeded, result.Phase)
				assert.Equal(t, 300, result.TimeoutSecond)
			},
		},
		{
			name: "failed ops job",
			job: &dbclient.OpsJob{
				JobId:        "failed-job-001",
				Cluster:      "test-cluster",
				Type:         string(v1.OpsJobPreflightType),
				Phase:        sql.NullString{String: string(v1.OpsJobFailed), Valid: true},
				CreationTime: pq.NullTime{Time: now, Valid: true},
				StartTime:    pq.NullTime{Time: now.Add(1 * time.Minute), Valid: true},
				EndTime:      pq.NullTime{Time: now.Add(2 * time.Minute), Valid: true},
				Timeout:      120,
			},
			validate: func(t *testing.T, result view.OpsJobResponseItem) {
				assert.Equal(t, v1.OpsJobFailed, result.Phase)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := cvtToOpsJobResponseItem(tt.job)
			tt.validate(t, result)
		})
	}
}

// TestBaseOpsJobRequestValidation tests BaseOpsJobRequest structure
func TestBaseOpsJobRequestValidation(t *testing.T) {
	tests := []struct {
		name     string
		request  view.BaseOpsJobRequest
		validate func(*testing.T, view.BaseOpsJobRequest)
	}{
		{
			name: "complete request",
			request: view.BaseOpsJobRequest{
				Name: "test-preflight",
				Type: v1.OpsJobPreflightType,
				Inputs: []v1.Parameter{
					{Name: "node", Value: "node-1"},
					{Name: "cluster", Value: "test-cluster"},
				},
				TimeoutSecond:           600,
				TTLSecondsAfterFinished: 3600,
			},
			validate: func(t *testing.T, req view.BaseOpsJobRequest) {
				assert.Equal(t, "test-preflight", req.Name)
				assert.Equal(t, v1.OpsJobPreflightType, req.Type)
				assert.Len(t, req.Inputs, 2)
				assert.Equal(t, 600, req.TimeoutSecond)
				assert.Equal(t, 3600, req.TTLSecondsAfterFinished)
			},
		},
		{
			name: "minimal request",
			request: view.BaseOpsJobRequest{
				Name: "simple-job",
				Type: v1.OpsJobAddonType,
				Inputs: []v1.Parameter{
					{Name: "addon", Value: "prometheus"},
				},
			},
			validate: func(t *testing.T, req view.BaseOpsJobRequest) {
				assert.Equal(t, "simple-job", req.Name)
				assert.Equal(t, v1.OpsJobAddonType, req.Type)
				assert.Len(t, req.Inputs, 1)
				assert.Equal(t, 0, req.TimeoutSecond)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.validate(t, tt.request)
		})
	}
}

// TestCreateAddonRequestValidation tests CreateAddonRequest structure
func TestCreateAddonRequestValidation(t *testing.T) {
	tests := []struct {
		name     string
		request  view.CreateAddonRequest
		validate func(*testing.T, view.CreateAddonRequest)
	}{
		{
			name: "addon request with batch settings",
			request: view.CreateAddonRequest{
				BaseOpsJobRequest: view.BaseOpsJobRequest{
					Name: "addon-upgrade",
					Type: v1.OpsJobAddonType,
					Inputs: []v1.Parameter{
						{Name: "addon", Value: "driver"},
					},
				},
				BatchCount:      5,
				AvailableRatio:  floatPtr(0.95),
				SecurityUpgrade: true,
			},
			validate: func(t *testing.T, req view.CreateAddonRequest) {
				assert.Equal(t, 5, req.BatchCount)
				assert.NotNil(t, req.AvailableRatio)
				assert.Equal(t, 0.95, *req.AvailableRatio)
				assert.True(t, req.SecurityUpgrade)
			},
		},
		{
			name: "addon request with defaults",
			request: view.CreateAddonRequest{
				BaseOpsJobRequest: view.BaseOpsJobRequest{
					Name: "addon-install",
					Type: v1.OpsJobAddonType,
					Inputs: []v1.Parameter{
						{Name: "addon", Value: "monitoring"},
					},
				},
			},
			validate: func(t *testing.T, req view.CreateAddonRequest) {
				assert.Equal(t, 0, req.BatchCount)
				assert.Nil(t, req.AvailableRatio)
				assert.False(t, req.SecurityUpgrade)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.validate(t, tt.request)
		})
	}
}

// TestCreatePreflightRequestValidation tests CreatePreflightRequest structure
func TestCreatePreflightRequestValidation(t *testing.T) {
	tests := []struct {
		name     string
		request  view.CreatePreflightRequest
		validate func(*testing.T, view.CreatePreflightRequest)
	}{
		{
			name: "preflight with resource requirements",
			request: view.CreatePreflightRequest{
				BaseOpsJobRequest: view.BaseOpsJobRequest{
					Name: "network-check",
					Type: v1.OpsJobPreflightType,
					Inputs: []v1.Parameter{
						{Name: "node", Value: "node-1"},
					},
				},
				Resource: &v1.WorkloadResource{
					CPU:     "1",
					Memory:  "2Gi",
					Replica: 1,
				},
				Image:      strPtr("preflight-checker:v1.0"),
				EntryPoint: strPtr("L2Jpbi9iYXNo"), // base64 encoded
				Env: map[string]string{
					"CHECK_TYPE": "network",
					"TIMEOUT":    "300",
				},
				Hostpath: []string{"/var/log", "/etc"},
			},
			validate: func(t *testing.T, req view.CreatePreflightRequest) {
				assert.NotNil(t, req.Resource)
				assert.Equal(t, "1", req.Resource.CPU)
				assert.NotNil(t, req.Image)
				assert.Equal(t, "preflight-checker:v1.0", *req.Image)
				assert.NotNil(t, req.EntryPoint)
				assert.Len(t, req.Env, 2)
				assert.Len(t, req.Hostpath, 2)
			},
		},
		{
			name: "minimal preflight request",
			request: view.CreatePreflightRequest{
				BaseOpsJobRequest: view.BaseOpsJobRequest{
					Name: "simple-check",
					Type: v1.OpsJobPreflightType,
					Inputs: []v1.Parameter{
						{Name: "check", Value: "disk"},
					},
				},
			},
			validate: func(t *testing.T, req view.CreatePreflightRequest) {
				assert.Nil(t, req.Resource)
				assert.Nil(t, req.Image)
				assert.Nil(t, req.EntryPoint)
				assert.Nil(t, req.Env)
				assert.Nil(t, req.Hostpath)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.validate(t, tt.request)
		})
	}
}

// TestGenerateOpsJobNodesInput_NodeParams tests generateOpsJobNodesInput (lines 672-678)
// where excluded nodes have their value cleared, and non-excluded nodes are kept.
func TestGenerateOpsJobNodesInput_NodeParams(t *testing.T) {
	tests := []struct {
		name           string
		inputs         []v1.Parameter
		excludedNodes  []string
		expectedInputs []v1.Parameter
	}{
		{
			name: "exclude one node - only excluded is cleared",
			inputs: []v1.Parameter{
				{Name: v1.ParameterNode, Value: "node-1"},
				{Name: v1.ParameterNode, Value: "node-2"},
				{Name: v1.ParameterNode, Value: "node-3"},
			},
			excludedNodes: []string{"node-2"},
			expectedInputs: []v1.Parameter{
				{Name: v1.ParameterNode, Value: "node-1"}, // kept
				{Name: v1.ParameterNode, Value: ""},       // excluded, cleared
				{Name: v1.ParameterNode, Value: "node-3"}, // kept
			},
		},
		{
			name: "exclude all nodes - all cleared",
			inputs: []v1.Parameter{
				{Name: v1.ParameterNode, Value: "node-1"},
				{Name: v1.ParameterNode, Value: "node-2"},
			},
			excludedNodes: []string{"node-1", "node-2"},
			expectedInputs: []v1.Parameter{
				{Name: v1.ParameterNode, Value: ""}, // excluded, cleared
				{Name: v1.ParameterNode, Value: ""}, // excluded, cleared
			},
		},
		{
			name: "no exclusions - all kept",
			inputs: []v1.Parameter{
				{Name: v1.ParameterNode, Value: "node-1"},
				{Name: "other", Value: "keep-this"},
			},
			excludedNodes: []string{},
			expectedInputs: []v1.Parameter{
				{Name: v1.ParameterNode, Value: "node-1"}, // kept
				{Name: "other", Value: "keep-this"},       // other param, kept
			},
		},
	}

	h := &Handler{}
	ctx := context.Background()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			job := &v1.OpsJob{
				Spec: v1.OpsJobSpec{
					Inputs:        tt.inputs,
					ExcludedNodes: tt.excludedNodes,
				},
			}

			// Call the actual function
			err := h.generateOpsJobNodesInput(ctx, job)
			assert.NoError(t, err)

			// Verify that job.Spec.Inputs was modified correctly
			assert.Equal(t, tt.expectedInputs, job.Spec.Inputs)
		})
	}
}

// Helper functions
func floatPtr(f float64) *float64 {
	return &f
}

func strPtr(s string) *string {
	return &s
}
