/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package ops_job

import (
	"context"
	"errors"
	"testing"

	"github.com/agiledragon/gomonkey/v2"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/pointer"
	ctrlruntime "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlfake "sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/event"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	commonconfig "github.com/AMD-AIG-AIMA/SAFE/common/pkg/config"
	dbclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
	mockclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client/mock"
	commonworkspace "github.com/AMD-AIG-AIMA/SAFE/common/pkg/workspace"
)

// patchHFS3Config stubs the S3 related configuration so the HF download paths
// behave as if object storage is enabled.
func patchHFS3Config() *gomonkey.Patches {
	p := gomonkey.NewPatches()
	p.ApplyFunc(commonconfig.IsS3Enable, func() bool { return true })
	p.ApplyFunc(commonconfig.GetS3Endpoint, func() string { return "https://minio:9000" })
	p.ApplyFunc(commonconfig.GetS3Bucket, func() string { return "bucket" })
	p.ApplyFunc(commonconfig.GetDownloadJoImage, func() string { return "download:1" })
	return p
}

func newHFController(t *testing.T, db dbclient.Interface, objs ...client.Object) *HFDatasetDownloadController {
	t.Helper()
	builder := ctrlfake.NewClientBuilder().WithScheme(fullScheme(t))
	for _, o := range objs {
		builder = builder.WithObjects(o)
	}
	return &HFDatasetDownloadController{Client: builder.Build(), dbClient: db}
}

func TestExtractHFJobFailureReason(t *testing.T) {
	tests := []struct {
		name     string
		job      *batchv1.Job
		expected string
	}{
		{
			name: "extract reason and message from failed condition",
			job: &batchv1.Job{
				Status: batchv1.JobStatus{
					Conditions: []batchv1.JobCondition{
						{
							Type:    batchv1.JobFailed,
							Status:  corev1.ConditionTrue,
							Reason:  "BackoffLimitExceeded",
							Message: "Job has reached the specified backoff limit",
						},
					},
				},
			},
			expected: "BackoffLimitExceeded: Job has reached the specified backoff limit",
		},
		{
			name: "no failed condition returns unknown error",
			job: &batchv1.Job{
				Status: batchv1.JobStatus{
					Conditions: []batchv1.JobCondition{
						{
							Type:   batchv1.JobComplete,
							Status: corev1.ConditionTrue,
						},
					},
				},
			},
			expected: "Unknown error during download",
		},
		{
			name: "empty conditions returns unknown error",
			job: &batchv1.Job{
				Status: batchv1.JobStatus{
					Conditions: []batchv1.JobCondition{},
				},
			},
			expected: "Unknown error during download",
		},
		{
			name: "failed condition with false status is ignored",
			job: &batchv1.Job{
				Status: batchv1.JobStatus{
					Conditions: []batchv1.JobCondition{
						{
							Type:    batchv1.JobFailed,
							Status:  corev1.ConditionFalse,
							Reason:  "SomeReason",
							Message: "Some message",
						},
					},
				},
			},
			expected: "Unknown error during download",
		},
		{
			name: "backoff limit exceeded",
			job: &batchv1.Job{
				Spec: batchv1.JobSpec{
					BackoffLimit: pointer.Int32(3),
				},
				Status: batchv1.JobStatus{
					Failed: 3,
				},
			},
			expected: "Maximum retry attempts exceeded",
		},
		{
			name: "multiple conditions picks failed one",
			job: &batchv1.Job{
				Status: batchv1.JobStatus{
					Conditions: []batchv1.JobCondition{
						{
							Type:   batchv1.JobComplete,
							Status: corev1.ConditionFalse,
						},
						{
							Type:    batchv1.JobFailed,
							Status:  corev1.ConditionTrue,
							Reason:  "DeadlineExceeded",
							Message: "Job was active longer than specified deadline",
						},
					},
				},
			},
			expected: "DeadlineExceeded: Job was active longer than specified deadline",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractHFJobFailureReason(tt.job)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestHFDatasetJobPredicateLogic(t *testing.T) {
	tests := []struct {
		name     string
		labels   map[string]string
		expected bool
	}{
		{
			name:     "job with hf-dataset-job=true should match",
			labels:   map[string]string{HFDatasetJobLabel: "true", HFDatasetIdLabel: "dataset-abc"},
			expected: true,
		},
		{
			name:     "job with hf-dataset-job=false should not match",
			labels:   map[string]string{HFDatasetJobLabel: "false"},
			expected: false,
		},
		{
			name:     "job without hf-dataset-job label should not match",
			labels:   map[string]string{"other": "value"},
			expected: false,
		},
		{
			name:     "nil labels should not match",
			labels:   nil,
			expected: false,
		},
		{
			name:     "empty labels should not match",
			labels:   map[string]string{},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test the underlying logic that hfDatasetJobPredicate uses
			result := false
			if tt.labels != nil {
				result = tt.labels[HFDatasetJobLabel] == "true"
			}
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestExtractDatasetBasePath tests the base path extraction for failover
func TestExtractDatasetBasePath(t *testing.T) {
	tests := []struct {
		name     string
		fullPath string
		expected string
	}{
		{
			name:     "standard dataset path",
			fullPath: "/wekafs/datasets/math_500",
			expected: "/wekafs",
		},
		{
			name:     "nested org dataset path",
			fullPath: "/apps/datasets/HuggingFaceH4/MATH-500",
			expected: "/apps",
		},
		{
			name:     "deep path",
			fullPath: "/mnt/storage/datasets/my-dataset",
			expected: "/mnt/storage",
		},
		{
			name:     "no datasets in path",
			fullPath: "/wekafs/models/llama",
			expected: "",
		},
		{
			name:     "empty path",
			fullPath: "",
			expected: "",
		},
		{
			name:     "datasets at root",
			fullPath: "/datasets/test",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractDatasetBasePath(tt.fullPath)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestParseTriedWorkspacesMap tests parsing of tried_workspaces JSON field
func TestParseTriedWorkspacesMap(t *testing.T) {
	tests := []struct {
		name     string
		data     string
		expected map[string][]string
	}{
		{
			name:     "empty string",
			data:     "",
			expected: map[string][]string{},
		},
		{
			name:     "empty object",
			data:     "{}",
			expected: map[string][]string{},
		},
		{
			name:     "empty array (invalid but handled)",
			data:     "[]",
			expected: map[string][]string{},
		},
		{
			name: "single path with one workspace",
			data: `{"/wekafs": ["workspace-a"]}`,
			expected: map[string][]string{
				"/wekafs": {"workspace-a"},
			},
		},
		{
			name: "single path with multiple workspaces",
			data: `{"/wekafs": ["workspace-a", "workspace-b"]}`,
			expected: map[string][]string{
				"/wekafs": {"workspace-a", "workspace-b"},
			},
		},
		{
			name: "multiple paths",
			data: `{"/wekafs": ["ws-a"], "/apps": ["ws-b", "ws-c"]}`,
			expected: map[string][]string{
				"/wekafs": {"ws-a"},
				"/apps":   {"ws-b", "ws-c"},
			},
		},
		{
			name:     "invalid JSON returns empty map",
			data:     "not-json",
			expected: map[string][]string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseTriedWorkspacesMap(tt.data)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestAppendUniqueStr tests the unique append helper
func TestAppendUniqueStr(t *testing.T) {
	tests := []struct {
		name     string
		slice    []string
		item     string
		expected []string
	}{
		{
			name:     "append to empty slice",
			slice:    nil,
			item:     "a",
			expected: []string{"a"},
		},
		{
			name:     "append new item",
			slice:    []string{"a", "b"},
			item:     "c",
			expected: []string{"a", "b", "c"},
		},
		{
			name:     "skip duplicate item",
			slice:    []string{"a", "b"},
			item:     "a",
			expected: []string{"a", "b"},
		},
		{
			name:     "skip duplicate at end",
			slice:    []string{"a", "b"},
			item:     "b",
			expected: []string{"a", "b"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := appendUniqueStr(tt.slice, tt.item)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestContainsStr tests the string contains helper
func TestContainsStr(t *testing.T) {
	tests := []struct {
		name     string
		slice    []string
		item     string
		expected bool
	}{
		{
			name:     "found in slice",
			slice:    []string{"a", "b", "c"},
			item:     "b",
			expected: true,
		},
		{
			name:     "not found in slice",
			slice:    []string{"a", "b", "c"},
			item:     "d",
			expected: false,
		},
		{
			name:     "empty slice",
			slice:    []string{},
			item:     "a",
			expected: false,
		},
		{
			name:     "nil slice",
			slice:    nil,
			item:     "a",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := containsStr(tt.slice, tt.item)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestHFDatasetJobPredicate(t *testing.T) {
	p := hfDatasetJobPredicate()
	withLabel := &batchv1.Job{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{HFDatasetJobLabel: "true"}}}
	assert.True(t, p.Create(event.CreateEvent{Object: withLabel}))
	assert.False(t, p.Create(event.CreateEvent{Object: &batchv1.Job{}}))
}

func TestHFJobStatusChangedPredicate(t *testing.T) {
	p := hfJobStatusChangedPredicate()
	old := &batchv1.Job{}
	upd := &batchv1.Job{Status: batchv1.JobStatus{Succeeded: 1}}
	assert.True(t, p.Update(event.UpdateEvent{ObjectOld: old, ObjectNew: upd}))
	assert.False(t, p.Update(event.UpdateEvent{ObjectOld: old, ObjectNew: old.DeepCopy()}))
}

func TestSetupHFDatasetDownloadControllerDBDisabled(t *testing.T) {
	// DB disabled by default -> returns nil without touching manager.
	assert.NoError(t, SetupHFDatasetDownloadController(context.Background(), nil))
}

func TestHFReconcileNotFound(t *testing.T) {
	r := newHFController(t, nil)
	res, err := r.Reconcile(context.Background(), ctrlruntime.Request{NamespacedName: types.NamespacedName{Name: "missing"}})
	assert.NoError(t, err)
	assert.Equal(t, ctrlruntime.Result{}, res)
}

func TestHFReconcileNoDatasetId(t *testing.T) {
	job := &batchv1.Job{ObjectMeta: metav1.ObjectMeta{Name: "j1"}}
	r := newHFController(t, nil, job)
	_, err := r.Reconcile(context.Background(), ctrlruntime.Request{NamespacedName: types.NamespacedName{Name: "j1"}})
	assert.NoError(t, err)
}

func TestHFReconcileGetDatasetError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	db := mockclient.NewMockInterface(ctrl)
	db.EXPECT().GetDataset(gomock.Any(), "ds1").Return(nil, errors.New("db error"))

	job := &batchv1.Job{ObjectMeta: metav1.ObjectMeta{
		Name:   "j1",
		Labels: map[string]string{HFDatasetIdLabel: "ds1"},
	}}
	r := newHFController(t, db, job)
	_, err := r.Reconcile(context.Background(), ctrlruntime.Request{NamespacedName: types.NamespacedName{Name: "j1"}})
	assert.Error(t, err)
}

func TestHFReconcileInProgress(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	db := mockclient.NewMockInterface(ctrl)
	db.EXPECT().GetDataset(gomock.Any(), "ds1").Return(&dbclient.Dataset{DatasetId: "ds1"}, nil)

	job := &batchv1.Job{ObjectMeta: metav1.ObjectMeta{
		Name:   "j1",
		Labels: map[string]string{HFDatasetIdLabel: "ds1"},
	}}
	r := newHFController(t, db, job)
	res, err := r.Reconcile(context.Background(), ctrlruntime.Request{NamespacedName: types.NamespacedName{Name: "j1"}})
	assert.NoError(t, err)
	assert.True(t, res.RequeueAfter > 0)
}

func TestHFReconcileFailed(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	db := mockclient.NewMockInterface(ctrl)
	db.EXPECT().GetDataset(gomock.Any(), "ds1").Return(&dbclient.Dataset{DatasetId: "ds1"}, nil)
	db.EXPECT().UpsertDataset(gomock.Any(), gomock.Any()).Return(nil)

	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{Name: "j1", Labels: map[string]string{HFDatasetIdLabel: "ds1"}},
		Status:     batchv1.JobStatus{Failed: 1, Active: 0},
	}
	r := newHFController(t, db, job)
	_, err := r.Reconcile(context.Background(), ctrlruntime.Request{NamespacedName: types.NamespacedName{Name: "j1"}})
	assert.NoError(t, err)
}

func TestHFInitializeLocalPathsPublic(t *testing.T) {
	r := newHFController(t, nil)
	// Public dataset (no workspace), no workspaces in cluster -> empty targets.
	jsonStr, targets, err := r.initializeLocalPaths(context.Background(), &dbclient.Dataset{DatasetId: "ds1", DisplayName: "d"})
	assert.NoError(t, err)
	assert.Equal(t, "[]", jsonStr)
	assert.Empty(t, targets)
}

func TestHFInitializeLocalPathsPrivateMissingWorkspace(t *testing.T) {
	r := newHFController(t, nil)
	_, _, err := r.initializeLocalPaths(context.Background(), &dbclient.Dataset{DatasetId: "ds1", Workspace: "missing"})
	assert.Error(t, err)
}

func TestHFGetS3FileInfoDisabled(t *testing.T) {
	r := newHFController(t, nil)
	size, count := r.getS3FileInfo(context.Background(), "path")
	assert.Equal(t, int64(0), size)
	assert.Equal(t, 0, count)
}

func TestHFCreateLocalDownloadOpsJobs(t *testing.T) {
	patches := patchHFS3Config()
	defer patches.Reset()

	ws := &v1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: "ws1"}, Spec: v1.WorkspaceSpec{Cluster: "c1"}}
	r := newHFController(t, nil, ws)
	targets := []commonworkspace.DownloadTarget{{Workspace: "ws1", Path: "/data"}}
	err := r.createLocalDownloadOpsJobs(context.Background(), &dbclient.Dataset{DatasetId: "ds1", DisplayName: "d", S3Path: "datasets/d"}, targets)
	assert.NoError(t, err)
}

func TestHFHandleJobSucceeded(t *testing.T) {
	patches := patchHFS3Config()
	defer patches.Reset()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	db := mockclient.NewMockInterface(ctrl)
	db.EXPECT().UpsertDataset(gomock.Any(), gomock.Any()).Return(nil)

	ws := &v1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: "ws1"}, Spec: v1.WorkspaceSpec{Cluster: "c1"}}
	job := &batchv1.Job{ObjectMeta: metav1.ObjectMeta{Name: "j1"}}
	r := newHFController(t, db, ws, job)
	// Private dataset bound to ws1 -> initializes local paths + creates OpsJobs.
	dataset := &dbclient.Dataset{DatasetId: "ds1", DisplayName: "d", Workspace: "ws1", S3Path: "datasets/d"}
	_, err := r.handleJobSucceeded(context.Background(), dataset, job)
	assert.NoError(t, err)
}

func TestHFHandleJobFailed(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	db := mockclient.NewMockInterface(ctrl)
	db.EXPECT().UpsertDataset(gomock.Any(), gomock.Any()).Return(nil)

	job := &batchv1.Job{ObjectMeta: metav1.ObjectMeta{Name: "j1"}}
	r := newHFController(t, db, job)
	_, err := r.handleJobFailed(context.Background(), &dbclient.Dataset{DatasetId: "ds1"}, job)
	assert.NoError(t, err)
}
